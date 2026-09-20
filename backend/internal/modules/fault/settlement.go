package fault

import "context"

// SettlementGuard 由维修质量回访模块实现: 回访未完成(未回访合格)的故障不允许结算关闭。
type SettlementGuard interface {
	AssertSettlementAllowed(ctx context.Context, faultID uint) error
}

// noopSettlementGuard 是未装配回访模块时的空实现。
type noopSettlementGuard struct{}

func (noopSettlementGuard) AssertSettlementAllowed(ctx context.Context, faultID uint) error {
	return nil
}
