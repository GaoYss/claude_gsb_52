package callback

import "streetlight/pkg/pagination"

// RecordRequest 登记回访结果请求。
// 未联系上时仅记录联系情况, 任务保持待回访; 已联系时需给出满意度与判定,
// 判定不合格将自动触发返修并关联原维修记录。
type RecordRequest struct {
	ContactResult   string `json:"contact_result" binding:"required,oneof=reached unreachable"`
	Satisfaction    string `json:"satisfaction" binding:"omitempty,oneof=satisfied neutral unsatisfied"`
	Verdict         string `json:"verdict" binding:"omitempty,oneof=qualified unqualified"`
	Visitor         string `json:"visitor" binding:"required,max=64"`
	VisitedAt       string `json:"visited_at" binding:"omitempty,max=32"`
	Feedback        string `json:"feedback" binding:"omitempty,max=512"`
	ReworkRepairman string `json:"rework_repairman" binding:"omitempty,max=64"` // 不合格时指定的返修人员, 缺省沿用原维修人员
}

// ListQuery 回访任务查询条件。
type ListQuery struct {
	pagination.Params
	Keyword      string `form:"keyword"` // 回访单号 / 故障单号 / 维修单号 / 路灯编号 / 回访人
	FaultID      uint   `form:"fault_id"`
	RepairID     uint   `form:"repair_id"`
	Status       string `form:"status"`
	Verdict      string `form:"verdict"`
	Satisfaction string `form:"satisfaction"`
	StartDate    string `form:"start_date"` // 任务生成日期起, 格式 YYYY-MM-DD
	EndDate      string `form:"end_date"`   // 任务生成日期止, 格式 YYYY-MM-DD
}

// Meta 回访模块字典, 供前端渲染下拉框。
type Meta struct {
	Statuses       []string `json:"statuses"`
	ContactResults []string `json:"contact_results"`
	Satisfactions  []string `json:"satisfactions"`
	Verdicts       []string `json:"verdicts"`
}
