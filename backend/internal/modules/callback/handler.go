package callback

import (
	"github.com/gin-gonic/gin"

	"streetlight/internal/httpx"
	"streetlight/internal/response"
)

// Handler 处理维修质量回访相关的 HTTP 请求。
type Handler struct {
	service *Service
}

// NewHandler 构造回访处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// List 查询回访任务列表。
func (h *Handler) List(c *gin.Context) {
	var query ListQuery
	if err := httpx.BindQuery(c, &query); err != nil {
		response.Fail(c, err)
		return
	}
	items, total, page, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, response.NewPageData(items, total, page.Page, page.PageSize))
}

// Get 查询回访任务详情(含联系记录)。
func (h *Handler) Get(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// AddContact 登记一次回访联系情况。
func (h *Handler) AddContact(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req ContactRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.service.AddContact(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, detail)
}

// Judge 回访合格/不合格判定, 不合格时触发返修。
func (h *Handler) Judge(c *gin.Context) {
	id, err := httpx.ParseID(c, "id")
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req JudgeRequest
	if err := httpx.BindJSON(c, &req); err != nil {
		response.Fail(c, err)
		return
	}
	detail, err := h.service.Judge(c.Request.Context(), id, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

// Metadata 返回回访字典。
func (h *Handler) Metadata(c *gin.Context) {
	meta, err := h.service.Metadata(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, meta)
}

// Statistics 回访与返修统计。
func (h *Handler) Statistics(c *gin.Context) {
	statistics, err := h.service.Statistics(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, statistics)
}
