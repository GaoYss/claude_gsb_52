package bootstrap

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"streetlight/internal/modules/callback"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/repair"
)

// seedCallbackBundle 收集一条回访任务及其联系记录, 任务落库后统一回填联系记录的 TaskID。
type seedCallbackBundle struct {
	task     callback.Task
	contacts []callback.Contact
}

// seedCallbacks 依据演示故障与维修记录补充质量回访数据, 覆盖:
// 待回访(含超期)、已联系待判定、回访合格、回访不合格触发返修并重新回访合格 等场景。
func seedCallbacks(db *gorm.DB, now time.Time, cases []seedFaultCase, faults []fault.Fault, repairs []repair.Repair, repairRanges [][2]int) error {
	bundles := make([]seedCallbackBundle, 0)
	taskSeq := map[string]int{}

	// lastFinishedFixed 返回某故障下最后一条"已修复"完工维修在 repairs 中的下标, 无则返回 -1。
	lastFinishedFixed := func(faultIndex int) int {
		start, end := repairRanges[faultIndex][0], repairRanges[faultIndex][1]
		for index := end - 1; index >= start; index-- {
			if repairs[index].Status == repair.StatusFinished && repairs[index].Result == repair.ResultFixed {
				return index
			}
		}
		return -1
	}

	intPtr := func(v int) *int { return &v }
	boolPtr := func(v bool) *bool { return &v }

	// newTaskNo 按"HF + 完工日期"生成回访演示单号。
	newTaskNo := func(finishedAt time.Time) string {
		prefix := "HF" + finishedAt.Format("20060102")
		taskSeq[prefix]++
		return fmt.Sprintf("%s%04d", prefix, taskSeq[prefix])
	}

	// fromRepair 依据某次完工维修构造一条回访任务, 生成时间取完工后 30 分钟以保证时间线顺序合理。
	fromRepair := func(record repair.Repair, round int, status string, dueAt, contactedAt *time.Time,
		satisfaction *int, reworkID *uint, reworkNo, remark string) callback.Task {
		return callback.Task{
			TaskNo:         newTaskNo(*record.FinishedAt),
			FaultID:        record.FaultID,
			FaultNo:        record.FaultNo,
			LampID:         record.LampID,
			LampCode:       record.LampCode,
			RepairID:       record.ID,
			RepairNo:       record.RepairNo,
			Round:          round,
			Status:         status,
			DueAt:          *dueAt,
			ContactedAt:    contactedAt,
			Satisfaction:   satisfaction,
			ResultRemark:   remark,
			ReworkRepairID: reworkID,
			ReworkRepairNo: reworkNo,
			CreatedAt:      record.FinishedAt.Add(30 * time.Minute),
		}
	}

	contact := func(channel, result, person, name string, satisfaction *int, qualified *bool,
		content string, at time.Time) callback.Contact {
		return callback.Contact{
			Channel:       channel,
			Result:        result,
			ContactPerson: person,
			ContactName:   name,
			Satisfaction:  satisfaction,
			Qualified:     qualified,
			Content:       content,
			ContactedAt:   at,
		}
	}

	// index 5: 已修复但回访尚未联系, 时限置为过去以演示超期。
	if index := lastFinishedFixed(5); index >= 0 {
		record := repairs[index]
		due := record.FinishedAt.Add(2 * time.Hour)
		bundles = append(bundles, seedCallbackBundle{
			task: fromRepair(record, 1, callback.StatusPending, &due, nil, nil, nil, "", ""),
		})
	}

	// index 6: 已电话联系并记录满意度, 等待合格判定。
	if index := lastFinishedFixed(6); index >= 0 {
		record := repairs[index]
		due := record.FinishedAt.Add(callback.DefaultCallbackWithin)
		contactedAt := now.Add(-30 * time.Hour)
		task := fromRepair(record, 1, callback.StatusContacted, &due, &contactedAt, intPtr(4), nil, "", "")
		bundles = append(bundles, seedCallbackBundle{
			task: task,
			contacts: []callback.Contact{
				contact(callback.ChannelPhone, callback.ResultConnected, "回访员林晓", "报修人王建国",
					intPtr(4), nil, "电话回访, 市民反馈灯具照明正常", contactedAt),
			},
		})
	}

	// index 7,8,9,10: 已闭环故障, 末次修复回访合格。
	for _, faultIndex := range []int{7, 8, 9, 10} {
		index := lastFinishedFixed(faultIndex)
		if index < 0 {
			continue
		}
		record := repairs[index]
		contactedAt := record.FinishedAt.Add(2 * time.Hour)
		due := record.FinishedAt.Add(callback.DefaultCallbackWithin)
		task := fromRepair(record, 1, callback.StatusQualified, &due, &contactedAt, intPtr(5), nil, "", "回访合格, 故障闭环结算")
		bundles = append(bundles, seedCallbackBundle{
			task: task,
			contacts: []callback.Contact{
				contact(callback.ChannelPhone, callback.ResultConnected, "回访员林晓", cases[faultIndex].reporter,
					intPtr(5), boolPtr(true), "市民确认照明恢复, 满意", contactedAt),
			},
		})
	}

	// index 11: 首次回访不合格触发返修, 返修完工后重新回访合格(完整链路)。
	if originIndex := lastFinishedFixed(11); originIndex >= 0 {
		origin := repairs[originIndex]

		// 返修记录: 关联原维修, 不改写原记录的完工时间与首次处置过程。
		reworkStart := now.Add(-5 * time.Hour)
		reworkFinish := now.Add(-2 * time.Hour)
		prefix := "WX" + reworkStart.Format("20060102")
		rework := repair.Repair{
			RepairNo:       fmt.Sprintf("%s9001", prefix),
			FaultID:        origin.FaultID,
			FaultNo:        origin.FaultNo,
			LampID:         origin.LampID,
			LampCode:       origin.LampCode,
			Repairman:      "陈鹏",
			RepairTeam:     "市政照明二班",
			ContactPhone:   "13900005678",
			StartedAt:      reworkStart,
			FinishedAt:     &reworkFinish,
			Status:         repair.StatusFinished,
			Result:         repair.ResultFixed,
			Content:        "回访发现接触器仍有异响, 再次更换并复测三个通断周期",
			Materials:      "交流接触器 1 只",
			Cost:           160,
			IsRework:       true,
			OriginRepairID: &origin.ID,
			OriginRepairNo: origin.RepairNo,
		}
		if err := db.Create(&rework).Error; err != nil {
			return fmt.Errorf("写入返修演示数据失败: %w", err)
		}
		// 返修后故障维修次数 +1, 最新维修指向返修单。
		if err := db.Model(&fault.Fault{}).Where("id = ?", origin.FaultID).
			Updates(map[string]any{"repair_count": gorm.Expr("repair_count + 1"), "latest_repair_id": rework.ID}).Error; err != nil {
			return fmt.Errorf("回填返修故障统计失败: %w", err)
		}

		// 第一轮: 不合格。
		contacted1 := origin.FinishedAt.Add(3 * time.Hour)
		due1 := origin.FinishedAt.Add(callback.DefaultCallbackWithin)
		unqualified := fromRepair(origin, 1, callback.StatusUnqualified, &due1, &contacted1, intPtr(2),
			&rework.ID, rework.RepairNo, "回访不合格: 接触器仍有异响, 夜间偶发不吸合")
		bundles = append(bundles, seedCallbackBundle{
			task: unqualified,
			contacts: []callback.Contact{
				contact(callback.ChannelPhone, callback.ResultConnected, "回访员林晓", "社区网格员",
					intPtr(2), nil, "市民反馈修好当晚又出现一次不亮", contacted1),
				contact(callback.ChannelPhone, callback.ResultConnected, "回访员林晓", "社区网格员",
					intPtr(2), boolPtr(false), "判定不合格, 已触发返修 "+rework.RepairNo, contacted1.Add(time.Minute)),
			},
		})

		// 第二轮: 返修完工后重新回访合格。
		contacted2 := reworkFinish.Add(2 * time.Hour)
		due2 := reworkFinish.Add(callback.DefaultCallbackWithin)
		qualified := callback.Task{
			TaskNo:       newTaskNo(reworkFinish),
			FaultID:      rework.FaultID,
			FaultNo:      rework.FaultNo,
			LampID:       rework.LampID,
			LampCode:     rework.LampCode,
			RepairID:     rework.ID,
			RepairNo:     rework.RepairNo,
			Round:        2,
			Status:       callback.StatusQualified,
			DueAt:        due2,
			ContactedAt:  &contacted2,
			Satisfaction: intPtr(5),
			ResultRemark: "返修后重新回访合格",
			CreatedAt:    reworkFinish.Add(30 * time.Minute),
		}
		bundles = append(bundles, seedCallbackBundle{
			task: qualified,
			contacts: []callback.Contact{
				contact(callback.ChannelOnsite, callback.ResultConnected, "回访员林晓", "社区网格员",
					intPtr(5), boolPtr(true), "返修后现场连续观察两晚, 开关灯正常, 满意", contacted2),
			},
		})
	}

	if len(bundles) == 0 {
		return nil
	}

	// 先落库任务取得 ID, 再回填联系记录的 TaskID 并批量写入。
	allContacts := make([]callback.Contact, 0)
	for index := range bundles {
		if err := db.Create(&bundles[index].task).Error; err != nil {
			return fmt.Errorf("写入回访任务演示数据失败: %w", err)
		}
		for contactIndex := range bundles[index].contacts {
			bundles[index].contacts[contactIndex].TaskID = bundles[index].task.ID
		}
		allContacts = append(allContacts, bundles[index].contacts...)
	}
	if len(allContacts) > 0 {
		if err := db.Create(&allContacts).Error; err != nil {
			return fmt.Errorf("写入回访联系记录演示数据失败: %w", err)
		}
	}
	return nil
}
