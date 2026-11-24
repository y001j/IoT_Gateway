package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/storage"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// StorageProvider 提供本地暂存服务和缓冲连接器运维能力（由 plugin.Manager 实现）
type StorageProvider interface {
	GetStore() *storage.Store
	GetBufferStats() []storage.RunnerStats
	ControlSinkBuffer(name, action string, from time.Time) error
}

// DataHandler 本地近期数据查询与本地暂存运维接口
type DataHandler struct {
	*BaseHandler
}

// NewDataHandler 创建数据查询处理器
func NewDataHandler(svc *services.Services) *DataHandler {
	return &DataHandler{BaseHandler: NewBaseHandler(svc)}
}

func (h *DataHandler) provider() StorageProvider {
	if h.services == nil || h.services.PluginManager == nil {
		return nil
	}
	p, _ := h.services.PluginManager.(StorageProvider)
	return p
}

// store 返回本地暂存服务；未开启时写入 503 响应并返回 nil
func (h *DataHandler) store(c *gin.Context) *storage.Store {
	if p := h.provider(); p != nil {
		if s := p.GetStore(); s != nil {
			return s
		}
	}
	h.ErrorResponse(c, http.StatusServiceUnavailable, "本地暂存未开启（storage.enabled）")
	return nil
}

// parseTime 解析 RFC3339 时间或 Unix 时间戳（秒或毫秒）；空字符串返回零值
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e11 {
			return time.UnixMilli(n), nil
		}
		return time.Unix(n, 0), nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("无效的时间 %q（使用 RFC3339 或 Unix 时间戳）", s)
	}
	return t, nil
}

func (h *DataHandler) parseQuery(c *gin.Context) (storage.RawQuery, error) {
	q := storage.RawQuery{
		DeviceID: c.Query("device_id"),
		Key:      c.Query("key"),
		Agg:      c.Query("agg"),
	}
	var err error
	if q.Start, err = parseTime(c.Query("start")); err != nil {
		return q, err
	}
	if q.End, err = parseTime(c.Query("end")); err != nil {
		return q, err
	}
	if s := c.Query("limit"); s != "" {
		if q.Limit, err = strconv.Atoi(s); err != nil || q.Limit < 0 {
			return q, fmt.Errorf("无效的 limit %q", s)
		}
	}
	return q, nil
}

// queryError 把存储层错误映射为 HTTP 状态码
func (h *DataHandler) queryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storage.ErrInvalidQuery):
		h.ErrorResponse(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, storage.ErrQueryBusy):
		h.ErrorResponse(c, http.StatusTooManyRequests, err.Error())
	default:
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
	}
}

// GetSeries GET /data/series?device_id=&offset=&limit=
func (h *DataHandler) GetSeries(c *gin.Context) {
	s := h.store(c)
	if s == nil {
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	list, total, err := s.ListSeries(c.Request.Context(), c.Query("device_id"), offset, limit)
	if err != nil {
		h.queryError(c, err)
		return
	}
	h.SuccessResponse(c, gin.H{"series": list, "total": total, "offset": offset, "limit": limit})
}

// GetLatest GET /data/latest?device_id=
func (h *DataHandler) GetLatest(c *gin.Context) {
	s := h.store(c)
	if s == nil {
		return
	}
	points, err := s.Latest(c.Request.Context(), c.Query("device_id"))
	if err != nil {
		h.queryError(c, err)
		return
	}
	h.SuccessResponse(c, gin.H{"device_id": c.Query("device_id"), "points": points})
}

// GetHistory GET /data/history?device_id=&key=&start=&end=&limit=&agg=&tier=auto|raw|rollup
func (h *DataHandler) GetHistory(c *gin.Context) {
	s := h.store(c)
	if s == nil {
		return
	}
	q, err := h.parseQuery(c)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.Query(c.Request.Context(), q, c.Query("tier"))
	if err != nil {
		h.queryError(c, err)
		return
	}
	h.SuccessResponse(c, res)
}

// ExportCSV GET /data/export?device_id=&key=&start=&end=  导出原始数据（不降采样，受扫描上限约束）
func (h *DataHandler) ExportCSV(c *gin.Context) {
	s := h.store(c)
	if s == nil {
		return
	}
	q, err := h.parseQuery(c)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	q.NoDownsample = true
	res, err := s.QueryRaw(c.Request.Context(), q)
	if err != nil {
		h.queryError(c, err)
		return
	}
	filename := fmt.Sprintf("%s_%s_%s.csv", safeFilePart(q.DeviceID), safeFilePart(q.Key), time.Now().Format("20060102150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if res.Truncated {
		c.Header("X-Truncated", "true")
	}
	c.Status(http.StatusOK)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"device_id", "key", "timestamp", "value", "quality"})
	for _, p := range res.Points {
		_ = w.Write([]string{res.DeviceID, res.Key, p.Time.Format(time.RFC3339Nano), csvValue(p.Value), strconv.Itoa(p.Quality)})
	}
	w.Flush()
}

// csvValue 把点值格式化为 CSV 单元格；复合类型输出 JSON。
// 以 = + - @ 开头的字符串加前缀 '，防止在表格软件中被当作公式执行。
func csvValue(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		if x != "" && strings.ContainsAny(x[:1], "=+-@\t\r") {
			return "'" + x
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case map[string]interface{}, []interface{}:
		b, _ := json.Marshal(x)
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

func safeFilePart(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "data"
	}
	return b.String()
}

// GetStorageStatus GET /storage/status  持久化流状态和缓冲连接器状态
func (h *DataHandler) GetStorageStatus(c *gin.Context) {
	p := h.provider()
	if p == nil || p.GetStore() == nil {
		h.SuccessResponse(c, gin.H{"enabled": false})
		return
	}
	resp := gin.H{"enabled": true, "sinks": p.GetBufferStats()}
	if st, err := p.GetStore().Status(); err == nil {
		resp["stream"] = st
	} else {
		resp["error"] = err.Error()
	}
	if ru := p.GetStore().Rollup(); ru != nil {
		resp["rollup"] = ru.Stats()
	}
	if alerts, err := p.GetStore().Assess(p.GetBufferStats()); err == nil {
		resp["alerts"] = alerts
	}
	h.SuccessResponse(c, resp)
}

// ControlSink POST /storage/sinks/:name/:action  （pause | resume | skip | replay，replay 需要 {"from_time": ...}）
func (h *DataHandler) ControlSink(c *gin.Context) {
	p := h.provider()
	if p == nil || p.GetStore() == nil {
		h.ErrorResponse(c, http.StatusServiceUnavailable, "本地暂存未开启（storage.enabled）")
		return
	}
	name, action := c.Param("name"), c.Param("action")
	var from time.Time
	switch action {
	case "pause", "resume", "skip":
	case "replay":
		var body struct {
			FromTime string `json:"from_time"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.FromTime == "" {
			h.ErrorResponse(c, http.StatusBadRequest, "replay 需要 from_time（RFC3339 或 Unix 时间戳）")
			return
		}
		t, err := parseTime(body.FromTime)
		if err != nil {
			h.ErrorResponse(c, http.StatusBadRequest, err.Error())
			return
		}
		from = t
	default:
		h.ErrorResponse(c, http.StatusBadRequest, "不支持的操作（pause/resume/skip/replay）")
		return
	}
	if err := p.ControlSinkBuffer(name, action, from); err != nil {
		if errors.Is(err, plugin.ErrSinkNotBuffered) {
			h.ErrorResponse(c, http.StatusNotFound, fmt.Sprintf("连接器 %s 未开启本地缓冲", name))
			return
		}
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.SuccessResponse(c, gin.H{"sink": name, "action": action})
}
