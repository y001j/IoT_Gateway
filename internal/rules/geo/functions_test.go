package geo

import (
	"math"
	"strings"
	"testing"
)

func newTestProcessor() *GeoProcessor {
	gp := NewGeoProcessor()
	gp.AddRegion(&Region{Name: "beijing", Center: Coordinate{Latitude: 39.9042, Longitude: 116.4074}, Radius: 50})
	gp.AddRegion(&Region{Name: "shanghai", Center: Coordinate{Latitude: 31.2304, Longitude: 121.4737}, Radius: 50})
	return gp
}

func TestDistanceFunction(t *testing.T) {
	f := NewDistanceFunction(newTestProcessor())
	if f.Name() != "distance" || !strings.Contains(f.Description(), "distance(") {
		t.Fatal("名称或描述错误")
	}
	// 北京 → 上海约 1068 km；支持 float/int/int64/string 参数
	v, err := f.Call(39.9042, "116.4074", 31.2304, 121.4737)
	if err != nil || math.Abs(v.(float64)-1068) > 5 {
		t.Fatalf("距离错误: %v %v", v, err)
	}
	if v, err := f.Call(0, int64(0), int32(0), float32(1)); err != nil || math.Abs(v.(float64)-111.19) > 0.5 {
		t.Fatalf("赤道上 1 度约 111 km: %v %v", v, err)
	}
	bad := [][]interface{}{
		{1.0, 2.0, 3.0},
		{"x", 0, 0, 0}, {0, "x", 0, 0}, {0, 0, "x", 0}, {0, 0, 0, true},
		{91.0, 0, 0, 0}, {0, 0, 0, 181.0},
	}
	for _, args := range bad {
		if _, err := f.Call(args...); err == nil {
			t.Errorf("参数 %v 应报错", args)
		}
	}
}

func TestInRegionAndNearestRegionFunctions(t *testing.T) {
	gp := newTestProcessor()
	in := NewInRegionFunction(gp)
	if in.Name() != "in_region" || in.Description() == "" {
		t.Fatal("名称或描述错误")
	}
	if v, err := in.Call(39.95, 116.4, "beijing"); err != nil || v != true {
		t.Fatalf("应在北京区域内: %v %v", v, err)
	}
	if v, err := in.Call(31.23, 121.47, "beijing"); err != nil || v != false {
		t.Fatalf("上海不在北京区域内: %v %v", v, err)
	}
	if v, _ := in.Call(39.95, 116.4, "nowhere"); v != false {
		t.Fatal("不存在的区域应返回 false")
	}
	for _, args := range [][]interface{}{{1.0, 2.0}, {"a", 0, "r"}, {0, "a", "r"}, {0, 0, 1}, {100.0, 0, "r"}} {
		if _, err := in.Call(args...); err == nil {
			t.Errorf("in_region 参数 %v 应报错", args)
		}
	}

	near := NewNearestRegionFunction(gp)
	if near.Name() != "nearest_region" || near.Description() == "" {
		t.Fatal("名称或描述错误")
	}
	if v, err := near.Call(32.0, 120.0); err != nil || v != "shanghai" {
		t.Fatalf("最近区域应为上海: %v %v", v, err)
	}
	if v, err := NewNearestRegionFunction(NewGeoProcessor()).Call(0, 0); err != nil || v != "" {
		t.Fatalf("没有区域时应返回空: %v %v", v, err)
	}
	for _, args := range [][]interface{}{{1.0}, {"a", 0}, {0, "a"}, {0, 200.0}} {
		if _, err := near.Call(args...); err == nil {
			t.Errorf("nearest_region 参数 %v 应报错", args)
		}
	}
}

func TestBearingAndValidCoordinateFunctions(t *testing.T) {
	gp := newTestProcessor()
	b := NewBearingFunction(gp)
	if b.Name() != "bearing" || b.Description() == "" {
		t.Fatal("名称或描述错误")
	}
	cases := []struct {
		args []interface{}
		want float64
	}{
		{[]interface{}{0, 0, 1, 0}, 0},   // 正北
		{[]interface{}{0, 0, 0, 1}, 90},  // 正东
		{[]interface{}{1, 0, 0, 0}, 180}, // 正南
		{[]interface{}{0, 1, 0, 0}, 270}, // 正西（负角度转换为 0-360）
	}
	for _, c := range cases {
		v, err := b.Call(c.args...)
		if err != nil || math.Abs(v.(float64)-c.want) > 1e-6 {
			t.Errorf("bearing%v = %v (%v), want %v", c.args, v, err, c.want)
		}
	}
	for _, args := range [][]interface{}{{0, 0, 0}, {"a", 0, 0, 0}, {0, "a", 0, 0}, {0, 0, "a", 0}, {0, 0, 0, "a"}, {-91.0, 0, 0, 0}, {0, 0, 0, -181.0}} {
		if _, err := b.Call(args...); err == nil {
			t.Errorf("bearing 参数 %v 应报错", args)
		}
	}

	vc := NewValidCoordinateFunction(gp)
	if vc.Name() != "valid_coordinate" || vc.Description() == "" {
		t.Fatal("名称或描述错误")
	}
	for args, want := range map[[2]interface{}]bool{
		{45.0, 90.0}: true, {"-90", "180"}: true, {91.0, 0}: false, {0, -181.0}: false, {"abc", 0}: false, {0, nil}: false,
	} {
		if v, err := vc.Call(args[0], args[1]); err != nil || v != want {
			t.Errorf("valid_coordinate(%v) = %v (%v), want %v", args, v, err, want)
		}
	}
	if _, err := vc.Call(1.0); err == nil {
		t.Error("参数个数错误应报错")
	}
}

func TestGeoHelpers(t *testing.T) {
	gp := newTestProcessor()
	// 中点：赤道上 0° 与 10° 的中点是 5°
	mid := gp.MidPoint(Coordinate{0, 0}, Coordinate{0, 10})
	if math.Abs(mid.Latitude) > 1e-9 || math.Abs(mid.Longitude-5) > 1e-9 {
		t.Fatalf("中点错误: %+v", mid)
	}
	// 目标点：从赤道向东 111.19 km 约为经度 1°；与 Haversine 距离互相验证
	start := Coordinate{Latitude: 30, Longitude: 120}
	dest := gp.DestinationPoint(start, 100, 45)
	if d := gp.HaversineDistance(start, dest); math.Abs(d-100) > 0.01 {
		t.Fatalf("目标点距离应为 100 km, got %v", d)
	}
	if br := gp.Bearing(start, dest); math.Abs(br-45) > 0.1 {
		t.Fatalf("目标点方位应约为 45°, got %v", br)
	}

	stats := gp.GetRegionStats()
	if stats["total_regions"] != 2 || len(stats["regions"].([]map[string]interface{})) != 2 {
		t.Fatalf("区域统计错误: %v", stats)
	}

	cases := []struct {
		data map[string]interface{}
		want []string
	}{
		{map[string]interface{}{"latitude": 30.0, "longitude": 120.0}, nil},
		{map[string]interface{}{"lat": "30.5", "lon": "120"}, nil},
		{map[string]interface{}{"lat": 30.0, "lng": 120.0}, nil},
		// 回归：只缺经度时以前不报错
		{map[string]interface{}{"latitude": 30.0}, []string{"missing longitude field"}},
		{map[string]interface{}{"longitude": 30.0}, []string{"missing latitude field"}},
		{map[string]interface{}{}, []string{"missing latitude field", "missing longitude field"}},
		{map[string]interface{}{"latitude": 95.0, "longitude": -200.0}, []string{"latitude out of range [-90, 90]", "longitude out of range [-180, 180]"}},
		{map[string]interface{}{"latitude": "north", "longitude": "east"}, []string{"invalid latitude format", "invalid longitude format"}},
		{map[string]interface{}{"latitude": true, "longitude": []int{1}}, []string{"latitude must be number or string", "longitude must be number or string"}},
	}
	for _, c := range cases {
		got := gp.ValidateGeoData(c.data)
		if strings.Join(got, ";") != strings.Join(c.want, ";") {
			t.Errorf("ValidateGeoData(%v) = %v, want %v", c.data, got, c.want)
		}
	}
	if v, err := toFloat64(uint64(7)); err != nil || v != 7 {
		t.Fatal("uint64 转换错误")
	}
	if v, err := toFloat64(uint32(7)); err != nil || v != 7 {
		t.Fatal("uint32 转换错误")
	}
	if v, err := toFloat64(" 1.5 "); err != nil || v != 1.5 {
		t.Fatal("带空白的数字字符串应可转换")
	}
}
