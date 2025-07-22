package api

import (
	"net/http"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"

	"github.com/gin-gonic/gin"
)

// PluginHandler 插件处理器
type PluginHandler struct {
	*BaseHandler
	pluginService services.PluginService
}

// NewPluginHandler 创建插件处理器
func NewPluginHandler(pluginService services.PluginService) *PluginHandler {
	return &PluginHandler{
		BaseHandler:   &BaseHandler{},
		pluginService: pluginService,
	}
}

// GetPlugins 获取插件列表
// @Summary 获取插件列表
// @Description 获取系统中所有插件的列表
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页大小" default(20)
// @Param type query string false "插件类型"
// @Param status query string false "插件状态"
// @Param search query string false "搜索关键词"
// @Success 200 {object} APIResponse{data=models.PagedResponse{data=[]models.Plugin}}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /plugins [get]
func (h *PluginHandler) GetPlugins(c *gin.Context) {
	page, pageSize := h.GetPaginationParams(c)

	req := &models.PluginListRequest{
		Page:     page,
		PageSize: pageSize,
		Type:     h.GetQueryParam(c, "type"),
		Status:   h.GetQueryParam(c, "status"),
		Search:   h.GetQueryParam(c, "search"),
	}

	plugins, total, err := h.pluginService.GetPlugins(req)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, &models.PagedResponse{
		Data: plugins,
		Pagination: models.Pagination{
			Total:  total,
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
}

// GetPlugin 获取单个插件
// @Summary 获取单个插件
// @Description 获取指定名称的插件详情
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse{data=models.Plugin}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name} [get]
func (h *PluginHandler) GetPlugin(c *gin.Context) {
	name := c.Param("id")
	plugin, err := h.pluginService.GetPlugin(name)
	if err != nil {
		h.ErrorResponse(c, http.StatusNotFound, err.Error())
		return
	}

	h.SuccessResponse(c, plugin)
}

// StartPlugin 启动插件
// @Summary 启动插件
// @Description 启动指定名称的插件
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/start [post]
func (h *PluginHandler) StartPlugin(c *gin.Context) {
	name := c.Param("id")
	if err := h.pluginService.StartPlugin(name); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, nil)
}

// StopPlugin 停止插件
// @Summary 停止插件
// @Description 停止指定名称的插件
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/stop [post]
func (h *PluginHandler) StopPlugin(c *gin.Context) {
	name := c.Param("id")
	if err := h.pluginService.StopPlugin(name); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, nil)
}

// RestartPlugin 重启插件
// @Summary 重启插件
// @Description 重启指定名称的插件
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/restart [post]
func (h *PluginHandler) RestartPlugin(c *gin.Context) {
	name := c.Param("id")
	if err := h.pluginService.RestartPlugin(name); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, nil)
}

// DeletePlugin 删除插件
// @Summary 删除插件
// @Description 删除指定名称的插件
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name} [delete]
func (h *PluginHandler) DeletePlugin(c *gin.Context) {
	name := c.Param("id")
	if err := h.pluginService.DeletePlugin(name); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, nil)
}

// GetPluginConfig 获取插件配置
// @Summary 获取插件配置
// @Description 获取指定名称的插件配置
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse{data=map[string]interface{}}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/config [get]
func (h *PluginHandler) GetPluginConfig(c *gin.Context) {
	name := c.Param("id")
	config, err := h.pluginService.GetPluginConfig(name)
	if err != nil {
		h.ErrorResponse(c, http.StatusNotFound, err.Error())
		return
	}

	h.SuccessResponse(c, config)
}

// UpdatePluginConfig 更新插件配置
// @Summary 更新插件配置
// @Description 更新指定名称的插件配置
// @Tags 插件管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param name path string true "插件名称"
// @Param config body map[string]interface{} true "插件配置"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/config [put]
func (h *PluginHandler) UpdatePluginConfig(c *gin.Context) {
	name := c.Param("id")
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "无效的配置格式")
		return
	}

	if err := h.pluginService.UpdatePluginConfig(name, config); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, nil)
}

// ValidatePluginConfig 验证插件配置
// @Summary 验证插件配置
// @Description 验证指定名称的插件配置
// @Tags 插件管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param name path string true "插件名称"
// @Param config body map[string]interface{} true "插件配置"
// @Success 200 {object} APIResponse{data=models.PluginConfigValidationResponse}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/config/validate [post]
func (h *PluginHandler) ValidatePluginConfig(c *gin.Context) {
	name := c.Param("id")
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "无效的配置格式")
		return
	}

	response, err := h.pluginService.ValidatePluginConfig(name, config)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, response)
}

// GetPluginLogs 获取插件日志
// @Summary 获取插件日志
// @Description 获取指定名称的插件日志
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页大小" default(20)
// @Param level query string false "日志级别"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Success 200 {object} APIResponse{data=models.PagedResponse{data=[]models.PluginLog}}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/logs [get]
func (h *PluginHandler) GetPluginLogs(c *gin.Context) {
	name := c.Param("id")
	page, pageSize := h.GetPaginationParams(c)

	req := &models.PluginLogRequest{
		Page:     page,
		PageSize: pageSize,
		Level:    h.GetQueryParam(c, "level"),
		From:     h.GetQueryParam(c, "start_time"),
		To:       h.GetQueryParam(c, "end_time"),
	}

	logs, total, err := h.pluginService.GetPluginLogs(name, req)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, &models.PagedResponse{
		Data: logs,
		Pagination: models.Pagination{
			Total:  total,
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
}

// GetPluginStats 获取插件统计
// @Summary 获取插件统计
// @Description 获取指定名称的插件统计信息
// @Tags 插件管理
// @Security ApiKeyAuth
// @Produce json
// @Param name path string true "插件名称"
// @Success 200 {object} APIResponse{data=models.PluginStats}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /plugins/{name}/stats [get]
func (h *PluginHandler) GetPluginStats(c *gin.Context) {
	name := c.Param("id")
	stats, err := h.pluginService.GetPluginStats(name)
	if err != nil {
		h.ErrorResponse(c, http.StatusNotFound, err.Error())
		return
	}

	h.SuccessResponse(c, stats)
}
