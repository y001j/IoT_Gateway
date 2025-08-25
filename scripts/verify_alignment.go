// verify_alignment.go
// 验证64位字段在32位ARM上的对齐情况
package main

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/y001j/iot-gateway/internal/web/api"
	"github.com/y001j/iot-gateway/internal/rules/actions"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/metrics"
	"github.com/y001j/iot-gateway/internal/northbound"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func main() {
	fmt.Println("=== IoT Gateway ARM32 对齐验证 ===")
	
	// 检查关键结构体的64位字段对齐
	checkStructAlignment("SmartThrottler", reflect.TypeOf(api.SmartThrottler{}))
	checkStructAlignment("WebSocketClient", reflect.TypeOf(api.WebSocketClient{}))
	checkStructAlignment("HighPerformanceStats", reflect.TypeOf(actions.HighPerformanceStats{}))
	checkStructAlignment("PerformanceMetrics", reflect.TypeOf(actions.PerformanceMetrics{}))
	checkStructAlignment("OptimizedWorkerPool", reflect.TypeOf(rules.OptimizedWorkerPool{}))
	checkStructAlignment("SystemMetrics", reflect.TypeOf(metrics.SystemMetrics{}))
	checkStructAlignment("DataMetrics", reflect.TypeOf(metrics.DataMetrics{}))
	checkStructAlignment("SinkStats", reflect.TypeOf(northbound.SinkStats{}))
	checkStructAlignment("BaseAdapter", reflect.TypeOf(southbound.BaseAdapter{}))
	
	fmt.Println("\n=== 验证完成 ===")
}

func checkStructAlignment(name string, t reflect.Type) {
	fmt.Printf("\n检查结构体: %s\n", name)
	
	var has64BitFields bool
	var first64BitOffset uintptr = ^uintptr(0) // 最大值，表示未找到
	
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		offset := field.Offset
		fieldType := field.Type
		
		// 检查是否是64位字段
		is64Bit := false
		switch fieldType.Kind() {
		case reflect.Int64, reflect.Uint64:
			is64Bit = true
		case reflect.Float64:
			is64Bit = true
		}
		
		if is64Bit {
			has64BitFields = true
			if first64BitOffset == ^uintptr(0) {
				first64BitOffset = offset
			}
			
			// 检查64位字段的对齐
			if offset%8 != 0 {
				fmt.Printf("  ❌ CRITICAL: %s.%s (offset=%d) 未8字节对齐！\n", 
					name, field.Name, offset)
			} else {
				fmt.Printf("  ✅ %s.%s (offset=%d) 正确对齐\n", 
					name, field.Name, offset)
			}
		}
	}
	
	if !has64BitFields {
		fmt.Printf("  ℹ️  无64位字段\n")
		return
	}
	
	// 检查第一个64位字段是否在结构体开头附近
	if first64BitOffset <= 8 { // 允许前8字节内有其他字段
		fmt.Printf("  ✅ 第一个64位字段在偏移 %d，ARM32对齐友好\n", first64BitOffset)
	} else {
		fmt.Printf("  ⚠️  第一个64位字段在偏移 %d，可能在ARM32上有对齐问题\n", first64BitOffset)
	}
}