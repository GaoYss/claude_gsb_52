package callback

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"streetlight/internal/apperr"
	"streetlight/pkg/pagination"
)

// Filter 是仓储层使用的回访任务查询条件。
type Filter struct {
	Keyword      string
	Status       string
	FaultID      uint
	RepairID     uint
	Round        int
	OpenStatuses []string
	DueBefore    *time.Time // 超期: 未完成且时限早于该时刻
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

// Repository 负责回访任务与联系记录的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造回访仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// CreateTask 新增回访任务。
func (r *Repository) CreateTask(ctx context.Context, entity *Task) error {
	if err := r.session(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("创建回访任务失败: %w", err)
	}
	return nil
}

// CreateTaskWithUniqueNo 生成唯一回访单号(HF + 日期 + 4 位序号)并落库。
func (r *Repository) CreateTaskWithUniqueNo(ctx context.Context, entity *Task, prefix string) error {
	for attempt := 0; attempt < 5; attempt++ {
		sequence, err := r.nextSequence(ctx, prefix)
		if err != nil {
			return err
		}
		entity.TaskNo = fmt.Sprintf("%s%04d", prefix, sequence+attempt)
		err = r.CreateTask(ctx, entity)
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return apperr.Conflict("回访单号生成冲突, 请稍后重试")
}

func (r *Repository) nextSequence(ctx context.Context, prefix string) (int, error) {
	var latest string
	err := r.session(ctx).Model(&Task{}).
		Where("task_no LIKE ?", prefix+"%").
		Order("task_no DESC").
		Limit(1).
		Pluck("task_no", &latest).Error
	if err != nil {
		return 0, fmt.Errorf("生成回访单号失败: %w", err)
	}
	if latest == "" {
		return 1, nil
	}
	value, convErr := strconv.Atoi(strings.TrimPrefix(latest, prefix))
	if convErr != nil {
		return 1, nil
	}
	return value + 1, nil
}

// UpdateTask 保存回访任务全部字段。
func (r *Repository) UpdateTask(ctx context.Context, entity *Task) error {
	if err := r.session(ctx).Save(entity).Error; err != nil {
		return fmt.Errorf("更新回访任务失败: %w", err)
	}
	return nil
}

// GetTaskByID 按主键查询回访任务。
func (r *Repository) GetTaskByID(ctx context.Context, id uint) (*Task, error) {
	var entity Task
	err := r.session(ctx).First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("回访任务不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询回访任务失败: %w", err)
	}
	return &entity, nil
}

// GetTaskByRepairID 查询某次维修对应的回访任务, 不存在时返回 nil。
func (r *Repository) GetTaskByRepairID(ctx context.Context, repairID uint) (*Task, error) {
	var entity Task
	err := r.session(ctx).Where("repair_id = ?", repairID).Order("id DESC").First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询维修回访任务失败: %w", err)
	}
	return &entity, nil
}

// LatestTaskByFault 查询某故障当前最新一轮回访任务, 不存在时返回 nil。
func (r *Repository) LatestTaskByFault(ctx context.Context, faultID uint) (*Task, error) {
	var entity Task
	err := r.session(ctx).Where("fault_id = ?", faultID).Order("round DESC, id DESC").First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询故障最新回访任务失败: %w", err)
	}
	return &entity, nil
}

// ListByFault 查询某条故障的全部回访任务, 按轮次正序。
func (r *Repository) ListByFault(ctx context.Context, faultID uint) ([]Task, error) {
	entities := make([]Task, 0)
	err := r.session(ctx).Where("fault_id = ?", faultID).Order("round ASC, id ASC").Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询故障回访任务失败: %w", err)
	}
	return entities, nil
}

// List 分页查询回访任务。
func (r *Repository) List(ctx context.Context, filter Filter, page pagination.Query) ([]Task, int64, error) {
	base := func() *gorm.DB {
		return applyFilter(r.session(ctx).Model(&Task{}), filter)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计回访任务失败: %w", err)
	}

	entities := make([]Task, 0)
	if err := base().Order(page.OrderClause()).Offset(page.Offset()).Limit(page.Limit()).Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("查询回访任务失败: %w", err)
	}
	return entities, total, nil
}

// CountByColumn 按列分组统计回访任务。
func (r *Repository) CountByColumn(ctx context.Context, column string) (map[string]int64, error) {
	type row struct {
		Label string
		Total int64
	}
	rows := make([]row, 0)
	err := r.session(ctx).Model(&Task{}).
		Select(column + " AS label, COUNT(*) AS total").
		Group(column).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("分组统计 %s 失败: %w", column, err)
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Label] = item.Total
	}
	return result, nil
}

// Count 统计回访任务总数。
func (r *Repository) Count(ctx context.Context) (int64, error) {
	var total int64
	if err := r.session(ctx).Model(&Task{}).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("统计回访任务总数失败: %w", err)
	}
	return total, nil
}

// CountOverdue 统计截止 before 仍未完成且已过回访时限的任务数量。
func (r *Repository) CountOverdue(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	err := r.session(ctx).Model(&Task{}).
		Where("status <> ? AND due_at < ?", StatusQualified, before).
		Count(&total).Error
	if err != nil {
		return 0, fmt.Errorf("统计超期回访任务失败: %w", err)
	}
	return total, nil
}

// DistinctValues 返回某列的去重取值, 用于下拉选项。
func (r *Repository) DistinctValues(ctx context.Context, column string) ([]string, error) {
	values := make([]string, 0)
	err := r.session(ctx).Model(&Contact{}).
		Where(column+" <> ''").
		Distinct().
		Order(column).
		Pluck(column, &values).Error
	if err != nil {
		return nil, fmt.Errorf("查询 %s 选项失败: %w", column, err)
	}
	return values, nil
}

// CreateContact 新增一条回访联系记录。
func (r *Repository) CreateContact(ctx context.Context, entity *Contact) error {
	if err := r.session(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("登记回访联系记录失败: %w", err)
	}
	return nil
}

// ListContactsByTask 查询某次回访任务的全部联系记录, 按联系时间正序。
func (r *Repository) ListContactsByTask(ctx context.Context, taskID uint) ([]Contact, error) {
	entities := make([]Contact, 0)
	err := r.session(ctx).Where("task_id = ?", taskID).
		Order("contacted_at ASC, id ASC").Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询回访联系记录失败: %w", err)
	}
	return entities, nil
}

// ListContactsByFaults 批量查询多条故障的联系记录, 供追踪视图组装时间线。
func (r *Repository) ListContactsByFaults(ctx context.Context, faultIDs []uint) (map[uint][]Contact, error) {
	result := make(map[uint][]Contact)
	if len(faultIDs) == 0 {
		return result, nil
	}
	tasks := make([]Task, 0)
	if err := r.session(ctx).Model(&Task{}).Where("fault_id IN ?", faultIDs).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("查询故障回访任务失败: %w", err)
	}
	taskIDs := make([]uint, 0, len(tasks))
	taskFault := make(map[uint]uint, len(tasks))
	for _, item := range tasks {
		taskIDs = append(taskIDs, item.ID)
		taskFault[item.ID] = item.FaultID
	}
	if len(taskIDs) == 0 {
		return result, nil
	}
	contacts := make([]Contact, 0)
	if err := r.session(ctx).Model(&Contact{}).Where("task_id IN ?", taskIDs).
		Order("contacted_at ASC, id ASC").Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("查询回访联系记录失败: %w", err)
	}
	for _, item := range contacts {
		faultID := taskFault[item.TaskID]
		result[faultID] = append(result[faultID], item)
	}
	return result, nil
}

// applyFilter 统一拼装回访任务查询条件。
func applyFilter(statement *gorm.DB, filter Filter) *gorm.DB {
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		statement = statement.Where(
			"task_no LIKE ? OR fault_no LIKE ? OR lamp_code LIKE ? OR repair_no LIKE ?",
			like, like, like, like,
		)
	}
	if filter.Status != "" {
		statement = statement.Where("status = ?", filter.Status)
	}
	if filter.FaultID > 0 {
		statement = statement.Where("fault_id = ?", filter.FaultID)
	}
	if filter.RepairID > 0 {
		statement = statement.Where("repair_id = ?", filter.RepairID)
	}
	if filter.Round > 0 {
		statement = statement.Where("round = ?", filter.Round)
	}
	if len(filter.OpenStatuses) > 0 {
		statement = statement.Where("status IN ?", filter.OpenStatuses)
	}
	if filter.DueBefore != nil {
		statement = statement.Where("status <> ? AND due_at < ?", StatusQualified, *filter.DueBefore)
	}
	if filter.CreatedFrom != nil {
		statement = statement.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		statement = statement.Where("created_at < ?", *filter.CreatedTo)
	}
	return statement
}

// isUniqueViolation 兼容 sqlite 与 postgres 的唯一约束冲突判断。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "unique violation")
}
