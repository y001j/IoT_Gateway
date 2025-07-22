package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
)

func main() {
	// 设置日志级别为debug
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// 测试不同类型的数据点
	testInt()
	testFloat()
	testBool()
	testString()
	testNatsJSONSerialization()
}

func testInt() {
	fmt.Println("\n=== 测试整数类型 ===")
	
	// 创建整数类型的数据点
	intValue := 42
	point := model.NewPoint("test_int", "test_device", intValue, model.TypeInt)
	
	// 检查类型
	checkType(point)
	
	// 测试JSON序列化
	testJSONSerialization(point)
}

func testFloat() {
	fmt.Println("\n=== 测试浮点类型 ===")
	
	// 创建浮点类型的数据点
	floatValue := 42.5
	point := model.NewPoint("test_float", "test_device", floatValue, model.TypeFloat)
	
	// 检查类型
	checkType(point)
	
	// 测试JSON序列化
	testJSONSerialization(point)
}

func testBool() {
	fmt.Println("\n=== 测试布尔类型 ===")
	
	// 创建布尔类型的数据点
	boolValue := true
	point := model.NewPoint("test_bool", "test_device", boolValue, model.TypeBool)
	
	// 检查类型
	checkType(point)
	
	// 测试JSON序列化
	testJSONSerialization(point)
}

func testString() {
	fmt.Println("\n=== 测试字符串类型 ===")
	
	// 创建字符串类型的数据点
	stringValue := "test_string"
	point := model.NewPoint("test_string", "test_device", stringValue, model.TypeString)
	
	// 检查类型
	checkType(point)
	
	// 测试JSON序列化
	testJSONSerialization(point)
}

func checkType(point model.Point) {
	fmt.Printf("数据点类型: %s\n", point.Type)
	
	// 检查值的实际Go类型
	switch v := point.Value.(type) {
	case int:
		fmt.Printf("值 %v 的Go类型: int\n", v)
	case float64:
		fmt.Printf("值 %v 的Go类型: float64\n", v)
	case bool:
		fmt.Printf("值 %v 的Go类型: bool\n", v)
	case string:
		fmt.Printf("值 %v 的Go类型: string\n", v)
	default:
		fmt.Printf("值 %v 的Go类型: %T\n", v, v)
	}
}

func testJSONSerialization(point model.Point) {
	// 序列化整个数据点
	fullJSON, err := json.Marshal(point)
	if err != nil {
		fmt.Printf("序列化数据点失败: %v\n", err)
		return
	}
	fmt.Printf("完整数据点JSON: %s\n", string(fullJSON))
	
	// 序列化只有值
	valueJSON, err := json.Marshal(point.Value)
	if err != nil {
		fmt.Printf("序列化值失败: %v\n", err)
		return
	}
	fmt.Printf("只有值的JSON: %s\n", string(valueJSON))
}

func testNatsJSONSerialization() {
	fmt.Println("\n=== 测试NATS JSON序列化 ===")
	
	// 创建不同类型的数据点
	intPoint := model.NewPoint("test_int", "test_device", 42, model.TypeInt)
	
	// 序列化为JSON
	data, err := json.Marshal(intPoint)
	if err != nil {
		fmt.Printf("序列化失败: %v\n", err)
		return
	}
	fmt.Printf("序列化后的JSON: %s\n", string(data))
	
	// 反序列化
	var deserializedPoint model.Point
	if err := json.Unmarshal(data, &deserializedPoint); err != nil {
		fmt.Printf("反序列化失败: %v\n", err)
		return
	}
	
	// 检查反序列化后的类型
	fmt.Printf("反序列化后的数据点类型: %s\n", deserializedPoint.Type)
	switch v := deserializedPoint.Value.(type) {
	case int:
		fmt.Printf("反序列化后值 %v 的Go类型: int\n", v)
	case float64:
		fmt.Printf("反序列化后值 %v 的Go类型: float64\n", v)
	case bool:
		fmt.Printf("反序列化后值 %v 的Go类型: bool\n", v)
	case string:
		fmt.Printf("反序列化后值 %v 的Go类型: string\n", v)
	default:
		fmt.Printf("反序列化后值 %v 的Go类型: %T\n", v, v)
	}
}
