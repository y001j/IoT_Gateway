package model

import (
	"encoding/json"
	"testing"
)

func benchPoint() Point {
	p := NewPoint("temperature", "dev-001", 42.5, TypeFloat)
	p.AddTag("source", "modbus")
	p.AddTag("mode", "tcp")
	p.AddTag("address", "40001")
	p.AddTag("type", "holding_register")
	return p
}

func BenchmarkNewPointWithTags(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = benchPoint()
	}
}

func BenchmarkPointJSONRoundTrip(b *testing.B) {
	p := benchPoint()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d, _ := json.Marshal(p)
		var q Point
		_ = json.Unmarshal(d, &q)
	}
}
