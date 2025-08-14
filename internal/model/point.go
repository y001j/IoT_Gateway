package model

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
	
	"github.com/y001j/iot-gateway/internal/utils"
)

// DataType 表示点位数据的类型
type DataType string

const (
	// 基础数据类型
	TypeInt    DataType = "int"
	TypeFloat  DataType = "float"
	TypeBool   DataType = "bool"
	TypeString DataType = "string"
	TypeBinary DataType = "binary"
	
	// 复合数据类型
	TypeLocation DataType = "location"  // GPS/地理位置
	TypeVector3D DataType = "vector3d"  // 三轴向量数据
	TypeColor    DataType = "color"     // 颜色数据
	
	// 通用复合数据类型
	TypeVector   DataType = "vector"    // 通用向量（任意维度）
	TypeArray    DataType = "array"     // 数组数据
	TypeMatrix   DataType = "matrix"    // 矩阵数据
	TypeTimeSeries DataType = "timeseries" // 时间序列数据
)

// CompositeData 复合数据接口
type CompositeData interface {
	Type() DataType
	Validate() error
	GetDerivedValues() map[string]interface{}
}

// LocationData GPS/地理位置数据
type LocationData struct {
	Latitude  float64 `json:"latitude"`            // 纬度 (-90 ~ 90)
	Longitude float64 `json:"longitude"`           // 经度 (-180 ~ 180)
	Altitude  float64 `json:"altitude,omitempty"`  // 海拔 (米，可选)
	Accuracy  float64 `json:"accuracy,omitempty"`  // GPS精度 (米，可选)
	Speed     float64 `json:"speed,omitempty"`     // 移动速度 (km/h，可选)
	Heading   float64 `json:"heading,omitempty"`   // 方向角 (度，可选)
}

func (l *LocationData) Type() DataType {
	return TypeLocation
}

func (l *LocationData) Validate() error {
	if l.Latitude < -90 || l.Latitude > 90 {
		return fmt.Errorf("latitude out of range: %f", l.Latitude)
	}
	if l.Longitude < -180 || l.Longitude > 180 {
		return fmt.Errorf("longitude out of range: %f", l.Longitude)
	}
	return nil
}

func (l *LocationData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	derived["coordinate_system"] = "WGS84"
	derived["has_altitude"] = l.Altitude != 0
	derived["has_speed"] = l.Speed != 0
	derived["has_heading"] = l.Heading != 0
	
	// 计算基本的地理信息
	if l.Altitude != 0 {
		derived["elevation_category"] = categorizeElevation(l.Altitude)
	}
	if l.Speed > 0 {
		derived["speed_category"] = categorizeSpeed(l.Speed)
	}
	
	return derived
}

// Vector3D 三轴向量数据
type Vector3D struct {
	X float64 `json:"x"` // X轴数值
	Y float64 `json:"y"` // Y轴数值
	Z float64 `json:"z"` // Z轴数值
}

func (v *Vector3D) Type() DataType {
	return TypeVector3D
}

func (v *Vector3D) Validate() error {
	// 检查是否为有效数值
	if math.IsNaN(v.X) || math.IsInf(v.X, 0) ||
	   math.IsNaN(v.Y) || math.IsInf(v.Y, 0) ||
	   math.IsNaN(v.Z) || math.IsInf(v.Z, 0) {
		return fmt.Errorf("invalid vector values: X=%f, Y=%f, Z=%f", v.X, v.Y, v.Z)
	}
	return nil
}

func (v *Vector3D) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	// 计算向量模长
	magnitude := math.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
	derived["magnitude"] = magnitude
	
	// 计算各轴分量的比例
	if magnitude > 0 {
		derived["x_ratio"] = v.X / magnitude
		derived["y_ratio"] = v.Y / magnitude
		derived["z_ratio"] = v.Z / magnitude
	}
	
	// 计算主导轴
	absX, absY, absZ := math.Abs(v.X), math.Abs(v.Y), math.Abs(v.Z)
	if absX >= absY && absX >= absZ {
		derived["dominant_axis"] = "x"
	} else if absY >= absZ {
		derived["dominant_axis"] = "y"
	} else {
		derived["dominant_axis"] = "z"
	}
	
	return derived
}

// ColorData 颜色数据
type ColorData struct {
	R uint8 `json:"r"` // 红色分量 (0-255)
	G uint8 `json:"g"` // 绿色分量 (0-255)
	B uint8 `json:"b"` // 蓝色分量 (0-255)
	A uint8 `json:"a"` // 透明度 (0-255，可选)
}

func (c *ColorData) Type() DataType {
	return TypeColor
}

func (c *ColorData) Validate() error {
	// RGB值自动在0-255范围内，无需额外验证
	return nil
}

func (c *ColorData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	// 转换为0-1范围的浮点数
	r := float64(c.R) / 255.0
	g := float64(c.G) / 255.0
	b := float64(c.B) / 255.0
	
	// 计算HSV值
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	diff := max - min
	
	// 亮度 (Lightness)
	lightness := (max + min) / 2.0
	derived["lightness"] = lightness
	
	// 饱和度 (Saturation)
	var saturation float64
	if diff == 0 {
		saturation = 0
	} else {
		if lightness > 0.5 {
			saturation = diff / (2.0 - max - min)
		} else {
			saturation = diff / (max + min)
		}
	}
	derived["saturation"] = saturation
	
	// 色相 (Hue)
	var hue float64
	if diff == 0 {
		hue = 0
	} else if max == r {
		hue = 60 * ((g - b) / diff)
		if hue < 0 {
			hue += 360
		}
	} else if max == g {
		hue = 60 * (2.0 + (b - r) / diff)
	} else {
		hue = 60 * (4.0 + (r - g) / diff)
	}
	derived["hue"] = hue
	
	return derived
}

// VectorData 通用向量数据（支持任意维度）
type VectorData struct {
	Values     []float64 `json:"values"`               // 向量分量值
	Dimension  int       `json:"dimension"`            // 向量维度
	Labels     []string  `json:"labels,omitempty"`     // 维度标签（可选）
	Unit       string    `json:"unit,omitempty"`       // 单位
}

func (v *VectorData) Type() DataType {
	return TypeVector
}

func (v *VectorData) Validate() error {
	if len(v.Values) == 0 {
		return fmt.Errorf("向量不能为空")
	}
	if v.Dimension <= 0 {
		v.Dimension = len(v.Values) // 自动设置维度
	}
	if v.Dimension != len(v.Values) {
		return fmt.Errorf("向量维度不匹配: 声明维度=%d, 实际值数量=%d", v.Dimension, len(v.Values))
	}
	if len(v.Labels) > 0 && len(v.Labels) != v.Dimension {
		return fmt.Errorf("标签数量与维度不匹配")
	}
	
	// 检查数值有效性
	for i, val := range v.Values {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return fmt.Errorf("向量第%d维包含无效数值: %f", i, val)
		}
	}
	
	return nil
}

func (v *VectorData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	// 基本统计信息
	derived["dimension"] = v.Dimension
	derived["length"] = len(v.Values)
	
	if len(v.Values) > 0 {
		// 计算模长/范数
		sumSquares := 0.0
		for _, val := range v.Values {
			sumSquares += val * val
		}
		magnitude := math.Sqrt(sumSquares)
		derived["magnitude"] = magnitude
		derived["norm"] = magnitude
		
		// 统计信息
		min := v.Values[0]
		max := v.Values[0]
		sum := 0.0
		
		for _, val := range v.Values {
			if val < min {
				min = val
			}
			if val > max {
				max = val
			}
			sum += val
		}
		
		derived["min"] = min
		derived["max"] = max
		derived["sum"] = sum
		derived["mean"] = sum / float64(len(v.Values))
		derived["range"] = max - min
		
		// 主导维度（绝对值最大的维度）
		maxAbsIndex := 0
		maxAbsValue := math.Abs(v.Values[0])
		for i := 1; i < len(v.Values); i++ {
			absVal := math.Abs(v.Values[i])
			if absVal > maxAbsValue {
				maxAbsValue = absVal
				maxAbsIndex = i
			}
		}
		derived["dominant_dimension"] = maxAbsIndex
		derived["dominant_value"] = v.Values[maxAbsIndex]
		
		// 如果有标签，使用标签名
		if len(v.Labels) > maxAbsIndex {
			derived["dominant_label"] = v.Labels[maxAbsIndex]
		}
	}
	
	return derived
}

// ArrayData 数组数据类型
type ArrayData struct {
	Values    []interface{} `json:"values"`              // 数组值
	DataType  string        `json:"data_type"`           // 元素数据类型
	Size      int           `json:"size"`                // 数组大小
	Unit      string        `json:"unit,omitempty"`      // 单位
	Labels    []string      `json:"labels,omitempty"`    // 元素标签
}

func (a *ArrayData) Type() DataType {
	return TypeArray
}

func (a *ArrayData) Validate() error {
	if len(a.Values) == 0 {
		return fmt.Errorf("数组不能为空")
	}
	if a.Size <= 0 {
		a.Size = len(a.Values) // 自动设置大小
	}
	if a.Size != len(a.Values) {
		return fmt.Errorf("数组大小不匹配: 声明大小=%d, 实际值数量=%d", a.Size, len(a.Values))
	}
	if len(a.Labels) > 0 && len(a.Labels) != a.Size {
		return fmt.Errorf("标签数量与数组大小不匹配")
	}
	
	return nil
}

func (a *ArrayData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	derived["size"] = a.Size
	derived["length"] = len(a.Values)
	derived["data_type"] = a.DataType
	
	if len(a.Values) > 0 {
		// 类型分布统计
		typeCount := make(map[string]int)
		numericValues := make([]float64, 0)
		
		for _, val := range a.Values {
			valType := fmt.Sprintf("%T", val)
			typeCount[valType]++
			
			// 如果是数值类型，收集用于统计
			if num, ok := toFloat64Value(val); ok {
				numericValues = append(numericValues, num)
			}
		}
		
		derived["type_distribution"] = typeCount
		derived["numeric_count"] = len(numericValues)
		derived["null_count"] = countNulls(a.Values)
		
		// 数值统计（如果包含数值）
		if len(numericValues) > 0 {
			min := numericValues[0]
			max := numericValues[0]
			sum := 0.0
			
			for _, val := range numericValues {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
				sum += val
			}
			
			derived["numeric_min"] = min
			derived["numeric_max"] = max
			derived["numeric_sum"] = sum
			derived["numeric_mean"] = sum / float64(len(numericValues))
			derived["numeric_range"] = max - min
		}
	}
	
	return derived
}

// MatrixData 矩阵数据类型
type MatrixData struct {
	Values [][]float64 `json:"values"`           // 矩阵值（行x列）
	Rows   int         `json:"rows"`             // 行数
	Cols   int         `json:"cols"`             // 列数
	Unit   string      `json:"unit,omitempty"`   // 单位
}

func (m *MatrixData) Type() DataType {
	return TypeMatrix
}

func (m *MatrixData) Validate() error {
	if len(m.Values) == 0 {
		return fmt.Errorf("矩阵不能为空")
	}
	
	if m.Rows <= 0 {
		m.Rows = len(m.Values)
	}
	if m.Cols <= 0 && len(m.Values) > 0 {
		m.Cols = len(m.Values[0])
	}
	
	if m.Rows != len(m.Values) {
		return fmt.Errorf("矩阵行数不匹配")
	}
	
	// 检查每行列数一致
	for i, row := range m.Values {
		if len(row) != m.Cols {
			return fmt.Errorf("矩阵第%d行列数不匹配: 期望%d列, 实际%d列", i, m.Cols, len(row))
		}
		
		// 检查数值有效性
		for j, val := range row {
			if math.IsNaN(val) || math.IsInf(val, 0) {
				return fmt.Errorf("矩阵[%d][%d]包含无效数值: %f", i, j, val)
			}
		}
	}
	
	return nil
}

func (m *MatrixData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	derived["rows"] = m.Rows
	derived["cols"] = m.Cols
	derived["size"] = m.Rows * m.Cols
	derived["is_square"] = m.Rows == m.Cols
	
	if len(m.Values) > 0 {
		// 统计信息
		min := m.Values[0][0]
		max := m.Values[0][0]
		sum := 0.0
		count := 0
		
		for _, row := range m.Values {
			for _, val := range row {
				if val < min {
					min = val
				}
				if val > max {
					max = val
				}
				sum += val
				count++
			}
		}
		
		derived["min"] = min
		derived["max"] = max
		derived["sum"] = sum
		derived["mean"] = sum / float64(count)
		derived["range"] = max - min
		
		// 矩阵特性
		if m.Rows == m.Cols {
			// 方阵的额外属性
			derived["trace"] = m.calculateTrace()
			derived["is_diagonal"] = m.isDiagonal()
			derived["is_identity"] = m.isIdentity()
		}
	}
	
	return derived
}

// calculateTrace 计算矩阵的迹（对角线元素之和）
func (m *MatrixData) calculateTrace() float64 {
	if m.Rows != m.Cols {
		return 0.0
	}
	
	trace := 0.0
	for i := 0; i < m.Rows; i++ {
		trace += m.Values[i][i]
	}
	return trace
}

// isDiagonal 检查是否为对角矩阵
func (m *MatrixData) isDiagonal() bool {
	if m.Rows != m.Cols {
		return false
	}
	
	for i := 0; i < m.Rows; i++ {
		for j := 0; j < m.Cols; j++ {
			if i != j && m.Values[i][j] != 0 {
				return false
			}
		}
	}
	return true
}

// isIdentity 检查是否为单位矩阵
func (m *MatrixData) isIdentity() bool {
	if !m.isDiagonal() {
		return false
	}
	
	for i := 0; i < m.Rows; i++ {
		if m.Values[i][i] != 1.0 {
			return false
		}
	}
	return true
}

// TimeSeriesData 时间序列数据类型
type TimeSeriesData struct {
	Timestamps []time.Time   `json:"timestamps"`      // 时间戳数组
	Values     []float64     `json:"values"`          // 对应的数值数组
	Unit       string        `json:"unit,omitempty"`  // 数值单位
	Interval   time.Duration `json:"interval,omitempty"` // 采样间隔
}

func (ts *TimeSeriesData) Type() DataType {
	return TypeTimeSeries
}

func (ts *TimeSeriesData) Validate() error {
	if len(ts.Timestamps) == 0 || len(ts.Values) == 0 {
		return fmt.Errorf("时间序列数据不能为空")
	}
	
	if len(ts.Timestamps) != len(ts.Values) {
		return fmt.Errorf("时间戳与数值数量不匹配: 时间戳=%d, 数值=%d", len(ts.Timestamps), len(ts.Values))
	}
	
	// 检查时间戳是否按顺序排列
	for i := 1; i < len(ts.Timestamps); i++ {
		if ts.Timestamps[i].Before(ts.Timestamps[i-1]) {
			return fmt.Errorf("时间戳顺序错误: 位置%d", i)
		}
	}
	
	// 检查数值有效性
	for i, val := range ts.Values {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return fmt.Errorf("时间序列第%d个数值无效: %f", i, val)
		}
	}
	
	return nil
}

func (ts *TimeSeriesData) GetDerivedValues() map[string]interface{} {
	derived := make(map[string]interface{})
	
	derived["length"] = len(ts.Values)
	derived["duration"] = ts.Timestamps[len(ts.Timestamps)-1].Sub(ts.Timestamps[0]).String()
	
	if len(ts.Values) > 1 {
		// 计算平均采样间隔
		totalDuration := ts.Timestamps[len(ts.Timestamps)-1].Sub(ts.Timestamps[0])
		avgInterval := totalDuration / time.Duration(len(ts.Timestamps)-1)
		derived["avg_interval"] = avgInterval.String()
		
		// 数值统计
		min := ts.Values[0]
		max := ts.Values[0]
		sum := 0.0
		
		for _, val := range ts.Values {
			if val < min {
				min = val
			}
			if val > max {
				max = val
			}
			sum += val
		}
		
		derived["min"] = min
		derived["max"] = max
		derived["sum"] = sum
		derived["mean"] = sum / float64(len(ts.Values))
		derived["range"] = max - min
		
		// 趋势分析（简单线性趋势）
		trend := ts.calculateLinearTrend()
		derived["trend_slope"] = trend
		if trend > 0.001 {
			derived["trend"] = "increasing"
		} else if trend < -0.001 {
			derived["trend"] = "decreasing"
		} else {
			derived["trend"] = "stable"
		}
	}
	
	return derived
}

// calculateLinearTrend 计算线性趋势斜率
func (ts *TimeSeriesData) calculateLinearTrend() float64 {
	n := len(ts.Values)
	if n < 2 {
		return 0.0
	}
	
	// 使用时间索引计算简单线性回归
	sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0
	
	for i, val := range ts.Values {
		x := float64(i)
		y := val
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	
	// 计算斜率: slope = (n*∑xy - ∑x*∑y) / (n*∑x² - (∑x)²)
	nf := float64(n)
	numerator := nf*sumXY - sumX*sumY
	denominator := nf*sumXX - sumX*sumX
	
	if denominator == 0 {
		return 0.0
	}
	
	return numerator / denominator
}

// 辅助函数
func categorizeElevation(altitude float64) string {
	if altitude < 0 {
		return "below_sea_level"
	} else if altitude < 500 {
		return "low_elevation"
	} else if altitude < 1500 {
		return "medium_elevation"
	} else {
		return "high_elevation"
	}
}

func categorizeSpeed(speed float64) string {
	if speed == 0 {
		return "stationary"
	} else if speed < 5 {
		return "very_slow"
	} else if speed < 30 {
		return "slow"
	} else if speed < 80 {
		return "moderate"
	} else {
		return "fast"
	}
}

// Point 表示从设备采集的一个数据点
type Point struct {
	// 点位唯一标识
	Key string `json:"key"`

	// 点位所属设备ID
	DeviceID string `json:"device_id"`

	// 采集时间戳
	Timestamp time.Time `json:"timestamp"`

	// 数据类型
	Type DataType `json:"type"`

	// 数据值，使用interface{}以支持多种类型
	Value interface{} `json:"value"`

	// 质量码，0表示正常，其他值表示异常
	Quality int `json:"quality"`

	// Go 1.24安全标签容器：高性能分片锁标签系统
	SafeTags *utils.ShardedTags `json:"-"`
}

// MarshalJSON 自定义JSON序列化，包含标签信息
func (p Point) MarshalJSON() ([]byte, error) {
	// 创建一个包含所有字段的临时结构体
	temp := struct {
		Key       string      `json:"key"`
		DeviceID  string      `json:"device_id"`
		Timestamp time.Time   `json:"timestamp"`
		Type      DataType    `json:"type"`
		Value     interface{} `json:"value"`
		Quality   int         `json:"quality"`
		Tags      map[string]string `json:"tags,omitempty"`
	}{
		Key:       p.Key,
		DeviceID:  p.DeviceID,
		Timestamp: p.Timestamp,
		Type:      p.Type,
		Value:     p.Value,
		Quality:   p.Quality,
	}
	
	// 添加标签信息（如果存在）
	if p.SafeTags != nil {
		tags := p.SafeTags.GetAll()
		if len(tags) > 0 {
			temp.Tags = tags
		}
	}
	
	return json.Marshal(temp)
}

// UnmarshalJSON 自定义JSON反序列化，恢复标签信息
func (p *Point) UnmarshalJSON(data []byte) error {
	// 创建一个包含所有字段的临时结构体
	temp := struct {
		Key       string      `json:"key"`
		DeviceID  string      `json:"device_id"`
		Timestamp time.Time   `json:"timestamp"`
		Type      DataType    `json:"type"`
		Value     interface{} `json:"value"`
		Quality   int         `json:"quality"`
		Tags      map[string]string `json:"tags,omitempty"`
	}{}
	
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	
	// 填充Point结构体
	p.Key = temp.Key
	p.DeviceID = temp.DeviceID
	p.Timestamp = temp.Timestamp
	p.Type = temp.Type
	p.Value = temp.Value
	p.Quality = temp.Quality
	
	// 恢复标签信息
	if len(temp.Tags) > 0 {
		p.SafeTags = utils.NewShardedTags(16)
		for k, v := range temp.Tags {
			p.SafeTags.Set(k, v)
		}
	}
	
	return nil
}

// NewPoint 创建一个新的数据点
func NewPoint(key, deviceID string, value interface{}, dataType DataType) Point {
	return Point{
		Key:       key,
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Type:      dataType,
		Value:     value,
		Quality:   0, // 默认为正常
		SafeTags:  utils.NewShardedTags(16),
	}
}

// NewCompositePoint 创建复合数据点
func NewCompositePoint(key, deviceID string, compositeData CompositeData) Point {
	point := Point{
		Key:       key,
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Type:      compositeData.Type(),
		Value:     compositeData,
		Quality:   0,
		SafeTags:  utils.NewShardedTags(16),
	}
	
	// 自动添加衍生值作为标签（使用SafeTags）
	if derivedValues := compositeData.GetDerivedValues(); derivedValues != nil {
		for k, v := range derivedValues {
			point.SafeTags.Set(k, fmt.Sprintf("%v", v))
		}
	}
	
	return point
}

// IsComposite 检查是否为复合数据点
func (p *Point) IsComposite() bool {
	switch p.Type {
	case TypeLocation, TypeVector3D, TypeColor, TypeVector, TypeArray, TypeMatrix, TypeTimeSeries:
		return true
	default:
		return false
	}
}

// GetCompositeData 获取复合数据
func (p *Point) GetCompositeData() (CompositeData, error) {
	if !p.IsComposite() {
		return nil, fmt.Errorf("point is not composite data type: %s", p.Type)
	}
	
	compositeData, ok := p.Value.(CompositeData)
	if !ok {
		return nil, fmt.Errorf("value is not composite data: %T", p.Value)
	}
	
	return compositeData, nil
}

// GetLocationData 获取地理位置数据
func (p *Point) GetLocationData() (*LocationData, error) {
	if p.Type != TypeLocation {
		return nil, fmt.Errorf("point is not location data type")
	}
	
	switch v := p.Value.(type) {
	case *LocationData:
		return v, nil
	case LocationData:
		return &v, nil
	default:
		return nil, fmt.Errorf("value is not LocationData: %T", v)
	}
}

// GetVector3DData 获取三轴向量数据
func (p *Point) GetVector3DData() (*Vector3D, error) {
	if p.Type != TypeVector3D {
		return nil, fmt.Errorf("point is not vector3d data type")
	}
	
	switch v := p.Value.(type) {
	case *Vector3D:
		return v, nil
	case Vector3D:
		return &v, nil
	default:
		return nil, fmt.Errorf("value is not Vector3D: %T", v)
	}
}

// AddTag 线程安全地添加标签
func (p *Point) AddTag(key, value string) {
	if p.SafeTags == nil {
		p.SafeTags = utils.NewShardedTags(16)
	}
	p.SafeTags.Set(key, value)
}

// GetTag 线程安全地获取标签值
func (p *Point) GetTag(key string) (string, bool) {
	if p.SafeTags == nil {
		return "", false
	}
	return p.SafeTags.Get(key)
}

// GetTagsCopy 线程安全地获取Tags的副本
func (p *Point) GetTagsCopy() map[string]string {
	if p.SafeTags == nil {
		return make(map[string]string)
	}
	return p.SafeTags.GetAll()
}

// SetTagsSafe 线程安全地设置整个Tags map
func (p *Point) SetTagsSafe(tags map[string]string) {
	if p.SafeTags == nil {
		p.SafeTags = utils.NewShardedTags(16)
	}
	p.SafeTags.SetMultiple(tags)
}

// SetQuality 设置质量码
func (p *Point) SetQuality(quality int) {
	p.Quality = quality
}

// GetTagsSafe 安全智能标签访问器（推荐使用） - 别名到GetTagsCopy
func (p *Point) GetTagsSafe() map[string]string {
	return p.GetTagsCopy()
}

// 通用复合数据类型的辅助函数

// toFloat64Value 尝试将interface{}转换为float64
func toFloat64Value(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// countNulls 计算数组中null值的数量
func countNulls(values []interface{}) int {
	count := 0
	for _, v := range values {
		if v == nil {
			count++
		}
	}
	return count
}
