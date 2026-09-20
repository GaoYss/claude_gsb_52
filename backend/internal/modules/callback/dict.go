package callback

var statusLabels = map[string]string{
	StatusPending:   "待回访",
	StatusCompleted: "已回访",
	StatusCancelled: "已取消",
}

var contactResultLabels = map[string]string{
	ContactReached:     "已联系",
	ContactUnreachable: "未联系上",
}

var satisfactionLabels = map[string]string{
	SatisfactionSatisfied:   "满意",
	SatisfactionNeutral:     "基本满意",
	SatisfactionUnsatisfied: "不满意",
}

var verdictLabels = map[string]string{
	VerdictQualified:   "合格",
	VerdictUnqualified: "不合格",
}

// StatusLabel 返回回访任务状态的中文名称。
func StatusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}

// ContactResultLabel 返回联系情况的中文名称。
func ContactResultLabel(value string) string {
	if label, ok := contactResultLabels[value]; ok {
		return label
	}
	return value
}

// SatisfactionLabel 返回满意度的中文名称。
func SatisfactionLabel(value string) string {
	if label, ok := satisfactionLabels[value]; ok {
		return label
	}
	return value
}

// VerdictLabel 返回回访判定的中文名称。
func VerdictLabel(value string) string {
	if label, ok := verdictLabels[value]; ok {
		return label
	}
	return value
}
