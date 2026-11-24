package geo

import (
	"math"
	"testing"
)

var (
	beijing  = Coordinate{Latitude: 39.9042, Longitude: 116.4074}
	shanghai = Coordinate{Latitude: 31.2304, Longitude: 121.4737}
)

func TestHaversineAndVincenty(t *testing.T) {
	gp := NewGeoProcessor()
	d := gp.HaversineDistance(beijing, shanghai)
	if math.Abs(d-1067) > 15 {
		t.Fatalf("北京-上海 Haversine 距离 = %.1f km，应约为 1067 km", d)
	}
	v := gp.VincentyDistance(beijing, shanghai)
	if math.Abs(v-d) > 10 {
		t.Fatalf("Vincenty(%.1f) 与 Haversine(%.1f) 差异过大", v, d)
	}
	if gp.HaversineDistance(beijing, beijing) != 0 {
		t.Fatalf("同一点距离应为 0")
	}
}

func TestRegions(t *testing.T) {
	gp := NewGeoProcessor()
	gp.AddRegion(&Region{Name: "北京", Center: beijing, Radius: 50})
	gp.AddRegion(&Region{Name: "上海", Center: shanghai, Radius: 50})
	near := Coordinate{Latitude: 39.95, Longitude: 116.45}
	if !gp.IsInRegion(near, "北京") || gp.IsInRegion(near, "上海") || gp.IsInRegion(near, "不存在") {
		t.Fatalf("区域判断错误")
	}
	if got := gp.IsInAnyRegion(near); len(got) != 1 || got[0] != "北京" {
		t.Fatalf("IsInAnyRegion = %v", got)
	}
	name, dist := gp.GetNearestRegion(Coordinate{Latitude: 32, Longitude: 120})
	if name != "上海" || dist <= 0 {
		t.Fatalf("最近区域 = %s %.1f", name, dist)
	}
	box := BoundingBox{SouthWest: Coordinate{Latitude: 30, Longitude: 115}, NorthEast: Coordinate{Latitude: 41, Longitude: 122}}
	if !gp.IsInBoundingBox(beijing, box) || gp.IsInBoundingBox(Coordinate{Latitude: 0, Longitude: 0}, box) {
		t.Fatalf("边界框判断错误")
	}
}

func TestParseCoordinate(t *testing.T) {
	gp := NewGeoProcessor()
	c, err := gp.ParseCoordinate("39.9", "116.4")
	if err != nil || c.Latitude != 39.9 || c.Longitude != 116.4 {
		t.Fatalf("解析坐标失败: %v", err)
	}
	for _, bad := range [][2]string{{"x", "1"}, {"1", "y"}, {"91", "0"}, {"0", "181"}} {
		if _, err := gp.ParseCoordinate(bad[0], bad[1]); err == nil {
			t.Errorf("%v 应解析失败", bad)
		}
	}
	if gp.IsValidCoordinate(-91, 0) || !gp.IsValidCoordinate(0, 0) {
		t.Fatalf("坐标有效性判断错误")
	}
}
