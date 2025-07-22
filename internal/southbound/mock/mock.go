package mock

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

func init() {
	// 注册适配器工厂
	southbound.Register("mock", func() southbound.Adapter {
		return NewMockAdapter()
	})
}

// MockAdapter 是一个模拟适配器，用于生成随机数据点
type MockAdapter struct {
	*southbound.BaseAdapter
	deviceID string
	interval time.Duration
	points   []mockPoint
	stopCh   chan struct{}
}

// mockPoint 定义了模拟点位的配置
type mockPoint struct {
	Key      string      `json:"key"`
	Min      float64     `json:"min"`
	Max      float64     `json:"max"`
	Type     string      `json:"type"`
	Variance float64     `json:"variance"`
	Constant interface{} `json:"constant"` // 常量值，如果设置则忽略Min/Max/Variance
	lastVal  float64     // 内部状态，不导出
}

// MockConfig 是模拟适配器的配置
type MockConfig struct {
	DeviceID string      `json:"device_id"`
	Interval int         `json:"interval_ms"` // 采样间隔(ms)
	Points   []mockPoint `json:"points"`
}

// NewMockAdapter 创建新的Mock适配器实例
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		BaseAdapter: southbound.NewBaseAdapter("mock-adapter", "mock"),
		deviceID:    "mock-device", 
		interval:    5000 * time.Millisecond,
		stopCh:      make(chan struct{}),
	}
}

// Init 初始化适配器
func (a *MockAdapter) Init(cfg json.RawMessage) error {
	// 重置状态
	a.points = make([]mockPoint, 0)
	a.SetHealthStatus("healthy", "Initializing mock adapter")

	// 如果配置为空或无效，使用默认配置
	if len(cfg) == 0 {
		log.Warn().Msg("配置为空，使用默认模拟配置")
		// 使用默认的模拟点位配置
		a.points = []mockPoint{
			{
				Key:      "temperature",
				Min:      20.0,
				Max:      30.0,
				Type:     "float",
				Variance: 0.1,
				lastVal:  25.0,
			},
			{
				Key:      "humidity",
				Min:      40.0,
				Max:      80.0,
				Type:     "float",
				Variance: 0.1,
				lastVal:  60.0,
			},
		}

		log.Info().
			Str("device_id", a.deviceID).
			Int("points", len(a.points)).
			Dur("interval", a.interval).
			Msg("模拟适配器使用默认配置初始化完成")
		return nil
	}

	var config MockConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		log.Warn().Err(err).Msg("解析配置失败，使用默认配置")
		// 使用默认配置
		return nil
	}

	if config.DeviceID != "" {
		a.deviceID = config.DeviceID
	}

	if config.Interval > 0 {
		a.interval = time.Duration(config.Interval) * time.Millisecond
		if a.interval < 100*time.Millisecond {
			a.interval = 100 * time.Millisecond // 最小间隔100ms
		}
	}

	if len(config.Points) > 0 {
		a.points = config.Points
		// 初始化每个点位的初始值
		for i := range a.points {
			// 如果没有设置常量，则生成随机初始值
			if a.points[i].Constant == nil {
				a.points[i].lastVal = a.points[i].Min + rand.Float64()*(a.points[i].Max-a.points[i].Min)
			}
		}
	}

	log.Info().
		Str("device_id", a.deviceID).
		Int("points", len(a.points)).
		Dur("interval", a.interval).
		Msg("模拟适配器初始化完成")

	return nil
}

// Start 启动适配器
func (a *MockAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	if a.IsRunning() {
		return nil
	}
	a.SetRunning(true)
	a.SetHealthStatus("healthy", "Mock adapter started successfully")

	// 如果没有配置的点位，不启动数据生成
	if len(a.points) == 0 {
		log.Warn().Str("device_id", a.deviceID).Msg("没有配置的点位，模拟适配器将不生成数据")
		log.Info().Str("device_id", a.deviceID).Msg("模拟适配器启动（无数据生成）")
		return nil
	}

	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 记录数据生成开始时间
				operationStart := time.Now()
				
				// 生成所有点位的数据
				for i := range a.points {
					p := &a.points[i]

					// 检查是否使用常量值
					var pointValue interface{}
					var pointType model.DataType

					if p.Constant != nil {
						// 使用常量值
						pointValue = p.Constant
					} else {
						// 生成随机波动
						variance := p.Variance
						if variance == 0 {
							variance = 0.1 // 默认波动为10%
						}

						// 在上次值的基础上添加随机波动
						delta := (rand.Float64()*2 - 1) * variance * (p.Max - p.Min)
						newVal := p.lastVal + delta

						// 确保值在范围内
						if newVal > p.Max {
							newVal = p.Max
						} else if newVal < p.Min {
							newVal = p.Min
						}
						p.lastVal = newVal
						pointValue = newVal
					}

					// 根据配置的类型转换值
					switch p.Type {
					case "int":
						// 对于整数类型，将浮点值转换为整数
						var intValue int
						switch v := pointValue.(type) {
						case float64:
							intValue = int(v)
						case int:
							intValue = v
						case int64:
							intValue = int(v)
						case float32:
							intValue = int(v)
						default:
							// 默认尝试转换为整数
							if fv, ok := v.(float64); ok {
								intValue = int(fv)
							} else {
								intValue = 0
							}
						}
						pointValue = intValue
						pointType = model.TypeInt

					case "bool":
						// 对于布尔类型，将值转换为布尔值
						var boolValue bool
						switch v := pointValue.(type) {
						case bool:
							boolValue = v
						case int:
							boolValue = v != 0
						case float64:
							boolValue = v != 0
						default:
							boolValue = false
						}
						pointValue = boolValue
						pointType = model.TypeBool

					case "string":
						// 对于字符串类型，确保值是字符串
						if str, ok := pointValue.(string); ok {
							pointValue = str
						} else {
							// 将其他类型转换为字符串
							pointValue = fmt.Sprintf("%v", pointValue)
						}
						pointType = model.TypeString

					default:
						// 默认为浮点类型
						var floatValue float64
						switch v := pointValue.(type) {
						case float64:
							floatValue = v
						case int:
							floatValue = float64(v)
						case float32:
							floatValue = float64(v)
						default:
							// 尝试转换为浮点数
							if iv, ok := v.(int); ok {
								floatValue = float64(iv)
							} else {
								floatValue = 0.0
							}
						}
						pointValue = floatValue
						pointType = model.TypeFloat
					}

					// 打印调试日志
					log.Debug().
						Str("key", p.Key).
						Str("device_id", a.deviceID).
						Str("config_type", p.Type).
						Str("actual_type", string(pointType)).
						Interface("value", pointValue).
						Msg("生成数据点")

					// 打印值的实际Go类型
					switch v := pointValue.(type) {
					case int:
						log.Debug().Str("key", p.Key).Msgf("生成前值 %v 的Go类型: int", v)
					case float64:
						log.Debug().Str("key", p.Key).Msgf("生成前值 %v 的Go类型: float64", v)
					case bool:
						log.Debug().Str("key", p.Key).Msgf("生成前值 %v 的Go类型: bool", v)
					case string:
						log.Debug().Str("key", p.Key).Msgf("生成前值 %v 的Go类型: string", v)
					default:
						log.Debug().Str("key", p.Key).Msgf("生成前值 %v 的Go类型: %T", v, v)
					}

					// 创建数据点
					point := model.NewPoint(p.Key, a.deviceID, pointValue, pointType)
					point.AddTag("source", "mock")

					// 检查创建后的数据点类型
					switch v := point.Value.(type) {
					case int:
						log.Debug().Str("key", p.Key).Msgf("创建后值 %v 的Go类型: int", v)
					case float64:
						log.Debug().Str("key", p.Key).Msgf("创建后值 %v 的Go类型: float64", v)
					case bool:
						log.Debug().Str("key", p.Key).Msgf("创建后值 %v 的Go类型: bool", v)
					case string:
						log.Debug().Str("key", p.Key).Msgf("创建后值 %v 的Go类型: string", v)
					default:
						log.Debug().Str("key", p.Key).Msgf("创建后值 %v 的Go类型: %T", v, v)
					}

					// 使用BaseAdapter的SafeSendDataPoint方法，自动处理统计
					a.SafeSendDataPoint(ch, point, operationStart)
				}
			case <-a.stopCh:
				log.Info().Str("device_id", a.deviceID).Msg("模拟适配器停止")
				return
			case <-ctx.Done():
				log.Info().Str("device_id", a.deviceID).Msg("模拟适配器上下文取消")
				return
			}
		}
	}()

	log.Info().Str("device_id", a.deviceID).Msg("模拟适配器启动")
	return nil
}

// Stop 停止适配器
func (a *MockAdapter) Stop() error {
	if !a.IsRunning() {
		return nil
	}

	close(a.stopCh)
	a.SetRunning(false)
	a.SetHealthStatus("healthy", "Mock adapter stopped")
	
	// 重新创建stopCh为下次使用
	a.stopCh = make(chan struct{})
	return nil
}

// NewAdapter 创建一个新的模拟适配器实例
func NewAdapter() southbound.Adapter {
	return NewMockAdapter()
}
