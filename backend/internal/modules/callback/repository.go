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
	FaultID      uint
	RepairID     uint
	Status       string
	Verdict      string
	Satisfaction string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

// Repository 负责回访任务的数据访问。
type Repository struct {
	db *gorm.DB
}

// NewRepository 构造回访任务仓储。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// Create 新增回访任务。
func (r *Repository) Create(ctx context.Context, entity *Callback) error {
	if err := r.session(ctx).Create(entity).Error; err != nil {
		return fmt.Errorf("生成回访任务失败: %w", err)
	}
	return nil
}

// CreateWithUniqueNo 生成唯一回访单号并落库, 冲突时自动重试。
func (r *Repository) CreateWithUniqueNo(ctx context.Context, entity *Callback, prefix string) error {
	for attempt := 0; attempt < 5; attempt++ {
		sequence, err := r.NextSequence(ctx, prefix)
		if err != nil {
			return err
		}
		entity.CallbackNo = fmt.Sprintf("%s%04d", prefix, sequence+attempt)
		err = r.Create(ctx, entity)
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return apperr.Conflict("回访单号生成冲突, 请稍后重试")
}

// NextSequence 返回指定前缀下可用的下一个流水号。
func (r *Repository) NextSequence(ctx context.Context, prefix string) (int, error) {
	var latest string
	err := r.session(ctx).Model(&Callback{}).
		Where("callback_no LIKE ?", prefix+"%").
		Order("callback_no DESC").
		Limit(1).
		Pluck("callback_no", &latest).Error
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

// Update 保存回访任务全部字段。
func (r *Repository) Update(ctx context.Context, entity *Callback) error {
	if err := r.session(ctx).Save(entity).Error; err != nil {
		return fmt.Errorf("更新回访任务失败: %w", err)
	}
	return nil
}

// GetByID 按主键查询回访任务。
func (r *Repository) GetByID(ctx context.Context, id uint) (*Callback, error) {
	var entity Callback
	err := r.session(ctx).First(&entity, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.NotFound("回访任务不存在: id=%d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询回访任务失败: %w", err)
	}
	return &entity, nil
}

// List 分页查询回访任务。
func (r *Repository) List(ctx context.Context, filter Filter, page pagination.Query) ([]Callback, int64, error) {
	base := func() *gorm.DB {
		return applyFilter(r.session(ctx).Model(&Callback{}), filter)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计回访任务失败: %w", err)
	}

	entities := make([]Callback, 0)
	if err := base().Order(page.OrderClause()).Offset(page.Offset()).Limit(page.Limit()).Find(&entities).Error; err != nil {
		return nil, 0, fmt.Errorf("查询回访任务失败: %w", err)
	}
	return entities, total, nil
}

// ListByFault 查询某条故障的全部回访任务, 按生成时间正序。
func (r *Repository) ListByFault(ctx context.Context, faultID uint) ([]Callback, error) {
	entities := make([]Callback, 0)
	err := r.session(ctx).Where("fault_id = ?", faultID).Order("created_at ASC, id ASC").Find(&entities).Error
	if err != nil {
		return nil, fmt.Errorf("查询故障回访任务失败: %w", err)
	}
	return entities, nil
}

// CountPendingByFault 统计某条故障待回访的任务数量, 用于结算关闭前校验。
func (r *Repository) CountPendingByFault(ctx context.Context, faultID uint) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&Callback{}).
		Where("fault_id = ? AND status = ?", faultID, StatusPending).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计待回访任务失败: %w", err)
	}
	return count, nil
}

// CountByFault 统计某条故障的回访任务数量, 用于推算回访轮次。
func (r *Repository) CountByFault(ctx context.Context, faultID uint) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&Callback{}).Where("fault_id = ?", faultID).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计回访任务数量失败: %w", err)
	}
	return count, nil
}

// CancelPendingByRepair 取消某条维修记录名下未完成的回访任务, 返回取消数量。
func (r *Repository) CancelPendingByRepair(ctx context.Context, repairID uint) (int64, error) {
	result := r.session(ctx).Model(&Callback{}).
		Where("repair_id = ? AND status = ?", repairID, StatusPending).
		Update("status", StatusCancelled)
	if result.Error != nil {
		return 0, fmt.Errorf("取消回访任务失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// CountByColumn 按列分组统计。
func (r *Repository) CountByColumn(ctx context.Context, column string) (map[string]int64, error) {
	type row struct {
		Label string
		Total int64
	}
	rows := make([]row, 0)
	err := r.session(ctx).Model(&Callback{}).
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
	if err := r.session(ctx).Model(&Callback{}).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("统计回访任务总数失败: %w", err)
	}
	return total, nil
}

// CountReworkTriggered 统计已触发返修的回访任务数量。
func (r *Repository) CountReworkTriggered(ctx context.Context) (int64, error) {
	var total int64
	err := r.session(ctx).Model(&Callback{}).Where("rework_repair_id IS NOT NULL").Count(&total).Error
	if err != nil {
		return 0, fmt.Errorf("统计触发返修数量失败: %w", err)
	}
	return total, nil
}

// applyFilter 统一拼装回访任务查询条件。
func applyFilter(statement *gorm.DB, filter Filter) *gorm.DB {
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		statement = statement.Where(
			"callback_no LIKE ? OR fault_no LIKE ? OR repair_no LIKE ? OR lamp_code LIKE ? OR visitor LIKE ?",
			like, like, like, like, like,
		)
	}
	if filter.FaultID > 0 {
		statement = statement.Where("fault_id = ?", filter.FaultID)
	}
	if filter.RepairID > 0 {
		statement = statement.Where("repair_id = ?", filter.RepairID)
	}
	if filter.Status != "" {
		statement = statement.Where("status = ?", filter.Status)
	}
	if filter.Verdict != "" {
		statement = statement.Where("verdict = ?", filter.Verdict)
	}
	if filter.Satisfaction != "" {
		statement = statement.Where("satisfaction = ?", filter.Satisfaction)
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
