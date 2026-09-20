package repair

import (
	"context"
	"time"
)

// FinishEvent 是维修完工事件, 完工后分发给回访模块用于按规则生成回访任务。
// 仅在结果为"已修复"时由完工流程携带完整信息派发。
type FinishEvent struct {
	RepairID   uint
	RepairNo   string
	FaultID    uint
	FaultNo    string
	LampID     uint
	LampCode   string
	Result     string
	Fixed      bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// CallbackHook 由回访模块实现, 维修模块在完工与删除时回调, 避免维修反向依赖回访。
type CallbackHook interface {
	// OnRepairFinished 维修完工后触发(已修复时回访模块据此生成回访任务)。
	OnRepairFinished(ctx context.Context, event FinishEvent)
	// AssertRepairDeletable 删除维修记录前校验(已生成回访任务的记录不允许删除)。
	AssertRepairDeletable(ctx context.Context, repairID uint) error
}

// noopCallbackHook 是未装配回访模块时的空实现。
type noopCallbackHook struct{}

func (noopCallbackHook) OnRepairFinished(ctx context.Context, event FinishEvent) {}
func (noopCallbackHook) AssertRepairDeletable(ctx context.Context, repairID uint) error {
	return nil
}
