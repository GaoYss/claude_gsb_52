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
)

// harness 装配真实的 路灯 -> 故障 -> 维修 -> 回访 模块链路。
type harness struct {
	lamps     *lamp.Service
	faults    *fault.Service
	repairs   *repair.Service
	callbacks *callback.Service
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

	require.NoError(t, db.AutoMigrate(
		&lamp.Lamp{}, &fault.Fault{}, &repair.Repair{},
		&callback.Task{}, &callback.Contact{},
	))

	lampRepository := lamp.NewRepository(db)
	lampService := lamp.NewService(lampRepository)

	faultRepository := fault.NewRepository(db)
	faultService := fault.NewService(faultRepository, lampService)
	lampService.SetOpenFaultCounter(faultRepository)

	repairRepository := repair.NewRepository(db)
	repairService := repair.NewService(repairRepository, faultService)

	callbackRepository := callback.NewRepository(db)
	callbackService := callback.NewService(callbackRepository, repairService)

	// 装配双向钩子, 与 bootstrap 保持一致。
	repairService.SetCallbackHook(callbackService)
	faultService.SetSettlementGuard(callbackService)

	return &harness{
		lamps:     lampService,
		faults:    faultService,
		repairs:   repairService,
		callbacks: callbackService,
	}
}

func (h *harness) prepareFinishedFault(t *testing.T, code string) (*lamp.Lamp, *fault.Fault, *repair.Repair) {
	t.Helper()
	ctx := context.Background()
	device, err := h.lamps.Create(ctx, lamp.CreateRequest{
		Code: code, Name: "测试灯杆", RoadName: "测试路", LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)
	entity, err := h.faults.Create(ctx, fault.CreateRequest{
		LampID: device.ID, FaultType: "灯不亮", Description: "回访流程测试", Reporter: "巡检员",
	})
	require.NoError(t, err)
	record, err := h.repairs.Create(ctx, repair.CreateRequest{
		FaultID: entity.ID, Repairman: "维修工甲", RepairTeam: "市政照明一班",
	})
	require.NoError(t, err)
	finished, err := h.repairs.Finish(ctx, record.ID, repair.FinishRequest{Result: repair.ResultFixed})
	require.NoError(t, err)
	return device, entity, finished
}

func requireConflict(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	businessErr, ok := apperr.As(err)
	require.True(t, ok, "期望业务错误, 实际: %v", err)
	require.Equal(t, http.StatusConflict, businessErr.Status, "错误信息: %s", businessErr.Message)
}

// 完工(已修复)后应按规则生成一条待回访任务, 且非已修复完工不生成。
func TestFinishGeneratesCallbackTask(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)

	_, entity, finished := h.prepareFinishedFault(t, "LD-C-001")

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	task := tasks[0]
	require.Equal(t, callback.StatusPending, task.Status)
	require.Equal(t, 1, task.Round)
	require.Equal(t, finished.ID, task.RepairID)
	require.Equal(t, finished.RepairNo, task.RepairNo)
	require.WithinDuration(t, finished.FinishedAt.Add(callback.DefaultCallbackWithin), task.DueAt, time.Second)

	// 待配件(非已修复)完工不应生成回访任务。
	device2, err := h.lamps.Create(ctx, lamp.CreateRequest{
		Code: "LD-C-002", RoadName: "测试路", LampType: lamp.LampTypeLED,
	})
	require.NoError(t, err)
	fault2, err := h.faults.Create(ctx, fault.CreateRequest{
		LampID: device2.ID, FaultType: "灯不亮", Description: "待配件不回访",
	})
	require.NoError(t, err)
	rec2, err := h.repairs.Create(ctx, repair.CreateRequest{FaultID: fault2.ID, Repairman: "维修工乙"})
	require.NoError(t, err)
	_, err = h.repairs.Finish(ctx, rec2.ID, repair.FinishRequest{Result: repair.ResultPendingParts})
	require.NoError(t, err)
	tasks2, err := h.callbacks.ListByFault(ctx, fault2.ID)
	require.NoError(t, err)
	require.Empty(t, tasks2)
}

// 回访未完成的故障不允许结算关闭; 回访合格后允许。
func TestSettlementBlockedUntilQualified(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, entity, _ := h.prepareFinishedFault(t, "LD-C-003")

	// 回访未完成 -> 禁止结算
	_, closeErr := h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "尝试结算"})
	requireConflict(t, closeErr)

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	taskID := tasks[0].ID

	// 登记联系情况
	detail, err := h.callbacks.AddContact(ctx, taskID, callback.ContactRequest{
		Result: callback.ResultConnected, ContactPerson: "回访员林晓", ContactName: "市民",
		Satisfaction: intPointer(4), Content: "照明正常",
	})
	require.NoError(t, err)
	require.Equal(t, callback.StatusContacted, detail.Status)

	// 已联系但未判定合格, 仍不允许结算
	_, closeErr = h.faults.Close(ctx, entity.ID, fault.CloseRequest{})
	requireConflict(t, closeErr)

	// 判定合格
	_, err = h.callbacks.Judge(ctx, taskID, callback.JudgeRequest{Qualified: true, Satisfaction: intPointer(5)})
	require.NoError(t, err)

	closed, err := h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "回访合格, 结算"})
	require.NoError(t, err)
	require.Equal(t, fault.StatusClosed, closed.Status)
}

// 不合格 -> 触发返修并关联原维修记录; 返修完工后生成第二轮回访。
func TestUnqualifiedTriggersReworkAndRecallback(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, entity, origin := h.prepareFinishedFault(t, "LD-C-004")

	originFinishedAt := *origin.FinishedAt
	originContent := origin.Content

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	taskID := tasks[0].ID

	// 判定不合格时缺少返修维修人, 应报参数错误
	_, err = h.callbacks.Judge(ctx, taskID, callback.JudgeRequest{Qualified: false, Satisfaction: intPointer(2), Remark: "仍不亮"})
	businessErr, ok := apperr.As(err)
	require.True(t, ok)
	require.Equal(t, http.StatusBadRequest, businessErr.Status)

	// 合格字段判定不合格并派返修
	judged, err := h.callbacks.Judge(ctx, taskID, callback.JudgeRequest{
		Qualified: false, Satisfaction: intPointer(2), Remark: "修好当晚又不亮",
		ReworkRepairman: "维修工丙", ReworkRepairTeam: "市政照明二班", ReworkContent: "返场重新处置",
	})
	require.NoError(t, err)
	require.Equal(t, callback.StatusUnqualified, judged.Status)
	require.NotNil(t, judged.ReworkRepairID)
	require.NotEmpty(t, judged.ReworkRepairNo)

	// 返修维修记录已创建、标记返修并关联原记录
	rework, err := h.repairs.GetByID(ctx, *judged.ReworkRepairID)
	require.NoError(t, err)
	require.True(t, rework.IsRework)
	require.NotNil(t, rework.OriginRepairID)
	require.Equal(t, origin.ID, *rework.OriginRepairID)
	require.Equal(t, origin.RepairNo, rework.OriginRepairNo)
	require.Equal(t, repair.StatusOngoing, rework.Status)

	// 故障因返修回到维修中
	faultAfterRework, err := h.faults.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	require.Equal(t, fault.StatusProcessing, faultAfterRework.Status)
	require.Equal(t, 2, faultAfterRework.RepairCount)

	// 原维修记录的完工时间与首次处置过程不被改写
	originAgain, err := h.repairs.GetByID(ctx, origin.ID)
	require.NoError(t, err)
	require.True(t, originAgain.FinishedAt.Equal(originFinishedAt), "原完工时间不应被改写")
	require.Equal(t, originContent, originAgain.Content)
	require.False(t, originAgain.IsRework, "原记录不应被标记为返修")

	// 返修未完工前, 结算仍被第一轮(未合格)任务拦截
	_, closeErr := h.faults.Close(ctx, entity.ID, fault.CloseRequest{})
	requireConflict(t, closeErr)

	// 返修完工 -> 生成第二轮回访任务
	reworkFinished, err := h.repairs.Finish(ctx, rework.ID, repair.FinishRequest{Result: repair.ResultFixed})
	require.NoError(t, err)
	allTasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)
	require.Len(t, allTasks, 2)
	require.Equal(t, 1, allTasks[0].Round)
	require.Equal(t, callback.StatusUnqualified, allTasks[0].Status)
	require.Equal(t, 2, allTasks[1].Round)
	require.Equal(t, callback.StatusPending, allTasks[1].Status)
	require.Equal(t, reworkFinished.ID, allTasks[1].RepairID)

	// 第二轮回访合格后允许结算
	_, err = h.callbacks.Judge(ctx, allTasks[1].ID, callback.JudgeRequest{Qualified: true, Satisfaction: intPointer(5)})
	require.NoError(t, err)
	_, err = h.faults.Close(ctx, entity.ID, fault.CloseRequest{Remark: "返修后回访合格"})
	require.NoError(t, err)
}

// 返修次数进入统计。
func TestReworkCountInStatistics(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, entity, _ := h.prepareFinishedFault(t, "LD-C-005")

	tasks, err := h.callbacks.ListByFault(ctx, entity.ID)
	require.NoError(t, err)

	_, err = h.callbacks.Judge(ctx, tasks[0].ID, callback.JudgeRequest{
		Qualified: false, Satisfaction: intPointer(1), Remark: "未修复",
		ReworkRepairman: "维修工丁",
	})
	require.NoError(t, err)

	stats, err := h.callbacks.Statistics(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.UnqualifiedTotal)
	require.Equal(t, int64(1), stats.ReworkTotal)
	require.Equal(t, int64(1), stats.OpenTotal)
}

// 已生成回访任务的维修记录不允许删除。
func TestRepairWithCallbackNotDeletable(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, _, finished := h.prepareFinishedFault(t, "LD-C-006")
	requireConflict(t, h.repairs.Delete(ctx, finished.ID))
}

func intPointer(v int) *int { return &v }
