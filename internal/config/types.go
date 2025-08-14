package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Common configuration types for adapters and sinks

// Duration is a custom duration type that can unmarshal from string
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler interface for Duration
func (d *Duration) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value) * time.Millisecond)
	case string:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		*d = Duration(duration)
	default:
		return fmt.Errorf("invalid duration type: %T", v)
	}
	return nil
}

// MarshalJSON implements json.Marshaler interface for Duration
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalYAML implements yaml.Unmarshaler interface for Duration
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}

	switch value := v.(type) {
	case int:
		*d = Duration(time.Duration(value) * time.Millisecond)
	case float64:
		*d = Duration(time.Duration(value) * time.Millisecond)
	case string:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse duration: %w", err)
		}
		*d = Duration(duration)
	default:
		return fmt.Errorf("invalid duration type: %T", v)
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler interface for Duration
func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// Duration returns the time.Duration value
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// BaseConfig represents common configuration for all adapters and sinks
type BaseConfig struct {
	Name        string            `json:"name" yaml:"name" validate:"required"`
	Type        string            `json:"type" yaml:"type" validate:"required"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Tags        map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// AdapterConfig represents configuration for southbound adapters
type AdapterConfig struct {
	BaseConfig `json:",inline" yaml:",inline"`
	Interval   Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout    Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// SinkConfig represents configuration for northbound sinks
type SinkConfig struct {
	BaseConfig  `json:",inline" yaml:",inline"`
	BatchSize   int      `json:"batch_size,omitempty" yaml:"batch_size,omitempty" validate:"min=1,max=10000"`
	BufferSize  int      `json:"buffer_size,omitempty" yaml:"buffer_size,omitempty" validate:"min=1,max=100000"`
	FlushTimeout Duration `json:"flush_timeout,omitempty" yaml:"flush_timeout,omitempty"`
}

// ModbusConfig represents Modbus adapter configuration
type ModbusConfig struct {
	AdapterConfig `json:",inline" yaml:",inline"`
	Host          string            `json:"host" yaml:"host" validate:"required"`
	Port          int               `json:"port" yaml:"port" validate:"port"`
	SlaveID       byte              `json:"slave_id" yaml:"slave_id" validate:"max=247"`
	Protocol      string            `json:"protocol,omitempty" yaml:"protocol,omitempty" validate:"oneof=tcp rtu"`
	Registers     []ModbusRegister  `json:"registers" yaml:"registers" validate:"required,min=1"`
}

// ModbusRegister represents a Modbus register configuration
type ModbusRegister struct {
	Address    uint16 `json:"address" yaml:"address"`
	Type       string `json:"type" yaml:"type" validate:"required,oneof=coil discrete_input input_register holding_register"`
	DataType   string `json:"data_type,omitempty" yaml:"data_type,omitempty" validate:"oneof=uint16 int16 uint32 int32 float32"`
	DeviceID   string `json:"device_id" yaml:"device_id" validate:"required"`
	Key        string `json:"key" yaml:"key" validate:"required"`
	Scale      float64 `json:"scale,omitempty" yaml:"scale,omitempty"`
	Offset     float64 `json:"offset,omitempty" yaml:"offset,omitempty"`
}

// HTTPConfig represents HTTP adapter configuration
type HTTPConfig struct {
	AdapterConfig `json:",inline" yaml:",inline"`
	URL           string            `json:"url" yaml:"url" validate:"required,url"`
	Method        string            `json:"method,omitempty" yaml:"method,omitempty" validate:"oneof=GET POST PUT DELETE"`
	Headers       map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body          string            `json:"body,omitempty" yaml:"body,omitempty"`
	DataPoints    []HTTPDataPoint   `json:"data_points,omitempty" yaml:"data_points,omitempty"`
	Parser        HTTPParser        `json:"parser,omitempty" yaml:"parser,omitempty"` // Deprecated, use DataPoints instead
}

// HTTPDataPoint represents a data point to extract from HTTP response
type HTTPDataPoint struct {
	Key      string            `json:"key" yaml:"key" validate:"required"`
	Path     string            `json:"path" yaml:"path" validate:"required"`
	Type     string            `json:"type" yaml:"type" validate:"oneof=int float bool string location vector3d color"`
	DeviceID string            `json:"device_id,omitempty" yaml:"device_id,omitempty"`
	Tags     map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	// 复合对象配置
	Composite *HTTPCompositeConfig `json:"composite,omitempty" yaml:"composite,omitempty"`
}

// HTTPCompositeConfig represents configuration for extracting composite data
type HTTPCompositeConfig struct {
	// Location 类型的字段映射
	Location *HTTPLocationConfig `json:"location,omitempty" yaml:"location,omitempty"`
	// Vector3D 类型的字段映射  
	Vector3D *HTTPVector3DConfig `json:"vector3d,omitempty" yaml:"vector3d,omitempty"`
	// Color 类型的字段映射
	Color *HTTPColorConfig `json:"color,omitempty" yaml:"color,omitempty"`
}

// HTTPLocationConfig represents GPS location field mapping
type HTTPLocationConfig struct {
	LatitudePath  string `json:"latitude_path" yaml:"latitude_path" validate:"required"`
	LongitudePath string `json:"longitude_path" yaml:"longitude_path" validate:"required"`
	AltitudePath  string `json:"altitude_path,omitempty" yaml:"altitude_path,omitempty"`
	AccuracyPath  string `json:"accuracy_path,omitempty" yaml:"accuracy_path,omitempty"`
	SpeedPath     string `json:"speed_path,omitempty" yaml:"speed_path,omitempty"`
	HeadingPath   string `json:"heading_path,omitempty" yaml:"heading_path,omitempty"`
}

// HTTPVector3DConfig represents 3D vector field mapping
type HTTPVector3DConfig struct {
	XPath string `json:"x_path" yaml:"x_path" validate:"required"`
	YPath string `json:"y_path" yaml:"y_path" validate:"required"`
	ZPath string `json:"z_path" yaml:"z_path" validate:"required"`
}

// HTTPColorConfig represents color field mapping
type HTTPColorConfig struct {
	RedPath   string `json:"red_path" yaml:"red_path" validate:"required"`
	GreenPath string `json:"green_path" yaml:"green_path" validate:"required"`
	BluePath  string `json:"blue_path" yaml:"blue_path" validate:"required"`
	AlphaPath string `json:"alpha_path,omitempty" yaml:"alpha_path,omitempty"`
}

// HTTPParser represents HTTP response parser configuration
type HTTPParser struct {
	Type     string                 `json:"type" yaml:"type" validate:"oneof=json xml text"`
	JSONPath map[string]string      `json:"json_path,omitempty" yaml:"json_path,omitempty"`
	Fields   map[string]interface{} `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// MQTTSubConfig represents MQTT subscriber adapter configuration
type MQTTSubConfig struct {
	AdapterConfig `json:",inline" yaml:",inline"`
	Broker        string            `json:"broker" yaml:"broker" validate:"required,url"`
	ClientID      string            `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	Username      string            `json:"username,omitempty" yaml:"username,omitempty"`
	Password      string            `json:"password,omitempty" yaml:"password,omitempty"`
	Topics        []MQTTTopicConfig `json:"topics" yaml:"topics" validate:"required,min=1"`
	DefaultQoS    byte              `json:"default_qos,omitempty" yaml:"default_qos,omitempty" validate:"max=2"`
	TLS           *TLSConfig        `json:"tls,omitempty" yaml:"tls,omitempty"`
}

// MQTTTopicConfig represents MQTT topic subscription configuration
type MQTTTopicConfig struct {
	Topic     string                     `json:"topic" yaml:"topic" validate:"required"`
	QoS       byte                       `json:"qos,omitempty" yaml:"qos,omitempty" validate:"max=2"`
	Key       string                     `json:"key" yaml:"key" validate:"required"`
	Type      string                     `json:"type,omitempty" yaml:"type,omitempty" validate:"oneof=int float bool string location vector3d color"`
	Path      string                     `json:"path,omitempty" yaml:"path,omitempty"`
	DeviceID  string                     `json:"device_id,omitempty" yaml:"device_id,omitempty"`
	Tags      map[string]string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Composite *MQTTCompositeConfig       `json:"composite,omitempty" yaml:"composite,omitempty"`
}

// MQTTCompositeConfig MQTT复合数据提取配置
type MQTTCompositeConfig struct {
	Location *MQTTLocationConfig `json:"location,omitempty" yaml:"location,omitempty"`
	Vector3D *MQTTVector3DConfig `json:"vector3d,omitempty" yaml:"vector3d,omitempty"`
	Color    *MQTTColorConfig    `json:"color,omitempty" yaml:"color,omitempty"`
}

// MQTTLocationConfig MQTT GPS位置提取配置
type MQTTLocationConfig struct {
	LatitudePath  string `json:"latitude_path" yaml:"latitude_path" validate:"required"`
	LongitudePath string `json:"longitude_path" yaml:"longitude_path" validate:"required"`
	AltitudePath  string `json:"altitude_path,omitempty" yaml:"altitude_path,omitempty"`
	AccuracyPath  string `json:"accuracy_path,omitempty" yaml:"accuracy_path,omitempty"`
	SpeedPath     string `json:"speed_path,omitempty" yaml:"speed_path,omitempty"`
	HeadingPath   string `json:"heading_path,omitempty" yaml:"heading_path,omitempty"`
}

// MQTTVector3DConfig MQTT 3D向量提取配置
type MQTTVector3DConfig struct {
	XPath string `json:"x_path" yaml:"x_path" validate:"required"`
	YPath string `json:"y_path" yaml:"y_path" validate:"required"`
	ZPath string `json:"z_path" yaml:"z_path" validate:"required"`
}

// MQTTColorConfig MQTT颜色提取配置
type MQTTColorConfig struct {
	RedPath   string `json:"red_path" yaml:"red_path" validate:"required"`
	GreenPath string `json:"green_path" yaml:"green_path" validate:"required"`
	BluePath  string `json:"blue_path" yaml:"blue_path" validate:"required"`
	AlphaPath string `json:"alpha_path,omitempty" yaml:"alpha_path,omitempty"`
}

// TLSConfig represents TLS configuration for secure connections
type TLSConfig struct {
	CACert     string `json:"ca_cert,omitempty" yaml:"ca_cert,omitempty"`
	ClientCert string `json:"client_cert,omitempty" yaml:"client_cert,omitempty"`
	ClientKey  string `json:"client_key,omitempty" yaml:"client_key,omitempty"`
	SkipVerify bool   `json:"skip_verify,omitempty" yaml:"skip_verify,omitempty"`
}

// MockConfig represents Mock adapter configuration
type MockConfig struct {
	AdapterConfig `json:",inline" yaml:",inline"`
	DeviceCount   int                    `json:"device_count,omitempty" yaml:"device_count,omitempty" validate:"min=1,max=1000"`
	DataPoints    []MockDataPoint        `json:"data_points" yaml:"data_points" validate:"required,min=1"`
	Pattern       string                 `json:"pattern,omitempty" yaml:"pattern,omitempty" validate:"oneof=sequential random sine"`
}

// MockDataPoint represents a mock data point configuration
type MockDataPoint struct {
	DeviceID string  `json:"device_id" yaml:"device_id" validate:"required"`
	Key      string  `json:"key" yaml:"key" validate:"required"`
	MinValue float64 `json:"min_value,omitempty" yaml:"min_value,omitempty"`
	MaxValue float64 `json:"max_value,omitempty" yaml:"max_value,omitempty"`
	Unit     string  `json:"unit,omitempty" yaml:"unit,omitempty"`
	
	// 复合数据类型配置
	DataType         string                  `json:"data_type,omitempty" yaml:"data_type,omitempty"`
	LocationConfig   *MockLocationConfig     `json:"location_config,omitempty" yaml:"location_config,omitempty"`
	Vector3DConfig   *MockVector3DConfig     `json:"vector3d_config,omitempty" yaml:"vector3d_config,omitempty"`
	ColorConfig      *MockColorConfig        `json:"color_config,omitempty" yaml:"color_config,omitempty"`
	
	// 通用复合数据类型配置
	VectorConfig     *MockVectorConfig       `json:"vector_config,omitempty" yaml:"vector_config,omitempty"`
	ArrayConfig      *MockArrayConfig        `json:"array_config,omitempty" yaml:"array_config,omitempty"`
	MatrixConfig     *MockMatrixConfig       `json:"matrix_config,omitempty" yaml:"matrix_config,omitempty"`
	TimeSeriesConfig *MockTimeSeriesConfig   `json:"timeseries_config,omitempty" yaml:"timeseries_config,omitempty"`
}

// MockLocationConfig GPS/地理位置模拟配置
type MockLocationConfig struct {
	StartLatitude   float64 `json:"start_latitude" yaml:"start_latitude" validate:"min=-90,max=90"`
	StartLongitude  float64 `json:"start_longitude" yaml:"start_longitude" validate:"min=-180,max=180"`
	LatitudeRange   float64 `json:"latitude_range,omitempty" yaml:"latitude_range,omitempty"`    // 纬度变化范围
	LongitudeRange  float64 `json:"longitude_range,omitempty" yaml:"longitude_range,omitempty"`  // 经度变化范围
	AltitudeMin     float64 `json:"altitude_min,omitempty" yaml:"altitude_min,omitempty"`
	AltitudeMax     float64 `json:"altitude_max,omitempty" yaml:"altitude_max,omitempty"`
	SpeedMin        float64 `json:"speed_min,omitempty" yaml:"speed_min,omitempty"`
	SpeedMax        float64 `json:"speed_max,omitempty" yaml:"speed_max,omitempty"`
	SimulateMovement bool   `json:"simulate_movement,omitempty" yaml:"simulate_movement,omitempty"`
	MovementPattern string  `json:"movement_pattern,omitempty" yaml:"movement_pattern,omitempty" validate:"oneof=random_walk circular linear"`
}

// MockVector3DConfig 三轴向量模拟配置
type MockVector3DConfig struct {
	XMin          float64 `json:"x_min,omitempty" yaml:"x_min,omitempty"`
	XMax          float64 `json:"x_max,omitempty" yaml:"x_max,omitempty"`
	YMin          float64 `json:"y_min,omitempty" yaml:"y_min,omitempty"`
	YMax          float64 `json:"y_max,omitempty" yaml:"y_max,omitempty"`
	ZMin          float64 `json:"z_min,omitempty" yaml:"z_min,omitempty"`
	ZMax          float64 `json:"z_max,omitempty" yaml:"z_max,omitempty"`
	Correlation   float64 `json:"correlation,omitempty" yaml:"correlation,omitempty" validate:"min=0,max=1"`     // 轴间相关性
	Oscillation   bool    `json:"oscillation,omitempty" yaml:"oscillation,omitempty"`                          // 是否模拟振荡
	Frequency     float64 `json:"frequency,omitempty" yaml:"frequency,omitempty"`                              // 振荡频率
}

// MockColorConfig 颜色模拟配置
type MockColorConfig struct {
	ColorMode        string   `json:"color_mode,omitempty" yaml:"color_mode,omitempty" validate:"oneof=random rainbow gradient fixed"`
	FixedColors      []string `json:"fixed_colors,omitempty" yaml:"fixed_colors,omitempty"`      // 固定颜色列表 (hex格式)
	BrightnessRange  [2]uint8 `json:"brightness_range,omitempty" yaml:"brightness_range,omitempty"` // 亮度范围 [min, max]
	SaturationRange  [2]uint8 `json:"saturation_range,omitempty" yaml:"saturation_range,omitempty"` // 饱和度范围
	HueChangeSpeed   float64  `json:"hue_change_speed,omitempty" yaml:"hue_change_speed,omitempty"`  // 色相变化速度
}

// MockVectorConfig 通用向量模拟配置（支持任意维度）
type MockVectorConfig struct {
	Dimension      int       `json:"dimension" yaml:"dimension" validate:"min=1,max=1000"`             // 向量维度
	MinValues      []float64 `json:"min_values,omitempty" yaml:"min_values,omitempty"`                 // 各维度最小值
	MaxValues      []float64 `json:"max_values,omitempty" yaml:"max_values,omitempty"`                 // 各维度最大值
	Labels         []string  `json:"labels,omitempty" yaml:"labels,omitempty"`                         // 维度标签
	GlobalMin      float64   `json:"global_min,omitempty" yaml:"global_min,omitempty"`                 // 全局最小值（如果不指定各维度）
	GlobalMax      float64   `json:"global_max,omitempty" yaml:"global_max,omitempty"`                 // 全局最大值
	Correlation    float64   `json:"correlation,omitempty" yaml:"correlation,omitempty" validate:"min=0,max=1"` // 维度间相关性
	Distribution   string    `json:"distribution,omitempty" yaml:"distribution,omitempty" validate:"oneof=uniform normal exponential"` // 分布类型
	ChangePattern  string    `json:"change_pattern,omitempty" yaml:"change_pattern,omitempty" validate:"oneof=random walk oscillate"` // 变化模式
	Unit           string    `json:"unit,omitempty" yaml:"unit,omitempty"`                             // 单位
}

// MockArrayConfig 数组模拟配置
type MockArrayConfig struct {
	Size             int         `json:"size" yaml:"size" validate:"min=1,max=10000"`                    // 数组大小
	ElementType      string      `json:"element_type" yaml:"element_type" validate:"oneof=int float string bool mixed"` // 元素类型
	MinValue         float64     `json:"min_value,omitempty" yaml:"min_value,omitempty"`                 // 数值元素最小值
	MaxValue         float64     `json:"max_value,omitempty" yaml:"max_value,omitempty"`                 // 数值元素最大值
	StringOptions    []string    `json:"string_options,omitempty" yaml:"string_options,omitempty"`       // 字符串选项（当元素类型为string时）
	BoolProbability  float64     `json:"bool_probability,omitempty" yaml:"bool_probability,omitempty" validate:"min=0,max=1"` // true的概率
	NullProbability  float64     `json:"null_probability,omitempty" yaml:"null_probability,omitempty" validate:"min=0,max=1"` // null的概率
	Labels           []string    `json:"labels,omitempty" yaml:"labels,omitempty"`                       // 元素标签
	ChangeElements   int         `json:"change_elements,omitempty" yaml:"change_elements,omitempty"`     // 每次改变的元素数量
	Unit             string      `json:"unit,omitempty" yaml:"unit,omitempty"`                           // 单位
}

// MockMatrixConfig 矩阵模拟配置
type MockMatrixConfig struct {
	Rows         int     `json:"rows" yaml:"rows" validate:"min=1,max=1000"`                      // 行数
	Cols         int     `json:"cols" yaml:"cols" validate:"min=1,max=1000"`                      // 列数
	MinValue     float64 `json:"min_value,omitempty" yaml:"min_value,omitempty"`                  // 元素最小值
	MaxValue     float64 `json:"max_value,omitempty" yaml:"max_value,omitempty"`                  // 元素最大值
	MatrixType   string  `json:"matrix_type,omitempty" yaml:"matrix_type,omitempty" validate:"oneof=general diagonal identity symmetric"` // 矩阵类型
	Distribution string  `json:"distribution,omitempty" yaml:"distribution,omitempty" validate:"oneof=uniform normal"` // 分布类型
	Sparsity     float64 `json:"sparsity,omitempty" yaml:"sparsity,omitempty" validate:"min=0,max=1"` // 稀疏度（0为稠密，1为全零）
	Unit         string  `json:"unit,omitempty" yaml:"unit,omitempty"`                            // 单位
}

// MockTimeSeriesConfig 时间序列模拟配置
type MockTimeSeriesConfig struct {
	Length          int           `json:"length" yaml:"length" validate:"min=2,max=10000"`                // 数据点数量
	StartTime       string        `json:"start_time,omitempty" yaml:"start_time,omitempty"`               // 开始时间（RFC3339格式）
	Interval        string        `json:"interval" yaml:"interval" validate:"required"`                   // 采样间隔
	BaseValue       float64       `json:"base_value,omitempty" yaml:"base_value,omitempty"`               // 基准值
	Trend           float64       `json:"trend,omitempty" yaml:"trend,omitempty"`                         // 趋势斜率（每个时间单位的变化）
	Seasonality     *SeasonalityConfig `json:"seasonality,omitempty" yaml:"seasonality,omitempty"`        // 季节性配置
	Noise           float64       `json:"noise,omitempty" yaml:"noise,omitempty"`                         // 噪声水平（标准差）
	Anomalies       *AnomaliesConfig   `json:"anomalies,omitempty" yaml:"anomalies,omitempty"`            // 异常值配置
	Unit            string        `json:"unit,omitempty" yaml:"unit,omitempty"`                           // 单位
}

// SeasonalityConfig 季节性配置
type SeasonalityConfig struct {
	Period    string  `json:"period" yaml:"period" validate:"required"`                       // 周期（如"24h"表示日周期）
	Amplitude float64 `json:"amplitude" yaml:"amplitude"`                                     // 振幅
	Phase     float64 `json:"phase,omitempty" yaml:"phase,omitempty"`                         // 相位偏移（弧度）
}

// AnomaliesConfig 异常值配置
type AnomaliesConfig struct {
	Probability float64 `json:"probability" yaml:"probability" validate:"min=0,max=1"`        // 异常值出现概率
	Magnitude   float64 `json:"magnitude" yaml:"magnitude"`                                   // 异常值的倍数（相对于正常范围）
	Duration    int     `json:"duration,omitempty" yaml:"duration,omitempty"`                 // 异常持续的数据点数
}

// MQTTSinkConfig represents MQTT sink configuration
type MQTTSinkConfig struct {
	SinkConfig `json:",inline" yaml:",inline"`
	Broker     string `json:"broker" yaml:"broker" validate:"required,url"`
	ClientID   string `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	Username   string `json:"username,omitempty" yaml:"username,omitempty"`
	Password   string `json:"password,omitempty" yaml:"password,omitempty"`
	Topic      string `json:"topic" yaml:"topic" validate:"required"`
	QoS        byte   `json:"qos,omitempty" yaml:"qos,omitempty" validate:"max=2"`
	Retain     bool   `json:"retain,omitempty" yaml:"retain,omitempty"`
}

// InfluxDBConfig represents InfluxDB sink configuration
type InfluxDBConfig struct {
	SinkConfig    `json:",inline" yaml:",inline"`
	URL           string                 `json:"url" yaml:"url" validate:"required,url"`
	Token         string                 `json:"token" yaml:"token" validate:"required"`
	Org           string                 `json:"org" yaml:"org" validate:"required"`
	Bucket        string                 `json:"bucket" yaml:"bucket" validate:"required"`
	Precision     string                 `json:"precision,omitempty" yaml:"precision,omitempty" validate:"oneof=ns us ms s"`
	FlushInterval int                    `json:"flush_interval_ms,omitempty" yaml:"flush_interval_ms,omitempty" validate:"min=100"`
	Points        map[string]PointConfig `json:"points,omitempty" yaml:"points,omitempty"`
}

// PointConfig represents InfluxDB point configuration
type PointConfig struct {
	Measurement string            `json:"measurement" yaml:"measurement" validate:"required"`
	Tags        map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Fields      map[string]string `json:"fields,omitempty" yaml:"fields,omitempty"`
}

// RedisConfig represents Redis sink configuration
type RedisConfig struct {
	SinkConfig `json:",inline" yaml:",inline"`
	Host       string `json:"host" yaml:"host" validate:"required"`
	Port       int    `json:"port" yaml:"port" validate:"port"`
	Password   string `json:"password,omitempty" yaml:"password,omitempty"`
	DB         int    `json:"db,omitempty" yaml:"db,omitempty" validate:"min=0,max=15"`
	KeyPrefix  string `json:"key_prefix,omitempty" yaml:"key_prefix,omitempty"`
	Expiration int    `json:"expiration,omitempty" yaml:"expiration,omitempty" validate:"min=0"`
}

// ConsoleConfig represents Console sink configuration
type ConsoleConfig struct {
	SinkConfig `json:",inline" yaml:",inline"`
	Format     string `json:"format,omitempty" yaml:"format,omitempty" validate:"oneof=json table plain"`
	Colors     bool   `json:"colors,omitempty" yaml:"colors,omitempty"`
}

// WebSocketConfig represents WebSocket sink configuration
type WebSocketConfig struct {
	SinkConfig `json:",inline" yaml:",inline"`
	Port       int    `json:"port" yaml:"port" validate:"port"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	Origins    []string `json:"origins,omitempty" yaml:"origins,omitempty"`
}

// JetStreamConfig represents JetStream sink configuration
type JetStreamConfig struct {
	SinkConfig `json:",inline" yaml:",inline"`
	Stream     string `json:"stream" yaml:"stream" validate:"required"`
	Subject    string `json:"subject" yaml:"subject" validate:"required"`
	Durable    string `json:"durable,omitempty" yaml:"durable,omitempty"`
}

// GetDefaults returns default values for each configuration type
func GetDefaultModbusConfig() ModbusConfig {
	return ModbusConfig{
		AdapterConfig: AdapterConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			Interval: Duration(5 * time.Second),
			Timeout:  Duration(3 * time.Second),
		},
		Port:     502,
		SlaveID:  1,
		Protocol: "tcp",
	}
}

func GetDefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		AdapterConfig: AdapterConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			Interval: Duration(10 * time.Second),
			Timeout:  Duration(5 * time.Second),
		},
		Method: "GET",
		Parser: HTTPParser{
			Type: "json",
		},
	}
}

func GetDefaultMQTTSubConfig() MQTTSubConfig {
	return MQTTSubConfig{
		AdapterConfig: AdapterConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
		},
		DefaultQoS: 0,
	}
}

func GetDefaultMockConfig() MockConfig {
	return MockConfig{
		AdapterConfig: AdapterConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			Interval: Duration(1 * time.Second),
		},
		DeviceCount: 1,
		Pattern:     "random",
	}
}

func GetDefaultMQTTSinkConfig() MQTTSinkConfig {
	return MQTTSinkConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:    1,
			BufferSize:   1000,
			FlushTimeout: Duration(5 * time.Second),
		},
		QoS:    0,
		Retain: false,
	}
}

func GetDefaultInfluxDBConfig() InfluxDBConfig {
	return InfluxDBConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:    100,
			BufferSize:   10000,
			FlushTimeout: Duration(10 * time.Second),
		},
		Precision:     "ms",
		FlushInterval: 1000,
	}
}

func GetDefaultRedisConfig() RedisConfig {
	return RedisConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:    10,
			BufferSize:   1000,
			FlushTimeout: Duration(5 * time.Second),
		},
		Port: 6379,
		DB:   0,
	}
}

func GetDefaultConsoleConfig() ConsoleConfig {
	return ConsoleConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:  1,
			BufferSize: 100,
		},
		Format: "json",
		Colors: true,
	}
}

func GetDefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:  1,
			BufferSize: 1000,
		},
		Port: 8083,
		Path: "/ws",
	}
}

func GetDefaultJetStreamConfig() JetStreamConfig {
	return JetStreamConfig{
		SinkConfig: SinkConfig{
			BaseConfig: BaseConfig{
				Enabled: true,
			},
			BatchSize:    100,
			BufferSize:   10000,
			FlushTimeout: Duration(5 * time.Second),
		},
	}
}