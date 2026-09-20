package callback

var taskStatusLabels = map[string]string{
	StatusPending:     "待回访",
	StatusContacted:   "已联系待判定",
	StatusQualified:   "回访合格",
	StatusUnqualified: "回访不合格",
}

var contactResultLabels = map[string]string{
	ResultConnected: "已联系",
	ResultNoAnswer:  "未接通",
	ResultPostponed: "受访人要求延期",
	ResultInvalid:   "无法联系",
}

var contactChannelLabels = map[string]string{
	ChannelPhone:  "电话回访",
	ChannelOnsite: "现场回访",
}

// TaskStatusLabel 返回回访任务状态的中文名称。
func TaskStatusLabel(status string) string {
	if label, ok := taskStatusLabels[status]; ok {
		return label
	}
	return status
}

// ContactResultLabel 返回联系结果的中文名称。
func ContactResultLabel(result string) string {
	if label, ok := contactResultLabels[result]; ok {
		return label
	}
	return result
}

// ContactChannelLabel 返回联系渠道的中文名称。
func ContactChannelLabel(channel string) string {
	if label, ok := contactChannelLabels[channel]; ok {
		return label
	}
	return channel
}
