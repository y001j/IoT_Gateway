package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
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
	tags     map[string]interface{} // 设备标签
	stopCh   chan struct{}
	parser   *config.ConfigParser[config.MockConfig]
}

// mockPoint 定义了模拟点位的配置
type mockPoint struct {
	Key      string        `json:"key"`
	DeviceID string        `json:"device_id"` // 每个数据点的device_id
	Min      float64       `json:"min"`
	Max      float64       `json:"max"`
	Type     string        `json:"type"`
	Variance float64       `json:"variance"`
	Constant interface{}   `json:"constant"` // 常量值，如果设置则忽略Min/Max/Variance
	Values   []interface{} `json:"values"`   // 预定义值列表，随机选择
	lastVal  float64       // 内部状态，不导出

	// 复合数据配置
	DataType       string                     `json:"data_type,omitempty"`
	LocationConfig *config.MockLocationConfig `json:"location_config,omitempty"`
	Vector3DConfig *config.MockVector3DConfig `json:"vector3d_config,omitempty"`
	ColorConfig    *config.MockColorConfig    `json:"color_config,omitempty"`

	// 通用复合数据类型配置
	VectorConfig     *config.MockVectorConfig     `json:"vector_config,omitempty"`
	ArrayConfig      *config.MockArrayConfig      `json:"array_config,omitempty"`
	MatrixConfig     *config.MockMatrixConfig     `json:"matrix_config,omitempty"`
	TimeSeriesConfig *config.MockTimeSeriesConfig `json:"timeseries_config,omitempty"`

	// 复合数据内部状态
	locationState   *locationState
	vector3dState   *vector3dState
	colorState      *colorState
	vectorState     *vectorState
	arrayState      *arrayState
	matrixState     *matrixState
	timeseriesState *timeseriesState
}

// locationState GPS位置模拟内部状态
type locationState struct {
	currentLat float64
	currentLng float64
	direction  float64 // 移动方向 (弧度)
	speed      float64 // 当前速度
	lastUpdate time.Time
}

// vector3dState 三轴向量模拟内部状态
type vector3dState struct {
	lastX, lastY, lastZ float64
	time                float64 // 用于振荡计算
}

// colorState 颜色模拟内部状态
type colorState struct {
	currentHue float64 // 当前色相 (0-360)
	colorIndex int     // 固定颜色索引
}

// vectorState 通用向量模拟内部状态
type vectorState struct {
	lastValues []float64 // 上次的值
	time       float64   // 时间参数（用于模式生成）
}

// arrayState 数组模拟内部状态
type arrayState struct {
	lastValues []interface{} // 上次的数组值
	generation int           // 生成次数
}

// matrixState 矩阵模拟内部状态
type matrixState struct {
	lastValues [][]float64 // 上次的矩阵值
	generation int         // 生成次数
}

// timeseriesState 时间序列模拟内部状态
type timeseriesState struct {
	values     []float64   // 当前数据序列
	timestamps []time.Time // 对应的时间戳
	lastValue  float64     // 上一个值（用于趋势生成）
	startTime  time.Time   // 开始时间
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
	log.Info().
		Str("method", "MockAdapter.Init").
		Int("config_size", len(cfg)).
		Str("device_id", a.deviceID).
		Msg("🔍 MockAdapter.Init() 开始执行 - 调试入口点")

	log.Debug().
		Int("config_size", len(cfg)).
		Str("device_id", a.deviceID).
		Msg("MockAdapter.Init() 开始执行")

	// 重置状态
	a.points = make([]mockPoint, 0)
	a.SetHealthStatus("healthy", "Initializing mock adapter")

	// 统一的配置初始化逻辑
	return a.initWithUnifiedConfig(cfg)
}

// initWithUnifiedConfig 统一的配置初始化方法，仅支持新格式
func (a *MockAdapter) initWithUnifiedConfig(cfg json.RawMessage) error {
	// 如果配置为空，使用默认配置
	if len(cfg) == 0 {
		log.Warn().Msg("配置为空，使用默认模拟配置")
		return a.setDefaultConfiguration()
	}

	// 解析新配置格式
	a.parser = config.NewParserWithDefaults(config.GetDefaultMockConfig())
	mockConfig, err := a.parser.Parse(cfg)
	if err != nil {
		log.Error().
			Err(err).
			RawJSON("raw_config", cfg).
			Msg("解析Mock配置失败")
		return fmt.Errorf("解析Mock配置失败: %w", err)
	}

	log.Info().
		Interface("new_config", mockConfig).
		Str("config_name", mockConfig.Name).
		Dur("config_interval", mockConfig.Interval.Duration()).
		Int("config_datapoints_count", len(mockConfig.DataPoints)).
		Int("config_tags_count", len(mockConfig.Tags)).
		Msg("✅ 成功使用新配置格式解析")

	return a.initFromNewConfig(mockConfig)
}

// setDefaultConfiguration 设置默认配置
func (a *MockAdapter) setDefaultConfiguration() error {
	a.points = []mockPoint{
		{
			Key:      "temperature",
			DeviceID: "mock_device", // 为默认配置设置device_id
			Min:      20.0,
			Max:      30.0,
			Type:     "float",
			Variance: 0.1,
			lastVal:  25.0,
		},
		{
			Key:      "humidity",
			DeviceID: "mock_device", // 为默认配置设置device_id
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

// initFromNewConfig 从新配置格式初始化
func (a *MockAdapter) initFromNewConfig(config *config.MockConfig) error {
	log.Debug().
		Str("config_name", config.Name).
		Dur("config_interval", config.Interval.Duration()).
		Int("config_datapoints_count", len(config.DataPoints)).
		Int("config_tags_count", len(config.Tags)).
		Msg("initFromNewConfig() 开始执行")

	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "mock")
	a.interval = config.Interval.Duration()

	// 从新配置格式转换到内部格式
	a.points = make([]mockPoint, len(config.DataPoints))
	for i, dp := range config.DataPoints {
		point := mockPoint{
			Key:      dp.Key,
			DeviceID: dp.DeviceID, // 每个数据点保留自己的device_id
			Min:      dp.MinValue,
			Max:      dp.MaxValue,
			Type:     "float", // 默认为浮点类型
			Variance: 0.1,     // 默认波动
			lastVal:  dp.MinValue + simFloat64()*(dp.MaxValue-dp.MinValue),
		}

		// 处理复合数据类型配置
		if dp.DataType != "" {
			point.DataType = dp.DataType
			point.Type = dp.DataType

			switch dp.DataType {
			case "location":
				if dp.LocationConfig != nil {
					point.LocationConfig = dp.LocationConfig
					point.locationState = &locationState{
						currentLat: dp.LocationConfig.StartLatitude,
						currentLng: dp.LocationConfig.StartLongitude,
						direction:  simFloat64() * 2 * math.Pi, // 随机初始方向
						speed:      dp.LocationConfig.SpeedMin + simFloat64()*(dp.LocationConfig.SpeedMax-dp.LocationConfig.SpeedMin),
						lastUpdate: time.Now(),
					}
				}
			case "vector3d":
				if dp.Vector3DConfig != nil {
					point.Vector3DConfig = dp.Vector3DConfig
					point.vector3dState = &vector3dState{
						lastX: dp.Vector3DConfig.XMin + simFloat64()*(dp.Vector3DConfig.XMax-dp.Vector3DConfig.XMin),
						lastY: dp.Vector3DConfig.YMin + simFloat64()*(dp.Vector3DConfig.YMax-dp.Vector3DConfig.YMin),
						lastZ: dp.Vector3DConfig.ZMin + simFloat64()*(dp.Vector3DConfig.ZMax-dp.Vector3DConfig.ZMin),
						time:  0,
					}
				}
			case "color":
				if dp.ColorConfig != nil {
					point.ColorConfig = dp.ColorConfig
					point.colorState = &colorState{
						currentHue: simFloat64() * 360,
						colorIndex: 0,
					}
				}
			// 通用复合数据类型
			case "vector":
				if dp.VectorConfig != nil {
					point.VectorConfig = dp.VectorConfig
					point.vectorState = a.initVectorState(dp.VectorConfig)
				}
			case "array":
				if dp.ArrayConfig != nil {
					point.ArrayConfig = dp.ArrayConfig
					point.arrayState = a.initArrayState(dp.ArrayConfig)
				}
			case "matrix":
				if dp.MatrixConfig != nil {
					point.MatrixConfig = dp.MatrixConfig
					point.matrixState = a.initMatrixState(dp.MatrixConfig)
				}
			case "timeseries":
				if dp.TimeSeriesConfig != nil {
					point.TimeSeriesConfig = dp.TimeSeriesConfig
					point.timeseriesState = a.initTimeSeriesState(dp.TimeSeriesConfig)
				}
			}
		}

		a.points[i] = point

		// 使用第一个设备ID作为适配器默认值（仅用于日志）
		if i == 0 {
			a.deviceID = dp.DeviceID
		}
	}

	// 如果没有配置点位，使用默认配置
	if len(a.points) == 0 {
		return a.setDefaultConfiguration()
	}

	// 设置设备标签（新格式：map[string]string）
	if len(config.Tags) > 0 {
		a.tags = make(map[string]interface{})
		for k, v := range config.Tags {
			a.tags[k] = v
		}
		log.Debug().
			Interface("adapter_tags", a.tags).
			Int("tags_count", len(a.tags)).
			Msg("设备标签设置完成 - 新配置格式")
	}

	log.Info().
		Str("device_id", a.deviceID).
		Int("points", len(a.points)).
		Int("tags", len(a.tags)).
		Dur("interval", a.interval).
		Msg("模拟适配器配置加载成功 - 新格式")

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

	// 捕获本次启动的停止通道：Stop 会替换 a.stopCh，
	// 协程如果读字段可能错过关闭信号而在停止后继续产生数据
	stopCh := a.stopCh
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

					// 检查是否为复合数据类型
					var pointValue interface{}
					var pointType model.DataType

					if p.DataType != "" {
						// 生成复合数据
						pointValue, pointType = a.generateCompositeData(p)
					} else if p.Constant != nil {
						// 使用常量值
						pointValue = p.Constant
					} else if len(p.Values) > 0 {
						// 从预定义值列表中随机选择
						idx := simIntn(len(p.Values))
						pointValue = p.Values[idx]
					} else {
						// 生成随机波动
						variance := p.Variance
						if variance == 0 {
							variance = 0.1 // 默认波动为10%
						}

						// 在上次值的基础上添加随机波动
						delta := (simFloat64()*2 - 1) * variance * (p.Max - p.Min)
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

					// 创建数据点 - 使用数据点自己的device_id而不是适配器级别的device_id
					var pointDeviceID string
					if p.DeviceID != "" {
						pointDeviceID = p.DeviceID
					} else {
						pointDeviceID = a.deviceID // 回退到适配器默认值
					}

					// 打印调试日志
					log.Debug().
						Str("key", p.Key).
						Str("point_device_id", pointDeviceID).
						Str("adapter_device_id", a.deviceID).
						Str("config_type", p.Type).
						Str("actual_type", string(pointType)).
						Interface("value", pointValue).
						Msg("🔍 生成数据点 - 修复后使用正确的device_id")

					// 创建数据点
					var point model.Point
					if p.DataType != "" && pointType != "" {
						// 复合数据点
						if compositeData, ok := pointValue.(model.CompositeData); ok {
							point = model.NewCompositePoint(p.Key, pointDeviceID, compositeData)
						} else {
							point = model.NewPoint(p.Key, pointDeviceID, pointValue, pointType)
						}
					} else {
						// 普通数据点
						point = model.NewPoint(p.Key, pointDeviceID, pointValue, pointType)
					}
					point.AddTag("source", "mock")

					// 添加设备标签
					if a.tags != nil {
						log.Debug().
							Str("device_id", pointDeviceID).
							Str("adapter_device_id", a.deviceID).
							Str("key", p.Key).
							Interface("available_tags", a.tags).
							Int("tags_count", len(a.tags)).
							Msg("准备添加设备标签到数据点")

						for tagKey, tagValue := range a.tags {
							tagValueStr := fmt.Sprintf("%v", tagValue)
							point.AddTag(tagKey, tagValueStr)

							log.Debug().
								Str("device_id", pointDeviceID).
								Str("adapter_device_id", a.deviceID).
								Str("key", p.Key).
								Str("tag_key", tagKey).
								Str("tag_value", tagValueStr).
								Msg("已添加标签到数据点")
						}

						// 验证标签是否正确添加
						actualTags := point.GetTagsCopy()
						log.Debug().
							Str("device_id", pointDeviceID).
							Str("adapter_device_id", a.deviceID).
							Str("key", p.Key).
							Interface("point_tags", actualTags).
							Int("point_tags_count", len(actualTags)).
							Msg("数据点最终标签状态")
					} else {
						log.Debug().
							Str("device_id", pointDeviceID).
							Str("adapter_device_id", a.deviceID).
							Str("key", p.Key).
							Msg("适配器没有配置标签")
					}

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
			case <-stopCh:
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

// generateCompositeData 生成复合数据
func (a *MockAdapter) generateCompositeData(p *mockPoint) (interface{}, model.DataType) {
	switch p.DataType {
	case "location":
		return a.generateLocationData(p)
	case "vector3d":
		return a.generateVector3DData(p)
	case "color":
		return a.generateColorData(p)
	// 通用复合数据类型
	case "vector":
		return a.generateVectorData(p)
	case "array":
		return a.generateArrayData(p)
	case "matrix":
		return a.generateMatrixData(p)
	case "timeseries":
		return a.generateTimeSeriesData(p)
	default:
		log.Warn().Str("data_type", p.DataType).Msg("未支持的复合数据类型")
		return nil, model.TypeString
	}
}

// generateLocationData 生成GPS位置数据
func (a *MockAdapter) generateLocationData(p *mockPoint) (*model.LocationData, model.DataType) {
	if p.LocationConfig == nil || p.locationState == nil {
		log.Error().Msg("位置配置或状态为空")
		return nil, model.TypeLocation
	}

	config := p.LocationConfig
	state := p.locationState

	// 如果启用了移动模拟
	if config.SimulateMovement {
		now := time.Now()
		timeDelta := now.Sub(state.lastUpdate).Seconds()

		// 更新位置
		switch config.MovementPattern {
		case "random_walk":
			// 随机游走
			state.direction += (simFloat64() - 0.5) * 0.2 // 方向略微变化
			speedKmh := state.speed
			speedMs := speedKmh / 3.6 // 转换为米/秒

			// 计算移动距离 (米)
			distance := speedMs * timeDelta

			// 地球半径 (米)
			const earthRadius = 6371000.0

			// 计算新的经纬度
			dLat := distance * math.Cos(state.direction) / earthRadius * (180.0 / math.Pi)
			dLng := distance * math.Sin(state.direction) / (earthRadius * math.Cos(state.currentLat*math.Pi/180.0)) * (180.0 / math.Pi)

			state.currentLat += dLat
			state.currentLng += dLng

			// 确保在指定范围内
			if config.LatitudeRange > 0 {
				if math.Abs(state.currentLat-config.StartLatitude) > config.LatitudeRange {
					state.currentLat = config.StartLatitude + (config.LatitudeRange * (simFloat64()*2 - 1))
				}
			}
			if config.LongitudeRange > 0 {
				if math.Abs(state.currentLng-config.StartLongitude) > config.LongitudeRange {
					state.currentLng = config.StartLongitude + (config.LongitudeRange * (simFloat64()*2 - 1))
				}
			}

		case "circular":
			// 圆形移动
			radius := 0.001                                       // 大约100米半径
			angle := time.Since(state.lastUpdate).Seconds() * 0.1 // 慢速旋转
			state.currentLat = config.StartLatitude + radius*math.Cos(angle)
			state.currentLng = config.StartLongitude + radius*math.Sin(angle)

		case "linear":
			// 线性移动
			speedKmh := state.speed
			speedMs := speedKmh / 3.6
			distance := speedMs * timeDelta
			const earthRadius = 6371000.0

			dLat := distance / earthRadius * (180.0 / math.Pi)
			state.currentLat += dLat
		}

		state.lastUpdate = now

		// 随机调整速度
		if simFloat64() < 0.1 { // 10%概率调整速度
			state.speed = config.SpeedMin + simFloat64()*(config.SpeedMax-config.SpeedMin)
		}
	}

	// 创建位置数据
	locationData := &model.LocationData{
		Latitude:  state.currentLat,
		Longitude: state.currentLng,
	}

	// 添加可选字段
	if config.AltitudeMin < config.AltitudeMax {
		locationData.Altitude = config.AltitudeMin + simFloat64()*(config.AltitudeMax-config.AltitudeMin)
	}

	if state.speed > 0 {
		locationData.Speed = state.speed
	}

	// 添加GPS精度 (3-10米)
	locationData.Accuracy = 3.0 + simFloat64()*7.0

	// 添加方向角
	locationData.Heading = state.direction * (180.0 / math.Pi)
	if locationData.Heading < 0 {
		locationData.Heading += 360
	}

	return locationData, model.TypeLocation
}

// generateVector3DData 生成三轴向量数据
func (a *MockAdapter) generateVector3DData(p *mockPoint) (*model.Vector3D, model.DataType) {
	if p.Vector3DConfig == nil || p.vector3dState == nil {
		log.Error().Msg("向量配置或状态为空")
		return nil, model.TypeVector3D
	}

	config := p.Vector3DConfig
	state := p.vector3dState

	var x, y, z float64

	if config.Oscillation && config.Frequency > 0 {
		// 振荡模式
		state.time += 0.1 // 时间步进

		x = (config.XMax-config.XMin)/2*math.Sin(2*math.Pi*config.Frequency*state.time) + (config.XMax+config.XMin)/2
		y = (config.YMax-config.YMin)/2*math.Sin(2*math.Pi*config.Frequency*state.time+2*math.Pi/3) + (config.YMax+config.YMin)/2
		z = (config.ZMax-config.ZMin)/2*math.Sin(2*math.Pi*config.Frequency*state.time+4*math.Pi/3) + (config.ZMax+config.ZMin)/2
	} else {
		// 随机波动模式
		variance := 0.1

		// 考虑轴间相关性
		if config.Correlation > 0 {
			// 生成相关的随机变化
			baseChange := (simFloat64() - 0.5) * variance

			x = state.lastX + baseChange*(config.XMax-config.XMin)
			y = state.lastY + (baseChange*config.Correlation+(simFloat64()-0.5)*variance*(1-config.Correlation))*(config.YMax-config.YMin)
			z = state.lastZ + (baseChange*config.Correlation+(simFloat64()-0.5)*variance*(1-config.Correlation))*(config.ZMax-config.ZMin)
		} else {
			// 独立的随机变化
			x = state.lastX + (simFloat64()-0.5)*variance*(config.XMax-config.XMin)
			y = state.lastY + (simFloat64()-0.5)*variance*(config.YMax-config.YMin)
			z = state.lastZ + (simFloat64()-0.5)*variance*(config.ZMax-config.ZMin)
		}

		// 确保在范围内
		if x > config.XMax {
			x = config.XMax
		}
		if x < config.XMin {
			x = config.XMin
		}
		if y > config.YMax {
			y = config.YMax
		}
		if y < config.YMin {
			y = config.YMin
		}
		if z > config.ZMax {
			z = config.ZMax
		}
		if z < config.ZMin {
			z = config.ZMin
		}

		// 更新状态
		state.lastX = x
		state.lastY = y
		state.lastZ = z
	}

	return &model.Vector3D{
		X: x,
		Y: y,
		Z: z,
	}, model.TypeVector3D
}

// generateColorData 生成颜色数据
func (a *MockAdapter) generateColorData(p *mockPoint) (*model.ColorData, model.DataType) {
	if p.ColorConfig == nil || p.colorState == nil {
		log.Error().Msg("颜色配置或状态为空")
		return nil, model.TypeColor
	}

	config := p.ColorConfig
	state := p.colorState

	var r, g, b uint8 = 255, 255, 255 // 默认白色

	switch config.ColorMode {
	case "random":
		// 完全随机颜色
		r = simByte()
		g = simByte()
		b = simByte()

	case "rainbow":
		// 彩虹色相循环
		state.currentHue += config.HueChangeSpeed
		if state.currentHue >= 360 {
			state.currentHue -= 360
		}

		// HSV转RGB
		h := state.currentHue / 60.0
		c := 1.0
		x := c * (1.0 - math.Abs(math.Mod(h, 2.0)-1.0))

		var r1, g1, b1 float64
		if h >= 0 && h < 1 {
			r1, g1, b1 = c, x, 0
		} else if h >= 1 && h < 2 {
			r1, g1, b1 = x, c, 0
		} else if h >= 2 && h < 3 {
			r1, g1, b1 = 0, c, x
		} else if h >= 3 && h < 4 {
			r1, g1, b1 = 0, x, c
		} else if h >= 4 && h < 5 {
			r1, g1, b1 = x, 0, c
		} else {
			r1, g1, b1 = c, 0, x
		}

		r = uint8(r1 * 255)
		g = uint8(g1 * 255)
		b = uint8(b1 * 255)

	case "fixed":
		// 固定颜色列表
		if len(config.FixedColors) > 0 {
			colorHex := config.FixedColors[state.colorIndex]
			if len(colorHex) == 7 && colorHex[0] == '#' {
				// 解析hex颜色
				if rv, err := strconv.ParseUint(colorHex[1:3], 16, 8); err == nil {
					r = uint8(rv)
				}
				if gv, err := strconv.ParseUint(colorHex[3:5], 16, 8); err == nil {
					g = uint8(gv)
				}
				if bv, err := strconv.ParseUint(colorHex[5:7], 16, 8); err == nil {
					b = uint8(bv)
				}
			}

			// 循环颜色索引
			if simFloat64() < 0.1 { // 10%概率切换颜色
				state.colorIndex = (state.colorIndex + 1) % len(config.FixedColors)
			}
		}

	default:
		// 默认随机模式
		r = simByte()
		g = simByte()
		b = simByte()
	}

	return &model.ColorData{
		R: r,
		G: g,
		B: b,
		A: 255, // 完全不透明
	}, model.TypeColor
}

// 通用复合数据类型初始化函数

// initVectorState 初始化通用向量状态
func (a *MockAdapter) initVectorState(config *config.MockVectorConfig) *vectorState {
	if config == nil {
		return nil
	}

	values := make([]float64, config.Dimension)
	for i := 0; i < config.Dimension; i++ {
		var minVal, maxVal float64
		if len(config.MinValues) > i {
			minVal = config.MinValues[i]
		} else {
			minVal = config.GlobalMin
		}
		if len(config.MaxValues) > i {
			maxVal = config.MaxValues[i]
		} else {
			maxVal = config.GlobalMax
		}

		// 根据分布类型生成初始值
		switch config.Distribution {
		case "normal":
			// 正态分布（简化版本）
			values[i] = (minVal+maxVal)/2.0 + (simFloat64()-0.5)*(maxVal-minVal)*0.3
		case "exponential":
			// 指数分布（简化版本）
			values[i] = minVal + (maxVal-minVal)*(-math.Log(1.0-simFloat64()))
		default:
			// 均匀分布
			values[i] = minVal + simFloat64()*(maxVal-minVal)
		}
	}

	return &vectorState{
		lastValues: values,
		time:       0,
	}
}

// initArrayState 初始化数组状态
func (a *MockAdapter) initArrayState(config *config.MockArrayConfig) *arrayState {
	if config == nil {
		return nil
	}

	values := make([]interface{}, config.Size)
	for i := 0; i < config.Size; i++ {
		values[i] = a.generateArrayElement(config)
	}

	return &arrayState{
		lastValues: values,
		generation: 0,
	}
}

// initMatrixState 初始化矩阵状态
func (a *MockAdapter) initMatrixState(config *config.MockMatrixConfig) *matrixState {
	if config == nil {
		return nil
	}

	values := make([][]float64, config.Rows)
	for i := 0; i < config.Rows; i++ {
		values[i] = make([]float64, config.Cols)
		for j := 0; j < config.Cols; j++ {
			values[i][j] = a.generateMatrixElement(config, i, j)
		}
	}

	return &matrixState{
		lastValues: values,
		generation: 0,
	}
}

// initTimeSeriesState 初始化时间序列状态
func (a *MockAdapter) initTimeSeriesState(config *config.MockTimeSeriesConfig) *timeseriesState {
	if config == nil {
		return nil
	}

	// 解析时间间隔
	interval, err := time.ParseDuration(config.Interval)
	if err != nil {
		log.Error().Err(err).Str("interval", config.Interval).Msg("解析时间间隔失败，使用默认值1s")
		interval = time.Second
	}

	// 解析开始时间
	var startTime time.Time
	if config.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, config.StartTime); err == nil {
			startTime = t
		} else {
			startTime = time.Now().Add(-time.Duration(config.Length) * interval)
		}
	} else {
		startTime = time.Now().Add(-time.Duration(config.Length) * interval)
	}

	// 生成初始时间序列
	values := make([]float64, 0, config.Length)
	timestamps := make([]time.Time, 0, config.Length)

	baseValue := config.BaseValue
	if baseValue == 0 {
		baseValue = 50.0 // 默认基准值
	}

	for i := 0; i < config.Length; i++ {
		timestamp := startTime.Add(time.Duration(i) * interval)
		value := baseValue

		// 添加趋势
		value += config.Trend * float64(i)

		// 添加季节性（如果配置了）
		if config.Seasonality != nil {
			periodDuration, err := time.ParseDuration(config.Seasonality.Period)
			if err == nil {
				seasonalPhase := float64(timestamp.UnixNano()) / float64(periodDuration.Nanoseconds()) * 2 * math.Pi
				value += config.Seasonality.Amplitude * math.Sin(seasonalPhase+config.Seasonality.Phase)
			}
		}

		// 添加噪声
		if config.Noise > 0 {
			value += (simFloat64() - 0.5) * 2 * config.Noise
		}

		// 添加异常值（如果配置了）
		if config.Anomalies != nil && simFloat64() < config.Anomalies.Probability {
			value += (simFloat64() - 0.5) * config.Anomalies.Magnitude * value
		}

		values = append(values, value)
		timestamps = append(timestamps, timestamp)
	}

	return &timeseriesState{
		values:     values,
		timestamps: timestamps,
		lastValue:  values[len(values)-1],
		startTime:  startTime,
	}
}

// 通用复合数据类型生成函数

// generateVectorData 生成通用向量数据
func (a *MockAdapter) generateVectorData(p *mockPoint) (*model.VectorData, model.DataType) {
	if p.VectorConfig == nil || p.vectorState == nil {
		log.Error().Msg("向量配置或状态为空")
		return nil, model.TypeVector
	}

	config := p.VectorConfig
	state := p.vectorState

	newValues := make([]float64, len(state.lastValues))
	copy(newValues, state.lastValues)

	switch config.ChangePattern {
	case "walk":
		// 随机游走
		for i := range newValues {
			var minVal, maxVal float64
			if len(config.MinValues) > i {
				minVal = config.MinValues[i]
			} else {
				minVal = config.GlobalMin
			}
			if len(config.MaxValues) > i {
				maxVal = config.MaxValues[i]
			} else {
				maxVal = config.GlobalMax
			}

			// 随机变化，有相关性
			change := (simFloat64() - 0.5) * 0.1 * (maxVal - minVal)
			if config.Correlation > 0 && i > 0 {
				// 与前一个维度有相关性
				prevChange := newValues[i-1] - state.lastValues[i-1]
				change += prevChange * config.Correlation
			}

			newValues[i] += change
			// 边界检查
			if newValues[i] < minVal {
				newValues[i] = minVal
			}
			if newValues[i] > maxVal {
				newValues[i] = maxVal
			}
		}

	case "oscillate":
		// 振荡模式
		state.time += 0.1
		for i := range newValues {
			amplitude := (config.MaxValues[i] - config.MinValues[i]) / 2.0
			center := (config.MaxValues[i] + config.MinValues[i]) / 2.0
			frequency := 1.0 + float64(i)*0.1 // 不同维度不同频率
			newValues[i] = center + amplitude*math.Sin(state.time*frequency)
		}

	default:
		// 随机模式
		for i := range newValues {
			var minVal, maxVal float64
			if len(config.MinValues) > i {
				minVal = config.MinValues[i]
			} else {
				minVal = config.GlobalMin
			}
			if len(config.MaxValues) > i {
				maxVal = config.MaxValues[i]
			} else {
				maxVal = config.GlobalMax
			}

			switch config.Distribution {
			case "normal":
				// 正态分布
				center := (minVal + maxVal) / 2.0
				newValues[i] = center + (simFloat64()-0.5)*(maxVal-minVal)*0.3
			case "exponential":
				// 指数分布
				newValues[i] = minVal + (maxVal-minVal)*(-math.Log(1.0-simFloat64()))
			default:
				// 均匀分布
				newValues[i] = minVal + simFloat64()*(maxVal-minVal)
			}
		}
	}

	// 更新状态
	state.lastValues = newValues

	return &model.VectorData{
		Values:    newValues,
		Dimension: len(newValues),
		Labels:    config.Labels,
		Unit:      config.Unit,
	}, model.TypeVector
}

// generateArrayData 生成数组数据
func (a *MockAdapter) generateArrayData(p *mockPoint) (*model.ArrayData, model.DataType) {
	if p.ArrayConfig == nil || p.arrayState == nil {
		log.Error().Msg("数组配置或状态为空")
		return nil, model.TypeArray
	}

	config := p.ArrayConfig
	state := p.arrayState

	// 决定要更改的元素数量
	changeCount := config.ChangeElements
	if changeCount <= 0 {
		changeCount = 1 // 至少改变一个元素
	}
	if changeCount > len(state.lastValues) {
		changeCount = len(state.lastValues)
	}

	// 复制当前值
	newValues := make([]interface{}, len(state.lastValues))
	copy(newValues, state.lastValues)

	// 随机选择要更改的元素
	indices := make([]int, len(newValues))
	for i := range indices {
		indices[i] = i
	}

	// 随机打乱
	for i := len(indices) - 1; i > 0; i-- {
		j := simIntn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}

	// 更改前几个元素
	for i := 0; i < changeCount; i++ {
		idx := indices[i]
		newValues[idx] = a.generateArrayElement(config)
	}

	// 更新状态
	state.lastValues = newValues
	state.generation++

	return &model.ArrayData{
		Values:   newValues,
		DataType: config.ElementType,
		Size:     len(newValues),
		Unit:     config.Unit,
		Labels:   config.Labels,
	}, model.TypeArray
}

// generateMatrixData 生成矩阵数据
func (a *MockAdapter) generateMatrixData(p *mockPoint) (*model.MatrixData, model.DataType) {
	if p.MatrixConfig == nil || p.matrixState == nil {
		log.Error().Msg("矩阵配置或状态为空")
		return nil, model.TypeMatrix
	}

	config := p.MatrixConfig
	state := p.matrixState

	// 复制当前矩阵
	newValues := make([][]float64, len(state.lastValues))
	for i := range newValues {
		newValues[i] = make([]float64, len(state.lastValues[i]))
		copy(newValues[i], state.lastValues[i])
	}

	// 根据生成次数决定更新策略
	if state.generation%10 == 0 {
		// 每10次完全重新生成
		for i := 0; i < config.Rows; i++ {
			for j := 0; j < config.Cols; j++ {
				newValues[i][j] = a.generateMatrixElement(config, i, j)
			}
		}
	} else {
		// 微调部分元素
		changeCount := max(1, (config.Rows*config.Cols)/10) // 改变10%的元素
		for k := 0; k < changeCount; k++ {
			i := simIntn(config.Rows)
			j := simIntn(config.Cols)
			change := (simFloat64() - 0.5) * (config.MaxValue - config.MinValue) * 0.1
			newValues[i][j] += change

			// 边界检查
			if newValues[i][j] < config.MinValue {
				newValues[i][j] = config.MinValue
			}
			if newValues[i][j] > config.MaxValue {
				newValues[i][j] = config.MaxValue
			}
		}
	}

	// 更新状态
	state.lastValues = newValues
	state.generation++

	return &model.MatrixData{
		Values: newValues,
		Rows:   config.Rows,
		Cols:   config.Cols,
		Unit:   config.Unit,
	}, model.TypeMatrix
}

// generateTimeSeriesData 生成时间序列数据
func (a *MockAdapter) generateTimeSeriesData(p *mockPoint) (*model.TimeSeriesData, model.DataType) {
	if p.TimeSeriesConfig == nil || p.timeseriesState == nil {
		log.Error().Msg("时间序列配置或状态为空")
		return nil, model.TypeTimeSeries
	}

	config := p.TimeSeriesConfig
	state := p.timeseriesState

	// 解析间隔
	interval, err := time.ParseDuration(config.Interval)
	if err != nil {
		interval = time.Second
	}

	// 添加新的数据点
	now := time.Now()
	newValue := state.lastValue

	// 添加趋势
	newValue += config.Trend

	// 添加季节性
	if config.Seasonality != nil {
		periodDuration, err := time.ParseDuration(config.Seasonality.Period)
		if err == nil {
			seasonalPhase := float64(now.UnixNano()) / float64(periodDuration.Nanoseconds()) * 2 * math.Pi
			newValue += config.Seasonality.Amplitude * math.Sin(seasonalPhase+config.Seasonality.Phase)
		}
	}

	// 添加噪声
	if config.Noise > 0 {
		newValue += (simFloat64() - 0.5) * 2 * config.Noise
	}

	// 添加异常值
	if config.Anomalies != nil && simFloat64() < config.Anomalies.Probability {
		newValue += (simFloat64() - 0.5) * config.Anomalies.Magnitude * newValue
	}

	// 更新序列（滑动窗口）
	state.values = append(state.values, newValue)
	state.timestamps = append(state.timestamps, now)
	state.lastValue = newValue

	// 保持固定长度
	if len(state.values) > config.Length {
		state.values = state.values[1:]
		state.timestamps = state.timestamps[1:]
	}

	// 计算采样间隔
	var avgInterval time.Duration
	if len(state.timestamps) > 1 {
		totalDuration := state.timestamps[len(state.timestamps)-1].Sub(state.timestamps[0])
		avgInterval = totalDuration / time.Duration(len(state.timestamps)-1)
	} else {
		avgInterval = interval
	}

	return &model.TimeSeriesData{
		Timestamps: append([]time.Time{}, state.timestamps...), // 复制切片
		Values:     append([]float64{}, state.values...),       // 复制切片
		Unit:       config.Unit,
		Interval:   avgInterval,
	}, model.TypeTimeSeries
}

// 辅助函数

// generateArrayElement 生成数组元素
func (a *MockAdapter) generateArrayElement(config *config.MockArrayConfig) interface{} {
	// 检查是否生成null值
	if config.NullProbability > 0 && simFloat64() < config.NullProbability {
		return nil
	}

	switch config.ElementType {
	case "int":
		return int(config.MinValue + simFloat64()*(config.MaxValue-config.MinValue))
	case "float":
		return config.MinValue + simFloat64()*(config.MaxValue-config.MinValue)
	case "string":
		if len(config.StringOptions) > 0 {
			return config.StringOptions[simIntn(len(config.StringOptions))]
		}
		return fmt.Sprintf("string_%d", simIntn(1000))
	case "bool":
		prob := config.BoolProbability
		if prob == 0 {
			prob = 0.5 // 默认50%概率
		}
		return simFloat64() < prob
	case "mixed":
		// 混合类型，随机选择
		switch simIntn(4) {
		case 0:
			return int(config.MinValue + simFloat64()*(config.MaxValue-config.MinValue))
		case 1:
			return config.MinValue + simFloat64()*(config.MaxValue-config.MinValue)
		case 2:
			if len(config.StringOptions) > 0 {
				return config.StringOptions[simIntn(len(config.StringOptions))]
			}
			return fmt.Sprintf("mixed_%d", simIntn(1000))
		case 3:
			return simFloat64() < 0.5
		}
	}

	return 0.0
}

// generateMatrixElement 生成矩阵元素
func (a *MockAdapter) generateMatrixElement(config *config.MockMatrixConfig, row, col int) float64 {
	// 检查稀疏度
	if config.Sparsity > 0 && simFloat64() < config.Sparsity {
		return 0.0
	}

	var value float64

	switch config.MatrixType {
	case "diagonal":
		if row != col {
			return 0.0
		}
		fallthrough
	case "identity":
		if row == col {
			if config.MatrixType == "identity" {
				return 1.0
			} else {
				value = config.MinValue + simFloat64()*(config.MaxValue-config.MinValue)
			}
		} else {
			return 0.0
		}
	case "symmetric":
		// 对称矩阵（简化版本，这里不做完整的对称性保证）
		fallthrough
	default:
		// 一般矩阵
		switch config.Distribution {
		case "normal":
			center := (config.MinValue + config.MaxValue) / 2.0
			value = center + (simFloat64()-0.5)*(config.MaxValue-config.MinValue)*0.3
		default:
			value = config.MinValue + simFloat64()*(config.MaxValue-config.MinValue)
		}
	}

	return value
}

// max 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
