package composite_mock

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	// 注册复合数据模拟器适配器工厂
	southbound.Register("composite_mock", func() southbound.Adapter {
		return NewCompositeMockAdapter()
	})
}

// CompositeMockAdapter 专门用于生成复合数据类型的模拟适配器
type CompositeMockAdapter struct {
	*southbound.BaseAdapter
	interval      time.Duration
	compositeData []CompositeDataPoint
	tags          map[string]interface{}
	stopCh        chan struct{}
	ch            chan<- model.Point
}

// CompositeMockConfig 复合数据模拟器配置
type CompositeMockConfig struct {
	Name              string                 `json:"name"`
	Type              string                 `json:"type"`
	Enabled           bool                   `json:"enabled"`
	Description       string                 `json:"description,omitempty"`
	Interval          string                 `json:"interval"`
	Tags              map[string]interface{} `json:"tags,omitempty"`
	CompositeDataTypes []CompositeDataPoint   `json:"composite_data_types"`
}

// CompositeDataPoint 复合数据点配置
type CompositeDataPoint struct {
	DeviceID       string                     `json:"device_id"`
	Key            string                     `json:"key"`
	DataType       string                     `json:"data_type"`
	LocationConfig *config.MockLocationConfig `json:"location_config,omitempty"`
	Vector3DConfig *config.MockVector3DConfig `json:"vector3d_config,omitempty"`
	ColorConfig    *config.MockColorConfig    `json:"color_config,omitempty"`
	VectorConfig   *config.MockVectorConfig   `json:"vector_config,omitempty"`
	ArrayConfig    *config.MockArrayConfig    `json:"array_config,omitempty"`
	
	// 内部状态（运行时）
	locationState *LocationState `json:"-"`
	vector3dState *Vector3DState `json:"-"`
	colorState    *ColorState    `json:"-"`
	vectorState   *VectorState   `json:"-"`
	arrayState    *ArrayState    `json:"-"`
}

// LocationState GPS位置模拟内部状态
type LocationState struct {
	CurrentLat  float64   `json:"current_lat"`
	CurrentLng  float64   `json:"current_lng"`
	CurrentAlt  float64   `json:"current_alt"`
	CurrentSpeed float64  `json:"current_speed"`
	Direction   float64   `json:"direction"` // 移动方向 (弧度)
	LastUpdate  time.Time `json:"last_update"`
}

// Vector3DState 三轴向量模拟内部状态
type Vector3DState struct {
	LastX    float64 `json:"last_x"`
	LastY    float64 `json:"last_y"`
	LastZ    float64 `json:"last_z"`
	Time     float64 `json:"time"` // 用于振荡计算
	Phase    float64 `json:"phase"` // 相位偏移
}

// ColorState 颜色模拟内部状态
type ColorState struct {
	CurrentHue float64 `json:"current_hue"` // 当前色相 (0-360)
	R          int     `json:"r"`
	G          int     `json:"g"`
	B          int     `json:"b"`
	Mode       string  `json:"mode"` // rainbow, fixed, random
}

// VectorState 通用向量模拟内部状态
type VectorState struct {
	LastValues []float64 `json:"last_values"`
	Time       float64   `json:"time"`
	Pattern    string    `json:"pattern"` // smooth, oscillate, random
}

// ArrayState 数组模拟内部状态
type ArrayState struct {
	LastValues []interface{} `json:"last_values"`
	Generation int           `json:"generation"`
	ElementType string       `json:"element_type"`
}

// NewCompositeMockAdapter 创建新的复合数据模拟器
func NewCompositeMockAdapter() *CompositeMockAdapter {
	return &CompositeMockAdapter{
		BaseAdapter: southbound.NewBaseAdapter("composite-mock-adapter", "composite_mock"),
		stopCh:      make(chan struct{}),
	}
}

// Init 初始化复合数据模拟器
func (a *CompositeMockAdapter) Init(cfg json.RawMessage) error {
	log.Info().
		Str("method", "CompositeMockAdapter.Init").
		Int("config_size", len(fmt.Sprintf("%v", cfg))).
		Str("device_id", "composite-mock-device").
		Msg("🔍 CompositeMockAdapter.Init() 开始执行 - 调试入口点")

	// 解析配置
	var parsedCfg CompositeMockConfig
	if err := json.Unmarshal(cfg, &parsedCfg); err != nil {
		return fmt.Errorf("解析复合数据模拟器配置失败: %w", err)
	}

	// 解析时间间隔
	interval, err := time.ParseDuration(parsedCfg.Interval)
	if err != nil {
		interval = 3 * time.Second // 默认3秒
		log.Warn().Str("interval", parsedCfg.Interval).Msg("解析间隔失败，使用默认值3秒")
	}

	a.interval = interval
	a.compositeData = parsedCfg.CompositeDataTypes
	a.tags = parsedCfg.Tags

	// 初始化每个复合数据点的状态
	for i := range a.compositeData {
		dataPoint := &a.compositeData[i]
		if err := a.initCompositeDataState(dataPoint); err != nil {
			log.Error().Err(err).
				Str("device_id", dataPoint.DeviceID).
				Str("data_type", dataPoint.DataType).
				Msg("初始化复合数据状态失败")
		}
	}

	log.Info().
		Str("config_name", parsedCfg.Name).
		Dur("config_interval", interval).
		Int("config_composite_types_count", len(a.compositeData)).
		Int("config_tags_count", len(a.tags)).
		Msg("复合数据模拟器配置加载成功")

	return nil
}

// initCompositeDataState 初始化复合数据状态
func (a *CompositeMockAdapter) initCompositeDataState(dataPoint *CompositeDataPoint) error {
	switch dataPoint.DataType {
	case "location":
		if dataPoint.LocationConfig != nil {
			dataPoint.locationState = &LocationState{
				CurrentLat:   dataPoint.LocationConfig.StartLatitude,
				CurrentLng:   dataPoint.LocationConfig.StartLongitude,
				CurrentAlt:   (dataPoint.LocationConfig.AltitudeMin + dataPoint.LocationConfig.AltitudeMax) / 2,
				CurrentSpeed: 0,
				Direction:    rand.Float64() * 2 * math.Pi,
				LastUpdate:   time.Now(),
			}
		}

	case "vector3d":
		if dataPoint.Vector3DConfig != nil {
			dataPoint.vector3dState = &Vector3DState{
				LastX: (dataPoint.Vector3DConfig.XMin + dataPoint.Vector3DConfig.XMax) / 2,
				LastY: (dataPoint.Vector3DConfig.YMin + dataPoint.Vector3DConfig.YMax) / 2,
				LastZ: (dataPoint.Vector3DConfig.ZMin + dataPoint.Vector3DConfig.ZMax) / 2,
				Time:  0,
				Phase: rand.Float64() * 2 * math.Pi,
			}
		}

	case "color":
		if dataPoint.ColorConfig != nil {
			dataPoint.colorState = &ColorState{
				CurrentHue: 0,
				R:          255,
				G:          0,
				B:          0,
				Mode:       "rainbow", // 默认彩虹模式
			}
		}

	case "vector":
		if dataPoint.VectorConfig != nil {
			initialValues := make([]float64, dataPoint.VectorConfig.Dimension)
			for i := 0; i < dataPoint.VectorConfig.Dimension; i++ {
				if i < len(dataPoint.VectorConfig.MinValues) && i < len(dataPoint.VectorConfig.MaxValues) {
					min := dataPoint.VectorConfig.MinValues[i]
					max := dataPoint.VectorConfig.MaxValues[i]
					initialValues[i] = min + rand.Float64()*(max-min)
				} else {
					initialValues[i] = rand.Float64() * 100
				}
			}
			dataPoint.vectorState = &VectorState{
				LastValues: initialValues,
				Time:       0,
				Pattern:    "smooth",
			}
		}

	case "array":
		if dataPoint.ArrayConfig != nil {
			initialValues := make([]interface{}, dataPoint.ArrayConfig.Size)
			for i := 0; i < dataPoint.ArrayConfig.Size; i++ {
				switch dataPoint.ArrayConfig.ElementType {
				case "float":
					initialValues[i] = rand.Float64() * dataPoint.ArrayConfig.MaxValue
				case "int":
					initialValues[i] = rand.Intn(int(dataPoint.ArrayConfig.MaxValue))
				case "bool":
					initialValues[i] = rand.Float32() > 0.5
				default:
					initialValues[i] = rand.Float64() * 100
				}
			}
			dataPoint.arrayState = &ArrayState{
				LastValues:  initialValues,
				Generation:  0,
				ElementType: dataPoint.ArrayConfig.ElementType,
			}
		}
	}

	return nil
}

// Start 启动复合数据模拟器
func (a *CompositeMockAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.ch = ch
	log.Info().Str("name", "composite_mock").Msg("启动复合数据模拟器")

	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			case <-ticker.C:
				a.generateAndPublishData()
			}
		}
	}()

	log.Info().Str("name", "composite_mock").Msg("复合数据模拟器启动成功")
	return nil
}

// Stop 停止复合数据模拟器
func (a *CompositeMockAdapter) Stop() error {
	close(a.stopCh)
	log.Info().Str("name", "composite_mock").Msg("复合数据模拟器已停止")
	return nil
}

// generateAndPublishData 生成并发布复合数据
func (a *CompositeMockAdapter) generateAndPublishData() {
	for _, dataPoint := range a.compositeData {
		value, err := a.generateCompositeValue(&dataPoint)
		if err != nil {
			log.Error().Err(err).
				Str("device_id", dataPoint.DeviceID).
				Str("data_type", dataPoint.DataType).
				Msg("生成复合数据失败")
			continue
		}

		// 创建数据点
		point := model.Point{
			DeviceID:  dataPoint.DeviceID,
			Key:       dataPoint.Key,
			Value:     value,
			Type:      model.DataType(dataPoint.DataType),
			Timestamp: time.Now(),
			Quality:   0,
			SafeTags:  nil, // 暂时不设置标签
		}

		// 发布数据
		select {
		case a.ch <- point:
		case <-time.After(100 * time.Millisecond):
			log.Warn().Str("device_id", dataPoint.DeviceID).Msg("发送数据超时")
		}

		log.Debug().
			Str("device_id", dataPoint.DeviceID).
			Str("key", dataPoint.Key).
			Str("data_type", dataPoint.DataType).
			Interface("value", value).
			Msg("发布复合数据")
	}
}

// generateCompositeValue 生成复合数据值
func (a *CompositeMockAdapter) generateCompositeValue(dataPoint *CompositeDataPoint) (interface{}, error) {
	switch dataPoint.DataType {
	case "location":
		return a.generateLocationData(dataPoint)
	case "vector3d":
		return a.generateVector3DData(dataPoint)
	case "color":
		return a.generateColorData(dataPoint)
	case "vector":
		return a.generateVectorData(dataPoint)
	case "array":
		return a.generateArrayData(dataPoint)
	default:
		return nil, fmt.Errorf("不支持的复合数据类型: %s", dataPoint.DataType)
	}
}

// generateLocationData 生成GPS位置数据
func (a *CompositeMockAdapter) generateLocationData(dataPoint *CompositeDataPoint) (interface{}, error) {
	if dataPoint.locationState == nil || dataPoint.LocationConfig == nil {
		return nil, fmt.Errorf("location状态或配置未初始化")
	}

	state := dataPoint.locationState
	config := dataPoint.LocationConfig

	// 模拟移动（如果启用）
	if config.SimulateMovement {
		// 更新位置
		elapsed := time.Since(state.LastUpdate).Seconds()
		if elapsed > 0 {
			// 随机改变方向
			if rand.Float32() < 0.1 { // 10%概率改变方向
				state.Direction += (rand.Float64()*0.4 - 0.2) // ±0.2弧度变化
			}

			// 更新速度
			state.CurrentSpeed = config.SpeedMin + rand.Float64()*(config.SpeedMax-config.SpeedMin)

			// 计算位移
			distance := state.CurrentSpeed * elapsed / 3600.0 // 转换为公里
			latChange := distance * math.Cos(state.Direction) / 111.0 // 大约111km/度
			lngChange := distance * math.Sin(state.Direction) / (111.0 * math.Cos(state.CurrentLat*math.Pi/180))

			state.CurrentLat += latChange
			state.CurrentLng += lngChange

			// 边界检查
			minLat := config.StartLatitude - config.LatitudeRange/2
			maxLat := config.StartLatitude + config.LatitudeRange/2
			minLng := config.StartLongitude - config.LongitudeRange/2
			maxLng := config.StartLongitude + config.LongitudeRange/2

			if state.CurrentLat < minLat || state.CurrentLat > maxLat {
				state.Direction = math.Pi - state.Direction // 反向
				state.CurrentLat = math.Max(minLat, math.Min(maxLat, state.CurrentLat))
			}
			if state.CurrentLng < minLng || state.CurrentLng > maxLng {
				state.Direction = -state.Direction // 反向
				state.CurrentLng = math.Max(minLng, math.Min(maxLng, state.CurrentLng))
			}

			state.LastUpdate = time.Now()
		}
	} else {
		// 静态位置，只在小范围内随机波动
		state.CurrentLat = config.StartLatitude + (rand.Float64()-0.5)*config.LatitudeRange*0.1
		state.CurrentLng = config.StartLongitude + (rand.Float64()-0.5)*config.LongitudeRange*0.1
		state.CurrentSpeed = config.SpeedMin + rand.Float64()*(config.SpeedMax-config.SpeedMin)
	}

	// 更新高度
	state.CurrentAlt = config.AltitudeMin + rand.Float64()*(config.AltitudeMax-config.AltitudeMin)

	return map[string]interface{}{
		"latitude":  state.CurrentLat,
		"longitude": state.CurrentLng,
		"altitude":  state.CurrentAlt,
		"speed":     state.CurrentSpeed,
	}, nil
}

// generateVector3DData 生成3D向量数据
func (a *CompositeMockAdapter) generateVector3DData(dataPoint *CompositeDataPoint) (interface{}, error) {
	if dataPoint.vector3dState == nil || dataPoint.Vector3DConfig == nil {
		return nil, fmt.Errorf("vector3d状态或配置未初始化")
	}

	state := dataPoint.vector3dState
	config := dataPoint.Vector3DConfig

	// 更新时间
	state.Time += 0.1

	var x, y, z float64

	if config.Oscillation {
		// 振荡模式
		freq := config.Frequency
		x = (config.XMin+config.XMax)/2 + (config.XMax-config.XMin)/4*math.Sin(state.Time*freq+state.Phase)
		y = (config.YMin+config.YMax)/2 + (config.YMax-config.YMin)/4*math.Sin(state.Time*freq*1.3+state.Phase+0.5)
		z = (config.ZMin+config.ZMax)/2 + (config.ZMax-config.ZMin)/4*math.Sin(state.Time*freq*0.7+state.Phase+1.0)
	} else {
		// 平滑随机变化
		x = state.LastX + (rand.Float64()-0.5)*0.5
		y = state.LastY + (rand.Float64()-0.5)*0.5
		z = state.LastZ + (rand.Float64()-0.5)*0.5

		// 边界检查
		x = math.Max(config.XMin, math.Min(config.XMax, x))
		y = math.Max(config.YMin, math.Min(config.YMax, y))
		z = math.Max(config.ZMin, math.Min(config.ZMax, z))
	}

	state.LastX, state.LastY, state.LastZ = x, y, z

	// 计算模长
	magnitude := math.Sqrt(x*x + y*y + z*z)

	return map[string]interface{}{
		"x":         x,
		"y":         y,
		"z":         z,
		"magnitude": magnitude,
	}, nil
}

// generateColorData 生成颜色数据
func (a *CompositeMockAdapter) generateColorData(dataPoint *CompositeDataPoint) (interface{}, error) {
	if dataPoint.colorState == nil || dataPoint.ColorConfig == nil {
		return nil, fmt.Errorf("color状态或配置未初始化")
	}

	state := dataPoint.colorState

	// 彩虹模式：色相循环
	state.CurrentHue += 2.0 // 每次增加2度
	if state.CurrentHue >= 360 {
		state.CurrentHue = 0
	}

	// HSV转RGB
	h := state.CurrentHue / 60.0
	c := 1.0 // 饱和度固定为1
	x := c * (1.0 - math.Abs(math.Mod(h, 2.0)-1.0))
	m := 0.0 // 亮度固定

	var r, g, b float64
	switch int(h) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default: // case 5
		r, g, b = c, 0, x
	}

	state.R = int((r + m) * 255)
	state.G = int((g + m) * 255)
	state.B = int((b + m) * 255)

	// 计算HSV值
	saturation := 100.0
	lightness := 50.0

	return map[string]interface{}{
		"r":          state.R,
		"g":          state.G,
		"b":          state.B,
		"hue":        state.CurrentHue,
		"saturation": saturation,
		"lightness":  lightness,
	}, nil
}

// generateVectorData 生成通用向量数据
func (a *CompositeMockAdapter) generateVectorData(dataPoint *CompositeDataPoint) (interface{}, error) {
	if dataPoint.vectorState == nil || dataPoint.VectorConfig == nil {
		return nil, fmt.Errorf("vector状态或配置未初始化")
	}

	state := dataPoint.vectorState
	config := dataPoint.VectorConfig

	state.Time += 0.1

	values := make([]float64, len(state.LastValues))
	for i := range values {
		min := 0.0
		max := 100.0
		if i < len(config.MinValues) {
			min = config.MinValues[i]
		}
		if i < len(config.MaxValues) {
			max = config.MaxValues[i]
		}

		switch state.Pattern {
		case "smooth":
			// 平滑变化
			change := (rand.Float64() - 0.5) * 5.0
			values[i] = state.LastValues[i] + change
			values[i] = math.Max(min, math.Min(max, values[i]))
		case "oscillate":
			// 振荡模式
			center := (min + max) / 2
			amplitude := (max - min) / 4
			freq := 0.5 + float64(i)*0.2
			values[i] = center + amplitude*math.Sin(state.Time*freq)
		default: // random
			values[i] = min + rand.Float64()*(max-min)
		}
	}

	state.LastValues = values

	// 构建标签
	labels := config.Labels
	if len(labels) < len(values) {
		// 补充默认标签
		for i := len(labels); i < len(values); i++ {
			labels = append(labels, fmt.Sprintf("分量%d", i+1))
		}
	}

	return map[string]interface{}{
		"values": values,
		"labels": labels,
		"unit":   config.Unit,
	}, nil
}

// generateArrayData 生成数组数据
func (a *CompositeMockAdapter) generateArrayData(dataPoint *CompositeDataPoint) (interface{}, error) {
	if dataPoint.arrayState == nil || dataPoint.ArrayConfig == nil {
		return nil, fmt.Errorf("array状态或配置未初始化")
	}

	state := dataPoint.arrayState
	config := dataPoint.ArrayConfig
	state.Generation++

	elements := make([]interface{}, config.Size)
	for i := 0; i < config.Size; i++ {
		switch config.ElementType {
		case "float":
			if len(state.LastValues) > i {
				if lastVal, ok := state.LastValues[i].(float64); ok {
					// 平滑变化
					change := (rand.Float64() - 0.5) * 10
					newVal := lastVal + change
					elements[i] = math.Max(0, math.Min(config.MaxValue, newVal))
				} else {
					elements[i] = rand.Float64() * config.MaxValue
				}
			} else {
				elements[i] = rand.Float64() * config.MaxValue
			}
		case "int":
			if len(state.LastValues) > i {
				if lastVal, ok := state.LastValues[i].(int); ok {
					change := rand.Intn(21) - 10 // -10到+10的变化
					newVal := lastVal + change
					elements[i] = int(math.Max(0, math.Min(config.MaxValue, float64(newVal))))
				} else {
					elements[i] = rand.Intn(int(config.MaxValue))
				}
			} else {
				elements[i] = rand.Intn(int(config.MaxValue))
			}
		case "bool":
			elements[i] = rand.Float32() > 0.5
		default:
			elements[i] = rand.Float64() * 100
		}
	}

	state.LastValues = elements

	// 构建标签
	labels := config.Labels
	if len(labels) < len(elements) {
		for i := len(labels); i < len(elements); i++ {
			labels = append(labels, fmt.Sprintf("元素%d", i+1))
		}
	}

	return map[string]interface{}{
		"elements": elements,
		"labels":   labels,
		"unit":     config.Unit,
	}, nil
}

// copyTags 复制标签
func (a *CompositeMockAdapter) copyTags() map[string]interface{} {
	tags := make(map[string]interface{})
	for k, v := range a.tags {
		tags[k] = v
	}
	return tags
}

// Name 返回适配器名称
func (a *CompositeMockAdapter) Name() string {
	return "composite_mock"
}