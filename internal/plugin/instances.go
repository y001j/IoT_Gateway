package plugin

import "sort"

// InstanceStatus 描述一个适配器/连接器实例的运行状态
type InstanceStatus struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // 插件类型，例如 mock、websocket
	Kind    string `json:"kind"` // adapter | sink
	Running bool   `json:"running"`
}

// InstanceStatuses 返回需要运行的适配器和连接器实例及其状态：
// 配置中启用的实例（初始化或启动失败、被停止的实例为未运行），
// 以及不在配置中但正在运行的实例（例如通过 plugin.json 启动的外部插件）。
//
// 健康检查和运行指标应使用它，而不是 GetPlugins：GetPlugins 列出的是所有可用的
// 插件类型，没有被配置使用的类型也会显示为 stopped。
func (m *Manager) InstanceStatuses() []InstanceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []InstanceStatus
	collect := func(kind, configKey string, running map[string]string) {
		seen := make(map[string]bool)
		if list, ok := m.v.Get(configKey).([]interface{}); ok {
			for _, item := range list {
				cfg, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := cfg["name"].(string)
				if name == "" || seen[name] {
					continue
				}
				if enabled, ok := cfg["enabled"].(bool); ok && !enabled {
					continue
				}
				seen[name] = true
				typ, _ := cfg["type"].(string)
				_, isRunning := running[name]
				result = append(result, InstanceStatus{Name: name, Type: typ, Kind: kind, Running: isRunning})
			}
		}
		extra := make([]string, 0)
		for name := range running {
			if !seen[name] {
				extra = append(extra, name)
			}
		}
		sort.Strings(extra)
		for _, name := range extra {
			result = append(result, InstanceStatus{Name: name, Type: running[name], Kind: kind, Running: true})
		}
	}

	adapters := make(map[string]string, len(m.adapters))
	for name := range m.adapters {
		adapters[name] = name
	}
	sinks := make(map[string]string, len(m.sinks))
	for name := range m.sinks {
		sinks[name] = name
	}
	collect(string(TypeAdapter), "southbound.adapters", adapters)
	collect(string(TypeSink), "northbound.sinks", sinks)
	return result
}
