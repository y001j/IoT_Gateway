package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/southbound"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// PluginService 插件服务接口
type PluginService interface {
	GetPlugins(req *models.PluginListRequest) ([]models.Plugin, int, error)
	GetPlugin(name string) (*models.Plugin, error)
	StartPlugin(name string) error
	StopPlugin(name string) error
	RestartPlugin(name string) error
	DeletePlugin(name string) error
	UpdatePluginConfig(name string, config map[string]interface{}) error
	GetPluginConfig(name string) (map[string]interface{}, error)
	ValidatePluginConfig(name string, config map[string]interface{}) (*models.PluginConfigValidationResponse, error)
	GetPluginLogs(name string, req *models.PluginLogRequest) ([]models.PluginLog, int, error)
	GetPluginStats(name string) (*models.PluginStats, error)
}

// pluginService 插件服务实现
type pluginService struct {
	manager plugin.PluginManager
}

// NewPluginService 创建插件服务
func NewPluginService(manager plugin.PluginManager) (PluginService, error) {
	if manager == nil {
		return nil, fmt.Errorf("plugin manager is required")
	}
	service := &pluginService{
		manager: manager,
	}
	return service, nil
}

// GetPlugins 获取插件列表
func (s *pluginService) GetPlugins(req *models.PluginListRequest) ([]models.Plugin, int, error) {
	// 按实例列出（名称与启停接口一致）。以前列出的是插件类型（mock、websocket、redis…），
	// 而启停接口按实例名工作，页面上的启停操作全部失败
	instances := s.manager.InstanceStatuses()
	allPlugins := make([]models.Plugin, 0, len(instances))
	for _, inst := range instances {
		allPlugins = append(allPlugins, instanceToModel(inst))
	}
	sort.SliceStable(allPlugins, func(i, j int) bool {
		if allPlugins[i].Type != allPlugins[j].Type {
			return allPlugins[i].Type == string(plugin.TypeAdapter)
		}
		return allPlugins[i].Name < allPlugins[j].Name
	})

	// 应用过滤器
	filteredPlugins := s.filterPlugins(allPlugins, req)

	// 计算分页
	total := len(filteredPlugins)
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize

	if start >= total {
		return []models.Plugin{}, total, nil
	}

	if end > total {
		end = total
	}

	result := filteredPlugins[start:end]
	return result, total, nil
}

// instanceToModel 把插件实例转换为 Web 模型
func instanceToModel(inst plugin.InstanceStatus) models.Plugin {
	status := "stopped"
	if inst.Running {
		status = "running"
	}
	kindText := "适配器"
	if inst.Kind == string(plugin.TypeSink) {
		kindText = "连接器"
	}
	desc := fmt.Sprintf("%s %s", inst.Type, kindText)
	switch {
	case inst.External:
		desc = fmt.Sprintf("外部插件（%s）：%s", inst.Mode, inst.Type)
	case !inst.Enabled && !inst.Running:
		desc += "（配置中已禁用）"
	}
	version := inst.Version
	if version == "" {
		version = "内置"
	}
	return models.Plugin{
		Name:        inst.Name,
		Type:        inst.Kind,
		PluginType:  inst.Type,
		Mode:        inst.Mode,
		Version:     version,
		Status:      status,
		Description: desc,
		Enabled:     inst.Enabled,
	}
}

func (s *pluginService) findInstance(name string) (plugin.InstanceStatus, bool) {
	for _, inst := range s.manager.InstanceStatuses() {
		if inst.Name == name {
			return inst, true
		}
	}
	return plugin.InstanceStatus{}, false
}

// filterPlugins 过滤插件列表
func (s *pluginService) filterPlugins(plugins []models.Plugin, req *models.PluginListRequest) []models.Plugin {
	var filtered []models.Plugin

	for _, plugin := range plugins {
		// 类型过滤
		if req.Type != "" && plugin.Type != req.Type {
			continue
		}

		// 状态过滤
		if req.Status != "" && plugin.Status != req.Status {
			continue
		}

		// 搜索过滤（名称、描述）
		if req.Search != "" {
			searchTerm := strings.ToLower(req.Search)
			pluginName := strings.ToLower(plugin.Name)
			pluginDesc := strings.ToLower(plugin.Description)

			if !strings.Contains(pluginName, searchTerm) && !strings.Contains(pluginDesc, searchTerm) {
				continue
			}
		}

		filtered = append(filtered, plugin)
	}

	return filtered
}

// GetPlugin 获取单个插件实例
func (s *pluginService) GetPlugin(name string) (*models.Plugin, error) {
	inst, ok := s.findInstance(name)
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", name)
	}
	p := instanceToModel(inst)
	return &p, nil
}

// StartPlugin 启动插件
func (s *pluginService) StartPlugin(name string) error {
	return s.manager.StartPlugin(name)
}

// StopPlugin 停止插件
func (s *pluginService) StopPlugin(name string) error {
	return s.manager.StopPlugin(name)
}

// RestartPlugin 重启插件
func (s *pluginService) RestartPlugin(name string) error {
	return s.manager.RestartPlugin(name)
}

// ... 其他方法的实现将同样委托给 s.manager ...
// 为了简洁，暂时省略

func (s *pluginService) DeletePlugin(name string) error {
	// TODO: 实际删除插件文件和配置
	// 目前只能停止插件，无法删除文件
	return fmt.Errorf("plugin deletion not supported yet")
}

func (s *pluginService) UpdatePluginConfig(name string, config map[string]interface{}) error {
	// TODO: 更新插件配置文件
	// 这需要插件管理器支持配置更新
	return fmt.Errorf("plugin config update not implemented yet")
}

// GetPluginConfig 返回实例的配置（密码、令牌等敏感字段会被隐藏）
func (s *pluginService) GetPluginConfig(name string) (map[string]interface{}, error) {
	if cfg, ok := s.manager.InstanceConfig(name); ok {
		maskSecrets(cfg)
		return cfg, nil
	}

	// 不在配置中的外部插件：返回 plugin.json 中的元数据
	meta, exists := s.manager.GetPlugin(name)
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}
	config := map[string]interface{}{
		"name":        meta.Name,
		"version":     meta.Version,
		"type":        meta.Type,
		"mode":        meta.Mode,
		"entry":       meta.Entry,
		"description": meta.Description,
	}
	if meta.ISPPort != 0 {
		config["isp_port"] = meta.ISPPort
	}
	return config, nil
}

// maskSecrets 隐藏配置中的敏感字段（配置接口对所有登录用户开放）
func maskSecrets(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if isSecretKey(k) {
				if str, ok := val.(string); !ok || str != "" {
					t[k] = "******"
				}
				continue
			}
			maskSecrets(val)
		}
	case []interface{}:
		for _, val := range t {
			maskSecrets(val)
		}
	}
}

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, word := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "credential", "private_key"} {
		if strings.Contains(k, word) {
			return true
		}
	}
	return false
}

func (s *pluginService) ValidatePluginConfig(name string, config map[string]interface{}) (*models.PluginConfigValidationResponse, error) {
	// 基本验证
	response := &models.PluginConfigValidationResponse{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// 检查必要字段
	if config["name"] == nil || config["name"] == "" {
		response.Valid = false
		response.Errors = append(response.Errors, "名称不能为空")
	}

	if config["type"] == nil || config["type"] == "" {
		response.Valid = false
		response.Errors = append(response.Errors, "类型不能为空")
	}

	// 检查类型是否有效
	if config["type"] != nil {
		validTypes := []string{"adapter", "sink"}
		typeStr := fmt.Sprintf("%v", config["type"])
		isValidType := false
		for _, validType := range validTypes {
			if typeStr == validType {
				isValidType = true
				break
			}
		}
		if !isValidType {
			response.Valid = false
			response.Errors = append(response.Errors, "无效的插件类型，必须是 adapter 或 sink")
		}
	}

	return response, nil
}

func (s *pluginService) GetPluginLogs(name string, req *models.PluginLogRequest) ([]models.PluginLog, int, error) {
	// 尝试从日志文件读取真实日志数据
	logs, err := s.readPluginLogs(name, req)
	if err != nil {
		// 没有日志文件时返回空列表。以前这里会编造"插件启动成功"等示例日志，
		// 用户无法分辨真假
		logs = nil
	}

	// 应用过滤器
	filteredLogs := s.filterLogs(logs, req)

	// 分页
	total := len(filteredLogs)
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize

	if start >= total {
		return []models.PluginLog{}, total, nil
	}
	if end > total {
		end = total
	}

	return filteredLogs[start:end], total, nil
}

// readPluginLogs 从日志文件读取插件日志
func (s *pluginService) readPluginLogs(name string, req *models.PluginLogRequest) ([]models.PluginLog, error) {
	// 常见的日志文件路径
	logPaths := []string{
		fmt.Sprintf("logs/%s.log", name),
		fmt.Sprintf("logs/plugin_%s.log", name),
		"logs/gateway.log", // 主日志文件
	}

	var allLogs []models.PluginLog
	logID := 1

	for _, logPath := range logPaths {
		logs, err := s.parseLogFile(logPath, name, &logID)
		if err == nil {
			allLogs = append(allLogs, logs...)
		}
	}

	if len(allLogs) == 0 {
		return nil, fmt.Errorf("no log files found for plugin %s", name)
	}

	// 按时间倒序排列（最新的在前）
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp.After(allLogs[j].Timestamp)
	})

	return allLogs, nil
}

// parseLogFile 解析日志文件
func (s *pluginService) parseLogFile(logPath, pluginName string, logID *int) ([]models.PluginLog, error) {
	file, err := os.Open(filepath.Clean(logPath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []models.PluginLog
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// 解析日志行，支持多种格式
		log := s.parseLogLine(line, pluginName, *logID)
		if log != nil {
			logs = append(logs, *log)
			*logID++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}

// parseLogLine 解析单行日志
func (s *pluginService) parseLogLine(line, pluginName string, logID int) *models.PluginLog {
	// 只返回与该插件相关的日志
	if !strings.Contains(strings.ToLower(line), strings.ToLower(pluginName)) {
		return nil
	}

	// 支持多种日志格式
	patterns := []struct {
		regex    *regexp.Regexp
		timeIdx  int
		levelIdx int
		msgIdx   int
	}{
		{
			// 格式: 2024-01-01 12:00:00 [INFO] message
			regex:    regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+\[(\w+)\]\s+(.+)$`),
			timeIdx:  1,
			levelIdx: 2,
			msgIdx:   3,
		},
		{
			// 格式: 2024-01-01T12:00:00Z INFO: message
			regex:    regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?)\s+(\w+):\s+(.+)$`),
			timeIdx:  1,
			levelIdx: 2,
			msgIdx:   3,
		},
		{
			// 格式: INFO 2024-01-01 12:00:00 message
			regex:    regexp.MustCompile(`^(\w+)\s+(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\s+(.+)$`),
			timeIdx:  2,
			levelIdx: 1,
			msgIdx:   3,
		},
	}

	for _, pattern := range patterns {
		matches := pattern.regex.FindStringSubmatch(line)
		if len(matches) > 3 {
			timeStr := matches[pattern.timeIdx]
			level := matches[pattern.levelIdx]
			message := matches[pattern.msgIdx]

			// 解析时间
			timestamp := s.parseTimestamp(timeStr)
			if timestamp.IsZero() {
				continue
			}

			return &models.PluginLog{
				ID:        int64(logID),
				Level:     strings.ToLower(level),
				Message:   message,
				Timestamp: timestamp,
				Source:    pluginName,
			}
		}
	}

	// 如果无法解析格式，创建一个简单的日志条目
	if strings.Contains(strings.ToLower(line), strings.ToLower(pluginName)) {
		return &models.PluginLog{
			ID:        int64(logID),
			Level:     "info",
			Message:   line,
			Timestamp: time.Now(),
			Source:    pluginName,
		}
	}

	return nil
}

// parseTimestamp 解析时间戳
func (s *pluginService) parseTimestamp(timeStr string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	return time.Time{}
}

func (s *pluginService) filterLogs(logs []models.PluginLog, req *models.PluginLogRequest) []models.PluginLog {
	var filtered []models.PluginLog

	for _, log := range logs {
		// 级别过滤
		if req.Level != "" && log.Level != req.Level {
			continue
		}

		// 时间范围过滤
		if !req.StartTime.IsZero() && log.Timestamp.Before(req.StartTime) {
			continue
		}
		if !req.EndTime.IsZero() && log.Timestamp.After(req.EndTime) {
			continue
		}

		// 关键词搜索
		if req.Search != "" {
			searchTerm := strings.ToLower(req.Search)
			if !strings.Contains(strings.ToLower(log.Message), searchTerm) {
				continue
			}
		}

		filtered = append(filtered, log)
	}

	return filtered
}

func (s *pluginService) GetPluginStats(name string) (*models.PluginStats, error) {
	// 检查插件是否存在
	plugin, exists := s.manager.GetPlugin(name)
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	// 尝试获取真实的适配器指标数据
	stats := &models.PluginStats{
		PluginID:        0,
		DataPointsTotal: 0,
		DataPointsHour:  0,
		ErrorsTotal:     0,
		ErrorsHour:      0,
		UptimeSeconds:   0,
		AverageLatency:  0,
		MemoryUsage:     0,
		CPUUsage:        0,
		LastUpdate:      time.Now(),
	}

	// 如果是适配器，获取真实的适配器指标
	if plugin.Type == "adapter" {
		if adapter, ok := s.manager.GetAdapter(name); ok {
			log.Debug().Str("plugin_name", name).Msg("找到适配器，尝试获取指标")
			// 检查适配器是否嵌入了BaseAdapter
			if extAdapter, ok := adapter.(interface {
				GetMetrics() (interface{}, error)
			}); ok {
				log.Debug().Str("plugin_name", name).Msg("适配器支持GetMetrics接口")
				if metrics, err := extAdapter.GetMetrics(); err == nil {
					log.Debug().Str("plugin_name", name).Interface("metrics", metrics).Msg("成功获取原始指标数据")
					// 将interface{}转换为AdapterMetrics
					if adapterMetrics, ok := metrics.(southbound.AdapterMetrics); ok {
						log.Debug().Str("plugin_name", name).Interface("adapter_metrics", adapterMetrics).Msg("成功转换为AdapterMetrics")
						stats.DataPointsTotal = adapterMetrics.DataPointsCollected
						stats.ErrorsTotal = adapterMetrics.ErrorsCount
						stats.UptimeSeconds = int64(adapterMetrics.ConnectionUptime.Seconds())

						// 计算最近一小时的数据点数（简化估算）
						if stats.UptimeSeconds > 0 {
							stats.DataPointsHour = stats.DataPointsTotal * 3600 / stats.UptimeSeconds
							if stats.DataPointsHour > stats.DataPointsTotal {
								stats.DataPointsHour = stats.DataPointsTotal
							}
						}

						// 使用真实的响应时间
						stats.AverageLatency = adapterMetrics.AverageResponseTime

						// 资源占用（内置插件无法单独统计）
						memUsage, cpuUsage := s.getResourceUsage(name)
						stats.MemoryUsage = memUsage
						stats.CPUUsage = cpuUsage
					} else {
						log.Info().Str("plugin_name", name).Interface("raw_metrics", metrics).Msg("❌ 无法转换为AdapterMetrics类型")
					}
				} else {
					log.Info().Str("plugin_name", name).Err(err).Msg("❌ GetMetrics调用失败")
				}
			} else {
				log.Info().Str("plugin_name", name).Msg("❌ 适配器不支持GetMetrics接口")
			}
		} else {
			log.Info().Str("plugin_name", name).Msg("❌ 未找到适配器")
		}
	} else if plugin.Type == "sink" {
		// 如果是连接器，获取真实的连接器指标
		if sink, ok := s.manager.GetSink(name); ok {
			// 检查连接器是否嵌入了BaseSink
			if extSink, ok := sink.(interface {
				GetMetrics() (interface{}, error)
			}); ok {
				if metrics, err := extSink.GetMetrics(); err == nil {
					// 将interface{}转换为SinkMetrics
					if sinkMetrics, ok := metrics.(northbound.SinkMetrics); ok {
						stats.DataPointsTotal = sinkMetrics.MessagesPublished
						stats.ErrorsTotal = sinkMetrics.ErrorsCount
						stats.UptimeSeconds = int64(sinkMetrics.ConnectionUptime.Seconds())

						// 计算最近一小时的消息数（简化估算）
						if stats.UptimeSeconds > 0 {
							stats.DataPointsHour = stats.DataPointsTotal * 3600 / stats.UptimeSeconds
							if stats.DataPointsHour > stats.DataPointsTotal {
								stats.DataPointsHour = stats.DataPointsTotal
							}
						}

						// 使用真实的响应时间
						stats.AverageLatency = sinkMetrics.AverageResponseTime

						// 资源占用（内置插件无法单独统计）
						memUsage, cpuUsage := s.getResourceUsage(name)
						stats.MemoryUsage = memUsage
						stats.CPUUsage = cpuUsage
					}
				}
			}
		}
	}

	// 智能插件状态检查：如果是内置插件，并且能获取到指标数据，则认为它正在运行
	isRunning := plugin.Status == "running"
	hasMetricsData := false

	// 检查是否成功获取到指标数据
	if plugin.Type == "adapter" && stats.DataPointsTotal > 0 {
		hasMetricsData = true
		log.Debug().Str("plugin_name", name).Int64("data_points", stats.DataPointsTotal).Msg("适配器有指标数据，认为正在运行")
	} else if plugin.Type == "sink" && (stats.DataPointsTotal > 0 || stats.MemoryUsage > 0 || stats.CPUUsage > 0) {
		hasMetricsData = true
		log.Debug().Str("plugin_name", name).Msg("连接器有指标数据，认为正在运行")
	}

	// 如果是内置插件且有指标数据，或者插件状态本来就是running，则认为正在运行
	if plugin.Mode == "builtin" && hasMetricsData {
		isRunning = true
		log.Debug().Str("plugin_name", name).Msg("内置插件有数据活动，设置为运行状态")
	}

	log.Debug().Str("plugin_name", name).Str("plugin_status", plugin.Status).Bool("is_running", isRunning).Msg("检查插件运行状态")
	if !isRunning {
		log.Debug().Str("plugin_name", name).Str("status", plugin.Status).Msg("插件未运行，重置资源使用率为0")
		stats.CPUUsage = 0
		stats.MemoryUsage = 0
		stats.UptimeSeconds = 0
	} else {
		log.Debug().Str("plugin_name", name).Int64("memory", stats.MemoryUsage).Float64("cpu", stats.CPUUsage).Msg("插件运行中，保持资源使用率")
	}

	return stats, nil
}

// getResourceUsage 获取插件的资源使用情况
func (s *pluginService) getResourceUsage(pluginName string) (memoryUsage int64, cpuUsage float64) {
	// 内置插件与网关运行在同一进程，无法单独统计资源占用，返回 0（前端显示"不可单独统计"）。
	// 以前返回 20~50MB、5%~25% 之间的随机数并标为"真实使用率"
	return 0, 0
}
