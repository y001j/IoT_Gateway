package northbound

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// ToInt64 将常见数值类型转换为 int64。
// 适配器可能产生 int/int8/.../uint64/float32/float64/json.Number/数字字符串 等各种类型，
// 以前各个 Sink 只处理 int 和 float64，导致 Modbus 产生的 int64/uint64 被写成 0 或整批写入失败。
func ToInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return math.MaxInt64, true
		}
		return int64(n), true
	case float32:
		return int64(n), true
	case float64:
		return int64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(f), true
		}
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

// ToFloat64 将常见数值类型转换为 float64
func ToFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// NormalizeValue 按数据点声明的类型规范化值，供各 Sink 输出使用。
// ok=false 表示值与声明的类型不兼容（调用方可决定跳过或按字符串输出）。
// 复合类型（location/vector3d/...）原样返回，由 JSON 序列化处理。
func NormalizeValue(point model.Point) (interface{}, bool) {
	switch point.Type {
	case model.TypeInt:
		return ToInt64(point.Value)
	case model.TypeFloat:
		return ToFloat64(point.Value)
	case model.TypeBool:
		switch v := point.Value.(type) {
		case bool:
			return v, true
		case string:
			b, err := strconv.ParseBool(v)
			return b, err == nil
		default:
			if f, ok := ToFloat64(v); ok {
				return f != 0, true
			}
			return false, false
		}
	case model.TypeString:
		if s, ok := point.Value.(string); ok {
			return s, true
		}
		return fmt.Sprintf("%v", point.Value), true
	default:
		if point.Value == nil {
			return nil, false
		}
		return point.Value, true
	}
}

// NormalizeValueOrZero 与 NormalizeValue 相同，但类型不兼容时返回该类型的零值
func NormalizeValueOrZero(point model.Point) interface{} {
	v, ok := NormalizeValue(point)
	if ok {
		return v
	}
	switch point.Type {
	case model.TypeInt:
		return int64(0)
	case model.TypeFloat:
		return 0.0
	case model.TypeBool:
		return false
	case model.TypeString:
		return ""
	}
	return v
}

// PointTime 返回数据点的采集时间；零值时回退为当前时间
func PointTime(point model.Point) time.Time {
	if point.Timestamp.IsZero() {
		return time.Now()
	}
	return point.Timestamp
}
