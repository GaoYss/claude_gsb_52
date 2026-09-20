package callback

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module 维修质量回访模块, 负责回访任务生成、回访登记与不合格返修联动。
type Module struct {
	repository *Repository
	service    *Service
	handler    *Handler
}

// New 构造质量回访模块, repairs 为维修模块提供的端口实现。
func New(db *gorm.DB, repairs RepairPort) *Module {
	repository := NewRepository(db)
	service := NewService(repository, repairs)
	return &Module{
		repository: repository,
		service:    service,
		handler:    NewHandler(service),
	}
}

// Repository 暴露仓储, 供状态查询模块装配。
func (m *Module) Repository() *Repository { return m.repository }

// Service 暴露业务服务, 供维修与故障模块装配回访端口。
func (m *Module) Service() *Service { return m.service }

// Name 实现 module.Module 接口。
func (m *Module) Name() string { return "质量回访" }

// Models 实现 module.Module 接口。
func (m *Module) Models() []any { return []any{&Callback{}} }

// RegisterRoutes 实现 module.Module 接口。
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/callbacks")
	{
		group.GET("", m.handler.List)
		group.GET("/meta", m.handler.Metadata)
		group.GET("/fault/:faultId", m.handler.ListByFault)
		group.GET("/:id", m.handler.Get)
		group.POST("/:id/record", m.handler.Record)
	}
}
