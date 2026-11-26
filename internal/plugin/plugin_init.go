package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/y001j/iot-gateway/internal/southbound"

	// 导入mock包以确保其init()函数被调用，注册MockAdapter到全局Registry
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
)

// initAdapters 初始化所有适配器
func (m *Manager) initAdapters() error {
	// 从配置中获取适配器配置
	log.Info().Msg("开始初始化适配器...")
	adaptersConfig := m.v.Get("southbound.adapters")
	if adaptersConfig == nil {
		log.Warn().Msg("未找到适配器配置")
		return nil
	}

	// 适配器配置（移除频繁debug日志）

	adaptersList, ok := adaptersConfig.([]interface{})
	if !ok {
		return fmt.Errorf("适配器配置格式错误")
	}

	for _, adapterConfig := range adaptersList {
		adapterMap, ok := adapterConfig.(map[string]interface{})
		if !ok {
			return fmt.Errorf("适配器配置项格式错误")
		}

		name, ok := adapterMap["name"].(string)
		if !ok {
			return fmt.Errorf("适配器名称格式错误")
		}

		adapterType, ok := adapterMap["type"].(string)
		if !ok {
			return fmt.Errorf("适配器类型格式错误")
		}

		// 检查是否启用
		if enabled, ok := adapterMap["enabled"].(bool); ok && !enabled {
			log.Info().Str("name", name).Str("type", adapterType).Msg("适配器未启用，跳过初始化")
			continue
		}

		// 获取适配器实例
		var adapterInterface southbound.Adapter
		found := false

		// 首先尝试从loader中查找适配器
		log.Info().Str("name", name).Str("type", adapterType).Msg("查找适配器")

		// 统一使用标准Registry机制，移除mock类型的硬编码特殊处理
		{
			// 尝试多种可能的键名来查找适配器
			possibleKeys := []string{adapterType, name, adapterType + "-adapter", adapterType + "-sidecar"}

			// 对于modbus类型，添加更多可能的键名
			if adapterType == "modbus" {
				possibleKeys = append(possibleKeys, "modbus", "modbus-sidecar", "modbus-adapter")
			}

			// 首先尝试从全局Registry中创建内置适配器
			if adapter, ok := southbound.Create(adapterType); ok {
				log.Info().Str("type", adapterType).Msg("从全局Registry中创建内置适配器")
				adapterInterface = adapter
				found = true
			} else {
				// 如果Registry中没有，再尝试从loader中查找
				for _, key := range possibleKeys {
					if adapter, ok := m.loader.GetAdapter(key); ok {
						log.Info().Str("key", key).Str("type", adapterType).Msg("从 loader 中找到适配器")
						adapterInterface = adapter
						found = true
						break
					}
				}

				// 如果仍然未找到，尝试从已初始化的适配器中查找
				if !found {
					for _, key := range possibleKeys {
						if adapter, ok := m.adapters[key]; ok {
							log.Info().Str("key", key).Str("type", adapterType).Msg("从已初始化适配器中找到")
							adapterInterface = adapter
							found = true
							break
						}
					}
				}
			}

			// 如果仍然未找到，报错
			if !found {
				// 打印调试信息
				log.Error().Str("type", adapterType).Strs("tried_keys", possibleKeys).Msg("未找到适配器")

				// 打印全局Registry中的可用类型
				availableTypes := make([]string, 0, len(southbound.Registry))
				for typeName := range southbound.Registry {
					availableTypes = append(availableTypes, typeName)
				}
				log.Error().Strs("available_types", availableTypes).Msg("全局Registry中的可用适配器类型")

				log.Error().Str("name", name).Str("type", adapterType).Msg("未找到适配器类型，跳过该适配器")
				continue
			}
		}

		// 成功获取适配器（移除debug日志）

		// 直接使用适配器接口
		adapter := adapterInterface

		// 初始化适配器
		log.Info().Str("name", name).Str("type", adapterType).Msg("初始化适配器")

		configData, err := adapterConfigData(adapterMap, name, adapterType)
		if err != nil {
			log.Error().Err(err).Str("name", name).Msg("序列化适配器配置失败，跳过该适配器")
			continue
		}

		// 单个适配器配置错误不应导致整个网关无法启动
		if err := adapter.Init(configData); err != nil {
			log.Error().Err(err).Str("name", name).Str("type", adapterType).Msg("初始化适配器失败，跳过该适配器")
			continue
		}

		// 保存已初始化的适配器
		m.adapters[name] = adapter
		log.Info().Str("name", name).Msg("适配器初始化成功")
	}

	return nil
}

// adapterConfigData 根据适配器配置项生成传给 Adapter.Init 的 JSON。
// 支持两种格式：旧格式（嵌套 config 字段）和新格式（扁平化，去掉 enabled 字段）。
func adapterConfigData(adapterMap map[string]interface{}, name, adapterType string) ([]byte, error) {
	if configField, hasConfig := adapterMap["config"]; hasConfig {
		// 嵌套的 config 里缺少 name/type 时用实例的值补上（以前要求在两处重复填写，漏写时适配器初始化失败）
		if inner, ok := configField.(map[string]interface{}); ok {
			if _, has := inner["name"]; !has || inner["type"] == nil {
				filled := make(map[string]interface{}, len(inner)+2)
				for k, v := range inner {
					filled[k] = v
				}
				if _, has := filled["name"]; !has {
					filled["name"] = name
				}
				if filled["type"] == nil {
					filled["type"] = adapterType
				}
				return json.Marshal(filled)
			}
		}
		return json.Marshal(configField)
	}
	configMap := make(map[string]interface{}, len(adapterMap))
	for k, v := range adapterMap {
		if k != "enabled" {
			configMap[k] = v
		}
	}
	configMap["name"] = name
	configMap["type"] = adapterType
	return json.Marshal(configMap)
}
