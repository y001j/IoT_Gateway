package utils

import "math"

// SaturatingInt64 把 uint64 转换为 int64，超出范围时取 math.MaxInt64（用于字节数、包数等计数器）
func SaturatingInt64(u uint64) int64 {
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(u)
}
