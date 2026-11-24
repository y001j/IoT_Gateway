package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/storage"
	"github.com/y001j/iot-gateway/internal/testutil"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// 插件管理器必须实现 StorageProvider，否则 Web 层拿不到本地暂存服务
var _ StorageProvider = (*plugin.Manager)(nil)

// fakeProvider 使用真实的 store，记录运维操作
type fakeProvider struct {
	*plugin.Manager
	store   *storage.Store
	mu      sync.Mutex
	actions []string
	from    time.Time
	err     error
}

func (f *fakeProvider) GetStore() *storage.Store { return f.store }
func (f *fakeProvider) GetBufferStats() []storage.RunnerStats {
	return []storage.RunnerStats{{Name: "influx", State: storage.StateBuffering, Pending: 42}}
}
func (f *fakeProvider) ControlSinkBuffer(name, action string, from time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, name+":"+action)
	f.from = from
	return f.err
}

func newDataRouter(t *testing.T, store *storage.Store) (*gin.Engine, *services.Services, *fakeProvider) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("IOT_GATEWAY_DATA_DIR", t.TempDir())
	t.Setenv("IOT_GATEWAY_ADMIN_PASSWORD", "route-test-pass")
	fp := &fakeProvider{Manager: plugin.NewManager(t.TempDir(), nil, viper.New()), store: store}
	svc, err := services.NewServices(&services.ServiceConfig{
		Auth:          &models.AuthConfig{JWTSecret: "data-test-secret-0123456789", TokenDuration: time.Hour, PasswordMinLength: 6, BcryptCost: 4},
		PluginManager: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	SetupRoutes(r, svc, nil)
	return r, svc, fp
}

func do(t *testing.T, r http.Handler, method, path, tok, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	var resp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Success {
		t.Fatalf("响应错误 %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		t.Fatal(err)
	}
}

func TestDataAPIDisabledStorage(t *testing.T) {
	r, svc, _ := newDataRouter(t, nil)
	tok := tokenFor(t, svc, "viewer")
	for _, p := range []string{"/api/v1/data/series", "/api/v1/data/latest?device_id=a", "/api/v1/data/history?device_id=a&key=b", "/api/v1/data/export?device_id=a&key=b"} {
		if w := do(t, r, "GET", p, tok, ""); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s 存储未开启应返回 503, got %d", p, w.Code)
		}
	}
	var st map[string]interface{}
	decodeData(t, do(t, r, "GET", "/api/v1/storage/status", tok, ""), &st)
	if st["enabled"] != false {
		t.Fatalf("status 应显示未开启: %v", st)
	}
	admin := tokenFor(t, svc, "admin")
	if w := do(t, r, "POST", "/api/v1/storage/sinks/x/pause", admin, ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("存储未开启时运维操作应返回 503, got %d", w.Code)
	}
	if w := do(t, r, "GET", "/api/v1/data/series", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401, got %d", w.Code)
	}
}

func TestDataAPIQueries(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	cfg := storage.DefaultConfig()
	cfg.MaxBytes, cfg.MaxDisk = 64<<20, 512<<20
	store, err := storage.New(nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := nc.JetStream()
	base := time.Now().Add(-time.Minute).Truncate(time.Second)
	pub := func(dev, key string, i int, v interface{}) {
		p := model.NewPoint(key, dev, v, model.TypeFloat)
		p.Timestamp = base.Add(time.Duration(i) * time.Second)
		data, _ := json.Marshal(p)
		if _, err := js.Publish(model.DataSubject(dev, key), data, nats.Context(context.Background())); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 30; i++ {
		pub("plc.1", "temp", i, float64(i))
	}
	pub("plc.1", "status", 0, "=cmd|' /C calc'!A0")
	pub("plc.1", "pos", 0, map[string]interface{}{"x": 1.0})

	r, svc, _ := newDataRouter(t, store)
	tok := tokenFor(t, svc, "viewer")

	var series struct {
		Series []storage.SeriesInfo `json:"series"`
		Total  int                  `json:"total"`
	}
	decodeData(t, do(t, r, "GET", "/api/v1/data/series?device_id=plc.1&limit=5000", tok, ""), &series)
	if series.Total != 3 || len(series.Series) != 3 {
		t.Fatalf("序列列表错误: %+v", series)
	}

	var latest struct {
		Points []model.Point `json:"points"`
	}
	decodeData(t, do(t, r, "GET", "/api/v1/data/latest?device_id=plc.1", tok, ""), &latest)
	if len(latest.Points) != 3 {
		t.Fatalf("最新值错误: %+v", latest)
	}
	if w := do(t, r, "GET", "/api/v1/data/latest", tok, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("缺少 device_id 应返回 400, got %d", w.Code)
	}

	var hist storage.SeriesResult
	q := "/api/v1/data/history?device_id=plc.1&key=temp&start=" + url.QueryEscape(base.Format(time.RFC3339)) +
		"&end=" + url.QueryEscape(base.Add(29*time.Second).Format(time.RFC3339)) + "&limit=10&agg=max"
	decodeData(t, do(t, r, "GET", q, tok, ""), &hist)
	if hist.Matched != 30 || len(hist.Points) > 10 || !hist.Downsampled || hist.Agg != "max" {
		t.Fatalf("历史查询错误: %+v", hist)
	}
	// Unix 秒和毫秒时间戳
	q = "/api/v1/data/history?device_id=plc.1&key=temp&start=" + itoa(base.Unix()) + "&end=" + itoa(base.Add(4*time.Second).UnixMilli())
	decodeData(t, do(t, r, "GET", q, tok, ""), &hist)
	if len(hist.Points) != 5 {
		t.Fatalf("Unix 时间戳查询应返回 5 点, got %d", len(hist.Points))
	}
	for _, bad := range []string{
		"/api/v1/data/history?device_id=plc.1&key=temp&start=yesterday",
		"/api/v1/data/history?device_id=plc.1&key=temp&end=bad",
		"/api/v1/data/history?device_id=plc.1&key=temp&limit=-1",
		"/api/v1/data/history?device_id=plc.1&key=temp&agg=median",
		"/api/v1/data/history?key=temp",
		"/api/v1/data/export?device_id=plc.1&key=temp&start=bad",
		"/api/v1/data/export?device_id=plc.1",
	} {
		if w := do(t, r, "GET", bad, tok, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s 应返回 400, got %d", bad, w.Code)
		}
	}

	// CSV 导出：不降采样；公式注入被转义；复合值输出 JSON
	w := do(t, r, "GET", "/api/v1/data/export?device_id=plc.1&key=temp&start="+itoa(base.Unix())+"&limit=3", tok, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "plc_1_temp_") {
		t.Fatalf("导出失败 %d %v", w.Code, w.Header())
	}
	rows, err := csv.NewReader(w.Body).ReadAll()
	if err != nil || len(rows) != 31 || rows[0][0] != "device_id" || rows[1][3] != "0" || rows[30][3] != "29" {
		t.Fatalf("CSV 内容错误 (%v): %d 行 %v", err, len(rows), rows[:2])
	}
	w = do(t, r, "GET", "/api/v1/data/export?device_id=plc.1&key=status&start="+itoa(base.Unix()), tok, "")
	rows, _ = csv.NewReader(w.Body).ReadAll()
	if len(rows) != 2 || !strings.HasPrefix(rows[1][3], "'=") {
		t.Fatalf("公式应被转义: %v", rows)
	}
	w = do(t, r, "GET", "/api/v1/data/export?device_id=plc.1&key=pos&start="+itoa(base.Unix()), tok, "")
	rows, _ = csv.NewReader(w.Body).ReadAll()
	if len(rows) != 2 || rows[1][3] != `{"x":1}` {
		t.Fatalf("复合值应输出 JSON: %v", rows)
	}

	// 状态
	var st struct {
		Enabled bool                  `json:"enabled"`
		Stream  storage.StreamStatus  `json:"stream"`
		Sinks   []storage.RunnerStats `json:"sinks"`
	}
	decodeData(t, do(t, r, "GET", "/api/v1/storage/status", tok, ""), &st)
	if !st.Enabled || st.Stream.Messages != 32 || len(st.Sinks) != 1 || st.Sinks[0].Pending != 42 {
		t.Fatalf("状态错误: %+v", st)
	}
}

func TestStorageControlAPI(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	cfg := storage.DefaultConfig()
	cfg.MaxBytes, cfg.MaxDisk = 64<<20, 512<<20
	store, err := storage.New(nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, svc, fp := newDataRouter(t, store)
	admin := tokenFor(t, svc, "admin")
	operator := tokenFor(t, svc, "operator")

	if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/pause", operator, ""); w.Code != http.StatusForbidden {
		t.Fatalf("运维操作仅限管理员, got %d", w.Code)
	}
	for _, a := range []string{"pause", "resume", "skip"} {
		if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/"+a, admin, ""); w.Code != http.StatusOK {
			t.Fatalf("%s 失败: %d %s", a, w.Code, w.Body.String())
		}
	}
	if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/replay", admin, `{"from_time":"2026-11-01T00:00:00Z"}`); w.Code != http.StatusOK {
		t.Fatalf("replay 失败: %d %s", w.Code, w.Body.String())
	}
	if !fp.from.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("replay 起始时间错误: %v", fp.from)
	}
	if strings.Join(fp.actions, ",") != "influx:pause,influx:resume,influx:skip,influx:replay" {
		t.Fatalf("操作记录错误: %v", fp.actions)
	}
	for body, code := range map[string]int{"": 400, `{"from_time":""}`: 400, `{"from_time":"bad"}`: 400} {
		if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/replay", admin, body); w.Code != code {
			t.Errorf("replay body=%q 应返回 %d, got %d", body, code, w.Code)
		}
	}
	if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/delete", admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("未知操作应返回 400, got %d", w.Code)
	}
	fp.err = plugin.ErrSinkNotBuffered
	if w := do(t, r, "POST", "/api/v1/storage/sinks/nope/pause", admin, ""); w.Code != http.StatusNotFound {
		t.Fatalf("未缓冲的连接器应返回 404, got %d", w.Code)
	}
	fp.err = errors.New("boom")
	if w := do(t, r, "POST", "/api/v1/storage/sinks/influx/skip", admin, ""); w.Code != http.StatusInternalServerError {
		t.Fatalf("内部错误应返回 500, got %d", w.Code)
	}
}

func TestDataHandlerHelpers(t *testing.T) {
	if got := csvValue(nil); got != "" {
		t.Fatal(got)
	}
	cases := map[interface{}]string{1.5: "1.5", true: "true", int64(7): "7", "plain": "plain", "+1": "'+1", "-x": "'-x", "@a": "'@a", "": ""}
	for in, want := range cases {
		if got := csvValue(in); got != want {
			t.Errorf("csvValue(%v) = %q, want %q", in, got, want)
		}
	}
	if got := csvValue([]interface{}{1.0, 2.0}); got != "[1,2]" {
		t.Fatal(got)
	}
	if safeFilePart("") != "data" || safeFilePart("a/b c") != "a_b_c" {
		t.Fatal("safeFilePart 错误")
	}
	if tm, err := parseTime(""); err != nil || !tm.IsZero() {
		t.Fatal("空时间应为零值")
	}
	// 没有插件管理器时 provider 为 nil
	h := NewDataHandler(&services.Services{})
	if h.provider() != nil {
		t.Fatal("没有插件管理器时 provider 应为 nil")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
