package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// 对象池优化 - 减少内存分配
var (
	// 数据点批次池
	pointBatchPool = sync.Pool{
		New: func() interface{} {
			return make([]model.Point, 0, 100)
		},
	}
	
	// JSON数据缓冲池
	jsonBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}
	
	// 设备数据点映射池
	deviceMapPool = sync.Pool{
		New: func() interface{} {
			return make(map[string][]model.Point)
		},
	}
)

type Meta struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Type        string                 `json:"type"` // adapter | sink
	Mode        string                 `json:"mode"` // go-plugin | isp-sidecar | builtin
	Entry       string                 `json:"entry"`
	Extra       map[string]interface{} `json:"-"` // 额外的元数据，不从JSON解析
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status,omitempty"` // 运行时状态: running, stopped
	ISPPort     int                   `json:"isp_port,omitempty"` // ISP端口配置
}

// PluginManager 定义了插件管理器的标准接口
type PluginManager interface {
	Init(cfg any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Name() string
	GetPlugins() []*Meta
	GetPlugin(name string) (*Meta, bool)
	StartPlugin(name string) error
	StopPlugin(name string) error
	RestartPlugin(name string) error
	GetAdapter(name string) (southbound.Adapter, bool)
	GetSink(name string) (northbound.Sink, bool)
}

type Manager struct {
	dir      string
	bus      *nats.Conn
	v        *viper.Viper
	watcher  *fsnotify.Watcher
	loader   *Loader
	mu       sync.Mutex
	ctx      context.Context
	dataChan chan model.Point

	// 已初始化的适配器和连接器
	adapters map[string]southbound.Adapter
	sinks    map[string]northbound.Sink
	
	// 插件元数据缓存优化
	pluginCache       []*Meta
	pluginCacheValid  bool
	pluginCacheMutex  sync.RWMutex
	lastCacheUpdate   time.Time
	cacheExpiration   time.Duration
}

func NewManager(dir string, bus *nats.Conn, v *viper.Viper) *Manager {
	return &Manager{
		dir:             dir,
		bus:             bus,
		v:               v,
		loader:          NewLoader(dir),
		adapters:        make(map[string]southbound.Adapter),
		sinks:           make(map[string]northbound.Sink),
		dataChan:        make(chan model.Point, 1000),
		cacheExpiration: 30 * time.Second, // 缓存30秒过期
	}
}

func (m *Manager) Name() string { return "plugin-manager" }
func (m *Manager) Init(cfg any) error {
	// 确保插件目录存在
	if err := ensureDir(m.dir); err != nil {
		return err
	}

	// 初始扫描加载插件元数据
	if err := m.scanAndLoadMeta(); err != nil {
		return err
	}

	// 加载所有发现的插件
	if err := m.loadAllDiscoveredPlugins(); err != nil {
		log.Warn().Err(err).Msg("加载插件时出现错误，继续初始化内置适配器")
	}

	// 打印所有可用的适配器
	adapters := m.loader.ListAdapters()
	log.Info().Strs("available_adapters", adapters).Msg("初始化适配器前的可用适配器")

	// 初始化所有适配器
	if err := m.initAdapters(); err != nil {
		return err
	}

	// 初始化所有连接器
	if err := m.initSinks(); err != nil {
		return err
	}

	return nil
}

// initSinks 初始化所有连接器
func (m *Manager) initSinks() error {
	// 从配置中获取连接器配置
	sinksConfig := m.v.Get("northbound.sinks")
	if sinksConfig == nil {
		log.Warn().Msg("未找到连接器配置")
		return nil
	}

	// 获取连接器配置列表
	sinksList, ok := sinksConfig.([]interface{})
	if !ok {
		return fmt.Errorf("连接器配置格式错误")
	}

	log.Info().Int("count", len(sinksList)).Msg("开始初始化连接器")

	for _, sinkConfig := range sinksList {
		sinkMap, ok := sinkConfig.(map[string]interface{})
		if !ok {
			return fmt.Errorf("连接器配置项格式错误")
		}

		name, ok := sinkMap["name"].(string)
		if !ok {
			return fmt.Errorf("连接器名称格式错误")
		}

		sinkType, ok := sinkMap["type"].(string)
		if !ok {
			return fmt.Errorf("连接器类型格式错误")
		}

		// 检查是否启用
		if enabled, ok := sinkMap["enabled"].(bool); ok && !enabled {
			log.Info().Str("name", name).Str("type", sinkType).Msg("连接器未启用，跳过初始化")
			continue
		}

		// 创建连接器实例
		sink, ok := northbound.Create(sinkType)
		if !ok {
			log.Error().Str("type", sinkType).Msg("未找到连接器类型")
			return fmt.Errorf("未找到类型为 %s 的连接器", sinkType)
		}

		// 初始化连接器
		log.Info().Str("name", name).Str("type", sinkType).Msg("初始化连接器")
		log.Error().Str("name", name).Msg("!!!! DEBUG: 这是来自manager.go的调试消息 !!!!")
		
		// 调试输出原始sink映射
		log.Debug().Str("name", name).Interface("sink_map", sinkMap).Msg("原始连接器映射")
		
		var configData []byte
		var err error
		
		// 检查是否使用新格式（扁平化配置）还是旧格式（嵌套config）
		if configField, hasConfig := sinkMap["config"]; hasConfig {
			// 旧格式：使用嵌套的config字段
			log.Debug().Str("name", name).Interface("config_field", configField).Msg("使用遗留连接器配置格式（嵌套config字段）")
			configData, err = json.Marshal(configField)
		} else {
			// 新格式：使用扁平化配置，移除控制字段但保留必需字段
			log.Debug().Str("name", name).Msg("使用新连接器配置格式（扁平化结构）")
			
			// 创建配置副本，移除控制字段但保留必需字段
			configMap := make(map[string]interface{})
			for k, v := range sinkMap {
				// 只跳过enabled字段，保留name和type字段
				if k != "enabled" {
					configMap[k] = v
				}
			}
			
			// 确保必需字段存在
			configMap["name"] = name
			configMap["type"] = sinkType
			
			// 调试输出配置映射
			log.Debug().Str("name", name).Interface("config_map", configMap).Msg("构建的连接器配置映射")
			
			configData, err = json.Marshal(configMap)
		}
		
		if err != nil {
			log.Error().Err(err).Str("name", name).Msg("序列化连接器配置失败")
			return fmt.Errorf("序列化连接器配置失败: %w", err)
		}

		log.Debug().Str("name", name).RawJSON("config", configData).Int("config_length", len(configData)).Msg("连接器配置")

		if err := sink.Init(configData); err != nil {
			return fmt.Errorf("初始化连接器 %s 失败: %w", name, err)
		}

		// 如果是MQTT连接器，设置NATS总线和名称
		if sinkType == "mqtt" {
			// 设置NATS总线
			if setter, ok := sink.(interface{ SetBus(*nats.Conn) }); ok {
				setter.SetBus(m.bus)
				log.Info().Str("name", name).Msg("为MQTT连接器设置NATS总线")
			}

			// 确保名称正确设置
			if nameSetter, ok := sink.(interface{ SetName(string) }); ok {
				nameSetter.SetName(name)
				log.Info().Str("name", name).Msg("为MQTT连接器设置名称")
			}
		}

		// 如果是NATS订阅器连接器，设置NATS连接
		if sinkType == "nats_subscriber" {
			if natsAwareSink, ok := sink.(northbound.NATSAwareSink); ok {
				natsAwareSink.SetNATSConnection(m.bus)
				log.Info().Str("name", name).Msg("为NATS订阅器连接器设置NATS连接")
			}
		}

		// 保存已初始化的连接器
		m.sinks[name] = sink
		log.Info().Str("name", name).Msg("连接器初始化成功")
	}

	return nil
}

// setupDataFlow 设置数据流
func (m *Manager) setupDataFlow(ctx context.Context) {
	// 使用管理器的数据通道
	// dataChan := make(chan model.Point, 1000) // old code

	// 启动所有连接器
	for name, sink := range m.sinks {
		log.Info().Str("name", name).Msg("启动连接器")
		if err := sink.Start(ctx); err != nil {
			log.Error().Err(err).Str("name", name).Msg("启动连接器失败")
			continue
		}
		log.Info().Str("name", name).Msg("连接器启动成功")
	}

	// 启动所有适配器，连接到数据通道
	for name, adapter := range m.adapters {
		log.Info().Str("name", name).Msg("启动适配器")
		if err := adapter.Start(ctx, m.dataChan); err != nil {
			log.Error().Err(err).Str("name", name).Msg("启动适配器失败")
			continue
		}
		log.Info().Str("name", name).Msg("适配器启动成功")
	}

	// 启动数据处理协程 - 使用对象池优化
	go func() {
		// 从对象池获取批次缓冲区
		batch := pointBatchPool.Get().([]model.Point)
		batch = batch[:0] // 重置长度但保留容量
		
		adapterCounts := make(map[string]int) // 统计每个适配器的数据点数量
		ticker := time.NewTicker(100 * time.Millisecond)
		statsTicker := time.NewTicker(10 * time.Second) // 每10秒输出一次统计信息
		defer ticker.Stop()
		defer statsTicker.Stop()
		
		// 用于跟踪指标的变量
		var totalDataPoints int64
		var totalBytesProcessed int64
		
		// 确保在退出时归还对象到池
		defer func() {
			batch = batch[:0]
			pointBatchPool.Put(batch)
		}()

		for {
			select {
			case <-ctx.Done():
				return

			case point := <-m.dataChan:
				// 记录数据来源
				adapterCounts[point.DeviceID]++

				// 更新指标计数
				totalDataPoints++
				// 估算数据点大小（设备ID + 键 + 值的字符串表示）
				pointSize := len(point.DeviceID) + len(point.Key) + len(fmt.Sprintf("%v", point.Value))
				totalBytesProcessed += int64(pointSize)

				// 更新轻量级指标收集器
				if lightweightMetrics := metrics.GetLightweightMetrics(); lightweightMetrics != nil {
					lightweightMetrics.UpdateDataMetrics(totalDataPoints, totalBytesProcessed, 0)
					log.Debug().Int64("total_data_points", totalDataPoints).Int64("total_bytes_processed", totalBytesProcessed).Msg("轻量级指标已更新")
				} else {
					log.Warn().Msg("轻量级指标收集器未初始化")
				}

				// 添加到批次
				batch = append(batch, point)

				// 详细记录每个数据点
				log.Debug().Str("device_id", point.DeviceID).Str("key", point.Key).Interface("value", point.Value).Msg("收到数据点")

				// 如果批次达到一定大小，立即发送
				if len(batch) >= 10 {
					m.sendBatchOptimized(batch)
					batch = batch[:0] // 重置长度但保留底层数组
				}

			case <-ticker.C:
				// 定期发送批次，即使没有达到批次大小
				if len(batch) > 0 {
					m.sendBatchOptimized(batch)
					batch = batch[:0] // 重置长度但保留底层数组
				}

			case <-statsTicker.C:
				// 输出统计信息
				if len(adapterCounts) > 0 {
					log.Info().Interface("adapter_counts", adapterCounts).Msg("适配器数据点统计")
					log.Info().Int64("total_data_points", totalDataPoints).Int64("total_bytes_processed", totalBytesProcessed).Msg("累计数据统计")
				}
			}
		}
	}()

	log.Info().Msg("数据流设置完成")
}

func (m *Manager) Start(ctx context.Context) error {
	log.Info().Msg("插件管理器开始启动")
	m.ctx = ctx // 保存上下文
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error().Err(err).Msg("创建文件监听器失败")
		return err
	}
	m.watcher = w
	if err := w.Add(m.dir); err != nil {
		log.Error().Err(err).Msg("添加插件目录监听失败")
		return err
	}
	log.Info().Msg("文件监听器设置完成")

	// Setup data flow between adapters and sinks
	log.Info().Msg("开始设置数据流")
	m.setupDataFlow(ctx)
	log.Info().Msg("数据流设置完成")

	go m.loop(ctx)
	log.Info().Msg("插件管理器启动完成")

	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	// 停止所有连接器（先停连接器）
	for name, sink := range m.sinks {
		log.Info().Str("name", name).Msg("停止连接器")
		if err := sink.Stop(); err != nil {
			log.Error().Err(err).Str("name", name).Msg("停止连接器失败")
		}
	}

	// 停止所有适配器
	for name, adapter := range m.adapters {
		log.Info().Str("name", name).Msg("停止适配器")
		if err := adapter.Stop(); err != nil {
			log.Error().Err(err).Str("name", name).Msg("停止适配器失败")
		}
	}

	// 关闭文件监听
	if m.watcher != nil {
		if err := m.watcher.Close(); err != nil {
			log.Error().Err(err).Msg("关闭插件目录监控失败")
		}
	}

	// 关闭插件加载器
	if m.loader != nil {
		m.loader.Close()
	}

	// 停止所有sidecar进程
	for name, proc := range m.loader.processes {
		if err := proc.Kill(); err != nil {
			log.Error().Err(err).Str("plugin", name).Msg("停止sidecar进程失败")
		}
	}

	return nil
}

func (m *Manager) loop(ctx context.Context) error {
	// 定期扫描插件目录，以防文件系统事件丢失
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case event := <-m.watcher.Events:
			log.Info().Str("file", event.Name).Str("op", event.Op.String()).Msg("检测到插件文件变动")
			// 文件变动后重新扫描
			if err := m.scanAndLoadMeta(); err != nil {
				log.Error().Err(err).Msg("重新扫描插件目录失败")
			}
		case err := <-m.watcher.Errors:
			log.Error().Err(err).Msg("监控插件目录失败")
		case <-ticker.C:
			// 定期扫描
			_ = m.scanAndLoadMeta()
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *Manager) scanAndLoadMeta() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.loader.Scan()
	if err == nil {
		// 扫描成功，使缓存失效
		m.invalidatePluginCache()
	}
	return err
}

// invalidatePluginCache 使插件缓存失效
func (m *Manager) invalidatePluginCache() {
	m.pluginCacheMutex.Lock()
	defer m.pluginCacheMutex.Unlock()
	
	m.pluginCacheValid = false
	m.pluginCache = nil
	log.Debug().Msg("插件缓存已失效")
}

// ensureDir 确保目录存在
func ensureDir(dir string) error {
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// 目录不存在，创建
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// GetAdapterInterface 获取已加载的适配器（接口形式）
func (m *Manager) GetAdapterInterface(name string) (interface{}, bool) {
	// 先从已初始化的适配器中查找
	if adapter, ok := m.adapters[name]; ok {
		return adapter, true
	}
	// 再从加载器中查找
	return m.loader.GetAdapter(name)
}

// GetSinkInterface 获取已加载的连接器（接口形式）
func (m *Manager) GetSinkInterface(name string) (interface{}, bool) {
	// 先从已初始化的连接器中查找
	if sink, ok := m.sinks[name]; ok {
		return sink, true
	}
	// 再从加载器中查找
	return m.loader.GetSink(name)
}

// sendBatch 发送批量数据点到所有连接器
func (m *Manager) sendBatch(points []model.Point) {
	// 按设备ID分组数据点，便于调试
	devicePoints := make(map[string][]model.Point)
	for _, point := range points {
		devicePoints[point.DeviceID] = append(devicePoints[point.DeviceID], point)
	}

	// 输出每个设备的数据点数量
	for deviceID, deviceData := range devicePoints {
		log.Info().Str("device_id", deviceID).Int("point_count", len(deviceData)).Msg("发送设备数据点")

		// 输出前几个数据点的详细信息
		for i, point := range deviceData {
			if i < 3 { // 只输出前3个数据点
				log.Info().Str("device_id", point.DeviceID).Str("key", point.Key).Interface("value", point.Value).Msg("数据点详情")
			}
		}
	}

	// 发送到所有连接器
	for name, sink := range m.sinks {
		// 直接发送到连接器
		if err := sink.Publish(points); err != nil {
			log.Error().Err(err).Str("name", name).Msg("发送数据到连接器失败")
		} else {
			log.Info().Str("name", name).Int("count", len(points)).Msg("成功发送数据到连接器")
		}

		// 同时发布到NATS总线，用于MQTT连接器和规则引擎订阅
		if m.bus != nil {
			topic := fmt.Sprintf("data.%s", name)
			for _, point := range points {
				data, err := json.Marshal(point)
				if err != nil {
					log.Error().Err(err).Str("name", name).Msg("序列化数据点失败")
					continue
				}

				// 发布到sink特定的主题（用于MQTT连接器）
				err = m.bus.Publish(topic, data)
				if err != nil {
					log.Error().Err(err).Str("name", name).Str("topic", topic).Msg("发布数据点到NATS失败")
				} else {
					log.Debug().Str("name", name).Str("topic", topic).Str("device_id", point.DeviceID).Str("key", point.Key).Msg("发布数据点到NATS")
				}

				// 同时发布到规则引擎主题
				rulesTopic := fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
				err = m.bus.Publish(rulesTopic, data)
				if err != nil {
					log.Error().Err(err).Str("topic", rulesTopic).Msg("发布数据点到规则引擎主题失败")
				} else {
					log.Debug().Str("topic", rulesTopic).Str("device_id", point.DeviceID).Str("key", point.Key).Msg("发布数据点到规则引擎主题")
				}
			}
		}
	}

	// 记录统计信息
	log.Info().Int("count", len(points)).Int("device_count", len(devicePoints)).Msg("发送数据点批次")
}

// sendBatchOptimized 优化的批量发送函数，使用对象池和批量NATS发布
func (m *Manager) sendBatchOptimized(points []model.Point) {
	if len(points) == 0 {
		return
	}

	// 从对象池获取设备映射
	devicePoints := deviceMapPool.Get().(map[string][]model.Point)
	defer func() {
		// 清理映射并归还到池
		for k := range devicePoints {
			delete(devicePoints, k)
		}
		deviceMapPool.Put(devicePoints)
	}()

	// 按设备ID分组数据点
	for _, point := range points {
		devicePoints[point.DeviceID] = append(devicePoints[point.DeviceID], point)
	}

	// 批量序列化数据点
	var serializedData [][]byte
	var natsSubjects []string
	jsonBuffer := jsonBufferPool.Get().([]byte)
	defer func() {
		jsonBuffer = jsonBuffer[:0]
		jsonBufferPool.Put(jsonBuffer)
	}()

	// 发送到所有连接器
	for name, sink := range m.sinks {
		// 直接发送到连接器
		if err := sink.Publish(points); err != nil {
			log.Error().Err(err).Str("name", name).Msg("发送数据到连接器失败")
		} else {
			log.Debug().Str("name", name).Int("count", len(points)).Msg("成功发送数据到连接器")
		}
	}

	// 批量发布到NATS总线 - 优化网络调用
	if m.bus != nil {
		// 预分配切片容量
		serializedData = make([][]byte, 0, len(points)*2)
		natsSubjects = make([]string, 0, len(points)*2)

		for _, point := range points {
			// 复用JSON缓冲区
			jsonBuffer = jsonBuffer[:0]
			data, err := json.Marshal(point)
			if err != nil {
				log.Error().Err(err).Msg("序列化数据点失败")
				continue
			}

			// 添加连接器特定主题
			for name := range m.sinks {
				topic := fmt.Sprintf("data.%s", name)
				serializedData = append(serializedData, data)
				natsSubjects = append(natsSubjects, topic)
			}

			// 添加规则引擎主题
			rulesTopic := fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
			serializedData = append(serializedData, data)
			natsSubjects = append(natsSubjects, rulesTopic)
		}

		// 批量发布所有消息
		if err := m.publishBatch(natsSubjects, serializedData); err != nil {
			log.Error().Err(err).Msg("批量发布NATS消息失败")
		}
	}

	// 输出统计信息（简化日志）
	log.Debug().Int("count", len(points)).Int("device_count", len(devicePoints)).Msg("发送数据点批次完成")
}

// publishBatch 批量发布NATS消息以减少网络开销
func (m *Manager) publishBatch(subjects []string, data [][]byte) error {
	if len(subjects) != len(data) {
		return fmt.Errorf("主题和数据数量不匹配")
	}

	// 使用异步发布提高性能
	for i, subject := range subjects {
		if err := m.bus.Publish(subject, data[i]); err != nil {
			log.Error().Err(err).Str("subject", subject).Msg("发布消息失败")
		}
	}

	// 刷新连接以确保消息发送
	return m.bus.Flush()
}

// updatePluginStatus 更新插件的运行状态
func (m *Manager) updatePluginStatus(meta *Meta) {
	// 对于内置插件，需要检查是否有基于此类型的实例正在运行
	if meta.Mode == "builtin" && meta.Type == "adapter" {
		// 检查是否有基于此类型的适配器实例正在运行
		log.Debug().Str("plugin", meta.Name).Int("running_adapters", len(m.adapters)).Msg("检查适配器状态")
		for instanceName := range m.adapters {
			log.Debug().Str("plugin", meta.Name).Str("instance", instanceName).Msg("检查适配器实例")
			// 从配置中获取适配器类型
			if adapterConfigs := m.v.Get("southbound.adapters"); adapterConfigs != nil {
				if configList, ok := adapterConfigs.([]interface{}); ok {
					for _, configItem := range configList {
						if configMap, ok := configItem.(map[string]interface{}); ok {
							configName := configMap["name"]
							configType := configMap["type"]
							log.Debug().Str("plugin", meta.Name).Str("instance", instanceName).Interface("config_name", configName).Interface("config_type", configType).Msg("检查配置匹配")
							if configMap["name"] == instanceName && configMap["type"] == meta.Name {
								log.Info().Str("plugin", meta.Name).Str("instance", instanceName).Msg("找到运行的适配器实例")
								meta.Status = "running"
								return
							}
						}
					}
				}
			}
		}
	} else if meta.Mode == "builtin" && meta.Type == "sink" {
		// 检查是否有基于此类型的连接器实例正在运行
		log.Debug().Str("plugin", meta.Name).Int("running_sinks", len(m.sinks)).Msg("检查连接器状态")
		for instanceName := range m.sinks {
			log.Debug().Str("plugin", meta.Name).Str("instance", instanceName).Msg("检查连接器实例")
			// 从配置中获取连接器类型
			if sinkConfigs := m.v.Get("northbound.sinks"); sinkConfigs != nil {
				if configList, ok := sinkConfigs.([]interface{}); ok {
					for _, configItem := range configList {
						if configMap, ok := configItem.(map[string]interface{}); ok {
							configName := configMap["name"]
							configType := configMap["type"]
							log.Debug().Str("plugin", meta.Name).Str("instance", instanceName).Interface("config_name", configName).Interface("config_type", configType).Msg("检查配置匹配")
							if configMap["name"] == instanceName && configMap["type"] == meta.Name {
								log.Info().Str("plugin", meta.Name).Str("instance", instanceName).Msg("找到运行的连接器实例")
								meta.Status = "running"
								return
							}
						}
					}
				}
			}
		}
	} else {
		// 对于外部插件，直接按名称查找
		log.Debug().Str("plugin", meta.Name).Msg("检查外部插件状态")
		if _, ok := m.adapters[meta.Name]; ok {
			log.Info().Str("plugin", meta.Name).Msg("找到运行的外部适配器")
			meta.Status = "running"
			return
		} else if _, ok := m.sinks[meta.Name]; ok {
			log.Info().Str("plugin", meta.Name).Msg("找到运行的外部连接器")
			meta.Status = "running"
			return
		}
	}
	// 如果没有找到运行的实例，状态为停止
	log.Debug().Str("plugin", meta.Name).Msg("未找到运行的实例，状态为停止")
	meta.Status = "stopped"
}

func (m *Manager) GetPlugins() []*Meta {
	// 检查缓存是否有效
	m.pluginCacheMutex.RLock()
	if m.pluginCacheValid && time.Since(m.lastCacheUpdate) < m.cacheExpiration {
		// 更新状态但使用缓存数据
		result := make([]*Meta, len(m.pluginCache))
		for i, p := range m.pluginCache {
			// 创建副本以避免并发修改
			meta := *p
			// 更新当前状态
			m.updatePluginStatus(&meta)
			result[i] = &meta
		}
		m.pluginCacheMutex.RUnlock()
		return result
	}
	m.pluginCacheMutex.RUnlock()

	// 需要重建缓存
	m.pluginCacheMutex.Lock()
	defer m.pluginCacheMutex.Unlock()

	// 双重检查，确保没有其他goroutine已经更新了缓存
	if m.pluginCacheValid && time.Since(m.lastCacheUpdate) < m.cacheExpiration {
		result := make([]*Meta, len(m.pluginCache))
		for i, p := range m.pluginCache {
			meta := *p
			m.updatePluginStatus(&meta)
			result[i] = &meta
		}
		return result
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取基于文件的外部插件
	plugins := m.loader.List()

	// 创建插件映射以避免重复
	pluginMap := make(map[string]*Meta)

	// 首先添加所有内置适配器
	for adapterType := range southbound.Registry {
		// 避免重复（如果已经有同名的外部插件）
		if _, exists := pluginMap[adapterType]; !exists {
			meta := &Meta{
				Name:        adapterType,
				Version:     "1.0.0",
				Type:        "adapter",
				Mode:        "builtin",
				Entry:       fmt.Sprintf("builtin://%s", adapterType),
				Description: fmt.Sprintf("内置%s适配器", adapterType),
				Status:      "stopped",
			}
			pluginMap[adapterType] = meta
		}
	}

	// 添加所有内置连接器
	for sinkType := range northbound.Registry {
		// 避免重复（如果已经有同名的外部插件）
		if _, exists := pluginMap[sinkType]; !exists {
			meta := &Meta{
				Name:        sinkType,
				Version:     "1.0.0",
				Type:        "sink",
				Mode:        "builtin",
				Entry:       fmt.Sprintf("builtin://%s", sinkType),
				Description: fmt.Sprintf("内置%s连接器", sinkType),
				Status:      "stopped",
			}
			pluginMap[sinkType] = meta
		}
	}

	// 添加真正的外部插件（过滤掉指向builtin://的包装文件）
	for _, p := range plugins {
		log.Debug().
			Str("name", p.Name).
			Str("entry", p.Entry).
			Str("type", p.Type).
			Bool("is_builtin_wrapper", strings.HasPrefix(p.Entry, "builtin://")).
			Msg("Processing external plugin")
		
		// 只添加真正的外部插件，跳过指向builtin://的包装文件
		if !strings.HasPrefix(p.Entry, "builtin://") {
			// 创建副本避免修改原始数据
			meta := &Meta{
				Name:        p.Name,
				Version:     p.Version,
				Type:        p.Type,
				Mode:        p.Mode,
				Entry:       p.Entry,
				Description: p.Description,
				Status:      "stopped", // 默认状态
			}
			pluginMap[p.Name] = meta
			log.Debug().Str("name", p.Name).Msg("Added external plugin to map")
		} else {
			log.Debug().Str("name", p.Name).Msg("Skipped builtin wrapper plugin")
		}
	}

	// 转换为列表并更新缓存
	m.pluginCache = make([]*Meta, 0, len(pluginMap))
	result := make([]*Meta, 0, len(pluginMap))
	
	for _, meta := range pluginMap {
		// 更新运行状态
		m.updatePluginStatus(meta)
		
		// 缓存副本（不包含动态状态）
		cacheMeta := &Meta{
			Name:        meta.Name,
			Version:     meta.Version,
			Type:        meta.Type,
			Mode:        meta.Mode,
			Entry:       meta.Entry,
			Description: meta.Description,
			Status:      "stopped", // 缓存中状态始终为停止，运行时动态更新
		}
		m.pluginCache = append(m.pluginCache, cacheMeta)
		result = append(result, meta)
	}

	// 更新缓存状态
	m.pluginCacheValid = true
	m.lastCacheUpdate = time.Now()

	// 最终调试信息
	log.Info().
		Int("total_plugins", len(result)).
		Strs("plugin_names", func() []string {
			names := make([]string, 0, len(result))
			for _, p := range result {
				names = append(names, fmt.Sprintf("%s(%s)", p.Name, p.Type))
			}
			return names
		}()).
		Msg("Final plugin list generated")

	return result
}

func (m *Manager) GetPlugin(name string) (*Meta, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 首先尝试从基于文件的插件中查找
	meta, ok := m.loader.Get(name)
	if ok {
		// 更新插件状态
		if _, adapterOk := m.adapters[meta.Name]; adapterOk {
			meta.Status = "running"
		} else if _, sinkOk := m.sinks[meta.Name]; sinkOk {
			meta.Status = "running"
		} else {
			meta.Status = "stopped"
		}
		return meta, true
	}

	// 如果没找到，尝试从内置适配器中查找
	if _, exists := southbound.Registry[name]; exists {
		meta := &Meta{
			Name:        name,
			Version:     "1.0.0",
			Type:        "adapter",
			Mode:        "builtin",
			Entry:       fmt.Sprintf("builtin://%s", name),
			Description: fmt.Sprintf("内置%s适配器", name),
			Status:      "stopped",
		}

		// 检查是否正在运行
		if _, ok := m.adapters[name]; ok {
			meta.Status = "running"
		}

		return meta, true
	}

	// 如果没找到，尝试从内置连接器中查找
	if _, exists := northbound.Registry[name]; exists {
		meta := &Meta{
			Name:        name,
			Version:     "1.0.0",
			Type:        "sink",
			Mode:        "builtin",
			Entry:       fmt.Sprintf("builtin://%s", name),
			Description: fmt.Sprintf("内置%s连接器", name),
			Status:      "stopped",
		}

		// 检查是否正在运行
		if _, ok := m.sinks[name]; ok {
			meta.Status = "running"
		}

		return meta, true
	}

	return nil, false
}

func (m *Manager) StartPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().Str("plugin_name", name).Msg("Attempting to start plugin")

	// 检查插件是否已经在运行
	if _, ok := m.adapters[name]; ok {
		return fmt.Errorf("adapter plugin '%s' is already running", name)
	}
	if _, ok := m.sinks[name]; ok {
		return fmt.Errorf("sink plugin '%s' is already running", name)
	}

	// 首先尝试从基于文件的插件中查找
	meta, ok := m.loader.Get(name)
	if ok {
		// 外部插件启动逻辑
		return m.startExternalPlugin(name, meta)
	}

	// 如果没找到外部插件，尝试启动内置适配器
	if _, exists := southbound.Registry[name]; exists {
		return m.startBuiltinAdapter(name)
	}

	// 如果没找到内置适配器，尝试启动内置连接器
	if _, exists := northbound.Registry[name]; exists {
		return m.startBuiltinSink(name)
	}

	return fmt.Errorf("plugin '%s' not found", name)
}

// startExternalPlugin 启动外部插件（基于文件）
func (m *Manager) startExternalPlugin(name string, meta *Meta) error {
	pluginConfigMap, found := m.findPluginConfigMap(name, meta.Type)
	if !found {
		log.Warn().Str("plugin_name", name).Msg("Configuration not found for plugin, proceeding with empty config.")
		pluginConfigMap = make(map[string]interface{})
	}

	var pluginConfig map[string]interface{}
	if cfg, ok := pluginConfigMap["config"].(map[string]interface{}); ok {
		pluginConfig = cfg
	} else {
		pluginConfig = make(map[string]interface{})
	}

	configData, err := json.Marshal(pluginConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal plugin config for %s: %w", name, err)
	}

	meta.Extra = pluginConfigMap
	if err := m.loader.LoadPlugin(*meta); err != nil {
		return fmt.Errorf("failed to load plugin '%s': %w", name, err)
	}

	if meta.Type == string(TypeAdapter) {
		adapter, ok := m.loader.GetAdapter(name)
		if !ok {
			return fmt.Errorf("failed to get adapter instance for '%s' after loading", name)
		}
		if err := adapter.Init(configData); err != nil {
			return fmt.Errorf("failed to init adapter '%s': %w", name, err)
		}
		if err := adapter.Start(m.ctx, m.dataChan); err != nil {
			return fmt.Errorf("failed to start adapter '%s': %w", name, err)
		}
		m.adapters[name] = adapter
		log.Info().Str("plugin_name", name).Msg("External adapter plugin started successfully")
	} else if meta.Type == string(TypeSink) {
		sink, ok := m.loader.GetSink(name)
		if !ok {
			return fmt.Errorf("failed to get sink instance for '%s' after loading", name)
		}
		if err := sink.Init(configData); err != nil {
			return fmt.Errorf("failed to init sink '%s': %w", name, err)
		}
		if setter, ok := sink.(interface{ SetBus(*nats.Conn) }); ok {
			setter.SetBus(m.bus)
		}
		if nameSetter, ok := sink.(interface{ SetName(string) }); ok {
			nameSetter.SetName(name)
		}
		if err := sink.Start(m.ctx); err != nil {
			return fmt.Errorf("failed to start sink '%s': %w", name, err)
		}
		m.sinks[name] = sink
		log.Info().Str("plugin_name", name).Msg("External sink plugin started successfully")
	}

	// 插件状态变化，使缓存失效
	m.invalidatePluginCache()
	
	return nil
}

// startBuiltinAdapter 启动内置适配器
func (m *Manager) startBuiltinAdapter(name string) error {
	// 从全局Registry创建适配器实例
	adapter, ok := southbound.Create(name)
	if !ok {
		return fmt.Errorf("failed to create builtin adapter '%s'", name)
	}

	// 获取配置（如果有的话）
	pluginConfigMap, found := m.findPluginConfigMap(name, "adapter")
	var pluginConfig map[string]interface{}
	if found {
		if cfg, ok := pluginConfigMap["config"].(map[string]interface{}); ok {
			pluginConfig = cfg
		}
	}
	if pluginConfig == nil {
		pluginConfig = make(map[string]interface{})
	}

	// 为内置适配器设置基本配置
	if pluginConfig["name"] == nil {
		pluginConfig["name"] = name
	}

	configData, err := json.Marshal(pluginConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal builtin adapter config for %s: %w", name, err)
	}

	if err := adapter.Init(configData); err != nil {
		return fmt.Errorf("failed to init builtin adapter '%s': %w", name, err)
	}
	if err := adapter.Start(m.ctx, m.dataChan); err != nil {
		return fmt.Errorf("failed to start builtin adapter '%s': %w", name, err)
	}

	m.adapters[name] = adapter
	log.Info().Str("plugin_name", name).Msg("Builtin adapter started successfully")
	
	// 插件状态变化，使缓存失效
	m.invalidatePluginCache()
	
	return nil
}

// startBuiltinSink 启动内置连接器
func (m *Manager) startBuiltinSink(name string) error {
	// 从全局Registry创建连接器实例
	sink, ok := northbound.Create(name)
	if !ok {
		return fmt.Errorf("failed to create builtin sink '%s'", name)
	}

	// 获取配置（如果有的话）
	pluginConfigMap, found := m.findPluginConfigMap(name, "sink")
	var pluginConfig map[string]interface{}
	if found {
		if cfg, ok := pluginConfigMap["config"].(map[string]interface{}); ok {
			pluginConfig = cfg
		}
	}
	if pluginConfig == nil {
		pluginConfig = make(map[string]interface{})
	}

	// 为内置连接器设置基本配置
	if pluginConfig["name"] == nil {
		pluginConfig["name"] = name
	}

	configData, err := json.Marshal(pluginConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal builtin sink config for %s: %w", name, err)
	}

	if err := sink.Init(configData); err != nil {
		return fmt.Errorf("failed to init builtin sink '%s': %w", name, err)
	}

	// 为特定类型的连接器设置特殊配置
	if setter, ok := sink.(interface{ SetBus(*nats.Conn) }); ok {
		setter.SetBus(m.bus)
	}
	if nameSetter, ok := sink.(interface{ SetName(string) }); ok {
		nameSetter.SetName(name)
	}

	if err := sink.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start builtin sink '%s': %w", name, err)
	}

	m.sinks[name] = sink
	log.Info().Str("plugin_name", name).Msg("Builtin sink started successfully")
	
	// 插件状态变化，使缓存失效
	m.invalidatePluginCache()
	
	return nil
}

// findPluginConfigMap finds the full configuration map for a plugin from viper.
func (m *Manager) findPluginConfigMap(name, pluginType string) (map[string]interface{}, bool) {
	var configKey string
	if pluginType == string(TypeAdapter) {
		configKey = "southbound.adapters"
	} else if pluginType == string(TypeSink) {
		configKey = "northbound.sinks"
	} else {
		return nil, false
	}

	configs := m.v.Get(configKey)
	if configs == nil {
		return nil, false
	}

	configList, ok := configs.([]interface{})
	if !ok {
		return nil, false
	}

	for _, cfg := range configList {
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			continue
		}
		if cfgName, ok := cfgMap["name"].(string); ok && cfgName == name {
			return cfgMap, true
		}
	}
	return nil, false
}

func (m *Manager) StopPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Info().Str("plugin_name", name).Msg("Attempting to stop plugin")

	// 检查是否是运行中的适配器
	if adapter, ok := m.adapters[name]; ok {
		log.Info().Str("plugin_name", name).Msg("Stopping adapter plugin")
		if err := adapter.Stop(); err != nil {
			log.Error().Err(err).Str("plugin_name", name).Msg("Failed to stop adapter cleanly, proceeding with cleanup")
		}
		delete(m.adapters, name)

		// 对于外部插件，需要卸载
		if _, isExternal := m.loader.Get(name); isExternal {
			if err := m.loader.Unload(name); err != nil {
				log.Error().Err(err).Str("plugin_name", name).Msg("Failed to unload external adapter")
			}
		}

		log.Info().Str("plugin_name", name).Msg("Adapter plugin stopped successfully")
		
		// 插件状态变化，使缓存失效
		m.invalidatePluginCache()
		
		return nil
	}

	// 检查是否是运行中的连接器
	if sink, ok := m.sinks[name]; ok {
		log.Info().Str("plugin_name", name).Msg("Stopping sink plugin")
		if err := sink.Stop(); err != nil {
			log.Error().Err(err).Str("plugin_name", name).Msg("Failed to stop sink cleanly, proceeding with cleanup")
		}
		delete(m.sinks, name)

		// 对于外部插件，需要卸载
		if _, isExternal := m.loader.Get(name); isExternal {
			if err := m.loader.Unload(name); err != nil {
				log.Error().Err(err).Str("plugin_name", name).Msg("Failed to unload external sink")
			}
		}

		log.Info().Str("plugin_name", name).Msg("Sink plugin stopped successfully")
		
		// 插件状态变化，使缓存失效
		m.invalidatePluginCache()
		
		return nil
	}

	return fmt.Errorf("plugin '%s' is not running", name)
}

func (m *Manager) RestartPlugin(name string) error {
	log.Info().Str("plugin_name", name).Msg("Attempting to restart plugin")
	if err := m.StopPlugin(name); err != nil {
		log.Error().Err(err).Str("plugin_name", name).Msg("Failed to stop plugin for restart, will still attempt to start")
	}
	return m.StartPlugin(name)
}

// GetAdapter returns the adapter instance by name
func (m *Manager) GetAdapter(name string) (southbound.Adapter, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	adapter, exists := m.adapters[name]
	return adapter, exists
}

// GetSink returns the sink instance by name
func (m *Manager) GetSink(name string) (northbound.Sink, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	sink, exists := m.sinks[name]
	return sink, exists
}

// loadAllDiscoveredPlugins 加载所有发现的插件
func (m *Manager) loadAllDiscoveredPlugins() error {
	plugins := m.loader.List()
	log.Info().Int("count", len(plugins)).Msg("开始加载发现的插件")
	
	var lastErr error
	loadedCount := 0
	
	for _, plugin := range plugins {
		log.Info().
			Str("name", plugin.Name).
			Str("type", plugin.Type).
			Str("mode", plugin.Mode).
			Msg("加载插件")
		
		if err := m.loader.LoadPlugin(*plugin); err != nil {
			log.Error().
				Err(err).
				Str("name", plugin.Name).
				Str("type", plugin.Type).
				Msg("加载插件失败")
			lastErr = err
			continue
		}
		
		loadedCount++
		log.Info().
			Str("name", plugin.Name).
			Str("type", plugin.Type).
			Msg("插件加载成功")
	}
	
	log.Info().
		Int("total", len(plugins)).
		Int("loaded", loadedCount).
		Msg("插件加载完成")
	
	return lastErr
}

// containsPluginWithType 检查插件映射中是否已经存在指定类型的插件
func containsPluginWithType(pluginMap map[string]*Meta, pluginType, expectedType string) bool {
	// 检查是否已经有外部插件使用了这个名称
	_, exists := pluginMap[pluginType]
	return exists
}
