package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	// 注册适配器工厂
	southbound.Register("http", func() southbound.Adapter {
		return &HTTPAdapter{}
	})
}

// HTTPAdapter 是一个HTTP适配器，用于从HTTP API获取数据
type HTTPAdapter struct {
	*southbound.BaseAdapter
	client    *http.Client
	endpoints []Endpoint
	deviceID  string
	interval  time.Duration
	stopCh    chan struct{}
	mutex     sync.Mutex
	running   bool
	parser    *config.ConfigParser[config.HTTPConfig]
}

// Endpoint 定义了要请求的HTTP端点
type Endpoint struct {
	URL        string            `json:"url"`         // HTTP端点URL
	Method     string            `json:"method"`      // HTTP方法: GET, POST, PUT等
	Headers    map[string]string `json:"headers"`     // HTTP请求头
	Body       string            `json:"body"`        // 请求体（用于POST/PUT）
	DataPoints []DataPoint       `json:"data_points"` // 要从响应中提取的数据点
	Timeout    int               `json:"timeout_ms"`  // 请求超时(ms)
}

// DataPoint 定义了从HTTP响应中提取的数据点
type DataPoint struct {
	Key       string                   `json:"key"`                 // 数据点标识符
	Path      string                   `json:"path"`                // JSON路径，用于从响应中提取值 (基础类型)
	Type      string                   `json:"type"`                // 数据类型: int, float, bool, string, location, vector3d, color
	DeviceID  string                   `json:"device_id"`           // 设备ID，如果为空则使用适配器默认值
	Tags      map[string]string        `json:"tags,omitempty"`      // 附加标签
	Composite *CompositeExtractConfig  `json:"composite,omitempty"` // 复合数据提取配置
}

// CompositeExtractConfig 复合数据提取配置
type CompositeExtractConfig struct {
	Location *LocationExtractConfig `json:"location,omitempty"`
	Vector3D *Vector3DExtractConfig `json:"vector3d,omitempty"`
	Color    *ColorExtractConfig    `json:"color,omitempty"`
}

// LocationExtractConfig GPS位置提取配置
type LocationExtractConfig struct {
	LatitudePath  string `json:"latitude_path"`
	LongitudePath string `json:"longitude_path"`
	AltitudePath  string `json:"altitude_path,omitempty"`
	AccuracyPath  string `json:"accuracy_path,omitempty"`
	SpeedPath     string `json:"speed_path,omitempty"`
	HeadingPath   string `json:"heading_path,omitempty"`
}

// Vector3DExtractConfig 3D向量提取配置  
type Vector3DExtractConfig struct {
	XPath string `json:"x_path"`
	YPath string `json:"y_path"`
	ZPath string `json:"z_path"`
}

// ColorExtractConfig 颜色提取配置
type ColorExtractConfig struct {
	RedPath   string `json:"red_path"`
	GreenPath string `json:"green_path"`
	BluePath  string `json:"blue_path"`
	AlphaPath string `json:"alpha_path,omitempty"`
}


// Name 返回适配器名称
func (a *HTTPAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *HTTPAdapter) Init(cfg json.RawMessage) error {
	// 创建配置解析器
	a.parser = config.NewParserWithDefaults(config.GetDefaultHTTPConfig())
	
	// 解析配置
	httpConfig, err := a.parser.Parse(cfg)
	if err != nil {
		return fmt.Errorf("解析HTTP配置失败: %w", err)
	}

	return a.initWithConfig(httpConfig)
}

// initWithConfig 使用新配置格式初始化
func (a *HTTPAdapter) initWithConfig(config *config.HTTPConfig) error {
	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "http")
	a.interval = config.Interval.Duration()
	a.stopCh = make(chan struct{})

	// 创建HTTP客户端
	a.client = &http.Client{
		Timeout: config.Timeout.Duration(),
	}

	// 转换端点配置，包含数据点信息
	dataPoints := make([]DataPoint, len(config.DataPoints))
	for i, dp := range config.DataPoints {
		dataPoint := DataPoint{
			Key:      dp.Key,
			Path:     dp.Path,
			Type:     dp.Type,
			DeviceID: dp.DeviceID,
			Tags:     dp.Tags,
		}
		
		// 转换复合对象配置
		if dp.Composite != nil {
			dataPoint.Composite = &CompositeExtractConfig{}
			
			if dp.Composite.Location != nil {
				dataPoint.Composite.Location = &LocationExtractConfig{
					LatitudePath:  dp.Composite.Location.LatitudePath,
					LongitudePath: dp.Composite.Location.LongitudePath,
					AltitudePath:  dp.Composite.Location.AltitudePath,
					AccuracyPath:  dp.Composite.Location.AccuracyPath,
					SpeedPath:     dp.Composite.Location.SpeedPath,
					HeadingPath:   dp.Composite.Location.HeadingPath,
				}
			}
			
			if dp.Composite.Vector3D != nil {
				dataPoint.Composite.Vector3D = &Vector3DExtractConfig{
					XPath: dp.Composite.Vector3D.XPath,
					YPath: dp.Composite.Vector3D.YPath,
					ZPath: dp.Composite.Vector3D.ZPath,
				}
			}
			
			if dp.Composite.Color != nil {
				dataPoint.Composite.Color = &ColorExtractConfig{
					RedPath:   dp.Composite.Color.RedPath,
					GreenPath: dp.Composite.Color.GreenPath,
					BluePath:  dp.Composite.Color.BluePath,
					AlphaPath: dp.Composite.Color.AlphaPath,
				}
			}
		}
		
		dataPoints[i] = dataPoint
	}
	
	a.endpoints = []Endpoint{
		{
			URL:        config.URL,
			Method:     config.Method,
			Headers:    config.Headers,
			Body:       config.Body,
			DataPoints: dataPoints,
			Timeout:    int(config.Timeout.Duration().Milliseconds()),
		},
	}

	log.Info().
		Str("name", a.Name()).
		Int("endpoints", len(a.endpoints)).
		Int("data_points", len(dataPoints)).
		Dur("interval", a.interval).
		Msg("HTTP适配器初始化完成")

	return nil
}


// Start 启动适配器
func (a *HTTPAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return nil
	}
	a.running = true

	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 请求所有端点
				for _, endpoint := range a.endpoints {
					// 创建请求
					req, err := a.createRequest(endpoint)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("创建HTTP请求失败")
						continue
					}

					// 设置超时
					timeout := time.Duration(endpoint.Timeout) * time.Millisecond
					if timeout > 0 {
						reqCtx, cancel := context.WithTimeout(ctx, timeout)
						req = req.WithContext(reqCtx)
						defer cancel()
					}

					// 记录请求开始时间
					requestStart := time.Now()
					
					// 发送请求
					resp, err := a.client.Do(req)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("HTTP请求失败")
						continue
					}

					// 确保响应体被关闭
					defer resp.Body.Close()

					// 检查响应状态码
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						log.Error().
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Int("status", resp.StatusCode).
							Msg("HTTP请求返回非成功状态码")
						continue
					}

					// 读取响应体
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("读取HTTP响应失败")
						continue
					}

					// 解析JSON响应
					var data map[string]interface{}
					if err := json.Unmarshal(body, &data); err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("解析HTTP响应JSON失败")
						continue
					}

					// 处理每个数据点
					for _, dp := range endpoint.DataPoints {
						// 确定设备ID
						deviceID := dp.DeviceID
						if deviceID == "" {
							deviceID = a.deviceID
						}
						if deviceID == "" {
							deviceID = "http"
						}

						var value interface{}
						var dataType model.DataType
						var err error

						// 根据类型处理数据提取
						switch dp.Type {
						case "location":
							value, dataType, err = a.extractLocationData(data, dp)
						case "vector3d":
							value, dataType, err = a.extractVector3DData(data, dp)
						case "color":
							value, dataType, err = a.extractColorData(data, dp)
						default:
							// 处理基础数据类型
							value, err = a.extractValue(data, dp.Path)
							if err != nil {
								log.Error().
									Err(err).
									Str("name", a.Name()).
									Str("url", endpoint.URL).
									Str("path", dp.Path).
									Msg("从HTTP响应中提取值失败")
								continue
							}
							dataType, value, err = a.convertBasicType(dp.Type, value, endpoint.URL)
						}

						if err != nil {
							log.Error().
								Err(err).
								Str("name", a.Name()).
								Str("url", endpoint.URL).
								Str("key", dp.Key).
								Msg("数据处理失败")
							continue
						}

						// 数据类型转换已经在上面的switch中处理完成

						// 创建数据点
						point := model.NewPoint(dp.Key, deviceID, value, dataType)

						// 添加标签
						point.AddTag("source", "http")
						point.AddTag("url", endpoint.URL)

						// 添加自定义标签
						for k, v := range dp.Tags {
							point.AddTag(k, v)
						}

						// 使用BaseAdapter的SafeSendDataPoint方法，自动处理统计
						a.SafeSendDataPoint(ch, point, requestStart)
						
						log.Debug().
							Str("name", a.Name()).
							Str("key", dp.Key).
							Str("url", endpoint.URL).
							Interface("value", value).
							Str("type", string(dataType)).
							Msg("发送HTTP数据点")
					}
				}
			case <-a.stopCh:
				log.Info().Str("name", a.Name()).Msg("HTTP适配器停止")
				return
			case <-ctx.Done():
				log.Info().Str("name", a.Name()).Msg("HTTP适配器上下文取消")
				return
			}
		}
	}()

	log.Info().Str("name", a.Name()).Msg("HTTP适配器启动")
	return nil
}

// createRequest 创建HTTP请求
func (a *HTTPAdapter) createRequest(endpoint Endpoint) (*http.Request, error) {
	method := strings.ToUpper(endpoint.Method)
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if endpoint.Body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		body = strings.NewReader(endpoint.Body)
	}

	req, err := http.NewRequest(method, endpoint.URL, body)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	for k, v := range endpoint.Headers {
		req.Header.Set(k, v)
	}

	// 如果没有设置Content-Type，且有请求体，默认为JSON
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// extractValue 从JSON中提取值
func (a *HTTPAdapter) extractValue(data map[string]interface{}, path string) (interface{}, error) {
	// 处理嵌套路径，如 "data.temperature"
	parts := strings.Split(path, ".")
	current := data

	// 遍历路径的每一部分，直到最后一个
	for i, part := range parts {
		if i == len(parts)-1 {
			// 最后一个部分，返回值
			if value, ok := current[part]; ok {
				return value, nil
			}
			return nil, fmt.Errorf("路径 %s 不存在", path)
		}

		// 不是最后一个部分，继续向下遍历
		next, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不存在", path, part)
		}

		// 确保下一级是一个对象
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不是一个对象", path, part)
		}

		current = nextMap
	}

	return nil, fmt.Errorf("无效的路径: %s", path)
}

// Stop 停止适配器
func (a *HTTPAdapter) Stop() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		return nil
	}

	close(a.stopCh)
	a.running = false
	return nil
}

// convertBasicType 转换基础数据类型
func (a *HTTPAdapter) convertBasicType(dataType string, value interface{}, url string) (model.DataType, interface{}, error) {
	switch dataType {
	case "int":
		switch v := value.(type) {
		case float64:
			return model.TypeInt, int(v), nil
		case string:
			var intVal int
			if _, err := fmt.Sscanf(v, "%d", &intVal); err != nil {
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为整数", v)
			}
			return model.TypeInt, intVal, nil
		case int:
			return model.TypeInt, v, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为整数", v)
		}
		
	case "float":
		switch v := value.(type) {
		case float64:
			return model.TypeFloat, v, nil
		case int:
			return model.TypeFloat, float64(v), nil
		case string:
			var floatVal float64
			if _, err := fmt.Sscanf(v, "%f", &floatVal); err != nil {
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为浮点数", v)
			}
			return model.TypeFloat, floatVal, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为浮点数", v)
		}
		
	case "bool":
		switch v := value.(type) {
		case bool:
			return model.TypeBool, v, nil
		case string:
			switch v {
			case "true", "1", "on", "yes":
				return model.TypeBool, true, nil
			case "false", "0", "off", "no":
				return model.TypeBool, false, nil
			default:
				return "", nil, fmt.Errorf("无法将字符串 '%s' 转换为布尔值", v)
			}
		case float64:
			return model.TypeBool, v != 0, nil
		case int:
			return model.TypeBool, v != 0, nil
		default:
			return "", nil, fmt.Errorf("无法将类型 %T 转换为布尔值", v)
		}
		
	case "string":
		if s, ok := value.(string); ok {
			return model.TypeString, s, nil
		}
		return model.TypeString, fmt.Sprintf("%v", value), nil
		
	default:
		// 默认为字符串类型
		if s, ok := value.(string); ok {
			return model.TypeString, s, nil
		}
		return model.TypeString, fmt.Sprintf("%v", value), nil
	}
}

// extractFloatValue 从JSON中提取浮点数值
func (a *HTTPAdapter) extractFloatValue(data map[string]interface{}, path string) (float64, error) {
	if path == "" {
		return 0, nil // 可选字段为空时返回0
	}
	
	value, err := a.extractValue(data, path)
	if err != nil {
		return 0, err
	}
	
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		var floatVal float64
		if _, err := fmt.Sscanf(v, "%f", &floatVal); err != nil {
			return 0, fmt.Errorf("无法将字符串 '%s' 转换为浮点数", v)
		}
		return floatVal, nil
	default:
		return 0, fmt.Errorf("无法将类型 %T 转换为浮点数", v)
	}
}

// extractLocationData 提取GPS位置数据
func (a *HTTPAdapter) extractLocationData(data map[string]interface{}, dp DataPoint) (interface{}, model.DataType, error) {
	if dp.Composite == nil || dp.Composite.Location == nil {
		return nil, "", fmt.Errorf("location类型数据点缺少复合配置")
	}
	
	config := dp.Composite.Location
	
	// 提取必需字段
	lat, err := a.extractFloatValue(data, config.LatitudePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取纬度失败: %w", err)
	}
	
	lon, err := a.extractFloatValue(data, config.LongitudePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取经度失败: %w", err)
	}
	
	location := &model.LocationData{
		Latitude:  lat,
		Longitude: lon,
	}
	
	// 提取可选字段
	if config.AltitudePath != "" {
		if alt, err := a.extractFloatValue(data, config.AltitudePath); err == nil {
			location.Altitude = alt
		}
	}
	
	if config.AccuracyPath != "" {
		if acc, err := a.extractFloatValue(data, config.AccuracyPath); err == nil {
			location.Accuracy = acc
		}
	}
	
	if config.SpeedPath != "" {
		if speed, err := a.extractFloatValue(data, config.SpeedPath); err == nil {
			location.Speed = speed
		}
	}
	
	if config.HeadingPath != "" {
		if heading, err := a.extractFloatValue(data, config.HeadingPath); err == nil {
			location.Heading = heading
		}
	}
	
	// 验证数据
	if err := location.Validate(); err != nil {
		return nil, "", fmt.Errorf("GPS位置数据验证失败: %w", err)
	}
	
	return location, model.TypeLocation, nil
}

// extractVector3DData 提取3D向量数据
func (a *HTTPAdapter) extractVector3DData(data map[string]interface{}, dp DataPoint) (interface{}, model.DataType, error) {
	if dp.Composite == nil || dp.Composite.Vector3D == nil {
		return nil, "", fmt.Errorf("vector3d类型数据点缺少复合配置")
	}
	
	config := dp.Composite.Vector3D
	
	// 提取XYZ坐标
	x, err := a.extractFloatValue(data, config.XPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取X坐标失败: %w", err)
	}
	
	y, err := a.extractFloatValue(data, config.YPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取Y坐标失败: %w", err)
	}
	
	z, err := a.extractFloatValue(data, config.ZPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取Z坐标失败: %w", err)
	}
	
	vector := &model.Vector3D{
		X: x,
		Y: y,
		Z: z,
	}
	
	// 验证数据
	if err := vector.Validate(); err != nil {
		return nil, "", fmt.Errorf("3D向量数据验证失败: %w", err)
	}
	
	return vector, model.TypeVector3D, nil
}

// extractColorData 提取颜色数据
func (a *HTTPAdapter) extractColorData(data map[string]interface{}, dp DataPoint) (interface{}, model.DataType, error) {
	if dp.Composite == nil || dp.Composite.Color == nil {
		return nil, "", fmt.Errorf("color类型数据点缺少复合配置")
	}
	
	config := dp.Composite.Color
	
	// 提取RGB值
	red, err := a.extractFloatValue(data, config.RedPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取红色分量失败: %w", err)
	}
	
	green, err := a.extractFloatValue(data, config.GreenPath)
	if err != nil {
		return nil, "", fmt.Errorf("提取绿色分量失败: %w", err)
	}
	
	blue, err := a.extractFloatValue(data, config.BluePath)
	if err != nil {
		return nil, "", fmt.Errorf("提取蓝色分量失败: %w", err)
	}
	
	// 将0-255范围的RGB值转换为uint8
	r := uint8(math.Min(255, math.Max(0, red)))
	g := uint8(math.Min(255, math.Max(0, green)))
	b := uint8(math.Min(255, math.Max(0, blue)))
	
	color := &model.ColorData{
		R: r,
		G: g,
		B: b,
	}
	
	// 提取Alpha值 (可选)，将0-1范围转换为0-255
	if config.AlphaPath != "" {
		if alpha, err := a.extractFloatValue(data, config.AlphaPath); err == nil {
			color.A = uint8(math.Min(255, math.Max(0, alpha*255)))
		} else {
			color.A = 255 // 默认不透明
		}
	} else {
		color.A = 255 // 默认不透明
	}
	
	// 验证数据
	if err := color.Validate(); err != nil {
		return nil, "", fmt.Errorf("颜色数据验证失败: %w", err)
	}
	
	return color, model.TypeColor, nil
}

// NewAdapter 创建一个新的HTTP适配器实例
func NewAdapter() southbound.Adapter {
	return &HTTPAdapter{}
}
