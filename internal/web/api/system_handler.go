package api

import (
	"fmt"
	"net/http"

	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"

	"github.com/gin-gonic/gin"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	*BaseHandler
	systemService services.SystemService
	metrics       *metrics.LightweightMetrics
}

// NewSystemHandler 创建系统处理器
func NewSystemHandler(systemService services.SystemService) *SystemHandler {
	return &SystemHandler{
		BaseHandler:   &BaseHandler{},
		systemService: systemService,
		metrics:       nil, // 将在运行时通过其他方式获取
	}
}

// GetStatus 获取系统状态
// @Summary 获取系统状态
// @Description 获取系统运行状态和基本信息
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=services.SystemStatusResponse}
// @Failure 401 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/status [get]
func (h *SystemHandler) GetStatus(c *gin.Context) {
	status, err := h.systemService.GetStatus()
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, status)
}

// GetMetrics 获取系统指标
// @Summary 获取系统指标
// @Description 获取系统性能指标，包括CPU、内存、磁盘等
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=services.SystemMetricsResponse}
// @Failure 401 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/metrics [get]
func (h *SystemHandler) GetMetrics(c *gin.Context) {
	metrics, err := h.systemService.GetMetrics()
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, metrics)
}

// GetLightweightMetrics 获取轻量级指标
// @Summary 获取轻量级指标
// @Description 获取轻量级指标系统的完整指标数据
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Param format query string false "输出格式 (json, text, plain)" default(json)
// @Success 200 {object} APIResponse{data=metrics.LightweightMetrics}
// @Failure 401 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/metrics/lightweight [get]
func (h *SystemHandler) GetLightweightMetrics(c *gin.Context) {
	format := c.Query("format")
	if format == "" {
		format = "json"
	}
	
	// 由于import cycle问题，这里暂时返回一个基本的响应
	// 实际的轻量级指标可以通过Gateway主服务的/metrics端点获取
	h.ErrorResponse(c, http.StatusNotImplemented, "轻量级指标请通过Gateway主服务的/metrics端点获取 (端口8080)")
}

// GetHealth 获取健康状态
// @Summary 获取健康状态
// @Description 获取系统健康检查结果
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=services.HealthResponse}
// @Failure 401 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/health [get]
func (h *SystemHandler) GetHealth(c *gin.Context) {
	health, err := h.systemService.GetHealth()
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, health)
}

// GetConfig 获取系统配置
// @Summary 获取系统配置
// @Description 获取当前系统配置
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=services.ConfigResponse}
// @Failure 401 {object} APIResponse
// @Failure 403 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/config [get]
func (h *SystemHandler) GetConfig(c *gin.Context) {
	// 调试信息
	fmt.Printf("GetConfig处理器 - 开始处理请求\n")
	
	// 只有管理员可以查看配置
	if !h.HasRole(c, "admin") {
		fmt.Printf("GetConfig处理器 - 用户无admin权限\n")
		h.ErrorResponse(c, http.StatusForbidden, "Only administrators can view system configuration")
		return
	}
	
	fmt.Printf("GetConfig处理器 - 用户有admin权限，继续处理\n")

	config, err := h.systemService.GetConfig()
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, config)
}

// UpdateConfig 更新系统配置
// @Summary 更新系统配置
// @Description 更新系统配置
// @Tags 系统管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param config body models.SystemConfig true "配置数据"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 403 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/config [put]
func (h *SystemHandler) UpdateConfig(c *gin.Context) {
	// 只有管理员可以修改配置
	if !h.HasRole(c, "admin") {
		h.ErrorResponse(c, http.StatusForbidden, "Only administrators can modify system configuration")
		return
	}

	var config models.SystemConfig
	if !h.BindJSON(c, &config) {
		return
	}

	err := h.systemService.UpdateConfig(&config)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, gin.H{"message": "Configuration updated successfully"})
}

// ValidateConfig 验证系统配置
// @Summary 验证系统配置
// @Description 验证配置的有效性
// @Tags 系统管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param config body models.SystemConfig true "配置数据"
// @Success 200 {object} APIResponse{data=models.ConfigValidationResponse}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 403 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/config/validate [post]
func (h *SystemHandler) ValidateConfig(c *gin.Context) {
	// 只有管理员可以验证配置
	if !h.HasRole(c, "admin") {
		h.ErrorResponse(c, http.StatusForbidden, "Only administrators can validate system configuration")
		return
	}

	var config models.SystemConfig
	if !h.BindJSON(c, &config) {
		return
	}

	// 尝试更新配置来验证
	err := h.systemService.UpdateConfig(&config)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, &models.ConfigValidationResponse{
		Valid: true,
	})
}

// Restart 重启系统
// @Summary 重启系统
// @Description 重启IoT Gateway系统
// @Tags 系统管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body object{delay=int} false "重启参数"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Failure 403 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/restart [post]
func (h *SystemHandler) Restart(c *gin.Context) {
	// 只有管理员可以重启系统
	if !h.HasRole(c, "admin") {
		h.ErrorResponse(c, http.StatusForbidden, "Only administrators can restart the system")
		return
	}

	var req struct {
		Delay int `json:"delay"` // 延迟秒数
	}

	// 绑定请求参数（可选）
	c.ShouldBindJSON(&req)

	err := h.systemService.RestartService("system")
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, gin.H{
		"message": "System restart initiated",
		"delay":   req.Delay,
	})
}

// GetSystemAlertStatus 获取系统告警状态
// @Summary 获取系统告警状态
// @Description 获取系统告警服务的状态信息
// @Tags 系统管理
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=map[string]interface{}}
// @Failure 401 {object} APIResponse
// @Failure 500 {object} APIResponse
// @Router /system/alert-status [get]
func (h *SystemHandler) GetSystemAlertStatus(c *gin.Context) {
	if h.services.SystemAlert == nil {
		h.ErrorResponse(c, http.StatusNotFound, "系统告警服务未启用")
		return
	}
	
	status := h.services.SystemAlert.GetStatus()
	h.SuccessResponse(c, status)
}
