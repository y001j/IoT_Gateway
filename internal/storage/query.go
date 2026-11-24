package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

// QueryConfig 近期数据查询的资源限制（storage.query 段）
type QueryConfig struct {
	// MaxConcurrent 同时执行的查询数，默认 2
	MaxConcurrent int
	// MaxScan 单次查询最多扫描的消息数，默认 200000；超出时返回已扫描部分并标记 truncated
	MaxScan int
	// DefaultLimit 默认返回点数（超出时服务端降采样），默认 1000
	DefaultLimit int
	// MaxLimit 允许请求的最大点数，默认 10000
	MaxLimit int
	// Timeout 单次查询超时，默认 10s
	Timeout time.Duration
	// Tolerance 起始位置向前放宽的时间：JetStream 的 StartTime 是入库时间而非采集时间，默认 1m
	Tolerance time.Duration
}

// DefaultQueryConfig 默认查询配置
func DefaultQueryConfig() QueryConfig {
	return QueryConfig{
		MaxConcurrent: 2,
		MaxScan:       200000,
		DefaultLimit:  1000,
		MaxLimit:      10000,
		Timeout:       10 * time.Second,
		Tolerance:     time.Minute,
	}
}

func (c *QueryConfig) applyDefaults() {
	d := DefaultQueryConfig()
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = d.MaxConcurrent
	}
	if c.MaxScan <= 0 {
		c.MaxScan = d.MaxScan
	}
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = d.DefaultLimit
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = d.MaxLimit
	}
	if c.DefaultLimit > c.MaxLimit {
		c.DefaultLimit = c.MaxLimit
	}
	if c.Timeout <= 0 {
		c.Timeout = d.Timeout
	}
	if c.Tolerance < 0 {
		c.Tolerance = 0
	}
}

// 查询错误
var (
	ErrQueryBusy    = errors.New("查询繁忙，请稍后重试")
	ErrInvalidQuery = errors.New("无效的查询参数")
)

// 支持的降采样方式
const (
	AggAvg  = "avg"
	AggMin  = "min"
	AggMax  = "max"
	AggLast = "last"
	AggLTTB = "lttb"
)

// RawQuery 原始数据查询条件
type RawQuery struct {
	DeviceID string
	Key      string
	Start    time.Time // 为空时为 End 之前 1 小时
	End      time.Time // 为空时为当前时间
	Limit    int       // 返回的最大点数，超出时降采样
	Agg      string    // avg | min | max | last | lttb，默认 avg
	// NoDownsample 返回全部原始点（用于导出），仍受 MaxScan 限制
	NoDownsample bool
}

// SeriesPoint 查询结果中的一个点
type SeriesPoint struct {
	Time    time.Time   `json:"t"`
	Value   interface{} `json:"v"`
	Quality int         `json:"q,omitempty"`
}

// SeriesResult 查询结果
type SeriesResult struct {
	DeviceID string        `json:"device_id"`
	Key      string        `json:"key"`
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Tier     string        `json:"tier"` // 数据层，目前只有 raw
	Points   []SeriesPoint `json:"points"`
	// Matched 时间范围内的原始点数（降采样前）
	Matched int `json:"matched"`
	// Scanned 扫描的消息数
	Scanned int `json:"scanned"`
	// Truncated 因扫描上限或超时没有读完整个时间范围
	Truncated   bool   `json:"truncated"`
	Downsampled bool   `json:"downsampled"`
	Agg         string `json:"agg,omitempty"`
}

// SeriesInfo 本地存储中的一个序列（设备 × 点位）
type SeriesInfo struct {
	DeviceID string `json:"device_id"`
	Key      string `json:"key"`
	Subject  string `json:"subject"`
	Messages uint64 `json:"messages"`
}

// normalize 校验查询条件并补齐默认值
func (s *Store) normalize(q *RawQuery) error {
	if strings.TrimSpace(q.DeviceID) == "" || strings.TrimSpace(q.Key) == "" {
		return fmt.Errorf("%w: device_id 和 key 不能为空", ErrInvalidQuery)
	}
	if q.End.IsZero() {
		q.End = time.Now()
	}
	if q.Start.IsZero() {
		q.Start = q.End.Add(-time.Hour)
	}
	if q.Start.After(q.End) {
		return fmt.Errorf("%w: start 不能晚于 end", ErrInvalidQuery)
	}
	qc := s.cfg.Query
	if q.Limit <= 0 {
		q.Limit = qc.DefaultLimit
	}
	if q.Limit > qc.MaxLimit {
		q.Limit = qc.MaxLimit
	}
	q.Agg = strings.ToLower(strings.TrimSpace(q.Agg))
	switch q.Agg {
	case "":
		q.Agg = AggAvg
	case AggAvg, AggMin, AggMax, AggLast, AggLTTB:
	default:
		return fmt.Errorf("%w: 不支持的 agg %q（可选 avg/min/max/last/lttb）", ErrInvalidQuery, q.Agg)
	}
	return nil
}

// acquire 获取查询许可（全局最多 MaxConcurrent 个查询同时执行）
func (s *Store) acquire(ctx context.Context) (func(), error) {
	select {
	case s.querySem <- struct{}{}:
		return func() { <-s.querySem }, nil
	case <-ctx.Done():
		return nil, ErrQueryBusy
	}
}

// QueryRaw 查询单个序列在时间范围内的原始数据；点数超过 Limit 时在服务端降采样。
func (s *Store) QueryRaw(ctx context.Context, q RawQuery) (*SeriesResult, error) {
	if err := s.normalize(&q); err != nil {
		return nil, err
	}
	qc := s.cfg.Query
	ctx, cancel := context.WithTimeout(ctx, qc.Timeout)
	defer cancel()
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	res := &SeriesResult{DeviceID: q.DeviceID, Key: q.Key, Start: q.Start, End: q.End, Tier: "raw", Points: []SeriesPoint{}}
	subject := model.DataSubject(q.DeviceID, q.Key)

	startAt := q.Start.Add(-qc.Tolerance)
	// 先看该序列最后一条消息：不存在或入库时间早于起点时直接返回，避免等待一个不会有消息的消费者
	last, err := s.js.GetLastMsg(s.cfg.Stream, subject, nats.Context(ctx))
	if err != nil {
		if errors.Is(err, nats.ErrMsgNotFound) {
			return res, nil
		}
		return nil, fmt.Errorf("读取序列最新消息失败: %w", err)
	}
	if last.Time.Before(startAt) {
		return res, nil
	}

	// 临时有序消费者：从 (start - 容忍窗口) 的入库时间开始顺序读取该序列。
	// 注意不能用 ConsumerInfo().NumPending 判断结束：推送式消费者创建后会立即把消息推送过来，
	// 此时服务端的待投递数已经是 0；每条消息元数据中的 NumPending 才是准确的剩余量。
	sub, err := s.js.SubscribeSync(subject,
		nats.BindStream(s.cfg.Stream),
		nats.OrderedConsumer(),
		nats.StartTime(startAt),
	)
	if err != nil {
		return nil, fmt.Errorf("创建查询消费者失败: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	stopAt := q.End.Add(qc.Tolerance)
	raw := make([]SeriesPoint, 0, minInt(q.Limit, qc.MaxScan))
	if q.NoDownsample {
		raw = make([]SeriesPoint, 0, 1024)
	}
	for {
		m, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				res.Truncated = true // 超时：返回已读取的部分
				break
			}
			return nil, fmt.Errorf("读取数据失败: %w", err)
		}
		res.Scanned++
		meta, _ := m.Metadata()
		pastEnd := false
		if m.Header == nil || m.Header.Get(ruleOutputHeader) == "" {
			var p model.Point
			if json.Unmarshal(m.Data, &p) == nil {
				if !p.Timestamp.Before(q.Start) && !p.Timestamp.After(q.End) {
					raw = append(raw, SeriesPoint{Time: p.Timestamp, Value: p.Value, Quality: p.Quality})
				}
				pastEnd = p.Timestamp.After(q.End)
			}
		}
		if meta == nil || meta.NumPending == 0 {
			break // 已读到该序列的最后一条
		}
		// 入库时间和采集时间都已超过结束时间：之后的数据（除极少数严重迟到的点外）不会落在范围内。
		// 只看入库时间会漏掉延迟上报的数据（例如 sidecar 批量补传的历史点）。
		if pastEnd && meta.Timestamp.After(stopAt) {
			break
		}
		if res.Scanned >= qc.MaxScan {
			res.Truncated = true
			break
		}
	}

	// 采集时间可能与入库顺序不一致，按采集时间排序
	sort.SliceStable(raw, func(i, j int) bool { return raw[i].Time.Before(raw[j].Time) })
	res.Matched = len(raw)
	if len(raw) > q.Limit && !q.NoDownsample {
		res.Points = Downsample(raw, q.Start, q.End, q.Limit, q.Agg)
		res.Downsampled = true
		res.Agg = q.Agg
	} else {
		res.Points = raw
	}
	return res, nil
}

// ListSeries 列出本地存储中的序列。deviceID 为空时列出全部设备。
// 返回分页后的序列和总数。
func (s *Store) ListSeries(ctx context.Context, deviceID string, offset, limit int) ([]SeriesInfo, int, error) {
	all, err := s.seriesFor(ctx, deviceID)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []SeriesInfo{}, total, nil
	}
	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}
	return all[offset:end], total, nil
}

// Latest 返回设备所有点位的最新值（按 key 排序）
func (s *Store) Latest(ctx context.Context, deviceID string) ([]model.Point, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("%w: device_id 不能为空", ErrInvalidQuery)
	}
	series, err := s.seriesFor(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	const maxSeries = 1000
	out := make([]model.Point, 0, len(series))
	for i, si := range series {
		if i >= maxSeries || ctx.Err() != nil {
			break
		}
		m, err := s.js.GetLastMsg(s.cfg.Stream, si.Subject)
		if err != nil {
			continue // 序列可能刚好被淘汰
		}
		var p model.Point
		if json.Unmarshal(m.Data, &p) != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// seriesFor 读取序列列表（带短时缓存，StreamInfo 的主题统计在序列很多时开销较大）
func (s *Store) seriesFor(ctx context.Context, deviceID string) ([]SeriesInfo, error) {
	filter := "iot.data.>"
	if deviceID != "" {
		filter = "iot.data." + model.EncodeSubjectToken(deviceID) + ".*"
	}
	s.seriesMu.Lock()
	if c, ok := s.seriesEntries[filter]; ok && time.Since(c.at) < seriesCacheTTL {
		s.seriesMu.Unlock()
		return c.list, nil
	}
	s.seriesMu.Unlock()

	si, err := s.js.StreamInfo(s.cfg.Stream, &nats.StreamInfoRequest{SubjectsFilter: filter}, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("读取序列列表失败: %w", err)
	}
	list := make([]SeriesInfo, 0, len(si.State.Subjects))
	for subj, n := range si.State.Subjects {
		parts := strings.Split(subj, ".")
		if len(parts) != 4 || parts[0] != "iot" || parts[1] != "data" {
			continue
		}
		list = append(list, SeriesInfo{
			DeviceID: model.DecodeSubjectToken(parts[2]),
			Key:      model.DecodeSubjectToken(parts[3]),
			Subject:  subj,
			Messages: n,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].DeviceID != list[j].DeviceID {
			return list[i].DeviceID < list[j].DeviceID
		}
		return list[i].Key < list[j].Key
	})
	s.seriesMu.Lock()
	s.seriesEntries[filter] = seriesCacheEntry{at: time.Now(), list: list}
	s.seriesMu.Unlock()
	return list, nil
}

const seriesCacheTTL = 5 * time.Second

type seriesCacheEntry struct {
	at   time.Time
	list []SeriesInfo
}

type seriesCacheState struct {
	seriesMu      sync.Mutex
	seriesEntries map[string]seriesCacheEntry
}

// numeric 把数值转换为 float64；bool 视为 0/1
func numeric(v interface{}) (float64, bool) {
	if b, ok := v.(bool); ok {
		if b {
			return 1, true
		}
		return 0, true
	}
	if _, isStr := v.(string); isStr {
		return 0, false
	}
	f, ok := northbound.ToFloat64(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// Downsample 把按时间排序的点降采样到不超过 limit 个。
// avg/min/max/last 按等宽时间桶聚合；lttb 用 Largest-Triangle-Three-Buckets 保留曲线形状。
// 只要有非数值点（字符串、复合类型），就退回按时间桶取最后一个值，保证返回的仍是原始值。
func Downsample(points []SeriesPoint, start, end time.Time, limit int, agg string) []SeriesPoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	vals := make([]float64, len(points))
	allNumeric := true
	for i, p := range points {
		f, ok := numeric(p.Value)
		if !ok {
			allNumeric = false
			break
		}
		vals[i] = f
	}
	if !allNumeric {
		agg = AggLast
	}
	if agg == AggLTTB && allNumeric {
		return lttb(points, vals, limit)
	}

	if !end.After(start) {
		start, end = points[0].Time, points[len(points)-1].Time
	}
	span := end.Sub(start)
	width := span / time.Duration(limit)
	if width <= 0 {
		width = 1
	}
	type bucket struct {
		n               int
		sum, minV, maxV float64
		last            SeriesPoint
	}
	buckets := make([]bucket, limit)
	for i, p := range points {
		idx := int(p.Time.Sub(start) / width)
		if idx < 0 {
			idx = 0
		}
		if idx >= limit {
			idx = limit - 1
		}
		b := &buckets[idx]
		if b.n == 0 {
			b.minV, b.maxV = math.Inf(1), math.Inf(-1)
		}
		b.n++
		b.last = p
		if allNumeric {
			v := vals[i]
			b.sum += v
			if v < b.minV {
				b.minV = v
			}
			if v > b.maxV {
				b.maxV = v
			}
		}
	}
	out := make([]SeriesPoint, 0, limit)
	for i := range buckets {
		b := &buckets[i]
		if b.n == 0 {
			continue
		}
		if agg == AggLast {
			out = append(out, b.last)
			continue
		}
		t := start.Add(time.Duration(i) * width)
		var v float64
		switch agg {
		case AggMin:
			v = b.minV
		case AggMax:
			v = b.maxV
		default:
			v = b.sum / float64(b.n)
		}
		out = append(out, SeriesPoint{Time: t, Value: v})
	}
	return out
}

// lttb Largest-Triangle-Three-Buckets 降采样（保留首尾点和曲线的形状特征）
func lttb(points []SeriesPoint, vals []float64, threshold int) []SeriesPoint {
	n := len(points)
	if threshold >= n || threshold < 3 {
		if threshold < 3 && n > threshold {
			out := []SeriesPoint{points[0]}
			if threshold == 2 {
				out = append(out, points[n-1])
			}
			return out
		}
		return points
	}
	x := func(i int) float64 { return float64(points[i].Time.UnixNano()) }
	out := make([]SeriesPoint, 0, threshold)
	out = append(out, points[0])
	every := float64(n-2) / float64(threshold-2)
	a := 0
	for i := 0; i < threshold-2; i++ {
		// 下一个桶的平均点
		avgStart := int(math.Floor(float64(i+1)*every)) + 1
		avgEnd := int(math.Floor(float64(i+2)*every)) + 1
		if avgEnd > n {
			avgEnd = n
		}
		var avgX, avgY float64
		cnt := avgEnd - avgStart
		if cnt <= 0 {
			avgX, avgY = x(n-1), vals[n-1]
		} else {
			for j := avgStart; j < avgEnd; j++ {
				avgX += x(j)
				avgY += vals[j]
			}
			avgX /= float64(cnt)
			avgY /= float64(cnt)
		}
		// 当前桶中与上一个选中点、下一个桶平均点构成最大三角形的点
		rangeStart := int(math.Floor(float64(i)*every)) + 1
		rangeEnd := int(math.Floor(float64(i+1)*every)) + 1
		if rangeEnd > n-1 {
			rangeEnd = n - 1
		}
		ax, ay := x(a), vals[a]
		maxArea, next := -1.0, rangeStart
		for j := rangeStart; j < rangeEnd; j++ {
			area := math.Abs((ax-avgX)*(vals[j]-ay)-(ax-x(j))*(avgY-ay)) * 0.5
			if area > maxArea {
				maxArea, next = area, j
			}
		}
		out = append(out, points[next])
		a = next
	}
	out = append(out, points[n-1])
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
