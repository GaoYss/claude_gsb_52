package callback

import "streetlight/pkg/pagination"

// ContactRequest 登记一次回访联系情况。
type ContactRequest struct {
	Channel       string `json:"channel" binding:"omitempty,oneof=phone onsite"`
	Result        string `json:"result" binding:"required,oneof=connected no_answer postponed invalid"`
	ContactPerson string `json:"contact_person" binding:"required,max=64"`
	ContactName   string `json:"contact_name" binding:"max=64"`
	Satisfaction  *int   `json:"satisfaction" binding:"omitempty,gte=1,lte=5"`
	Content       string `json:"content" binding:"max=512"`
	ContactedAt   string `json:"contacted_at" binding:"omitempty,max=32"`
}

// JudgeRequest 回访合格/不合格判定。合格则闭环; 不合格则触发返修。
type JudgeRequest struct {
	Qualified    bool   `json:"qualified"`
	Satisfaction *int   `json:"satisfaction" binding:"omitempty,gte=1,lte=5"`
	Remark       string `json:"remark" binding:"max=512"`
	// 不合格触发返修时的派工信息, qualified=false 时由服务层校验维修人必填。
	ReworkRepairman  string   `json:"rework_repairman" binding:"max=64"`
	ReworkRepairTeam string   `json:"rework_repair_team" binding:"max=64"`
	ReworkContent    string   `json:"rework_content" binding:"max=512"`
	ReworkCost       *float64 `json:"rework_cost" binding:"omitempty,min=0"`
}

// ListQuery 回访任务查询条件。
type ListQuery struct {
	pagination.Params
	Keyword     string `form:"keyword"` // 回访单号 / 故障单号 / 路灯编号 / 维修单号
	Status      string `form:"status"`
	FaultID     uint   `form:"fault_id"`
	RepairID    uint   `form:"repair_id"`
	Round       int    `form:"round"`
	OnlyOpen    bool   `form:"only_open"`
	OnlyOverdue bool   `form:"only_overdue"` // 仅查询超期未完成
	StartDate   string `form:"start_date"`
	EndDate     string `form:"end_date"`
}

// Meta 回访模块字典。
type Meta struct {
	TaskStatuses    []string `json:"task_statuses"`
	ContactResults  []string `json:"contact_results"`
	ContactChannels []string `json:"contact_channels"`
	ContactPersons  []string `json:"contact_persons"`
}

// Statistics 回访统计结果。
type Statistics struct {
	Total            int64 `json:"total"`
	PendingTotal     int64 `json:"pending_total"`
	ContactedTotal   int64 `json:"contacted_total"`
	QualifiedTotal   int64 `json:"qualified_total"`
	UnqualifiedTotal int64 `json:"unqualified_total"`
	OpenTotal        int64 `json:"open_total"`
	OverdueTotal     int64 `json:"overdue_total"`
	ReworkTotal      int64 `json:"rework_total"` // 触发返修的回访单数(返修次数)
}

// TaskDetail 回访任务详情: 任务 + 全部联系记录。
type TaskDetail struct {
	Task
	Contacts []Contact `json:"contacts"`
}
