package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// MockAdapterConfig 模拟适配器配置
type MockAdapterConfig struct {
	DeviceID   string      `json:"device_id"`
	IntervalMs int         `json:"interval_ms"`
	Points     []MockPoint `json:"points"`
}

// MockPoint 模拟数据点配置
type MockPoint struct {
	Key      string  `json:"key"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Type     string  `json:"type"`
	Variance float64 `json:"variance"`
}

// MockAdapter 是一个模拟适配器，用于在真实适配器不可用时提供备用功能
type MockAdapter struct {
	name    string
	running bool
	points  chan model.Point
	config  MockAdapterConfig
}

// Init 初始化适配器
func (a *MockAdapter) Init(config json.RawMessage) error {
	log.Info().Str("name", a.name).Msg("初始化 Mock 适配器")
	a.points = make(chan model.Point, 100)

	// 解析配置
	if err := json.Unmarshal(config, &a.config); err != nil {
		log.Error().Err(err).Msg("解析 Mock 适配器配置失败")
		return err
	}

	// 设置默认值
	if a.config.DeviceID == "" {
		a.config.DeviceID = "mock_device"
	}

	if a.config.IntervalMs <= 0 {
		a.config.IntervalMs = 5000 // 默认5秒
	}

	// 如果没有配置数据点，添加一个默认数据点
	if len(a.config.Points) == 0 {
		a.config.Points = []MockPoint{
			{
				Key:      "temperature",
				Min:      20.0,
				Max:      30.0,
				Type:     "float",
				Variance: 0.05,
			},
		}
	}

	log.Info().Str("name", a.name).Str("device_id", a.config.DeviceID).Int("interval_ms", a.config.IntervalMs).Int("points_count", len(a.config.Points)).Msg("模拟适配器配置加载成功")
	return nil
}

// Start 启动适配器
func (a *MockAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	log.Info().Str("name", a.name).Msg("启动 Mock 适配器")
	a.running = true

	// 启动一个协程，定期生成模拟数据
	go func() {
		// 使用配置中的间隔
		interval := time.Duration(a.config.IntervalMs) * time.Millisecond
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Info().Str("device_id", a.config.DeviceID).Int("interval_ms", a.config.IntervalMs).Msg("模拟适配器启动")

		for a.running {
			select {
			case <-ticker.C:
				// 生成每个配置的数据点
				for _, pointConfig := range a.config.Points {
					// 根据配置生成值
					var value interface{}
					var pointType model.DataType

					switch pointConfig.Type {
					case "float":
						// 生成在指定范围内的随机浮点数
						range_size := pointConfig.Max - pointConfig.Min
						base := pointConfig.Min + range_size*rand.Float64()
						// 添加小的变化
						variance := (rand.Float64()*2 - 1) * pointConfig.Variance * range_size
						value = base + variance
						pointType = model.TypeFloat
					case "int":
						// 生成在指定范围内的随机整数
						min := int(pointConfig.Min)
						max := int(pointConfig.Max)
						value = min + rand.Intn(max-min+1)
						pointType = model.TypeInt
					default:
						// 默认使用浮点数
						value = pointConfig.Min + (pointConfig.Max-pointConfig.Min)*rand.Float64()
						pointType = model.TypeFloat
					}

					// 创建数据点
					point := model.Point{
						Key:       pointConfig.Key,
						DeviceID:  a.config.DeviceID,
						Value:     value,
						Timestamp: time.Now(),
						Type:      pointType,
						Quality:   0,
						Tags: map[string]string{
							"source":  "mock_adapter",
							"adapter": a.name,
						},
					}

					// 发送数据点到提供的通道
					select {
					case ch <- point:
						log.Debug().Str("adapter", a.name).Str("device_id", point.DeviceID).Str("key", point.Key).Interface("value", point.Value).Msg("发送模拟数据点")
					case <-ctx.Done():
						log.Info().Str("device_id", a.config.DeviceID).Msg("模拟适配器上下文取消")
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop 停止适配器
func (a *MockAdapter) Stop() error {
	log.Info().Str("name", a.name).Msg("停止 Mock 适配器")
	a.running = false
	return nil
}

// Status 获取适配器状态
func (a *MockAdapter) Status() (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":    a.name,
		"running": a.running,
		"type":    "mock",
	}, nil
}

// GetPoints 获取数据点通道 - 保留此方法用于向后兼容
func (a *MockAdapter) GetPoints() <-chan model.Point {
	return a.points
}

// Close 关闭适配器
func (a *MockAdapter) Close() error {
	log.Info().Str("name", a.name).Msg("关闭 Mock 适配器")
	a.running = false
	close(a.points)
	return nil
}

// Name 返回适配器的唯一名称
func (a *MockAdapter) Name() string {
	return a.name
}

// NewMockAdapter 创建一个新的模拟适配器
func NewMockAdapter(name string) *MockAdapter {
	log.Info().Str("name", name).Msg("创建新的模拟适配器")
	return &MockAdapter{name: name}
}

// initAdapters 初始化所有适配器
func (m *Manager) initAdapters() error {
	// 从配置中获取适配器配置
	log.Debug().Msg("开始初始化适配器...")
	adaptersConfig := m.v.Get("southbound.adapters")
	if adaptersConfig == nil {
		log.Warn().Msg("未找到适配器配置")
		return nil
	}

	log.Debug().Interface("config", adaptersConfig).Msg("适配器配置")

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

		// 对于mock类型，每次都创建新实例，确保配置不会冲突
		if adapterType == "mock" {
			log.Info().Str("name", name).Msg("为mock适配器创建新实例")
			adapterInterface = NewMockAdapter(name)
			found = true
		} else {
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

				return fmt.Errorf("未找到类型为 %s 的适配器", adapterType)
			}
		}

		log.Debug().Str("type", adapterType).Msg("成功获取适配器")

		// 直接使用适配器接口
		adapter := adapterInterface

		// 初始化适配器
		log.Info().Str("name", name).Str("type", adapterType).Msg("初始化适配器")
		configData, err := json.Marshal(adapterMap["config"])
		if err != nil {
			log.Error().Err(err).Str("name", name).Msg("序列化适配器配置失败")
			return fmt.Errorf("序列化适配器配置失败: %w", err)
		}

		log.Debug().Str("name", name).RawJSON("config", configData).Msg("适配器配置")

		if err := adapter.Init(configData); err != nil {
			return fmt.Errorf("初始化适配器 %s 失败: %w", name, err)
		}

		// 保存已初始化的适配器
		m.adapters[name] = adapter
		log.Info().Str("name", name).Msg("适配器初始化成功")
	}

	return nil
}
