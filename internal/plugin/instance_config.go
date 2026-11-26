package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// SecretMask 配置接口中隐藏敏感字段使用的占位值
const SecretMask = "******"

// IsSecretKey 判断配置项是否为密码、令牌等敏感字段
func IsSecretKey(key string) bool {
	k := strings.ToLower(key)
	for _, word := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "credential", "private_key"} {
		if strings.Contains(k, word) {
			return true
		}
	}
	return false
}

// MaskSecrets 把配置中非空的敏感字段替换为 SecretMask（原地修改）
func MaskSecrets(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if IsSecretKey(k) {
				if str, ok := val.(string); !ok || str != "" {
					t[k] = SecretMask
				}
				continue
			}
			MaskSecrets(val)
		}
	case []interface{}:
		for _, val := range t {
			MaskSecrets(val)
		}
	}
}

// restoreMaskedSecrets 用旧配置中的值替换新配置里仍是 SecretMask 的敏感字段
// （页面上读到的配置已隐藏敏感字段，保存时不应把占位值写回）
func restoreMaskedSecrets(newCfg, oldCfg interface{}) {
	switch n := newCfg.(type) {
	case map[string]interface{}:
		o, _ := oldCfg.(map[string]interface{})
		for k, val := range n {
			if IsSecretKey(k) && val == SecretMask {
				if o != nil {
					if old, ok := o[k]; ok {
						n[k] = old
						continue
					}
				}
				delete(n, k)
				continue
			}
			var oldVal interface{}
			if o != nil {
				oldVal = o[k]
			}
			restoreMaskedSecrets(val, oldVal)
		}
	case []interface{}:
		o, _ := oldCfg.([]interface{})
		for i, val := range n {
			var oldVal interface{}
			if i < len(o) {
				oldVal = o[i]
			}
			restoreMaskedSecrets(val, oldVal)
		}
	}
}

// ErrInstanceNotConfigured 实例不在配置文件中（例如插件目录里的外部插件），无法在页面上修改
var ErrInstanceNotConfigured = errors.New("只能修改 config.yaml 中定义的适配器和连接器")

// prepareInstanceConfig 校验并补全新配置：名称、类型不能修改，敏感字段的占位值换回原值，
// 再用新配置初始化一个临时实例（不启动），配置有误时返回错误
func (m *Manager) prepareInstanceConfig(name string, cfg map[string]interface{}) (kind PluginType, merged, old map[string]interface{}, err error) {
	for _, k := range []PluginType{TypeAdapter, TypeSink} {
		if c, ok := m.findPluginConfigMap(name, string(k)); ok {
			kind, old = k, c
			break
		}
	}
	if old == nil {
		return "", nil, nil, ErrInstanceNotConfigured
	}
	merged = copyConfigValue(cfg).(map[string]interface{})
	typ, _ := old["type"].(string)
	if n, ok := merged["name"]; ok && n != name {
		return "", nil, nil, fmt.Errorf("不能修改实例名称（%v）", n)
	}
	if t, ok := merged["type"]; ok && t != typ {
		return "", nil, nil, fmt.Errorf("不能修改插件类型（%v → %v），请新建实例", typ, t)
	}
	merged["name"], merged["type"] = name, typ
	restoreMaskedSecrets(merged, old)

	if kind == TypeAdapter {
		a, ok := southbound.Create(typ)
		if !ok {
			return "", nil, nil, fmt.Errorf("未知的适配器类型 %s", typ)
		}
		data, err := adapterConfigData(merged, name, typ)
		if err != nil {
			return "", nil, nil, err
		}
		if err := a.Init(data); err != nil {
			return "", nil, nil, fmt.Errorf("配置校验失败: %w", err)
		}
	} else {
		// 连接器的 Init 可能连接目标系统（redis 会 ping、jetstream 会建立连接），这里只做结构校验；
		// 实例正在运行时用新配置重启，失败则回滚（UpdateInstanceConfig）
		if _, ok := northbound.Registry[typ]; !ok {
			return "", nil, nil, fmt.Errorf("未知的连接器类型 %s", typ)
		}
		if _, err := parseSinkRouting(merged); err != nil {
			return "", nil, nil, fmt.Errorf("配置校验失败: %w", err)
		}
		if _, err := sinkConfigData(merged, name, typ); err != nil {
			return "", nil, nil, err
		}
	}
	return kind, merged, old, nil
}

// ValidateInstanceConfig 只校验，不保存
func (m *Manager) ValidateInstanceConfig(name string, cfg map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _, _, err := m.prepareInstanceConfig(name, cfg)
	return err
}

// UpdateInstanceConfig 校验后把实例配置写回配置文件（只替换该实例这一项，文件其余部分和注释保留），
// 更新内存中的配置；实例正在运行时用新配置重启，重启失败则回滚到旧配置。返回是否重启了实例。
func (m *Manager) UpdateInstanceConfig(name string, cfg map[string]interface{}) (restarted bool, err error) {
	m.mu.Lock()
	kind, merged, old, err := m.prepareInstanceConfig(name, cfg)
	if err != nil {
		m.mu.Unlock()
		return false, err
	}
	key := "southbound.adapters"
	if kind == TypeSink {
		key = "northbound.sinks"
	}
	path := m.v.ConfigFileUsed()
	if path == "" {
		m.mu.Unlock()
		return false, errors.New("网关没有使用配置文件，无法保存")
	}
	if err := m.applyInstanceConfigLocked(path, key, name, merged); err != nil {
		m.mu.Unlock()
		return false, err
	}
	_, runningAdapter := m.adapters[name]
	_, runningSink := m.sinks[name]
	m.mu.Unlock()

	log.Info().Str("instance", name).Str("file", path).Msg("插件实例配置已保存")
	if !runningAdapter && !runningSink {
		return false, nil
	}
	if err := m.RestartPlugin(name); err != nil {
		// 新配置无法启动：恢复旧配置并用它重新启动，避免实例停摆、配置文件留下坏配置
		m.mu.Lock()
		rbErr := m.applyInstanceConfigLocked(path, key, name, old)
		m.mu.Unlock()
		if rbErr != nil {
			return false, fmt.Errorf("新配置启动失败（%v），恢复旧配置也失败: %w", err, rbErr)
		}
		if rsErr := m.StartPlugin(name); rsErr != nil {
			log.Error().Err(rsErr).Str("instance", name).Msg("恢复旧配置后启动失败")
		}
		return false, fmt.Errorf("新配置启动失败，已恢复原配置: %w", err)
	}
	return true, nil
}

// applyInstanceConfigLocked 写入配置文件并更新内存中的配置（持有 m.mu）
func (m *Manager) applyInstanceConfigLocked(path, key, name string, cfg map[string]interface{}) error {
	if err := writeInstanceToFile(path, key, name, cfg); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	list, _ := m.v.Get(key).([]interface{})
	updated := make([]interface{}, len(list))
	for i, item := range list {
		updated[i] = item
		if c, ok := item.(map[string]interface{}); ok && c["name"] == name {
			updated[i] = cfg
		}
	}
	m.v.Set(key, updated)
	m.invalidatePluginCache()
	return nil
}

// writeInstanceToFile 在 YAML 文件中找到 key（如 northbound.sinks）列表里 name 匹配的一项并替换，
// 其余内容（包括注释）保持不变；先写临时文件再改名，避免写到一半的文件
func writeInstanceToFile(path, key, name string, cfg map[string]interface{}) error {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return errors.New("配置文件为空")
	}
	node := doc.Content[0]
	for _, part := range strings.Split(key, ".") {
		node = mappingValue(node, part)
		if node == nil {
			return fmt.Errorf("配置文件中没有 %s", key)
		}
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s 不是列表", key)
	}
	var item *yaml.Node
	for _, it := range node.Content {
		if v := mappingValue(it, "name"); v != nil && v.Value == name {
			item = it
			break
		}
	}
	if item == nil {
		return fmt.Errorf("配置文件的 %s 中没有 %s", key, name)
	}
	var replacement yaml.Node
	if err := replacement.Encode(cfg); err != nil {
		return err
	}
	replacement.HeadComment, replacement.LineComment, replacement.FootComment = item.HeadComment, item.LineComment, item.FootComment
	*item = replacement

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	_ = enc.Close()

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // 改名成功后文件已不存在，Remove 返回错误可忽略
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// mappingValue 返回 YAML 映射节点中键对应的值节点
func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
