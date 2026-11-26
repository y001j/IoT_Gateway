package northbound

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"fmt"
	"github.com/y001j/iot-gateway/internal/model"
)

func TestToInt64(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int64
		ok   bool
	}{
		{int(1), 1, true}, {int8(-2), -2, true}, {int16(3), 3, true}, {int32(4), 4, true},
		{int64(5), 5, true}, {uint(6), 6, true}, {uint8(7), 7, true}, {uint16(8), 8, true},
		{uint32(9), 9, true}, {uint64(10), 10, true}, {uint64(math.MaxUint64), math.MaxInt64, true},
		{float32(1.9), 1, true}, {float64(-2.7), -2, true}, {true, 1, true}, {false, 0, true},
		{json.Number("42"), 42, true}, {json.Number("4.5"), 4, true}, {"17", 17, true},
		{"3.9", 3, true}, {"abc", 0, false}, {nil, 0, false}, {[]int{1}, 0, false},
	}
	for _, c := range cases {
		got, ok := ToInt64(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ToInt64(%#v) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestToFloat64(t *testing.T) {
	cases := []struct {
		in   interface{}
		want float64
		ok   bool
	}{
		{1.5, 1.5, true}, {float32(2.5), 2.5, true}, {int64(3), 3, true}, {uint64(4), 4, true},
		{int8(-1), -1, true}, {uint16(9), 9, true}, {true, 1, true}, {json.Number("1.25"), 1.25, true},
		{"2.5", 2.5, true}, {"x", 0, false}, {struct{}{}, 0, false},
	}
	for _, c := range cases {
		got, ok := ToFloat64(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ToFloat64(%#v) = %v,%v want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestNormalizeValue(t *testing.T) {
	// 回归测试：Modbus 产生的 int64/uint64 以前被 Sink 写成 0
	p := model.Point{Type: model.TypeInt, Value: int64(1234)}
	if v, ok := NormalizeValue(p); !ok || v.(int64) != 1234 {
		t.Fatalf("int64 规范化失败: %v %v", v, ok)
	}
	p = model.Point{Type: model.TypeInt, Value: uint64(99)}
	if v := NormalizeValueOrZero(p); v.(int64) != 99 {
		t.Fatalf("uint64 规范化失败: %v", v)
	}
	p = model.Point{Type: model.TypeFloat, Value: int32(7)}
	if v := NormalizeValueOrZero(p); v.(float64) != 7 {
		t.Fatalf("int32->float 失败: %v", v)
	}
	p = model.Point{Type: model.TypeBool, Value: "true"}
	if v, ok := NormalizeValue(p); !ok || v != true {
		t.Fatalf("字符串布尔失败")
	}
	p = model.Point{Type: model.TypeBool, Value: 0}
	if v, ok := NormalizeValue(p); !ok || v != false {
		t.Fatalf("数值布尔失败")
	}
	p = model.Point{Type: model.TypeBool, Value: []int{}}
	if _, ok := NormalizeValue(p); ok {
		t.Fatalf("无法转换的布尔应返回 ok=false")
	}
	p = model.Point{Type: model.TypeString, Value: 12}
	if v := NormalizeValueOrZero(p); v != "12" {
		t.Fatalf("非字符串值应格式化: %v", v)
	}
	p = model.Point{Type: model.TypeInt, Value: "oops"}
	if v := NormalizeValueOrZero(p); v.(int64) != 0 {
		t.Fatalf("不兼容值应返回零值")
	}
	loc := &model.LocationData{Latitude: 1}
	p = model.Point{Type: model.TypeLocation, Value: loc}
	if v, ok := NormalizeValue(p); !ok || v != loc {
		t.Fatalf("复合类型应原样返回")
	}
	if _, ok := NormalizeValue(model.Point{Type: model.TypeLocation}); ok {
		t.Fatalf("nil 复合值应返回 ok=false")
	}
	for _, typ := range []model.DataType{model.TypeFloat, model.TypeBool, model.TypeString} {
		if NormalizeValueOrZero(model.Point{Type: typ, Value: struct{}{}}) == nil {
			t.Errorf("%s 零值不应为 nil", typ)
		}
	}
}

func TestPointTime(t *testing.T) {
	ts := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := PointTime(model.Point{Timestamp: ts}); !got.Equal(ts) {
		t.Fatalf("应使用采集时间")
	}
	if got := PointTime(model.Point{}); time.Since(got) > time.Second {
		t.Fatalf("零时间应回退为当前时间")
	}
}

func TestBaseSinkConfigAndStats(t *testing.T) {
	b := NewBaseSink("test")
	cfg, err := b.ParseStandardConfig(json.RawMessage(`{"name":"s1","batch_size":50,"buffer_size":500,"tags":{"site":"A"},"params":{"x":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if b.Name() != "s1" || b.GetBatchSize() != 50 || b.GetBufferSize() != 500 || b.GetTags()["site"] != "A" {
		t.Fatalf("标准配置解析错误")
	}
	if string(cfg.Params) != `{"x":1}` {
		t.Fatalf("Params = %s", cfg.Params)
	}
	if _, err := b.ParseStandardConfig(json.RawMessage(`{bad`)); err == nil {
		t.Fatalf("非法配置应报错")
	}

	points := []model.Point{model.NewPoint("k", "d", 1, model.TypeInt), {Key: "k2"}}
	b.AddTags(points)
	for _, p := range points {
		if v, _ := p.GetTag("site"); v != "A" {
			t.Fatalf("AddTags 失败")
		}
	}

	// SafePublishBatch 成功和失败分别更新统计
	if err := b.SafePublishBatch(points, func([]model.Point) error { return nil }, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := b.SafePublishBatch(points, func([]model.Point) error { return errors.New("boom") }, time.Now()); err == nil {
		t.Fatalf("应返回发布错误")
	}
	if b.GetLastError() == nil || b.GetLastError().Error() != "boom" {
		t.Fatalf("GetLastError = %v", b.GetLastError())
	}
	// 含 % 的错误信息不能被当作格式串（回归 go vet 问题）
	b.SetLastError(errors.New("100% full"))
	if b.GetLastError().Error() != "100% full" {
		t.Fatalf("错误信息被格式化篡改: %q", b.GetLastError().Error())
	}
}

func TestRegistry(t *testing.T) {
	Register("unit-test-sink", func() Sink { return nil })
	if _, ok := Create("unit-test-sink"); !ok {
		t.Fatalf("注册的连接器应能创建")
	}
	if _, ok := Create("does-not-exist"); ok {
		t.Fatalf("未注册的连接器不应创建成功")
	}
}

func TestBaseSinkHealthAndStats(t *testing.T) {
	b := NewBaseSink("h")
	if h, _ := b.Health(); h.Status != "unhealthy" {
		t.Fatalf("未运行时应为 unhealthy: %+v", h)
	}
	b.SetRunning(true)
	if h, _ := b.Health(); h.Status != "healthy" {
		t.Fatalf("运行中应为 healthy: %+v", h)
	}
	b.HandleError(errors.New("写入失败"), "测试")
	if h, _ := b.Health(); h.Status != "degraded" || h.Message == "" {
		t.Fatalf("有错误时应为 degraded: %+v", h)
	}
	if st := b.GetStats(); !st.Running || st.LastError == "" {
		t.Fatalf("stats = %+v", st)
	}
	if !IsPermanent(fmt.Errorf("%w: bad data", ErrPermanent)) || IsPermanent(errors.New("x")) {
		t.Fatal("IsPermanent")
	}
}
