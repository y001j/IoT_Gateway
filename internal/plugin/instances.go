package plugin

import (
	"sort"
	"strings"
)

// InstanceStatus 描述一个适配器/连接器实例的状态
type InstanceStatus struct {
	Name     string `json:"name"`
	Type     string `json:"type"`     // 插件类型，例如 mock、websocket；外部插件为其入口
	Kind     string `json:"kind"`     // adapter | sink
	Mode     string `json:"mode"`     // builtin | 外部插件的运行方式（如 isp-sidecar）
	Version  string `json:"version"`  // 外部插件的版本；内置插件为空
	Enabled  bool   `json:"enabled"`  // 配置中启用（网关启动时会创建）
	Running  bool   `json:"running"`  // 当前正在运行
	External bool   `json:"external"` // 来自插件目录的 plugin.json
}

// Counted 表示该实例是否计入健康状态和运行指标：配置中启用的实例，或正在运行的实例。
// 配置中禁用、插件目录中发现但未启动的插件不影响健康状态。
func (s InstanceStatus) Counted() bool { return s.Enabled || s.Running }

// InstanceStatuses 返回所有可以按名称启停的实例：配置中的适配器和连接器（包括禁用的），
// 插件目录中发现的外部插件，以及其他正在运行的实例。StartPlugin/StopPlugin 使用同样的名称。
//
// 以前 Web 插件页面、健康检查和运行指标都使用 GetPlugins，它列出的是所有可用的插件类型：
// 页面上的名称（如 mock）与启停接口使用的实例名（如 mock_temperature_sensors）对不上，
// 没有被配置使用的类型也被统计为"未运行"。
func (m *Manager) InstanceStatuses() []InstanceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []InstanceStatus
	seen := make(map[string]bool)
	isRunning := func(name string) bool {
		_, a := m.adapters[name]
		_, s := m.sinks[name]
		return a || s
	}

	for _, kind := range []PluginType{TypeAdapter, TypeSink} {
		key := "southbound.adapters"
		if kind == TypeSink {
			key = "northbound.sinks"
		}
		list, _ := m.v.Get(key).([]interface{})
		for _, item := range list {
			cfg, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := cfg["name"].(string)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			typ, _ := cfg["type"].(string)
			enabled := true
			if e, ok := cfg["enabled"].(bool); ok {
				enabled = e
			}
			result = append(result, InstanceStatus{Name: name, Type: typ, Kind: string(kind), Mode: "builtin",
				Enabled: enabled, Running: isRunning(name)})
		}
	}

	// 插件目录中发现的外部插件（启停按 plugin.json 中的名称）
	external := m.loader.List()
	sort.Slice(external, func(i, j int) bool { return external[i].Name < external[j].Name })
	for _, meta := range external {
		if meta.Name == "" || seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		typ := meta.Entry
		if strings.HasPrefix(typ, "builtin://") {
			typ = strings.TrimPrefix(typ, "builtin://")
		}
		result = append(result, InstanceStatus{Name: meta.Name, Type: typ, Kind: meta.Type, Mode: meta.Mode,
			Version: meta.Version, Running: isRunning(meta.Name), External: true})
	}

	// 其他正在运行的实例（例如通过 Web 接口按内置类型名启动的实例）
	extra := func(kind PluginType, names []string) {
		sort.Strings(names)
		for _, name := range names {
			if !seen[name] {
				seen[name] = true
				result = append(result, InstanceStatus{Name: name, Type: name, Kind: string(kind), Mode: "builtin", Running: true})
			}
		}
	}
	adapters := make([]string, 0, len(m.adapters))
	for name := range m.adapters {
		adapters = append(adapters, name)
	}
	sinks := make([]string, 0, len(m.sinks))
	for name := range m.sinks {
		sinks = append(sinks, name)
	}
	extra(TypeAdapter, adapters)
	extra(TypeSink, sinks)
	return result
}

// InstanceConfig 返回配置中某个实例的配置（副本）
func (m *Manager) InstanceConfig(name string) (map[string]interface{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, kind := range []PluginType{TypeAdapter, TypeSink} {
		if cfg, ok := m.findPluginConfigMap(name, string(kind)); ok {
			return copyConfigValue(cfg).(map[string]interface{}), true
		}
	}
	return nil, false
}

func copyConfigValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = copyConfigValue(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = copyConfigValue(val)
		}
		return out
	default:
		return v
	}
}
