package redis

import (
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func TestFormatKey(t *testing.T) {
	s := NewRedisSink()
	p := model.Point{DeviceID: "dev-1", Key: "temp"}
	if got := s.formatKey(PointConfig{}, p); got != "dev-1:temp" {
		t.Fatalf("默认键 = %q", got)
	}
	if got := s.formatKey(PointConfig{KeyFormat: "iot/{key}/{device_id}", KeyPrefix: "gw"}, p); got != "gw:iot/temp/dev-1" {
		t.Fatalf("自定义键 = %q", got)
	}
}

func TestConvertValue(t *testing.T) {
	s := NewRedisSink()
	// 回归：int64 以前被写成 0
	if v := s.convertValue(model.Point{Type: model.TypeInt, Value: int64(77)}); v.(int64) != 77 {
		t.Fatalf("int64 = %v", v)
	}
	if v := s.convertValue(model.Point{Type: model.TypeFloat, Value: uint32(3)}); v.(float64) != 3 {
		t.Fatalf("uint32->float = %v", v)
	}
}
