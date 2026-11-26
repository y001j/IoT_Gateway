package plugin

import (
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

// BenchmarkCopyBatchForSink 管道为每个连接器复制一批数据点（100 个点、每点 3 个标签）的开销
func BenchmarkCopyBatchForSink(b *testing.B) {
	pts := make([]model.Point, 100)
	for i := range pts {
		p := model.NewPoint("temperature", "dev", 21.5, model.TypeFloat)
		p.AddTag("source", "mock")
		p.AddTag("unit", "celsius")
		p.AddTag("site", "plant-1")
		pts[i] = p
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = copyBatchForSink(pts)
	}
}
