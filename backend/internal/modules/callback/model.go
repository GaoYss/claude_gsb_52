package callback

import "time"

// 回访任务状态。
const (
	StatusPending   = "pending"   // 待回访
	StatusCompleted = "completed" // 已回访
	StatusCancelled = "cancelled" // 已取消(对应维修记录被删除)
)

// 联系情况。
const (
	ContactReached     = "reached"     // 已联系
	ContactUnreachable = "unreachable" // 未联系上
)

// 满意度。
const (
	SatisfactionSatisfied   = "satisfied"   // 满意
	SatisfactionNeutral     = "neutral"     // 基本满意
	SatisfactionUnsatisfied = "unsatisfied" // 不满意
)

// 回访判定。
const (
	VerdictQualified   = "qualified"   // 合格
	VerdictUnqualified = "unqualified" // 不合格
)

// Statuses 返回全部回访任务状态取值。
func Statuses() []string {
	return []string{StatusPending, StatusCompleted, StatusCancelled}
}

// ContactResults 返回全部联系情况取值。
func ContactResults() []string {
	return []string{ContactReached, ContactUnreachable}
}

// Satisfactions 返回全部满意度取值。
func Satisfactions() []string {
	return []string{SatisfactionSatisfied, SatisfactionNeutral, SatisfactionUnsatisfied}
}

// Verdicts 返回全部回访判定取值。
func Verdicts() []string {
	return []string{VerdictQualified, VerdictUnqualified}
}

// IsValidStatus 校验回访任务状态取值。
func IsValidStatus(status string) bool {
	for _, item := range Statuses() {
		if item == status {
			return true
		}
	}
	return false
}

// IsValidContactResult 校验联系情况取值。
func IsValidContactResult(value string) bool {
	for _, item := range ContactResults() {
		if item == value {
			return true
		}
	}
	return false
}

// IsValidSatisfaction 校验满意度取值。
func IsValidSatisfaction(value string) bool {
	for _, item := range Satisfactions() {
		if item == value {
			return true
		}
	}
	return false
}

// IsValidVerdict 校验回访判定取值。
func IsValidVerdict(value string) bool {
	for _, item := range Verdicts() {
		if item == value {
			return true
		}
	}
	return false
}

// Callback 维修质量回访任务, 维修完工后按规则生成, 记录联系情况与满意度。
// 判定不合格时触发返修并关联原维修记录, 返修完工后生成新一轮回访任务。
type Callback struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	CallbackNo     string     `gorm:"size:64;uniqueIndex;not null" json:"callback_no"`
	FaultID        uint       `gorm:"index;not null" json:"fault_id"`
	FaultNo        string     `gorm:"size:64;index" json:"fault_no"`
	RepairID       uint       `gorm:"index;not null" json:"repair_id"`
	RepairNo       string     `gorm:"size:64;index" json:"repair_no"`
	LampID         uint       `gorm:"index" json:"lamp_id"`
	LampCode       string     `gorm:"size:64;index" json:"lamp_code"`
	Round          int        `gorm:"not null;default:1" json:"round"` // 第几轮回访, 返修后重新回访递增
	Status         string     `gorm:"size:32;index;not null;default:pending" json:"status"`
	ContactResult  string     `gorm:"size:32;index" json:"contact_result"`
	Satisfaction   string     `gorm:"size:32;index" json:"satisfaction"`
	Verdict        string     `gorm:"size:32;index" json:"verdict"`
	Visitor        string     `gorm:"size:64" json:"visitor"`
	VisitedAt      *time.Time `json:"visited_at"`
	Feedback       string     `gorm:"size:512" json:"feedback"`
	ReworkRepairID *uint      `json:"rework_repair_id"` // 判定不合格时触发的返修维修记录
	ReworkRepairNo string     `gorm:"size:64" json:"rework_repair_no"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (Callback) TableName() string { return "callback" }
