package plugin

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// PluginType 表示插件类型
type PluginType string

const (
	TypeAdapter PluginType = "adapter" // 南向设备适配器
	TypeSink    PluginType = "sink"    // 北向连接器
)

// Loader 负责加载插件
type Loader struct {
	dir string
	// metadatas holds all discovered plugin metadata, even if not loaded.
	metadatas map[string]Meta
	adapters  map[string]southbound.Adapter
	sinks     map[string]northbound.Sink
	mu        sync.RWMutex
	// 维护已加载的插件句柄，防止GC回收
	handles map[string]*plugin.Plugin
	// 维护运行中的sidecar进程
	processes map[string]*os.Process
}

// NewLoader 创建一个新的插件加载器
func NewLoader(dir string) *Loader {
	return &Loader{
		dir:       dir,
		metadatas: make(map[string]Meta),
		adapters:  make(map[string]southbound.Adapter),
		sinks:     make(map[string]northbound.Sink),
		handles:   make(map[string]*plugin.Plugin),
		processes: make(map[string]*os.Process),
	}
}

// Scan scans the plugin directory for all plugin.json files and populates metadata.
func (l *Loader) Scan() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.metadatas = make(map[string]Meta) // Clear existing

	files, err := ioutil.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("could not read plugin directory %s: %w", l.dir, err)
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(l.dir, file.Name())
		data, err := ioutil.ReadFile(filepath.Clean(filePath))
		if err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("Failed to read plugin meta file")
			continue
		}

		var meta Meta
		if err := json.Unmarshal(data, &meta); err != nil {
			log.Warn().Err(err).Str("file", filePath).Msg("Failed to unmarshal plugin meta file")
			continue
		}

		// Use filename (without .json) as the unique key if name is empty
		if meta.Name == "" {
			meta.Name = strings.TrimSuffix(file.Name(), ".json")
		}

		l.metadatas[meta.Name] = meta
		log.Info().Str("name", meta.Name).Str("type", meta.Type).Msg("Discovered plugin")
	}
	return nil
}

// List returns all discovered plugin metadata.
func (l *Loader) List() []*Meta {
	l.mu.RLock()
	defer l.mu.RUnlock()

	list := make([]*Meta, 0, len(l.metadatas))
	for i := range l.metadatas {
		m := l.metadatas[i]
		list = append(list, &m)
	}
	return list
}

// Get returns a specific plugin's metadata.
func (l *Loader) Get(name string) (*Meta, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	meta, ok := l.metadatas[name]
	if !ok {
		return nil, false
	}
	return &meta, true
}

// LoadPlugin 加载一个插件
func (l *Loader) LoadPlugin(meta Meta) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	log.Info().
		Str("name", meta.Name).
		Str("version", meta.Version).
		Str("type", meta.Type).
		Msg("加载插件")

	// 确定插件类型
	pType := PluginType(meta.Type)
	if pType != TypeAdapter && pType != TypeSink {
		return fmt.Errorf("未知的插件类型: %s", meta.Type)
	}

	// 检查是否为内置插件
	if strings.HasPrefix(meta.Entry, "builtin://") {
		return l.loadBuiltinPlugin(meta)
	}

	// 确定插件路径，避免路径重复
	var pluginPath string
	if filepath.IsAbs(meta.Entry) {
		// 绝对路径，直接使用
		pluginPath = meta.Entry
	} else if strings.HasPrefix(meta.Entry, l.dir+"/") || strings.HasPrefix(meta.Entry, l.dir+"\\") {
		// 已经包含插件目录前缀，直接使用
		pluginPath = meta.Entry
	} else {
		// 相对路径，需要加上插件目录前缀
		pluginPath = filepath.Join(l.dir, meta.Entry)
	}

	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf("插件文件不存在: %w", err)
	}

	// 根据插件类型和扩展名决定加载方式
	ext := filepath.Ext(meta.Entry)
	switch ext {
	case ".so":
		return l.loadGoPlugin(meta, pluginPath)
	case ".exe", "":
		return l.loadSidecar(meta, pluginPath)
	default:
		return fmt.Errorf("不支持的插件文件类型: %s", ext)
	}
}

// loadGoPlugin 加载Go插件(.so)
func (l *Loader) loadGoPlugin(meta Meta, path string) error {
	// 加载插件
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("打开插件失败: %w", err)
	}

	// 保存插件句柄防止GC
	l.handles[meta.Name] = p

	// 查找初始化函数
	var initSymbol string
	if meta.Type == string(TypeAdapter) {
		initSymbol = "NewAdapter"
	} else {
		initSymbol = "NewSink"
	}

	initSym, err := p.Lookup(initSymbol)
	if err != nil {
		return fmt.Errorf("查找初始化函数失败: %w", err)
	}

	// 根据插件类型执行不同的初始化
	if meta.Type == string(TypeAdapter) {
		// 适配器初始化
		adapterInit, ok := initSym.(func() southbound.Adapter)
		if !ok {
			return fmt.Errorf("适配器初始化函数签名不匹配")
		}

		adapter := adapterInit()
		l.adapters[meta.Name] = adapter
		log.Info().Str("name", meta.Name).Msg("适配器注册成功")
	} else {
		// 连接器初始化
		sinkInit, ok := initSym.(func() northbound.Sink)
		if !ok {
			return fmt.Errorf("连接器初始化函数签名不匹配")
		}

		sink := sinkInit()
		l.sinks[meta.Name] = sink
		log.Info().Str("name", meta.Name).Msg("连接器注册成功")
	}

	return nil
}

// loadSidecar 加载sidecar插件(可执行文件)
func (l *Loader) loadSidecar(meta Meta, path string) error {
	// 检查是否已有同名进程运行
	if proc, exists := l.processes[meta.Name]; exists {
		// 尝试终止旧进程
		if err := proc.Kill(); err != nil {
			log.Warn().Err(err).Str("name", meta.Name).Msg("终止旧进程失败")
		}
		delete(l.processes, meta.Name)
	}

	// 打印完整路径信息
	log.Info().Str("plugin_path", path).Msg("尝试启动 sidecar 进程")

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 如果文件不存在，尝试使用相对于插件目录的路径
		pluginName := filepath.Base(meta.Name)
		alternatePath := filepath.Join(l.dir, pluginName, filepath.Base(path))
		log.Info().Str("alternate_path", alternatePath).Msg("尝试替代路径")

		if _, err := os.Stat(alternatePath); os.IsNotExist(err) {
			return fmt.Errorf("插件文件不存在: %s 或 %s", path, alternatePath)
		}

		// 使用替代路径
		path = alternatePath
	}

	// 启动sidecar进程
	cmd := exec.Command(path)

	// 设置环境变量，特别是ISP端口
	cmd.Env = os.Environ() // 继承系统环境变量
	if meta.ISPPort > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ISP_PORT=%d", meta.ISPPort))
		log.Info().
			Int("isp_port", meta.ISPPort).
			Str("name", meta.Name).
			Msg("🔵 [调试] 设置sidecar ISP端口环境变量")
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动sidecar进程失败: %w", err)
	}

	// 保存进程引用
	l.processes[meta.Name] = cmd.Process

	log.Info().
		Str("name", meta.Name).
		Int("pid", cmd.Process.Pid).
		Msg("sidecar进程启动成功")

	// 等待sidecar启动
	time.Sleep(500 * time.Millisecond)

	// 根据插件模式和类型创建不同的代理对象
	log.Debug().Str("plugin_name", meta.Name).Str("plugin_type", meta.Type).Str("plugin_mode", meta.Mode).Msg("开始创建代理对象")
	switch PluginType(meta.Type) {
	case TypeAdapter:
		var adapterProxy southbound.Adapter
		var err error

		// 根据模式选择不同的代理类型
		switch meta.Mode {
		case "isp-sidecar":
			// 获取ISP端口
			ispPort := 50052 // 默认ISP端口
			if meta.ISPPort != 0 {
				ispPort = meta.ISPPort
			}
			ispAddress := fmt.Sprintf("127.0.0.1:%d", ispPort)

			log.Info().
				Int("configured_port", meta.ISPPort).
				Int("actual_port", ispPort).
				Str("address", ispAddress).
				Msg("配置ISP连接地址")

			// 创建ISP适配器代理
			adapterProxy, err = NewISPAdapterProxy(meta.Name, ispAddress)
			if err != nil {
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				return fmt.Errorf("创建 ISP 适配器代理失败: %w", err)
			}
		default:
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return fmt.Errorf("不支持的sidecar模式: %s", meta.Mode)
		}

		// 注册适配器
		adapterType := meta.Name // 默认使用插件名称作为类型名

		// 从元数据中提取类型名称，如 "modbus"
		parts := strings.Split(meta.Name, "-")
		if len(parts) > 0 {
			adapterType = parts[0]
		}

		// 先清除可能存在的旧注册
		delete(l.adapters, meta.Name)
		delete(l.adapters, adapterType)
		delete(l.adapters, "modbus")

		// 重新注册到Loader
		l.adapters[meta.Name] = adapterProxy
		l.adapters[adapterType] = adapterProxy

		// 特殊处理 modbus 适配器
		if meta.Name == "modbus" || adapterType == "modbus" {
			log.Info().Msg("显式注册 modbus 适配器")
			l.adapters["modbus"] = adapterProxy
		}

		// 重要：同时注册到全局Registry，这样plugin_init.go中的southbound.Create()就能找到
		// 为sidecar插件生成组合类型名，格式：{原类型}-{模式}
		sidecarTypeName := fmt.Sprintf("%s-%s", adapterType, meta.Mode)

		// 注册原始类型名
		southbound.Register(adapterType, func() southbound.Adapter {
			return adapterProxy
		})

		// 注册组合类型名（用于避免与内置适配器冲突）
		southbound.Register(sidecarTypeName, func() southbound.Adapter {
			return adapterProxy
		})

		log.Info().
			Str("original_type", adapterType).
			Str("sidecar_type", sidecarTypeName).
			Msg("注册sidecar适配器类型")

		// 再次检查是否成功注册
		_, exists := l.adapters["modbus"]
		log.Info().Bool("modbus_registered", exists).Msg("检查 modbus 适配器注册状态")

		// 打印所有注册的适配器
		adapterNames := make([]string, 0, len(l.adapters))
		for k := range l.adapters {
			adapterNames = append(adapterNames, k)
		}
		log.Info().Str("name", meta.Name).Str("type", adapterType).Strs("registered_adapters", adapterNames).Msg("ISP适配器代理注册成功")

		// 检查是否成功注册了 modbus 适配器
		_, modbus_exists := l.adapters["modbus"]
		log.Debug().Bool("modbus_exists", modbus_exists).Str("mode", meta.Mode).Msg("检查 modbus 适配器是否存在")

	case TypeSink:
		// TODO: 实现ISP连接器代理
		return fmt.Errorf("暂不支持ISP Sidecar连接器")

	default:
		return fmt.Errorf("未知的插件类型: %s", meta.Type)
	}

	return nil
}

// loadBuiltinPlugin 加载内置插件
func (l *Loader) loadBuiltinPlugin(meta Meta) error {
	// 解析内置插件名称
	parts := strings.Split(meta.Entry, "//")
	if len(parts) != 2 || parts[1] == "" {
		log.Error().Str("entry", meta.Entry).Msg("无效的内置插件路径格式")
		return fmt.Errorf("无效的内置插件路径: %s", meta.Entry)
	}

	builtinName := parts[1]
	log.Info().Str("name", meta.Name).Str("builtin", builtinName).Str("type", meta.Type).Msg("加载内置插件")

	// 根据插件类型从注册表创建内置插件。以前适配器只支持写死的 mock，连接器也是写死的列表
	switch PluginType(meta.Type) {
	case TypeAdapter:
		adapter, ok := southbound.Create(builtinName)
		if !ok {
			return fmt.Errorf("未知的内置适配器: %s", builtinName)
		}
		if _, exists := l.adapters[meta.Name]; exists {
			log.Warn().Str("name", meta.Name).Msg("适配器已存在，将被覆盖")
		}
		// 同时使用内置名称和配置名称作为键
		l.adapters[builtinName] = adapter
		l.adapters[meta.Name] = adapter
		log.Info().Str("name", meta.Name).Str("type", builtinName).Msg("内置适配器注册成功")
		return nil

	case TypeSink:
		sink := northbound.CreateSink(builtinName)
		if sink == nil {
			return fmt.Errorf("未知的内置连接器: %s", builtinName)
		}
		l.sinks[builtinName] = sink
		l.sinks[meta.Name] = sink
		log.Info().Str("name", meta.Name).Str("type", builtinName).Msg("内置连接器注册成功")
		return nil
	}

	return fmt.Errorf("未知的插件类型: %s", meta.Type)
}

// GetAdapter 获取已加载的适配器
func (l *Loader) GetAdapter(name string) (southbound.Adapter, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// 打印调试信息
	log.Debug().Str("requested_name", name).Msg("查找适配器")

	// 打印所有可用的适配器
	adapterNames := make([]string, 0, len(l.adapters))
	for k := range l.adapters {
		adapterNames = append(adapterNames, k)
	}
	log.Debug().Str("requested_name", name).Strs("available_adapters", adapterNames).Msg("查找适配器")

	// 直接查找适配器（作为管理器查找的后备路径，找不到是正常情况，不记为错误）
	adapter, exists := l.adapters[name]
	if !exists {
		log.Debug().Str("requested_name", name).Strs("available_adapters", adapterNames).Msg("适配器未找到")
	}
	return adapter, exists
}

// ListAdapters 列出所有可用的适配器类型
func (l *Loader) ListAdapters() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	adapterNames := make([]string, 0, len(l.adapters))
	for k := range l.adapters {
		adapterNames = append(adapterNames, k)
	}
	return adapterNames
}

// GetSink 获取已加载的连接器
func (l *Loader) GetSink(name string) (northbound.Sink, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	sink, ok := l.sinks[name]
	return sink, ok
}

// Unload stops and unloads a single plugin by name.
// For sidecars, it terminates the process. For .so plugins, it removes the reference.
func (l *Loader) Unload(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	log.Info().Str("plugin_name", name).Msg("Unloading plugin")

	// 1. Stop and remove sidecar process if it exists
	if proc, exists := l.processes[name]; exists {
		log.Info().Int("pid", proc.Pid).Str("plugin_name", name).Msg("Terminating sidecar process")
		if err := proc.Kill(); err != nil {
			// Log error but continue cleanup
			log.Error().Err(err).Str("plugin_name", name).Msg("Failed to kill sidecar process")
		}
		delete(l.processes, name)
	}

	// 2. Remove from handles if it's a Go plugin
	if _, exists := l.handles[name]; exists {
		// Note: .so files cannot be truly unloaded in Go. We just remove the handle.
		// The OS will hold the code in memory until the main process exits.
		delete(l.handles, name)
		log.Info().Str("plugin_name", name).Msg("Removed Go plugin handle")
	}

	// 3. Remove from adapter/sink maps
	delete(l.adapters, name)
	delete(l.sinks, name)

	log.Info().Str("plugin_name", name).Msg("Plugin successfully unloaded")
	return nil
}

// Close unloads all plugins and cleans up resources
func (l *Loader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清理所有sidecar进程
	for name, proc := range l.processes {
		if err := proc.Kill(); err != nil {
			log.Error().Err(err).Str("name", name).Msg("终止进程失败")
		}
	}
	l.processes = make(map[string]*os.Process)

	// 清理插件引用
	l.handles = make(map[string]*plugin.Plugin)
	l.adapters = make(map[string]southbound.Adapter)
	l.sinks = make(map[string]northbound.Sink)

	return nil
}
