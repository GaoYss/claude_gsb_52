package bootstrap

import (
	"gorm.io/gorm"

	"streetlight/internal/module"
	"streetlight/internal/modules/callback"
	"streetlight/internal/modules/fault"
	"streetlight/internal/modules/lamp"
	"streetlight/internal/modules/repair"
	"streetlight/internal/modules/status"
)

// buildModules 按依赖方向装配业务模块。
//
// 依赖关系: 路灯台账 <- 故障登记 <- 维修记录 <- 质量回访, 维修状态查询依赖前四者的只读仓储。
// 回访模块需要维修模块创建返修单, 而维修模块完工后又要回调回访模块生成回访任务,
// 故障结算关闭也需要回访模块把关, 因此通过 SetCallbackHook / SetSettlementGuard
// 在构造完成后回填, 避免构造函数循环依赖。
func buildModules(db *gorm.DB) []module.Module {
	lampModule := lamp.New(db)

	faultModule := fault.New(db, lampModule.Service())
	lampModule.Service().SetOpenFaultCounter(faultModule.Repository())

	repairModule := repair.New(db, faultModule.Service())

	callbackModule := callback.New(db, repairModule.Service())
	repairModule.Service().SetCallbackHook(callbackModule.Service())
	faultModule.Service().SetSettlementGuard(callbackModule.Service())

	statusModule := status.New(
		db,
		lampModule.Repository(),
		faultModule.Repository(),
		repairModule.Repository(),
		callbackModule.Repository(),
	)

	return []module.Module{
		lampModule,
		faultModule,
		repairModule,
		callbackModule,
		statusModule,
	}
}
