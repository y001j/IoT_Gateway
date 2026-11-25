package composite_mock

import "math/rand"

// 模拟数据使用的伪随机数。这里生成的是演示/测试用的传感器读数，不用于任何安全场景，
// 因此使用 math/rand（集中在此处，避免每个调用点都标注）。

func simFloat64() float64 {
	return rand.Float64() // #nosec G404 -- 模拟传感器数据，非安全用途
}

func simFloat32() float32 {
	return rand.Float32() // #nosec G404 -- 模拟传感器数据，非安全用途
}

func simIntn(n int) int {
	return rand.Intn(n) // #nosec G404 -- 模拟传感器数据，非安全用途
}

// simIntUpTo 返回 [0, max) 内的随机整数；max < 1 时返回 0（rand.Intn 遇到非正数会 panic）
func simIntUpTo(max float64) int {
	if max < 1 {
		return 0
	}
	return simIntn(int(max))
}
