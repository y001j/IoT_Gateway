package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
	"github.com/y001j/iot-gateway/internal/storage"
)

// 数据管道默认参数（可通过 gateway.pipeline.* 配置覆盖）
const (
	defaultPipelineBatchSize     = 100
	defaultPipelineFlushInterval = 50 * time.Millisecond
	defaultPipelineChannelSize   = 10000
	defaultSinkQueueSize         = 256
)

type Meta struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Type        string                 `json:"type"` // adapter | sink
	Mode        string                 `json:"mode"` // go-plugin | isp-sidecar | builtin
	Entry       string                 `json:"entry"`
	Extra       map[string]interface{} `json:"-"` // 额外的元数据，不从JSON解析
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status,omitempty"`   // 运行时状态: running, stopped
	ISPPort     int                    `json:"isp_port,omitempty"` // ISP端口配置
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

	// 每个连接器独立的分发器（队列 + 写入协程）
	dispatchers map[string]*sinkDispatcher
	dispMu      sync.RWMutex

	// 本地暂存（断网续传）：开启缓冲的连接器由 SinkRunner 从持久化流驱动，不走分发器
	store         *storage.Store
	bufferConfigs map[string]storage.BufferConfig
	runners       map[string]*storage.SinkRunner
	bufferOpMu    sync.Mutex

	// 数据管道参数
	batchSize     int
	flushInterval time.Duration
	sinkQueueSize int

	// 管道统计
	pointsReceived  atomic.Uint64
	natsPublishErrs atomic.Uint64

	// 插件元数据缓存优化
	pluginCache      []*Meta
	pluginCacheValid bool
	pluginCacheMutex sync.RWMutex
	lastCacheUpdate  time.Time
	cacheExpiration  time.Duration
}

func NewManager(dir string, bus *nats.Conn, v *viper.Viper) *Manager {
	m := &Manager{
		dir:             dir,
		bus:             bus,
		v:               v,
		loader:          NewLoader(dir),
		adapters:        make(map[string]southbound.Adapter),
		sinks:           make(map[string]northbound.Sink),
		dispatchers:     make(map[string]*sinkDispatcher),
		bufferConfigs:   make(map[string]storage.BufferConfig),
		runners:         make(map[string]*storage.SinkRunner),
		batchSize:       defaultPipelineBatchSize,
		flushInterval:   defaultPipelineFlushInterval,
		sinkQueueSize:   defaultSinkQueueSize,
		cacheExpiration: 30 * time.Second, // 缓存30秒过期
	}
	channelSize := defaultPipelineChannelSize
	if v != nil {
		if n := v.GetInt("gateway.pipeline.batch_size"); n > 0 {
			m.batchSize = n
		}
		if d := v.GetDuration("gateway.pipeline.flush_interval"); d > 0 {
			m.flushInterval = d
		}
		if n := v.GetInt("gateway.pipeline.sink_queue_size"); n > 0 {
			m.sinkQueueSize = n
		}
		if n := v.GetInt("gateway.pipeline.channel_size"); n > 0 {
			channelSize = n
		}
	}
	m.dataChan = make(chan model.Point, channelSize)
	return m
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
			// 单个连接器配置错误不应导致整个网关无法启动
			log.Error().Str("name", name).Str("type", sinkType).Msg("未找到连接器类型，跳过该连接器")
			continue
		}

		// 初始化连接器
		log.Info().Str("name", name).Str("type", sinkType).Msg("初始化连接器")

		// 调试输出原始sink映射
		log.Debug().Str("name", name).Interface("sink_map", sinkMap).Msg("原始连接器映射")

		configData, err := sinkConfigData(sinkMap, name, sinkType)

		if err != nil {
			log.Error().Err(err).Str("name", name).Msg("序列化连接器配置失败，跳过该连接器")
			continue
		}

		if err := sink.Init(configData); err != nil {
			log.Error().Err(err).Str("name", name).Str("type", sinkType).Msg("初始化连接器失败，跳过该连接器")
			continue
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

		// 记录缓冲（断网续传）配置
		if bc := storage.ParseBufferConfig(sinkMap["buffer"]); bc.Enabled {
			m.bufferConfigs[name] = bc
		}

		// 保存已初始化的连接器
		m.sinks[name] = sink
		log.Info().Str("name", name).Msg("连接器初始化成功")
	}

	return nil
}

// setupDataFlow 设置数据流
func (m *Manager) setupDataFlow(ctx context.Context) {
	// 启动所有连接器，并为每个连接器创建独立的分发器
	for name, sink := range m.sinks {
		log.Info().Str("name", name).Msg("启动连接器")
		if err := sink.Start(ctx); err != nil {
			log.Error().Err(err).Str("name", name).Msg("启动连接器失败")
			continue
		}
		m.attachSink(name, sink)
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

	go m.runPipeline(ctx)
	log.Info().
		Int("batch_size", m.batchSize).
		Dur("flush_interval", m.flushInterval).
		Int("sink_queue_size", m.sinkQueueSize).
		Msg("数据流设置完成")
}

// runPipeline 数据管道主循环：从适配器通道收集数据点，按批次分发。
// 这个协程只负责组批、入队和发布到 NATS，不做任何阻塞的 Sink 写入。
func (m *Manager) runPipeline(ctx context.Context) {
	batch := make([]model.Point, 0, m.batchSize)
	ticker := time.NewTicker(m.flushInterval)
	defer ticker.Stop()

	var totalPoints, totalBytes int64
	flush := func() {
		if len(batch) == 0 {
			return
		}
		m.dispatchBatch(batch)
		if lm := metrics.GetLightweightMetrics(); lm != nil {
			lm.UpdateDataMetrics(totalPoints, totalBytes, 0)
		}
		// 分发时各连接器拿到的是副本，这里可以安全复用底层数组
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case point := <-m.dataChan:
			m.pointsReceived.Add(1)
			totalPoints++
			// 粗略估算字节数（不做格式化，避免热路径分配）
			totalBytes += int64(len(point.DeviceID) + len(point.Key) + 16)
			batch = append(batch, point)
			if len(batch) >= m.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// dispatchBatch 把一批数据点分发给所有连接器，并发布到 NATS 供规则引擎消费。
func (m *Manager) dispatchBatch(points []model.Point) {
	if len(points) == 0 {
		return
	}

	m.dispMu.RLock()
	for _, d := range m.dispatchers {
		d.enqueue(copyBatchForSink(points))
	}
	m.dispMu.RUnlock()

	if m.bus == nil {
		return
	}
	for i := range points {
		data, err := json.Marshal(points[i])
		if err != nil {
			m.natsPublishErrs.Add(1)
			log.Error().Err(err).Str("device_id", points[i].DeviceID).Str("key", points[i].Key).Msg("序列化数据点失败")
			continue
		}
		if err := m.bus.Publish(model.DataSubject(points[i].DeviceID, points[i].Key), data); err != nil {
			m.natsPublishErrs.Add(1)
			log.Error().Err(err).Msg("发布数据点到NATS失败")
		}
	}
}

// SetStore 设置本地暂存服务（必须在 Start 之前调用）
func (m *Manager) SetStore(store *storage.Store) {
	m.store = store
}

// attachSink 把连接器接入数据流：开启缓冲且本地暂存可用时使用 SinkRunner，否则使用分发器
func (m *Manager) attachSink(name string, sink northbound.Sink) {
	if bc, ok := m.bufferConfigs[name]; ok && bc.Enabled {
		if m.store == nil {
			log.Warn().Str("sink", name).Msg("连接器配置了 buffer，但本地暂存未启用（storage.enabled），退回直连模式")
		} else {
			ctx := m.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			r, err := m.store.StartSinkRunner(ctx, name, sink, bc)
			if err == nil {
				m.dispMu.Lock()
				if old := m.runners[name]; old != nil {
					defer old.Stop()
				}
				m.runners[name] = r
				m.dispMu.Unlock()
				return
			}
			log.Error().Err(err).Str("sink", name).Msg("启动缓冲运行器失败，退回直连模式")
		}
	}
	m.addDispatcher(name, sink)
}

// detachSink 停止连接器的分发器或缓冲运行器
func (m *Manager) detachSink(name string) {
	m.removeDispatcher(name)
	m.dispMu.Lock()
	r := m.runners[name]
	delete(m.runners, name)
	m.dispMu.Unlock()
	if r != nil {
		r.Stop()
	}
}

// GetBufferStats 返回开启缓冲的连接器的状态（积压量、丢失量等）
func (m *Manager) GetBufferStats() []storage.RunnerStats {
	m.dispMu.RLock()
	runners := make([]*storage.SinkRunner, 0, len(m.runners))
	for _, r := range m.runners {
		runners = append(runners, r)
	}
	m.dispMu.RUnlock()
	out := make([]storage.RunnerStats, 0, len(runners))
	for _, r := range runners {
		out = append(out, r.Stats())
	}
	return out
}

// ErrSinkNotBuffered 连接器未开启本地缓冲
var ErrSinkNotBuffered = errors.New("连接器未开启本地缓冲")

// ControlSinkBuffer 对开启缓冲的连接器执行运维操作：
// pause（暂停写入，数据在本地积压）、resume（恢复）、skip（丢弃积压，从最新数据开始）、
// replay（从 from 时刻起重新投递，会产生重复数据）。
func (m *Manager) ControlSinkBuffer(name, action string, from time.Time) error {
	m.bufferOpMu.Lock()
	defer m.bufferOpMu.Unlock()
	m.dispMu.RLock()
	r := m.runners[name]
	m.dispMu.RUnlock()
	if r == nil || m.store == nil {
		return ErrSinkNotBuffered
	}
	var startFrom string
	switch action {
	case "pause":
		r.Pause()
		return nil
	case "resume":
		r.Resume()
		return nil
	case "skip":
		startFrom = "new"
	case "replay":
		startFrom = "time"
	default:
		return fmt.Errorf("不支持的操作 %q", action)
	}
	nr, err := m.store.ResetSinkRunner(r, startFrom, from)
	if err != nil {
		return err
	}
	m.dispMu.Lock()
	if m.runners[name] == r {
		m.runners[name] = nr
		nr = nil
	}
	m.dispMu.Unlock()
	if nr != nil {
		nr.Stop() // 期间连接器被移除
	}
	log.Info().Str("sink", name).Str("action", action).Time("from", from).Msg("已执行本地缓冲运维操作")
	return nil
}

// GetStore 返回本地暂存服务（未启用时为 nil）
func (m *Manager) GetStore() *storage.Store { return m.store }

// addDispatcher 为连接器创建分发器（若已存在则先停止旧的）
func (m *Manager) addDispatcher(name string, sink northbound.Sink) {
	d := newSinkDispatcher(name, sink, m.sinkQueueSize)
	m.dispMu.Lock()
	old := m.dispatchers[name]
	m.dispatchers[name] = d
	m.dispMu.Unlock()
	if old != nil {
		old.stop(5 * time.Second)
	}
}

// removeDispatcher 停止并移除连接器的分发器
func (m *Manager) removeDispatcher(name string) {
	m.dispMu.Lock()
	d := m.dispatchers[name]
	delete(m.dispatchers, name)
	m.dispMu.Unlock()
	if d != nil {
		d.stop(5 * time.Second)
	}
}

// GetSinkDispatchStats 返回各连接器的分发统计（队列长度、发送和丢弃数量）
func (m *Manager) GetSinkDispatchStats() []SinkDispatchStats {
	m.dispMu.RLock()
	defer m.dispMu.RUnlock()
	out := make([]SinkDispatchStats, 0, len(m.dispatchers))
	for _, d := range m.dispatchers {
		out = append(out, d.stats())
	}
	return out
}

// PipelineStats 管道统计
type PipelineStats struct {
	PointsReceived  uint64 `json:"points_received"`
	NATSPublishErrs uint64 `json:"nats_publish_errors"`
	ChannelLen      int    `json:"channel_len"`
	ChannelCap      int    `json:"channel_cap"`
}

// GetPipelineStats 返回数据管道统计
func (m *Manager) GetPipelineStats() PipelineStats {
	return PipelineStats{
		PointsReceived:  m.pointsReceived.Load(),
		NATSPublishErrs: m.natsPublishErrs.Load(),
		ChannelLen:      len(m.dataChan),
		ChannelCap:      cap(m.dataChan),
	}
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
	// 先停止分发器（会尽量把队列中的数据写完），再停止连接器
	m.dispMu.Lock()
	dispatchers := m.dispatchers
	m.dispatchers = make(map[string]*sinkDispatcher)
	m.dispMu.Unlock()
	for _, d := range dispatchers {
		d.stop(5 * time.Second)
	}
	m.dispMu.Lock()
	runners := m.runners
	m.runners = make(map[string]*storage.SinkRunner)
	m.dispMu.Unlock()
	for _, r := range runners {
		r.Stop()
	}

	// 停止所有连接器
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

	if m.loader != nil {
		// 停止所有sidecar进程
		for name, proc := range m.loader.processes {
			if proc == nil {
				continue
			}
			if err := proc.Kill(); err != nil {
				log.Error().Err(err).Str("plugin", name).Msg("停止sidecar进程失败")
			}
		}
		// 关闭插件加载器
		_ = m.loader.Close()
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
		if err := os.MkdirAll(dir, 0o750); err != nil {
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

	// 配置文件中按名称定义的实例（例如 name: mock1, type: mock）。
	// 以前只把 name 当作类型去注册表里查，实例名与类型不同时停止后无法再启动，
	// RestartPlugin（停止 + 启动）会把正在运行的实例停掉。
	if _, found := m.findPluginConfigMap(name, string(TypeAdapter)); found {
		return m.startBuiltinAdapter(name)
	}
	if _, found := m.findPluginConfigMap(name, string(TypeSink)); found {
		return m.startBuiltinSink(name)
	}

	// 没有配置时，把名称当作内置类型启动（使用默认配置）
	if _, exists := southbound.Registry[name]; exists {
		return m.startBuiltinAdapter(name)
	}
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
		m.attachSink(name, sink)
		log.Info().Str("plugin_name", name).Msg("External sink plugin started successfully")
	}

	// 插件状态变化，使缓存失效
	m.invalidatePluginCache()

	return nil
}

// startBuiltinAdapter 启动内置适配器
func (m *Manager) startBuiltinAdapter(name string) error {
	// 配置中有同名实例时使用它的类型和配置（与网关启动时 initAdapters 的方式一致）
	adapterType := name
	var configData []byte
	var err error
	if cfgMap, found := m.findPluginConfigMap(name, string(TypeAdapter)); found {
		if t, ok := cfgMap["type"].(string); ok && t != "" {
			adapterType = t
		}
		configData, err = adapterConfigData(cfgMap, name, adapterType)
	} else {
		configData, err = json.Marshal(map[string]interface{}{"name": name, "type": adapterType})
	}
	if err != nil {
		return fmt.Errorf("failed to build builtin adapter config for %s: %w", name, err)
	}

	adapter, ok := southbound.Create(adapterType)
	if !ok {
		return fmt.Errorf("failed to create builtin adapter '%s' (type %s)", name, adapterType)
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
	// 配置中有同名实例时使用它的类型和配置（与网关启动时 initSinks 的方式一致）
	sinkType := name
	var cfgMap map[string]interface{}
	if c, found := m.findPluginConfigMap(name, string(TypeSink)); found {
		cfgMap = c
		if t, ok := c["type"].(string); ok && t != "" {
			sinkType = t
		}
	} else {
		cfgMap = map[string]interface{}{"name": name, "type": sinkType}
	}
	configData, err := sinkConfigData(cfgMap, name, sinkType)
	if err != nil {
		return fmt.Errorf("failed to build builtin sink config for %s: %w", name, err)
	}

	sink, ok := northbound.Create(sinkType)
	if !ok {
		return fmt.Errorf("failed to create builtin sink '%s' (type %s)", name, sinkType)
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
	if natsAwareSink, ok := sink.(northbound.NATSAwareSink); ok {
		natsAwareSink.SetNATSConnection(m.bus)
	}
	if bc := storage.ParseBufferConfig(cfgMap["buffer"]); bc.Enabled {
		m.bufferConfigs[name] = bc
	}

	if err := sink.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start builtin sink '%s': %w", name, err)
	}

	m.sinks[name] = sink
	m.attachSink(name, sink)
	log.Info().Str("plugin_name", name).Msg("Builtin sink started successfully")

	// 插件状态变化，使缓存失效
	m.invalidatePluginCache()

	return nil
}

// sinkConfigData 生成连接器的初始化配置：旧格式使用嵌套的 config 字段，
// 新格式使用扁平化配置（去掉 enabled，确保 name/type 存在）
func sinkConfigData(sinkMap map[string]interface{}, name, sinkType string) ([]byte, error) {
	if configField, hasConfig := sinkMap["config"]; hasConfig {
		return json.Marshal(configField)
	}
	configMap := make(map[string]interface{}, len(sinkMap)+2)
	for k, v := range sinkMap {
		if k != "enabled" {
			configMap[k] = v
		}
	}
	configMap["name"] = name
	configMap["type"] = sinkType
	return json.Marshal(configMap)
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
		m.detachSink(name)
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
