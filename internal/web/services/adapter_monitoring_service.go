package services

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/southbound"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// AdapterMonitoringService 适配器监控服务
type AdapterMonitoringService struct {
	pluginManager plugin.PluginManager
	natsConn      *nats.Conn

	// 监控数据存储
	adapterMetrics  map[string]*AdapterRuntimeMetrics
	sinkMetrics     map[string]*SinkRuntimeMetrics
	dataFlowMetrics map[string]*DataFlowRuntimeMetrics
	metricsLock     sync.RWMutex

	// 监控配置
	metricsInterval   time.Duration
	retentionDuration time.Duration

	// 启动时间跟踪
	adapterStartTimes map[string]time.Time
	sinkStartTimes    map[string]time.Time
	startTimesLock    sync.RWMutex

	// 停止信号
	ctx    context.Context
	cancel context.CancelFunc
}

// AdapterRuntimeMetrics 适配器运行时指标
type AdapterRuntimeMetrics struct {
	Name            string
	Type            string
	Status          string
	StartTime       time.Time
	LastDataTime    time.Time
	DataPointsCount int64
	ErrorsCount     int64
	LastError       string
	ResponseTimes   []float64 // 响应时间历史(毫秒)
	Health          string
	HealthMessage   string
	Config          map[string]interface{}
	Tags            map[string]string
	LastUpdateTime  time.Time
}

// SinkRuntimeMetrics 连接器运行时指标
type SinkRuntimeMetrics struct {
	Name              string
	Type              string
	Status            string
	StartTime         time.Time
	LastPublishTime   time.Time
	MessagesPublished int64
	ErrorsCount       int64
	LastError         string
	ResponseTimes     []float64 // 响应时间历史(毫秒)
	Health            string
	HealthMessage     string
	Config            map[string]interface{}
	Tags              map[string]string
	LastUpdateTime    time.Time
}

// DataFlowRuntimeMetrics 数据流运行时指标
type DataFlowRuntimeMetrics struct {
	AdapterName      string
	DeviceID         string
	Key              string
	DataPoints       []DataPointRecord
	BytesTransferred int64
	ErrorsCount      int64
	LastValue        interface{}
	LastTimestamp    time.Time
	LastUpdateTime   time.Time
}

// DataPointRecord 数据点记录
type DataPointRecord struct {
	Timestamp time.Time
	Value     interface{}
	Size      int
	Latency   time.Duration
}

// NewAdapterMonitoringService 创建适配器监控服务
func NewAdapterMonitoringService(pluginManager plugin.PluginManager, natsConn *nats.Conn) *AdapterMonitoringService {
	ctx, cancel := context.WithCancel(context.Background())

	service := &AdapterMonitoringService{
		pluginManager:     pluginManager,
		natsConn:          natsConn,
		adapterMetrics:    make(map[string]*AdapterRuntimeMetrics),
		sinkMetrics:       make(map[string]*SinkRuntimeMetrics),
		dataFlowMetrics:   make(map[string]*DataFlowRuntimeMetrics),
		adapterStartTimes: make(map[string]time.Time),
		sinkStartTimes:    make(map[string]time.Time),
		metricsInterval:   10 * time.Second,
		retentionDuration: 24 * time.Hour,
		ctx:               ctx,
		cancel:            cancel,
	}

	// 启动监控协程
	go service.startMonitoring()

	// 订阅数据流事件
	if natsConn != nil {
		go service.subscribeToDataFlow()
	}

	return service
}

// startMonitoring 开始监控
func (s *AdapterMonitoringService) startMonitoring() {
	ticker := time.NewTicker(s.metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.collectMetrics()
			s.cleanupOldMetrics()
		}
	}
}

// collectMetrics 收集指标
func (s *AdapterMonitoringService) collectMetrics() {
	plugins := s.pluginManager.GetPlugins()

	s.metricsLock.Lock()
	defer s.metricsLock.Unlock()

	now := time.Now()

	for _, pluginMeta := range plugins {
		if pluginMeta.Type == "adapter" {
			s.updateAdapterMetrics(pluginMeta, now)
		} else if pluginMeta.Type == "sink" {
			s.updateSinkMetrics(pluginMeta, now)
		}
	}
}

// updateAdapterMetrics 更新适配器指标
func (s *AdapterMonitoringService) updateAdapterMetrics(pluginMeta *plugin.Meta, now time.Time) {
	name := pluginMeta.Name

	// 获取或创建指标记录
	metrics, exists := s.adapterMetrics[name]
	if !exists {
		s.startTimesLock.RLock()
		startTime, hasStartTime := s.adapterStartTimes[name]
		s.startTimesLock.RUnlock()

		if !hasStartTime {
			startTime = now
		}

		metrics = &AdapterRuntimeMetrics{
			Name:           name,
			Type:           pluginMeta.Type,
			Status:         pluginMeta.Status,
			StartTime:      startTime,
			ResponseTimes:  make([]float64, 0),
			Health:         "unknown",
			HealthMessage:  "初始化中",
			Config:         make(map[string]interface{}),
			Tags:           make(map[string]string),
			LastUpdateTime: now,
		}
		s.adapterMetrics[name] = metrics
	}

	// 更新基本状态
	metrics.Status = pluginMeta.Status
	metrics.LastUpdateTime = now

	// 如果适配器正在运行，尝试获取详细指标
	if pluginMeta.Status == "running" {
		if adapter, ok := s.pluginManager.GetAdapter(name); ok {
			s.updateAdapterDetailedMetrics(adapter, metrics)
		}
	} else {
		metrics.Health = "stopped"
		metrics.HealthMessage = "适配器已停止"
	}
}

// updateAdapterDetailedMetrics 更新适配器详细指标
func (s *AdapterMonitoringService) updateAdapterDetailedMetrics(adapterInterface interface{}, metrics *AdapterRuntimeMetrics) {
	// 尝试获取扩展适配器接口
	if extAdapter, ok := adapterInterface.(southbound.ExtendedAdapter); ok {
		// 有扩展接口，可以获取详细指标
		// 获取健康状态
		if health, err := extAdapter.Health(); err == nil {
			metrics.Health = health.Status
			metrics.HealthMessage = health.Message
		} else {
			metrics.Health = "unhealthy"
			metrics.HealthMessage = fmt.Sprintf("健康检查失败: %v", err)
		}

		// 获取运行指标
		if adapterMetrics, err := extAdapter.GetMetrics(); err == nil {
			metrics.DataPointsCount = adapterMetrics.DataPointsCollected
			metrics.ErrorsCount = adapterMetrics.ErrorsCount
			metrics.LastError = adapterMetrics.LastError

			if !adapterMetrics.LastDataPointTime.IsZero() {
				metrics.LastDataTime = adapterMetrics.LastDataPointTime
			}
		}

		// 获取最后错误
		if lastErr := extAdapter.GetLastError(); lastErr != nil {
			metrics.LastError = lastErr.Error()
		}
	} else {
		// 基础适配器，只能获取基本信息
		if _, ok := adapterInterface.(southbound.Adapter); ok {
			metrics.Health = "healthy"
			metrics.HealthMessage = "适配器运行正常"

			// 基础适配器没有详细指标，设置默认值
			if metrics.LastDataTime.IsZero() {
				metrics.LastDataTime = time.Now()
			}
		}
	}
}

// updateSinkMetrics 更新连接器指标
func (s *AdapterMonitoringService) updateSinkMetrics(pluginMeta *plugin.Meta, now time.Time) {
	name := pluginMeta.Name

	// 获取或创建指标记录
	metrics, exists := s.sinkMetrics[name]
	if !exists {
		s.startTimesLock.RLock()
		startTime, hasStartTime := s.sinkStartTimes[name]
		s.startTimesLock.RUnlock()

		if !hasStartTime {
			startTime = now
		}

		metrics = &SinkRuntimeMetrics{
			Name:           name,
			Type:           pluginMeta.Type,
			Status:         pluginMeta.Status,
			StartTime:      startTime,
			ResponseTimes:  make([]float64, 0),
			Health:         "unknown",
			HealthMessage:  "初始化中",
			Config:         make(map[string]interface{}),
			Tags:           make(map[string]string),
			LastUpdateTime: now,
		}
		s.sinkMetrics[name] = metrics
	}

	// 更新基本状态
	metrics.Status = pluginMeta.Status
	metrics.LastUpdateTime = now

	// 如果连接器正在运行，尝试获取详细指标
	if pluginMeta.Status == "running" {
		if sinkInterface, ok := s.pluginManager.GetSink(name); ok {
			s.updateSinkDetailedMetrics(sinkInterface, metrics)
		}
	} else {
		metrics.Health = "stopped"
		metrics.HealthMessage = "连接器已停止"
	}
}

// updateSinkDetailedMetrics 更新连接器详细指标
func (s *AdapterMonitoringService) updateSinkDetailedMetrics(sinkInterface interface{}, metrics *SinkRuntimeMetrics) {
	// 连接器通常没有扩展接口，设置基本健康状态
	if _, ok := sinkInterface.(northbound.Sink); ok {
		metrics.Health = "healthy"
		metrics.HealthMessage = "连接器运行正常"

		// 基础连接器没有详细指标，保持现有数据
		if metrics.LastPublishTime.IsZero() {
			metrics.LastPublishTime = time.Now()
		}
	}
}

// subscribeToDataFlow 订阅数据流事件
func (s *AdapterMonitoringService) subscribeToDataFlow() {
	// 订阅所有数据流主题
	sub, err := s.natsConn.Subscribe("iot.data.>", func(msg *nats.Msg) {
		s.handleDataFlowMessage(msg)
	})
	if err != nil {
		log.Error().Err(err).Msg("订阅数据流事件失败")
		return
	}

	// 等待上下文结束
	<-s.ctx.Done()

	// 取消订阅
	if err := sub.Unsubscribe(); err != nil {
		log.Error().Err(err).Msg("取消数据流订阅失败")
	}
}

// handleDataFlowMessage 处理数据流消息
func (s *AdapterMonitoringService) handleDataFlowMessage(msg *nats.Msg) {
	var point model.Point
	if err := point.UnmarshalJSON(msg.Data); err != nil {
		log.Error().Err(err).Msg("解析数据流消息失败")
		return
	}

	s.metricsLock.Lock()
	defer s.metricsLock.Unlock()

	// 更新数据流指标
	key := fmt.Sprintf("%s:%s:%s", point.DeviceID, point.DeviceID, point.Key)

	flowMetrics, exists := s.dataFlowMetrics[key]
	if !exists {
		flowMetrics = &DataFlowRuntimeMetrics{
			AdapterName:    point.DeviceID, // 假设DeviceID包含适配器信息
			DeviceID:       point.DeviceID,
			Key:            point.Key,
			DataPoints:     make([]DataPointRecord, 0),
			LastUpdateTime: time.Now(),
		}
		s.dataFlowMetrics[key] = flowMetrics
	}

	// 添加数据点记录
	record := DataPointRecord{
		Timestamp: point.Timestamp,
		Value:     point.Value,
		Size:      len(msg.Data),
		Latency:   time.Since(point.Timestamp),
	}

	flowMetrics.DataPoints = append(flowMetrics.DataPoints, record)
	flowMetrics.BytesTransferred += int64(record.Size)
	flowMetrics.LastValue = point.Value
	flowMetrics.LastTimestamp = point.Timestamp
	flowMetrics.LastUpdateTime = time.Now()

	// 限制数据点记录数量，保留最近的1000个
	if len(flowMetrics.DataPoints) > 1000 {
		flowMetrics.DataPoints = flowMetrics.DataPoints[len(flowMetrics.DataPoints)-1000:]
	}
}

// cleanupOldMetrics 清理旧指标
func (s *AdapterMonitoringService) cleanupOldMetrics() {
	s.metricsLock.Lock()
	defer s.metricsLock.Unlock()

	now := time.Now()
	cutoff := now.Add(-s.retentionDuration)

	// 清理数据流指标中的旧数据点
	for key, metrics := range s.dataFlowMetrics {
		// 过滤掉旧的数据点
		filteredPoints := make([]DataPointRecord, 0)
		for _, point := range metrics.DataPoints {
			if point.Timestamp.After(cutoff) {
				filteredPoints = append(filteredPoints, point)
			}
		}
		metrics.DataPoints = filteredPoints

		// 如果没有数据点了，删除整个指标
		if len(metrics.DataPoints) == 0 && metrics.LastUpdateTime.Before(cutoff) {
			delete(s.dataFlowMetrics, key)
		}
	}

	// 清理适配器指标中的旧响应时间数据
	for _, metrics := range s.adapterMetrics {
		if len(metrics.ResponseTimes) > 100 {
			metrics.ResponseTimes = metrics.ResponseTimes[len(metrics.ResponseTimes)-100:]
		}
	}

	// 清理连接器指标中的旧响应时间数据
	for _, metrics := range s.sinkMetrics {
		if len(metrics.ResponseTimes) > 100 {
			metrics.ResponseTimes = metrics.ResponseTimes[len(metrics.ResponseTimes)-100:]
		}
	}
}

// GetAdapterStatus 获取适配器状态列表
func (s *AdapterMonitoringService) GetAdapterStatus() ([]models.AdapterStatus, []models.SinkStatus, models.ConnectionOverview, error) {
	s.metricsLock.RLock()
	defer s.metricsLock.RUnlock()

	var adapters []models.AdapterStatus
	var sinks []models.SinkStatus

	// 构建适配器状态列表
	for _, metrics := range s.adapterMetrics {
		status := models.AdapterStatus{
			Name:             metrics.Name,
			Type:             metrics.Type,
			Status:           metrics.Status,
			Health:           metrics.Health,
			HealthMessage:    metrics.HealthMessage,
			StartTime:        &metrics.StartTime,
			LastDataTime:     &metrics.LastDataTime,
			ConnectionUptime: int64(time.Since(metrics.StartTime).Seconds()),
			DataPointsCount:  metrics.DataPointsCount,
			ErrorsCount:      metrics.ErrorsCount,
			LastError:        metrics.LastError,
			ResponseTimeMS:   s.calculateAverageResponseTime(metrics.ResponseTimes),
			Config:           s.sanitizeConfig(metrics.Config),
			Tags:             metrics.Tags,
		}
		adapters = append(adapters, status)
	}

	// 构建连接器状态列表
	for _, metrics := range s.sinkMetrics {
		status := models.SinkStatus{
			Name:              metrics.Name,
			Type:              metrics.Type,
			Status:            metrics.Status,
			Health:            metrics.Health,
			HealthMessage:     metrics.HealthMessage,
			StartTime:         &metrics.StartTime,
			LastPublishTime:   &metrics.LastPublishTime,
			ConnectionUptime:  int64(time.Since(metrics.StartTime).Seconds()),
			MessagesPublished: metrics.MessagesPublished,
			ErrorsCount:       metrics.ErrorsCount,
			LastError:         metrics.LastError,
			ResponseTimeMS:    s.calculateAverageResponseTime(metrics.ResponseTimes),
			Config:            s.sanitizeConfig(metrics.Config),
			Tags:              metrics.Tags,
		}
		sinks = append(sinks, status)
	}

	// 构建概览信息
	overview := s.buildConnectionOverview(adapters, sinks)

	return adapters, sinks, overview, nil
}

// GetDataFlowMetrics 获取数据流指标
func (s *AdapterMonitoringService) GetDataFlowMetrics(timeRange string) ([]models.DataFlowMetrics, error) {
	s.metricsLock.RLock()
	defer s.metricsLock.RUnlock()

	var metrics []models.DataFlowMetrics

	// 解析时间范围
	duration, err := time.ParseDuration(timeRange)
	if err != nil {
		duration = time.Hour // 默认1小时
	}

	cutoff := time.Now().Add(-duration)

	for _, flowMetrics := range s.dataFlowMetrics {
		// 过滤时间范围内的数据点
		var recentPoints []DataPointRecord
		for _, point := range flowMetrics.DataPoints {
			if point.Timestamp.After(cutoff) {
				recentPoints = append(recentPoints, point)
			}
		}

		if len(recentPoints) == 0 {
			continue
		}

		// 计算指标
		dataPointsPerSec := float64(len(recentPoints)) / duration.Seconds()
		bytesPerSec := float64(flowMetrics.BytesTransferred) / duration.Seconds()

		var totalLatency time.Duration
		for _, point := range recentPoints {
			totalLatency += point.Latency
		}
		avgLatency := totalLatency / time.Duration(len(recentPoints))

		errorRate := float64(flowMetrics.ErrorsCount) / float64(len(recentPoints))

		metric := models.DataFlowMetrics{
			AdapterName:      flowMetrics.AdapterName,
			DeviceID:         flowMetrics.DeviceID,
			Key:              flowMetrics.Key,
			DataPointsPerSec: dataPointsPerSec,
			BytesPerSec:      bytesPerSec,
			LatencyMS:        float64(avgLatency.Nanoseconds()) / 1e6,
			ErrorRate:        errorRate,
			LastValue:        flowMetrics.LastValue,
			LastTimestamp:    flowMetrics.LastTimestamp,
		}

		metrics = append(metrics, metric)
	}

	// 按数据点/秒排序
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].DataPointsPerSec > metrics[j].DataPointsPerSec
	})

	return metrics, nil
}

// calculateAverageResponseTime 计算平均响应时间
func (s *AdapterMonitoringService) calculateAverageResponseTime(responseTimes []float64) float64 {
	if len(responseTimes) == 0 {
		return 0
	}

	var sum float64
	for _, time := range responseTimes {
		sum += time
	}

	return sum / float64(len(responseTimes))
}

// sanitizeConfig 脱敏配置信息
func (s *AdapterMonitoringService) sanitizeConfig(config map[string]interface{}) map[string]interface{} {
	sanitized := make(map[string]interface{})

	for key, value := range config {
		// 隐藏敏感信息
		lowerKey := fmt.Sprintf("%s", key)
		if containsSensitiveKey(lowerKey) {
			sanitized[key] = "***HIDDEN***"
		} else {
			sanitized[key] = value
		}
	}

	return sanitized
}

// containsSensitiveKey 检查是否是敏感配置键
func containsSensitiveKey(key string) bool {
	sensitiveKeys := []string{
		"password", "passwd", "secret", "token", "key", "auth",
		"credential", "private", "cert", "certificate",
	}

	for _, sensitive := range sensitiveKeys {
		if len(key) >= len(sensitive) {
			for i := 0; i <= len(key)-len(sensitive); i++ {
				if key[i:i+len(sensitive)] == sensitive {
					return true
				}
			}
		}
	}

	return false
}

// buildConnectionOverview 构建连接概览
func (s *AdapterMonitoringService) buildConnectionOverview(adapters []models.AdapterStatus, sinks []models.SinkStatus) models.ConnectionOverview {
	overview := models.ConnectionOverview{
		TotalAdapters: len(adapters),
		TotalSinks:    len(sinks),
	}

	// 统计适配器状态
	for _, adapter := range adapters {
		if adapter.Status == "running" {
			overview.RunningAdapters++
		}
		if adapter.Health == "healthy" {
			overview.HealthyAdapters++
		}
	}

	// 统计连接器状态
	for _, sink := range sinks {
		if sink.Status == "running" {
			overview.RunningSinks++
		}
		if sink.Health == "healthy" {
			overview.HealthySinks++
		}
	}

	// 计算系统健康状态
	if overview.RunningAdapters == 0 && overview.RunningSinks == 0 {
		overview.SystemHealth = "stopped"
	} else if overview.HealthyAdapters == overview.RunningAdapters && overview.HealthySinks == overview.RunningSinks {
		overview.SystemHealth = "healthy"
	} else if overview.HealthyAdapters > 0 || overview.HealthySinks > 0 {
		overview.SystemHealth = "degraded"
	} else {
		overview.SystemHealth = "unhealthy"
	}

	overview.ActiveConnections = overview.RunningAdapters + overview.RunningSinks

	return overview
}

// TrackAdapterStart 记录适配器启动时间
func (s *AdapterMonitoringService) TrackAdapterStart(name string) {
	s.startTimesLock.Lock()
	defer s.startTimesLock.Unlock()

	s.adapterStartTimes[name] = time.Now()
}

// TrackSinkStart 记录连接器启动时间
func (s *AdapterMonitoringService) TrackSinkStart(name string) {
	s.startTimesLock.Lock()
	defer s.startTimesLock.Unlock()

	s.sinkStartTimes[name] = time.Now()
}

// Stop 停止监控服务
func (s *AdapterMonitoringService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}
