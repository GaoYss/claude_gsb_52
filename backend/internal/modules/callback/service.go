package callback

import (
	"context"
	"strings"
	"time"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/repair"
	"streetlight/pkg/pagination"
)

// callbackSortSpec 定义回访任务列表允许的排序字段白名单。
var callbackSortSpec = pagination.SortSpec{
	Allowed: map[string]string{
		"callback_no": "callback_no",
		"round":       "round",
		"status":      "status",
		"verdict":     "verdict",
		"visited_at":  "visited_at",
		"created_at":  "created_at",
	},
	Default: "created_at",
}

// RepairPort 由维修记录模块实现, 回访模块通过它在判定不合格时触发返修。
type RepairPort interface {
	CreateRework(ctx context.Context, originalID uint, req repair.ReworkRequest) (*repair.Repair, error)
}

// Service 承载维修质量回访的业务规则。
type Service struct {
	repo    *Repository
	repairs RepairPort
}

// NewService 构造质量回访服务。
func NewService(repo *Repository, repairs RepairPort) *Service {
	return &Service{repo: repo, repairs: repairs}
}

// Get 查询回访任务详情。
func (s *Service) Get(ctx context.Context, id uint) (*Callback, error) {
	return s.repo.GetByID(ctx, id)
}

// List 分页查询回访任务。
func (s *Service) List(ctx context.Context, query ListQuery) ([]Callback, int64, pagination.Query, error) {
	page := pagination.Parse(query.Params, callbackSortSpec)
	filter, err := buildFilter(query)
	if err != nil {
		return nil, 0, page, err
	}
	items, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return nil, 0, page, err
	}
	return items, total, page, nil
}

// ListByFault 查询某条故障的全部回访任务。
func (s *Service) ListByFault(ctx context.Context, faultID uint) ([]Callback, error) {
	return s.repo.ListByFault(ctx, faultID)
}

// OnRepairFinished 维修完工钩子: 结果为已修复时按规则生成回访任务, 其它结果不生成。
// 同一故障每完工一次(含返修)生成一轮新任务, 轮次递增。
func (s *Service) OnRepairFinished(ctx context.Context, entity *repair.Repair) error {
	if entity == nil || entity.Result != repair.ResultFixed {
		return nil
	}

	existing, err := s.repo.CountByFault(ctx, entity.FaultID)
	if err != nil {
		return err
	}

	task := &Callback{
		FaultID:  entity.FaultID,
		FaultNo:  entity.FaultNo,
		RepairID: entity.ID,
		RepairNo: entity.RepairNo,
		LampID:   entity.LampID,
		LampCode: entity.LampCode,
		Round:    int(existing) + 1,
		Status:   StatusPending,
	}
	return s.repo.CreateWithUniqueNo(ctx, task, "HF"+time.Now().Format("20060102"))
}

// OnRepairDeleted 维修记录删除钩子: 取消其名下未完成的回访任务。
func (s *Service) OnRepairDeleted(ctx context.Context, repairID uint) error {
	_, err := s.repo.CancelPendingByRepair(ctx, repairID)
	return err
}

// HasUnfinished 判断故障是否存在未完成的回访任务, 实现故障模块的 CallbackGuard 端口。
func (s *Service) HasUnfinished(ctx context.Context, faultID uint) (bool, error) {
	count, err := s.repo.CountPendingByFault(ctx, faultID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Record 登记回访结果: 记录联系情况与满意度。
// 未联系上时仅记录联系情况, 任务保持待回访; 已联系时完成任务,
// 判定不合格自动触发返修并关联原维修记录, 返修完工后会生成新一轮回访任务。
func (s *Service) Record(ctx context.Context, id uint, req RecordRequest) (*Callback, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.Status != StatusPending {
		return nil, apperr.Conflict("回访任务 %s 当前状态为 %s, 不允许重复登记", task.CallbackNo, StatusLabel(task.Status))
	}

	visitor := strings.TrimSpace(req.Visitor)
	if visitor == "" {
		return nil, apperr.BadRequest("回访人不能为空")
	}

	visitedAt, err := parseTime(req.VisitedAt, time.Now())
	if err != nil {
		return nil, err
	}

	contactResult := strings.TrimSpace(req.ContactResult)
	if !IsValidContactResult(contactResult) {
		return nil, apperr.BadRequest("非法的联系情况: %s", contactResult)
	}

	task.ContactResult = contactResult
	task.Visitor = visitor
	task.VisitedAt = &visitedAt
	task.Feedback = strings.TrimSpace(req.Feedback)

	// 未联系上: 仅记录联系情况, 任务保持待回访, 等待再次回访。
	if contactResult == ContactUnreachable {
		if err := s.repo.Update(ctx, task); err != nil {
			return nil, err
		}
		return task, nil
	}

	satisfaction := strings.TrimSpace(req.Satisfaction)
	if !IsValidSatisfaction(satisfaction) {
		return nil, apperr.BadRequest("已联系时必须填写有效的满意度")
	}
	verdict := strings.TrimSpace(req.Verdict)
	if !IsValidVerdict(verdict) {
		return nil, apperr.BadRequest("已联系时必须填写有效的回访判定")
	}

	// 判定不合格: 先触发返修并关联原维修记录, 返修失败则本次登记不生效。
	if verdict == VerdictUnqualified {
		rework, err := s.repairs.CreateRework(ctx, task.RepairID, repair.ReworkRequest{
			Repairman: strings.TrimSpace(req.ReworkRepairman),
			Reason:    task.Feedback,
		})
		if err != nil {
			return nil, err
		}
		task.ReworkRepairID = &rework.ID
		task.ReworkRepairNo = rework.RepairNo
	}

	task.Satisfaction = satisfaction
	task.Verdict = verdict
	task.Status = StatusCompleted

	if err := s.repo.Update(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// Metadata 返回回访模块字典。
func (s *Service) Metadata() *Meta {
	return &Meta{
		Statuses:       Statuses(),
		ContactResults: ContactResults(),
		Satisfactions:  Satisfactions(),
		Verdicts:       Verdicts(),
	}
}

// buildFilter 将查询参数转换为仓储条件并解析日期区间。
func buildFilter(query ListQuery) (Filter, error) {
	filter := Filter{
		Keyword:      strings.TrimSpace(query.Keyword),
		FaultID:      query.FaultID,
		RepairID:     query.RepairID,
		Status:       strings.TrimSpace(query.Status),
		Verdict:      strings.TrimSpace(query.Verdict),
		Satisfaction: strings.TrimSpace(query.Satisfaction),
	}
	if filter.Status != "" && !IsValidStatus(filter.Status) {
		return filter, apperr.BadRequest("非法的回访状态: %s", filter.Status)
	}
	if filter.Verdict != "" && !IsValidVerdict(filter.Verdict) {
		return filter, apperr.BadRequest("非法的回访判定: %s", filter.Verdict)
	}
	if filter.Satisfaction != "" && !IsValidSatisfaction(filter.Satisfaction) {
		return filter, apperr.BadRequest("非法的满意度: %s", filter.Satisfaction)
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
