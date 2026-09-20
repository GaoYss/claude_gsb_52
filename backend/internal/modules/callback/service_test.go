package callback_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"streetlight/internal/apperr"
	"streetlight/internal/modules/callback"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/repair"
	"streetlight/internal/modules/status"
)

// harness 使用内存数据库装配真实模块, 验证回访与返修的跨模块业务流程。
type harness struct {
	lamps     *lamp.Service
	faults    *fault.Service
	repairs   *repair.Service
	callbacks *callback.Service
	status    *status.Service
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&lamp.Lamp{}, &fault.Fault{}, &repair.Repair{}, &callback.Callback{}))

	lampRepository := lamp.NewRepository(db)
	lampService := lamp.NewService(lampRepository)

	faultRepository := fault.NewRepository(db)
	faultService := fault.NewService(faultRepository, lampService)
	lampService.SetOpenFaultCounter(faultRepository)

	repairRepository := repair.NewRepository(db)
	repairService := repair.NewService(repairRepository, faultService)

	callbackRepository := callback.NewRepository(db)
	callbackService := callback.NewService(callbackRepository, repairService)
	repairService.SetCallbackPort(callbackService)
	faultService.SetCallbackGuard(callbackService)

	return &harness{
		lamps:     lampService,
		faults:    faultService,
		repairs:   repairService,
		callbacks: callbackService,
		status:    status.NewService(db, lampRepository, faultRepository, repairRepository, callbackRepository),
	}
}

func (h *harness) createLamp(t *testing.T, code string) *lamp.Lamp {
	t.Helper()
	entity, err := h.lamps.Create(context.Background(), lamp.CreateRequest{
		Code:     code,
		Name:     "测试灯杆",
		RoadName: "测试路",
		LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)
	return entity
}

func (h *harness) createFault(t *testing.T, lampID uint) *fault.Fault {
	t.Helper()
	entity, err := h.faults.Create(context.Background(), fault.CreateRequest{
		LampID:      lampID,
		FaultType:   "灯不亮",
		FaultLevel:  fault.LevelHigh,
		Source:      fault.SourceInspection,
		Description: "回访流程测试故障",
		Reporter:    "巡检员",
	})
	require.NoError(t, err)
	return entity
}

// finishFixed 登记维修并以"已修复"完工, 返回完工记录。
func (h *harness) finishFixed(t *testing.T, faultID uint, repairman string) *repair.Repair {
	t.Helper()
	ctx := context.Background()
	record, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: faultID, Repairman: repairman})
	require.NoError(t, err)
	finished, err := h.repairs.Finish(ctx, record.ID, repair.FinishRequest{Result: repair.ResultFixed, Content: "更换驱动电源"})
	require.NoError(t, err)
	return finished
}

// pendingCallback 返回故障当前唯一待回访任务。
func (h *harness) pendingCallback(t *testing.T, faultID uint) *callback.Callback {
	t.Helper()
	tasks, err := h.callbacks.ListByFault(context.Background(), faultID)
	require.NoError(t, err)
	for index := range tasks {
		if tasks[index].Status == callback.StatusPending {
			return &tasks[index]
		}
	}
	t.Fatalf("故障 %d 不存在待回访任务", faultID)
	return nil
}

// requireConflict 断言错误是 409 业务冲突。
func requireConflict(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusConflict, businessErr.Status, "错误信息: %s", businessErr.Message)
}

// requireBadRequest 断言错误是 400 参数错误。
func requireBadRequest(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusBadRequest, businessErr.Status, "错误信息: %s", businessErr.Message)
}

func TestCallbackTaskGeneratedOnFixedFinish(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-001")
	entity := h.createFault(t, device.ID)

	finished := h.finishFixed(t, entity.ID, "维修工甲")

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1, "已修复完工后应生成一条回访任务")

	task := tasks[0]
	require.Regexp(t, `^HF\d{8}\d{4}$`, task.CallbackNo)
	require.Equal(t, callback.StatusPending, task.Status)
	require.Equal(t, 1, task.Round)
	require.Equal(t, finished.ID, task.RepairID)
	require.Equal(t, finished.RepairNo, task.RepairNo)
	require.Equal(t, entity.FaultNo, task.FaultNo)
	require.Equal(t, device.Code, task.LampCode)
}

func TestNoCallbackForNonFixedResult(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-002")
	entity := h.createFault(t, device.ID)

	record, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: entity.ID, Repairman: "维修工乙"})
	require.NoError(t, err)
	_, err = h.repairs.Finish(ctx, record.ID, repair.FinishRequest{Result: repair.ResultPendingParts})
	require.NoError(t, err)

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Empty(t, tasks, "非已修复结果不应生成回访任务")
}

func TestCloseBlockedUntilCallbackCompleted(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-003")
	entity := h.createFault(t, device.ID)
	h.finishFixed(t, entity.ID, "维修工丙")

	// 回访未完成, 不允许结算关闭
	_, err := h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "尝试结算"})
	requireConflict(t, err)

	// 未联系上: 仅记录联系情况, 任务保持待回访, 仍不允许结算
	task := h.pendingCallback(t, entity.ID)
	recorded, err := h.callbacks.Record(ctx, task.ID, callback.RecordRequest{
		ContactResult: callback.ContactUnreachable,
		Visitor:       "回访员",
		Feedback:      "电话无人接听",
	})
	require.NoError(t, err)
	require.Equal(t, callback.StatusPending, recorded.Status)
	require.Equal(t, callback.ContactUnreachable, recorded.ContactResult)
	require.NotNil(t, recorded.VisitedAt)

	_, err = h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "再次尝试结算"})
	requireConflict(t, err)

	// 已联系但缺少满意度或判定时参数错误
	_, err = h.callbacks.Record(ctx, task.ID, callback.RecordRequest{
		ContactResult: callback.ContactReached,
		Visitor:       "回访员",
	})
	requireBadRequest(t, err)

	// 回访合格: 任务完成, 允许结算关闭
	completed, err := h.callbacks.Record(ctx, task.ID, callback.RecordRequest{
		ContactResult: callback.ContactReached,
		Satisfaction:  callback.SatisfactionSatisfied,
		Verdict:       callback.VerdictQualified,
		Visitor:       "回访员",
		Feedback:      "市民确认照明恢复",
	})
	require.NoError(t, err)
	require.Equal(t, callback.StatusCompleted, completed.Status)
	require.Equal(t, callback.VerdictQualified, completed.Verdict)

	closed, err := h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "回访合格, 结算闭环"})
	require.NoError(t, err)
	require.Equal(t, fault.StatusClosed, closed.Status)

	// 已完成的任务不允许重复登记
	_, err = h.callbacks.Record(ctx, task.ID, callback.RecordRequest{
		ContactResult: callback.ContactReached,
		Satisfaction:  callback.SatisfactionSatisfied,
		Verdict:       callback.VerdictQualified,
		Visitor:       "回访员",
	})
	requireConflict(t, err)
}

func TestUnqualifiedCallbackTriggersReworkAndRevisit(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-004")
	entity := h.createFault(t, device.ID)

	original := h.finishFixed(t, entity.ID, "维修工丁")
	originalFinishedAt := *original.FinishedAt
	originalContent := original.Content

	task := h.pendingCallback(t, entity.ID)
	completed, err := h.callbacks.Record(ctx, task.ID, callback.RecordRequest{
		ContactResult: callback.ContactReached,
		Satisfaction:  callback.SatisfactionUnsatisfied,
		Verdict:       callback.VerdictUnqualified,
		Visitor:       "回访员",
		Feedback:      "市民反映灯具仍不亮",
	})
	require.NoError(t, err)
	require.Equal(t, callback.StatusCompleted, completed.Status)
	require.NotNil(t, completed.ReworkRepairID, "判定不合格应触发返修")
	require.NotEmpty(t, completed.ReworkRepairNo)

	// 返修记录关联原维修记录, 故障回到维修中
	rework, err := h.repairs.Get(ctx, *completed.ReworkRepairID)
	require.NoError(t, err)
	require.Equal(t, repair.StatusOngoing, rework.Status)
	require.NotNil(t, rework.ReworkOfID)
	require.Equal(t, original.ID, *rework.ReworkOfID)
	require.Equal(t, original.RepairNo, rework.ReworkOfNo)
	require.Equal(t, original.Repairman, rework.Repairman, "未指定返修人员时沿用原维修人员")
	require.Contains(t, rework.Content, "市民反映灯具仍不亮")

	faultAfterRework, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusProcessing, faultAfterRework.Status)
	require.Equal(t, 2, faultAfterRework.RepairCount)

	// 原维修记录的完工时间与首次处置内容不被改写
	originalAfter, err := h.repairs.Get(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, repair.StatusFinished, originalAfter.Status)
	require.WithinDuration(t, originalFinishedAt, *originalAfter.FinishedAt, time.Second, "原完工时间不应被改写")
	require.Equal(t, originalContent, originalAfter.Content, "首次处置内容不应被改写")

	// 返修期间不允许重复触发返修
	_, err = h.repairs.CreateRework(ctx, original.ID, repair.ReworkRequest{})
	requireConflict(t, err)

	// 返修完工后生成新一轮回访任务
	reworkFinished, err := h.repairs.Finish(ctx, rework.ID, repair.FinishRequest{Result: repair.ResultFixed, Content: "重新更换灯具"})
	require.NoError(t, err)
	require.Equal(t, repair.StatusFinished, reworkFinished.Status)

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 2, "返修完工后应生成第二轮回访任务")
	require.Equal(t, 2, tasks[1].Round)
	require.Equal(t, callback.StatusPending, tasks[1].Status)
	require.Equal(t, rework.ID, tasks[1].RepairID)

	// 第二轮回访未完成仍不允许结算
	_, err = h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "尝试结算"})
	requireConflict(t, err)

	// 第二轮回访合格后可以结算关闭
	_, err = h.callbacks.Record(ctx, tasks[1].ID, callback.RecordRequest{
		ContactResult: callback.ContactReached,
		Satisfaction:  callback.SatisfactionSatisfied,
		Verdict:       callback.VerdictQualified,
		Visitor:       "回访员",
	})
	require.NoError(t, err)

	closed, err := h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "返修回访合格, 结算闭环"})
	require.NoError(t, err)
	require.Equal(t, fault.StatusClosed, closed.Status)

	// 概览统计: 返修次数与回访汇总
	overview, err := h.status.Overview(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), overview.Repair.ReworkTotal)
	require.Equal(t, int64(2), overview.Callback.Total)
	require.Equal(t, int64(0), overview.Callback.PendingTotal)
	require.Equal(t, int64(2), overview.Callback.CompletedTotal)
	require.Equal(t, int64(1), overview.Callback.UnqualifiedTotal)

	// 追踪链路包含回访记录与返修时间线节点
	track, err := h.status.Track(ctx, status.TrackQuery{FaultNo: entity.FaultNo})
	require.NoError(t, err)
	require.Len(t, track.Callbacks, 2)
	stages := make([]string, 0, len(track.Timeline))
	labels := make([]string, 0, len(track.Timeline))
	for _, event := range track.Timeline {
		stages = append(stages, event.Stage)
		labels = append(labels, event.Label)
	}
	require.Contains(t, stages, "callback_unqualified")
	require.Contains(t, stages, "callback_qualified")
	require.Contains(t, labels, "返修开工")
}

func TestRepairDeleteCancelsPendingCallback(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	device := h.createLamp(t, "LD-C-005")
	entity := h.createFault(t, device.ID)

	finished := h.finishFixed(t, entity.ID, "维修工戊")
	h.pendingCallback(t, entity.ID)

	require.NoError(t, h.repairs.Delete(ctx, finished.ID))

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, callback.StatusCancelled, tasks[0].Status, "维修记录删除后待回访任务应被取消")

	// 无待回访任务, 允许结算关闭
	_, err = h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "维修记录作废"})
	require.NoError(t, err)
}
