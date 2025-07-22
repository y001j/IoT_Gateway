package rules

import (
	"fmt"
	"time"
)

// SafeValueForJSON 安全地转换值用于JSON序列化
func SafeValueForJSON(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	
	// 处理各种类型
	switch v := value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return v
	case error:
		if v == nil {
			return nil
		}
		return v.Error()
	case time.Time:
		return v
	case []byte:
		return string(v)
	case map[string]interface{}:
		// 递归处理嵌套map
		result := make(map[string]interface{})
		for k, val := range v {
			result[k] = SafeValueForJSON(val)
		}
		return result
	case []interface{}:
		// 递归处理数组
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = SafeValueForJSON(val)
		}
		return result
	default:
		// 对于其他类型，尝试转换为字符串
		return fmt.Sprintf("%v", v)
	}
}