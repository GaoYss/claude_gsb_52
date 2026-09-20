package callback

import "time"

// 回访任务状态。
const (
	StatusPending     = "pending"     // 待回访
	StatusContacted   = "contacted"   // 已联系, 待判定
	StatusQualified   = "qualified"   // 回访合格
	StatusUnqualified = "unqualified" // 回访不合格, 已触发返修
)

// 联系结果。
const (
	ResultConnected = "connected" // 已联系
	ResultNoAnswer  = "no_answer" // 未接通
	ResultPostponed = "postponed" // 受访人要求延期
	ResultInvalid   = "invalid"   // 号码有误/无法联系
)

// 联系渠道。
const (
	ChannelPhone  = "phone"  // 电话回访
	ChannelOnsite = "onsite" // 现场回访
)

// DefaultCallbackWithin 是默认的回访时限: 维修完工后 24 小时内完成回访。
const DefaultCallbackWithin = 24 * time.Hour

// TaskStatuses 返回全部回访任务状态。
func TaskStatuses() []string {
	return []string{StatusPending, StatusContacted, StatusQualified, StatusUnqualified}
}

// ContactResults 返回全部联系结果取值。
func ContactResults() []string {
	return []string{ResultConnected, ResultNoAnswer, ResultPostponed, ResultInvalid}
}

// ContactChannels 返回全部联系渠道取值。
func ContactChannels() []string {
	return []string{ChannelPhone, ChannelOnsite}
}

// IsValidTaskStatus 校验回访任务状态取值。
func IsValidTaskStatus(status string) bool {
	for _, item := range TaskStatuses() {
		if item == status {
			return true
		}
	}
	return false
}

// IsValidContactResult 校验联系结果取值。
func IsValidContactResult(result string) bool {
	for _, item := range ContactResults() {
		if item == result {
			return true
		}
	}
	return false
}

// IsOpen 判断回访任务是否尚未给出合格结论(含待回访/已联系待判定/已触发返修等待重新回访)。
func (t *Task) IsOpen() bool {
	return t.Status == StatusPending || t.Status == StatusContacted || t.Status == StatusUnqualified
}

// Task 维修质量回访任务, 每次"已修复"完工按规则生成一条; 返修完工后生成新一轮任务。
type Task struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TaskNo         string     `gorm:"size:64;uniqueIndex;not null" json:"task_no"`
	FaultID        uint       `gorm:"index;not null" json:"fault_id"`
	FaultNo        string     `gorm:"size:64;index" json:"fault_no"`
	LampID         uint       `gorm:"index" json:"lamp_id"`
	LampCode       string     `gorm:"size:64;index" json:"lamp_code"`
	RepairID       uint       `gorm:"index;not null" json:"repair_id"`
	RepairNo       string     `gorm:"size:64;index" json:"repair_no"`
	Round          int        `gorm:"not null;default:1" json:"round"` // 回访轮次: 首次回访为 1, 返修后递增
	Status         string     `gorm:"size:32;index;not null;default:pending" json:"status"`
	DueAt          time.Time  `gorm:"index;not null" json:"due_at"` // 计划完成回访时限
	ContactedAt    *time.Time `gorm:"index" json:"contacted_at"`    // 末次有效联系时间
	Satisfaction   *int       `json:"satisfaction"`                 // 满意度 1-5
	ResultRemark   string     `gorm:"size:512" json:"result_remark"`
	ReworkRepairID *uint      `gorm:"index" json:"rework_repair_id"` // 不合格时触发的返修维修单
	ReworkRepairNo string     `gorm:"size:64" json:"rework_repair_no"`

	// Contacts 为联系情况明细, 仅详情/追踪接口组装, 不落库。
	Contacts []Contact `gorm:"-" json:"contacts,omitempty"`
	// Overdue 仅用于响应展示: 是否已超期未完成, 不落库。
	Overdue bool `gorm:"-" json:"overdue,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Task) TableName() string { return "callback_task" }

// Contact 回访联系情况记录, 一条任务可多次联系(未接通可再次回访)。
type Contact struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TaskID        uint      `gorm:"index;not null" json:"task_id"`
	Channel       string    `gorm:"size:16;not null;default:phone" json:"channel"`
	Result        string    `gorm:"size:16;index;not null" json:"result"`
	ContactPerson string    `gorm:"size:64;not null" json:"contact_person"` // 回访人
	ContactName   string    `gorm:"size:64" json:"contact_name"`            // 受访人
	Satisfaction  *int      `json:"satisfaction"`                           // 满意度 1-5, 仅有效联系时记录
	Qualified     *bool     `json:"qualified"`                              // 终判结论, 仅判定动作写入
	Content       string    `gorm:"size:512" json:"content"`                // 联系情况/不合格原因
	ContactedAt   time.Time `gorm:"index;not null" json:"contacted_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Contact) TableName() string { return "callback_contact" }
