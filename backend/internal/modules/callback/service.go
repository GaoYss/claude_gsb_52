package callback

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/repair"
	"streetlight/pkg/pagination"
)

// callbackSortSpec 定义回访任务列表允许的排序字段白名单。
var callbackSortSpec = pagination.SortSpec{
	Allowed: map[string]string{
		"task_no":      "task_no",
		"status":       "status",
		"due_at":       "due_at",
		"contacted_at": "contacted_at",
		"round":        "round",
		"created_at":   "created_at",
	},
	Default: "created_at",
}

// Service 承载维修质量回访的业务规则:
// 完工生成回访任务、登记联系情况、满意度与合格判定、不合格触发返修、返修后重新回访。
type Service struct {
	repo    *Repository
	repairs RepairPort
}

// RepairPort 由维修模块实现, 回访模块通过它读取完工维修单并创建返修单。
type RepairPort interface {
	GetByID(ctx context.Context, id uint) (*repair.Repair, error)
	CreateRework(ctx context.Context, req repair.ReworkRequest) (*repair.Repair, error)
}

// NewService 构造回访服务。
func NewService(repo *Repository, repairs RepairPort) *Service {
	return &Service{repo: repo, repairs: repairs}
}

// OnRepairFinished 实现 repair.CallbackHook: 维修完工且结果为"已修复"时, 按规则生成回访任务。
// 返修完工同样会触发, 此时生成新一轮回访任务; 原完工时间与首次处置过程不在此处改写。
func (s *Service) OnRepairFinished(ctx context.Context, event repair.FinishEvent) {
	if !event.Fixed {
		return
	}
	if err := s.createTaskForRepair(ctx, event, time.Now()); err != nil {
		slog.Error("生成回访任务失败", "repair_id", event.RepairID, "repair_no", event.RepairNo, "error", err)
	}
}

// AssertRepairDeletable 实现 repair.CallbackHook: 已生成回访任务的维修单不允许删除, 避免回访链路断链。
func (s *Service) AssertRepairDeletable(ctx context.Context, repairID uint) error {
	task, err := s.repo.GetTaskByRepairID(ctx, repairID)
	if err != nil {
		return err
	}
	if task != nil {
		return apperr.Conflict("维修单 %s 已生成回访任务 %s, 不允许删除", task.RepairNo, task.TaskNo)
	}
	return nil
}

// AssertSettlementAllowed 实现 fault.SettlementGuard: 回访未完成(未回访合格)的故障不允许结算关闭。
func (s *Service) AssertSettlementAllowed(ctx context.Context, faultID uint) error {
	task, err := s.repo.LatestTaskByFault(ctx, faultID)
	if err != nil {
		return err
	}
	if task != nil && task.Status != StatusQualified {
		return apperr.Conflict("故障 %s 的回访(第 %d 轮)尚未完成, 当前状态: %s, 回访合格前不允许结算",
			task.FaultNo, task.Round, TaskStatusLabel(task.Status))
	}
	return nil
}

// createTaskForRepair 为一次"已修复"完工创建回访任务, 返修完工时轮次递增。
func (s *Service) createTaskForRepair(ctx context.Context, event repair.FinishEvent, now time.Time) error {
	// 幂等: 同一维修单重复完工事件只生成一条任务。
	exists, err := s.repo.GetTaskByRepairID(ctx, event.RepairID)
	if err != nil {
		return err
	}
	if exists != nil {
		return nil
	}

	round := 1
	previous, err := s.repo.LatestTaskByFault(ctx, event.FaultID)
	if err != nil {
		return err
	}
	if previous != nil {
		round = previous.Round + 1
	}

	task := &Task{
		FaultID:  event.FaultID,
		FaultNo:  event.FaultNo,
		LampID:   event.LampID,
		LampCode: event.LampCode,
		RepairID: event.RepairID,
		RepairNo: event.RepairNo,
		Round:    round,
		Status:   StatusPending,
		DueAt:    event.FinishedAt.Add(DefaultCallbackWithin),
	}
	if err := s.repo.CreateTaskWithUniqueNo(ctx, task, "HF"+event.FinishedAt.Format("20060102")); err != nil {
		return err
	}
	slog.Info("已生成维修质量回访任务",
		"task_no", task.TaskNo, "repair_no", task.RepairNo, "round", task.Round, "due_at", task.DueAt.Format("2006-01-02 15:04"))
	return nil
}

// Get 查询回访任务详情(含全部联系记录)。
func (s *Service) Get(ctx context.Context, id uint) (*TaskDetail, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.assembleDetail(ctx, task)
}

// List 分页查询回访任务。
func (s *Service) List(ctx context.Context, query ListQuery) ([]Task, int64, pagination.Query, error) {
	page := pagination.Parse(query.Params, callbackSortSpec)
	filter, err := s.buildFilter(query)
	if err != nil {
		return nil, 0, page, err
	}
	items, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, 0, page, err
	}
	now := time.Now()
	for index := range items {
		if items[index].Status != StatusQualified && items[index].DueAt.Before(now) {
			items[index].Overdue = true
		}
	}
	return items, total, page, nil
}

// AddContact 登记一次回访联系情况, 有效联系后任务进入"已联系待判定"。
func (s *Service) AddContact(ctx context.Context, id uint, req ContactRequest) (*TaskDetail, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.Status == StatusQualified {
		return nil, apperr.Conflict("回访任务 %s 已判定合格并闭环, 无需再登记联系情况", task.TaskNo)
	}

	result := strings.TrimSpace(req.Result)
	if !IsValidContactResult(result) {
		return nil, apperr.BadRequest("非法的联系结果: %s", result)
	}
	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = ChannelPhone
	}
	contactPerson := strings.TrimSpace(req.ContactPerson)
	if contactPerson == "" {
		return nil, apperr.BadRequest("回访人不能为空")
	}
	if req.Satisfaction != nil && result != ResultConnected {
		return nil, apperr.BadRequest("仅有效联系(已联系)时可以记录满意度")
	}

	contactedAt, err := parseTime(req.ContactedAt, time.Now())
	if err != nil {
		return nil, err
	}

	contact := &Contact{
		TaskID:        task.ID,
		Channel:       channel,
		Result:        result,
		ContactPerson: contactPerson,
		ContactName:   strings.TrimSpace(req.ContactName),
		Satisfaction:  req.Satisfaction,
		Content:       strings.TrimSpace(req.Content),
		ContactedAt:   contactedAt,
	}
	if err := s.repo.CreateContact(ctx, contact); err != nil {
		return nil, err
	}

	// 已联系 / 受访人要求延期视为取得联系: 待回访任务进入待判定。
	// 已不合格的任务正处于返修流程, 仅追加联系记录, 不回退状态(返修完工后会生成新一轮任务)。
	if (result == ResultConnected || result == ResultPostponed) && task.Status == StatusPending {
		task.Status = StatusContacted
		task.ContactedAt = &contactedAt
		if req.Satisfaction != nil {
			task.Satisfaction = req.Satisfaction
		}
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			return nil, err
		}
	}

	return s.assembleDetail(ctx, task)
}

// Judge 回访合格判定: 合格则闭环; 不合格则登记结论并触发返修, 等待返修完工后的新一轮回访。
func (s *Service) Judge(ctx context.Context, id uint, req JudgeRequest) (*TaskDetail, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.Status == StatusQualified {
		return nil, apperr.Conflict("回访任务 %s 已判定合格, 无需重复判定", task.TaskNo)
	}
	if task.Status == StatusUnqualified {
		return nil, apperr.Conflict("回访任务 %s 已判定不合格, 返修单 %s 完工后将自动发起重新回访", task.TaskNo, task.ReworkRepairNo)
	}
	if req.Satisfaction != nil && !validSatisfaction(*req.Satisfaction) {
		return nil, apperr.BadRequest("满意度取值应为 1-5")
	}

	now := time.Now()
	contact := &Contact{
		TaskID:        task.ID,
		Channel:       ChannelPhone,
		Result:        ResultConnected,
		ContactPerson: "回访判定",
		Satisfaction:  req.Satisfaction,
		Qualified:     &req.Qualified,
		Content:       strings.TrimSpace(req.Remark),
		ContactedAt:   now,
	}

	if req.Qualified {
		task.Status = StatusQualified
		task.ContactedAt = &now
		if req.Satisfaction != nil {
			task.Satisfaction = req.Satisfaction
		}
		task.ResultRemark = strings.TrimSpace(req.Remark)
		if err := s.repo.CreateContact(ctx, contact); err != nil {
			return nil, err
		}
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			return nil, err
		}
		return s.assembleDetail(ctx, task)
	}

	// 不合格: 校验返修派工信息后触发返修, 并关联原维修记录。
	repairman := strings.TrimSpace(req.ReworkRepairman)
	if repairman == "" {
		return nil, apperr.BadRequest("回访不合格时必须填写返修维修人员")
	}
	origin, err := s.repairs.GetByID(ctx, task.RepairID)
	if err != nil {
		return nil, err
	}

	rework, err := s.repairs.CreateRework(ctx, repair.ReworkRequest{
		FaultID:        task.FaultID,
		OriginRepairID: origin.ID,
		Repairman:      repairman,
		RepairTeam:     strings.TrimSpace(req.ReworkRepairTeam),
		Content:        reworkContent(req.ReworkContent, req.Remark),
		Cost:           req.ReworkCost,
	})
	if err != nil {
		return nil, err
	}

	remark := strings.TrimSpace(req.Remark)
	task.Status = StatusUnqualified
	task.ContactedAt = &now
	if req.Satisfaction != nil {
		task.Satisfaction = req.Satisfaction
	}
	task.ResultRemark = remark
	task.ReworkRepairID = &rework.ID
	task.ReworkRepairNo = rework.RepairNo
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	contact.Content = reworkContent(contact.Content, "")
	contact.Content = strings.TrimSpace(contact.Content + " 已触发返修: " + rework.RepairNo)
	if err := s.repo.CreateContact(ctx, contact); err != nil {
		return nil, err
	}

	slog.Warn("回访不合格, 已触发返修",
		"task_no", task.TaskNo, "origin_repair_no", task.RepairNo, "rework_repair_no", rework.RepairNo)
	return s.assembleDetail(ctx, task)
}

// Metadata 返回回访模块字典。
func (s *Service) Metadata(ctx context.Context) (*Meta, error) {
	persons, err := s.repo.DistinctValues(ctx, "contact_person")
	if err != nil {
		return nil, err
	}
	return &Meta{
		TaskStatuses:    TaskStatuses(),
		ContactResults:  ContactResults(),
		ContactChannels: ContactChannels(),
		ContactPersons:  persons,
	}, nil
}

// Statistics 汇总回访与返修统计。
func (s *Service) Statistics(ctx context.Context) (*Statistics, error) {
	total, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}
	byStatus, err := s.repo.CountByColumn(ctx, "status")
	if err != nil {
		return nil, err
	}
	overdue, err := s.repo.CountOverdue(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	result := &Statistics{
		Total:            total,
		PendingTotal:     byStatus[StatusPending],
		ContactedTotal:   byStatus[StatusContacted],
		QualifiedTotal:   byStatus[StatusQualified],
		UnqualifiedTotal: byStatus[StatusUnqualified],
		OverdueTotal:     overdue,
		ReworkTotal:      byStatus[StatusUnqualified],
	}
	result.OpenTotal = result.PendingTotal + result.ContactedTotal + result.UnqualifiedTotal
	return result, nil
}

// ListByFault 查询某条故障的全部回访任务(供状态追踪使用)。
func (s *Service) ListByFault(ctx context.Context, faultID uint) ([]Task, error) {
	return s.repo.ListByFault(ctx, faultID)
}

// ListContactsByFaults 批量查询多条故障的回访联系记录(供状态追踪使用)。
func (s *Service) ListContactsByFaults(ctx context.Context, faultIDs []uint) (map[uint][]Contact, error) {
	return s.repo.ListContactsByFaults(ctx, faultIDs)
}

// Repository 暴露仓储, 供状态查询模块装配。
func (s *Service) Repository() *Repository { return s.repo }

func (s *Service) assembleDetail(ctx context.Context, task *Task) (*TaskDetail, error) {
	contacts, err := s.repo.ListContactsByTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if task.Status != StatusQualified && task.DueAt.Before(now) {
		task.Overdue = true
	}
	return &TaskDetail{Task: *task, Contacts: contacts}, nil
}

func (s *Service) buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		Keyword:  strings.TrimSpace(query.Keyword),
		Status:   strings.TrimSpace(query.Status),
		FaultID:  query.FaultID,
		RepairID: query.RepairID,
		Round:    query.Round,
	}
	if filter.Status != "" && !IsValidTaskStatus(filter.Status) {
		return filter, apperr.BadRequest("非法的回访任务状态: %s", filter.Status)
	}
	if query.OnlyOpen {
		filter.OpenStatuses = []string{StatusPending, StatusContacted, StatusUnqualified}
	}
	if query.OnlyOverdue {
		now := time.Now()
		filter.DueBefore = &now
	}
	if value := strings.TrimSpace(query.StartDate); value != "" {
		from, err := parseDay(value)
		if err != nil {
			return filter, err
		}
		filter.CreatedFrom = &from
	}
	if value := strings.TrimSpace(query.EndDate); value != "" {
		to, err := parseDay(value)
		if err != nil {
			return filter, err
		}
		to = to.AddDate(0, 0, 1)
		filter.CreatedTo = &to
	}
	if filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedTo.Before(*filter.CreatedFrom) {
		return filter, apperr.BadRequest("结束日期不能早于开始日期")
	}
	return filter, nil
}

func validSatisfaction(value int) bool {
	return value >= 1 && value <= 5
}

func reworkContent(content, remark string) string {
	content = strings.TrimSpace(content)
	remark = strings.TrimSpace(remark)
	switch {
	case content != "" && remark != "":
		return content + "(回访不合格原因: " + remark + ")"
	case content != "":
		return content
	case remark != "":
		return "回访不合格返修: " + remark
	default:
		return "回访不合格, 返场重新处置"
	}
}

// parseTime 解析时间字符串, 为空时返回 fallback。
func parseTime(value string, fallback time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, apperr.BadRequest("时间格式不正确, 建议使用 YYYY-MM-DD HH:mm:ss: %s", value)
}

// parseDay 解析 YYYY-MM-DD 日期, 返回当天零点。
func parseDay(value string) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, apperr.BadRequest("日期格式应为 YYYY-MM-DD, 当前值: %s", value)
	}
	return date, nil
}
