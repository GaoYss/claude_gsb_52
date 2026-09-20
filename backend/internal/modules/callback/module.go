package callback

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module 维修质量回访模块: 完工生成回访任务、记录联系情况与满意度、不合格触发返修。
type Module struct {
	repository *Repository
	service    *Service
	handler    *Handler
}

// New 构造回访模块, repairs 为维修模块提供的端口实现。
func New(db *gorm.DB, repairs RepairPort) *Module {
	repository := NewRepository(db)
	service := NewService(repository, repairs)
	return &Module{
		repository: repository,
		service:    service,
		handler:    NewHandler(service),
	}
}

// Service 暴露业务服务, 供故障模块装配结算守卫。
func (m *Module) Service() *Service { return m.service }

// Repository 暴露仓储, 供状态查询模块装配。
func (m *Module) Repository() *Repository { return m.repository }

// Name 实现 module.Module 接口。
func (m *Module) Name() string { return "维修质量回访" }

// Models 实现 module.Module 接口。
func (m *Module) Models() []any { return []any{&Task{}, &Contact{}} }

// RegisterRoutes 实现 module.Module 接口。
func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	group := api.Group("/callbacks")
	{
		group.GET("", m.handler.List)
		group.GET("/meta", m.handler.Metadata)
		group.GET("/statistics", m.handler.Statistics)
		group.GET("/:id", m.handler.Get)
		group.POST("/:id/contacts", m.handler.AddContact)
		group.POST("/:id/judge", m.handler.Judge)
	}
}
