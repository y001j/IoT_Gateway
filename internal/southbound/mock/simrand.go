package mock

import "math/rand"

// 模拟数据使用的伪随机数。这里生成的是演示/测试用的传感器读数，不用于任何安全场景，
// 因此使用 math/rand（集中在此处，避免每个调用点都标注）。

func simFloat64() float64 {
	return rand.Float64() // #nosec G404 -- 模拟传感器数据，非安全用途
}

func simIntn(n int) int {
	return rand.Intn(n) // #nosec G404 -- 模拟传感器数据，非安全用途
}

func simByte() uint8 {
	return uint8(rand.Intn(256)) // #nosec G404 G115 -- 模拟颜色分量，取值 0-255
}
