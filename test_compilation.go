// +build ignore

// 这是一个简单的编译测试文件，用于验证规则引擎的代码是否可以正确编译
package main

import (
	"fmt"
	_ "github.com/y001j/iot-gateway/internal/rules"
	_ "github.com/y001j/iot-gateway/internal/rules/actions"
)

func main() {
	fmt.Println("编译测试通过")
}