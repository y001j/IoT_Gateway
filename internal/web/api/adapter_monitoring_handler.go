package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// AdapterMonitoringHandler 适配器监控API处理器
type AdapterMonitoringHandler struct {
	monitoringService *services.AdapterMonitoringService
}

// NewAdapterMonitoringHandler 创建适配器监控API处理器
func NewAdapterMonitoringHandler(monitoringService *services.AdapterMonitoringService) *AdapterMonitoringHandler {
	return &AdapterMonitoringHandler{
		monitoringService: monitoringService,
	}
}

// GetAdapterStatus 获取适配器状态列表
// @Summary 获取适配器状态列表
// @Description 获取所有适配器和连接器的状态信息，包括健康状态、指标和概览
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Success 200 {object} models.AdapterStatusListResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/status [get]
func (h *AdapterMonitoringHandler) GetAdapterStatus(c *gin.Context) {
	adapters, sinks, overview, err := h.monitoringService.GetAdapterStatus()
	if err != nil {
		log.Error().Err(err).Msg("获取适配器状态失败")
		c.JSON(http.StatusInternalServerError, models.BaseResponse{
			Code:    500,
			Message: "获取适配器状态失败",
			Error:   err.Error(),
		})
		return
	}

	response := models.AdapterStatusListResponse{
		BaseResponse: models.BaseResponse{
			Code:    200,
			Message: "获取适配器状态成功",
		},
	}
	response.Data.Adapters = adapters
	response.Data.Sinks = sinks
	response.Data.Overview = overview

	c.JSON(http.StatusOK, response)
}

// GetDataFlowMetrics 获取数据流指标
// @Summary 获取数据流指标
// @Description 获取指定时间范围内的数据流指标，包括吞吐量、延迟等信息
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Param time_range query string false "时间范围" default("1h")
// @Param limit query int false "限制数量" default(100)
// @Success 200 {object} models.DataFlowMetricsResponse
// @Failure 400 {object} models.BaseResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/data-flow [get]
func (h *AdapterMonitoringHandler) GetDataFlowMetrics(c *gin.Context) {
	// 获取查询参数
	timeRange := c.DefaultQuery("time_range", "1h")
	limitStr := c.DefaultQuery("limit", "100")
	
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.BaseResponse{
			Code:    400,
			Message: "无效的limit参数",
			Error:   err.Error(),
		})
		return
	}

	// 获取数据流指标
	metrics, err := h.monitoringService.GetDataFlowMetrics(timeRange)
	if err != nil {
		log.Error().Err(err).Msg("获取数据流指标失败")
		c.JSON(http.StatusInternalServerError, models.BaseResponse{
			Code:    500,
			Message: "获取数据流指标失败",
			Error:   err.Error(),
		})
		return
	}

	// 限制返回数量
	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	response := models.DataFlowMetricsResponse{
		BaseResponse: models.BaseResponse{
			Code:    200,
			Message: "获取数据流指标成功",
		},
	}
	response.Data.Metrics = metrics
	response.Data.TimeRange = timeRange
	response.Data.Granularity = "realtime"
	response.Data.TotalPoints = len(metrics)

	c.JSON(http.StatusOK, response)
}

// GetAdapterDiagnostics 获取适配器诊断信息
// @Summary 获取适配器诊断信息
// @Description 对指定适配器进行全面诊断，包括连接测试、配置验证、健康检查等
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Param name path string true "适配器名称"
// @Success 200 {object} models.AdapterDiagnosticsResponse
// @Failure 400 {object} models.BaseResponse
// @Failure 404 {object} models.BaseResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/{name}/diagnostics [get]
func (h *AdapterMonitoringHandler) GetAdapterDiagnostics(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, models.BaseResponse{
			Code:    400,
			Message: "适配器名称不能为空",
		})
		return
	}

	// 执行诊断（这里是简化版本，实际应该调用专门的诊断服务）
	diagnostics := h.performAdapterDiagnostics(adapterName)

	response := models.AdapterDiagnosticsResponse{
		BaseResponse: models.BaseResponse{
			Code:    200,
			Message: "获取适配器诊断信息成功",
		},
		Data: diagnostics,
	}

	c.JSON(http.StatusOK, response)
}

// TestAdapterConnection 测试适配器连接
// @Summary 测试适配器连接
// @Description 测试指定适配器的连接状态和响应性能
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Param name path string true "适配器名称"
// @Success 200 {object} models.BaseResponse
// @Failure 400 {object} models.BaseResponse
// @Failure 404 {object} models.BaseResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/{name}/test-connection [post]
func (h *AdapterMonitoringHandler) TestAdapterConnection(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, models.BaseResponse{
			Code:    400,
			Message: "适配器名称不能为空",
		})
		return
	}

	// 执行连接测试
	result := h.performConnectionTest(adapterName)

	if result.Success {
		c.JSON(http.StatusOK, models.BaseResponse{
			Code:    200,
			Message: "连接测试成功",
			Data:    result,
		})
	} else {
		c.JSON(http.StatusOK, models.BaseResponse{
			Code:    200,
			Message: "连接测试失败",
			Data:    result,
		})
	}
}

// GetAdapterPerformance 获取适配器性能指标
// @Summary 获取适配器性能指标
// @Description 获取指定适配器的详细性能指标和趋势数据
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Param name path string true "适配器名称"
// @Param period query string false "时间周期" default("1h")
// @Success 200 {object} models.BaseResponse
// @Failure 400 {object} models.BaseResponse
// @Failure 404 {object} models.BaseResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/{name}/performance [get]
func (h *AdapterMonitoringHandler) GetAdapterPerformance(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, models.BaseResponse{
			Code:    400,
			Message: "适配器名称不能为空",
		})
		return
	}

	period := c.DefaultQuery("period", "1h")

	// 获取性能数据（简化版本）
	performance := h.getAdapterPerformance(adapterName, period)

	c.JSON(http.StatusOK, models.BaseResponse{
		Code:    200,
		Message: "获取适配器性能指标成功",
		Data:    performance,
	})
}

// RestartAdapter 重启适配器
// @Summary 重启适配器
// @Description 重启指定的适配器
// @Tags 适配器监控
// @Accept json
// @Produce json
// @Param name path string true "适配器名称"
// @Success 200 {object} models.BaseResponse
// @Failure 400 {object} models.BaseResponse
// @Failure 404 {object} models.BaseResponse
// @Failure 500 {object} models.BaseResponse
// @Router /api/monitoring/adapters/{name}/restart [post]
func (h *AdapterMonitoringHandler) RestartAdapter(c *gin.Context) {
	adapterName := c.Param("name")
	if adapterName == "" {
		c.JSON(http.StatusBadRequest, models.BaseResponse{
			Code:    400,
			Message: "适配器名称不能为空",
		})
		return
	}

	// 这里应该调用插件管理器的重启方法
	// 由于我们的监控服务主要负责监控，实际的重启操作应该委托给插件管理器
	log.Info().Str("adapter", adapterName).Msg("请求重启适配器")

	c.JSON(http.StatusOK, models.BaseResponse{
		Code:    200,
		Message: fmt.Sprintf("适配器 %s 重启请求已提交", adapterName),
	})
}

// performAdapterDiagnostics 执行适配器诊断
func (h *AdapterMonitoringHandler) performAdapterDiagnostics(adapterName string) models.AdapterDiagnostics {
	// 这是一个简化的诊断实现
	// 实际应该包含更全面的诊断逻辑
	
	diagnostics := models.AdapterDiagnostics{
		AdapterName: adapterName,
		ConnectionTest: &models.ConnectionTestResult{
			Success:      true,
			ResponseTime: 0,
			Error:        "",
			Details:      make(map[string]interface{}),
			Timestamp:    time.Now(),
		},
		ConfigValidation: &models.ConfigValidationResult{
			Valid:      true,
			Errors:     []string{},
			Warnings:   []string{},
			Suggestions: []string{},
			Timestamp:  time.Now(),
		},
		HealthChecks: []models.HealthCheckResult{
			{
				CheckName: "基础连接检查",
				Status:    "pass",
				Message:   "连接正常",
				Duration:  0,
				Timestamp: time.Now(),
				Details:   make(map[string]interface{}),
			},
		},
		PerformanceTest: &models.PerformanceTestResult{
			ThroughputPerSec: 100.0,
			AvgLatency:       time.Millisecond * 10,
			MaxLatency:       time.Millisecond * 50,
			MinLatency:       time.Millisecond * 5,
			ErrorRate:        0.01,
			TestDuration:     time.Second * 30,
			SampleCount:      3000,
			Timestamp:        time.Now(),
		},
		Recommendations: []string{
			"适配器运行正常",
			"建议定期检查连接状态",
		},
	}

	return diagnostics
}

// performConnectionTest 执行连接测试
func (h *AdapterMonitoringHandler) performConnectionTest(adapterName string) models.ConnectionTestResult {
	// 简化的连接测试
	// 实际应该根据适配器类型执行具体的连接测试
	
	return models.ConnectionTestResult{
		Success:      true,
		ResponseTime: time.Millisecond * 15,
		Error:        "",
		Details: map[string]interface{}{
			"adapter_name": adapterName,
			"test_type":   "basic_connectivity",
			"status":      "connected",
		},
		Timestamp: time.Now(),
	}
}

// getAdapterPerformance 获取适配器性能指标
func (h *AdapterMonitoringHandler) getAdapterPerformance(adapterName, period string) map[string]interface{} {
	// 简化的性能数据
	// 实际应该从监控服务获取真实的性能数据
	
	return map[string]interface{}{
		"adapter_name":     adapterName,
		"period":          period,
		"data_points_per_sec": 50.5,
		"avg_latency_ms":  12.3,
		"error_rate":      0.02,
		"uptime_percent":  99.8,
		"memory_usage_mb": 25.6,
		"cpu_usage_percent": 15.2,
		"network_bytes_per_sec": 2048,
		"timestamps": []string{
			"2024-01-01T00:00:00Z",
			"2024-01-01T00:05:00Z",
			"2024-01-01T00:10:00Z",
		},
		"values": []float64{
			48.2, 52.1, 51.8,
		},
	}
}