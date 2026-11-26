package actions

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// statsSnapshot 是 IncrementalStats 可序列化的完整状态（用于持久化和重启后恢复）
type statsSnapshot struct {
	Count           int64            `json:"count"`
	Sum             float64          `json:"sum"`
	SumSquares      float64          `json:"sum_squares"`
	Min             *float64         `json:"min,omitempty"` // 没有数据时为正负无穷，JSON 无法表示，用 nil 代替
	Max             *float64         `json:"max,omitempty"`
	LastValue       float64          `json:"last_value"`
	FirstValue      float64          `json:"first_value"`
	FirstValueSet   bool             `json:"first_value_set"`
	LastUpdateTime  time.Time        `json:"last_update_time"`
	FirstUpdateTime time.Time        `json:"first_update_time"`
	WindowSize      int              `json:"window_size"`
	Values          []float64        `json:"values,omitempty"`
	ValueIndex      int              `json:"value_index"`
	WindowFull      bool             `json:"window_full"`
	WindowType      string           `json:"window_type"`
	WindowDuration  time.Duration    `json:"window_duration"`
	TimeValues      []TimestampValue `json:"time_values,omitempty"`
	TimeIndex       int              `json:"time_index"`
	Alignment       string           `json:"alignment"`
	NullCount       int64            `json:"null_count"`
	ValidCount      int64            `json:"valid_count"`
	UpperLimit      *float64         `json:"upper_limit,omitempty"`
	LowerLimit      *float64         `json:"lower_limit,omitempty"`
	OutlierThresh   float64          `json:"outlier_threshold"`
}

func finiteOrNil(v float64) *float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return nil
	}
	return &v
}

// snapshot 导出统计状态
func (s *IncrementalStats) snapshot() statsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return statsSnapshot{
		Count: s.count, Sum: s.sum, SumSquares: s.sumSquares,
		Min: finiteOrNil(s.min), Max: finiteOrNil(s.max),
		LastValue: s.lastValue, FirstValue: s.firstValue, FirstValueSet: s.firstValueSet,
		LastUpdateTime: s.lastUpdateTime, FirstUpdateTime: s.firstUpdateTime,
		WindowSize: s.windowSize, Values: append([]float64(nil), s.values...), ValueIndex: s.valueIndex, WindowFull: s.windowFull,
		WindowType: s.windowType, WindowDuration: s.windowDuration,
		TimeValues: append([]TimestampValue(nil), s.timeValues...), TimeIndex: s.timeIndex, Alignment: s.alignment,
		NullCount: s.nullCount, ValidCount: s.validCount,
		UpperLimit: s.upperLimit, LowerLimit: s.lowerLimit, OutlierThresh: s.outlierThreshold,
	}
}

// statsFromSnapshot 从快照恢复统计状态
func statsFromSnapshot(sn statsSnapshot) *IncrementalStats {
	s := NewIncrementalStatsWithWindow(sn.WindowSize, sn.WindowType, sn.WindowDuration, sn.Alignment)
	s.count, s.sum, s.sumSquares = sn.Count, sn.Sum, sn.SumSquares
	if sn.Min != nil {
		s.min = *sn.Min
	}
	if sn.Max != nil {
		s.max = *sn.Max
	}
	s.lastValue, s.firstValue, s.firstValueSet = sn.LastValue, sn.FirstValue, sn.FirstValueSet
	s.lastUpdateTime, s.firstUpdateTime = sn.LastUpdateTime, sn.FirstUpdateTime
	// 缓冲区长度必须与窗口大小一致（AddValue 按 windowSize 取模写入）
	if len(sn.Values) == len(s.values) {
		copy(s.values, sn.Values)
		s.valueIndex, s.windowFull = sn.ValueIndex, sn.WindowFull
	}
	if len(sn.TimeValues) == len(s.timeValues) {
		copy(s.timeValues, sn.TimeValues)
		s.timeIndex = sn.TimeIndex
	}
	s.nullCount, s.validCount = sn.NullCount, sn.ValidCount
	s.upperLimit, s.lowerLimit = sn.UpperLimit, sn.LowerLimit
	if sn.OutlierThresh > 0 {
		s.outlierThreshold = sn.OutlierThresh
	}
	return s
}

// aggStateSnapshot 一个聚合状态（规则 + 分组）的快照
type aggStateSnapshot struct {
	Key         string        `json:"key"`
	Fingerprint string        `json:"fingerprint"`
	TTL         time.Duration `json:"ttl"`
	LastAccess  time.Time     `json:"last_access"`
	WindowSize  int           `json:"window_size"`
	Stats       statsSnapshot `json:"stats"`
}

// AggregateStatePersister 保存和加载聚合状态快照
type AggregateStatePersister interface {
	// Load 返回全部快照
	Load() ([]aggStateSnapshot, error)
	// Save 写入（覆盖）给定的快照，并删除 removed 中的状态
	Save(snapshots []aggStateSnapshot, removed []string) error
}

// configFingerprint 计算会影响统计状态的配置的指纹：这些配置变化时旧状态不能继续使用
func configFingerprint(windowSize int, cfg *AggregateConfig) string {
	if cfg == nil {
		return fmt.Sprintf("size=%d", windowSize)
	}
	parts := struct {
		Size      int
		Type      string
		Duration  time.Duration
		Alignment string
		Upper     *float64
		Lower     *float64
		Outlier   *float64
		Output    map[string]interface{}
	}{windowSize, cfg.WindowType, cfg.WindowDuration, cfg.Alignment, cfg.UpperLimit, cfg.LowerLimit, cfg.OutlierThreshold, cfg.Output}
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// stateTTL 状态的闲置过期时间：至少覆盖两个时间窗口，避免低频数据的长窗口在中途被清掉
func stateTTL(defaultTTL time.Duration, cfg *AggregateConfig) time.Duration {
	ttl := defaultTTL
	if cfg != nil {
		if cfg.TTL > ttl {
			ttl = cfg.TTL
		}
		if w := 2 * cfg.WindowDuration; w > ttl {
			ttl = w
		}
	}
	return ttl
}

// EnablePersistence 从持久化存储恢复状态，并按 interval 定期保存发生变化的状态。
// 必须在处理数据之前调用；Close 时会再保存一次。
func (m *AggregateManager) EnablePersistence(p AggregateStatePersister, interval time.Duration) error {
	if p == nil {
		return errors.New("persister 不能为空")
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	snaps, err := p.Load()
	if err != nil {
		return fmt.Errorf("加载聚合状态失败: %w", err)
	}
	now := time.Now()
	restored, expired := 0, []string{}
	m.mu.Lock()
	for _, sn := range snaps {
		ttl := sn.TTL
		if ttl <= 0 {
			ttl = m.defaultTTL
		}
		if now.Sub(sn.LastAccess) > ttl {
			expired = append(expired, sn.Key)
			continue
		}
		m.states[sn.Key] = &AggregateState{
			GroupKey:    sn.Key,
			WindowSize:  sn.WindowSize,
			Stats:       statsFromSnapshot(sn.Stats),
			LastAccess:  sn.LastAccess,
			ttl:         ttl,
			fingerprint: sn.Fingerprint,
		}
		m.currentMem += m.estimateStateSize(sn.WindowSize)
		restored++
	}
	m.persister = p
	m.lastSave = now
	m.removed = append(m.removed, expired...)
	m.mu.Unlock()
	log.Info().Int("restored", restored).Int("expired", len(expired)).Msg("聚合状态已从持久化存储恢复")

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				if err := m.saveChanged(); err != nil {
					log.Warn().Err(err).Msg("保存聚合状态失败")
				}
			}
		}
	}()
	return nil
}

// saveChanged 保存上次保存之后有变化的状态，并删除已清理的状态
func (m *AggregateManager) saveChanged() error {
	m.mu.Lock()
	p := m.persister
	if p == nil {
		m.mu.Unlock()
		return nil
	}
	since := m.lastSave
	now := time.Now()
	var snaps []aggStateSnapshot
	for key, st := range m.states {
		st.mu.RLock()
		changed := st.LastAccess.After(since)
		sn := aggStateSnapshot{Key: key, Fingerprint: st.fingerprint, TTL: st.ttl, LastAccess: st.LastAccess, WindowSize: st.WindowSize}
		st.mu.RUnlock()
		if !changed {
			continue
		}
		sn.Stats = st.Stats.snapshot()
		snaps = append(snaps, sn)
	}
	removed := m.removed
	m.removed = nil
	m.lastSave = now
	m.mu.Unlock()

	if len(snaps) == 0 && len(removed) == 0 {
		return nil
	}
	if err := p.Save(snaps, removed); err != nil {
		// 失败时下次重试：回退保存时间并保留待删除列表
		m.mu.Lock()
		m.lastSave = since
		m.removed = append(m.removed, removed...)
		m.mu.Unlock()
		return err
	}
	return nil
}

// AggStateKVBucket 保存聚合状态的 JetStream KV 存储桶
const AggStateKVBucket = "IOT_AGG_STATE"

// kvPersister 用 JetStream KV 保存聚合状态（每个状态一个键）
type kvPersister struct {
	kv nats.KeyValue
}

// NewKVAggregateStatePersister 打开（不存在时创建）聚合状态的 KV 存储桶
func NewKVAggregateStatePersister(js nats.JetStreamContext) (AggregateStatePersister, error) {
	kv, err := js.KeyValue(AggStateKVBucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      AggStateKVBucket,
			Description: "规则引擎聚合窗口状态（重启后恢复）",
			History:     1,
			Storage:     nats.FileStorage,
		})
	}
	if err != nil {
		return nil, err
	}
	return &kvPersister{kv: kv}, nil
}

// KV 的键只允许字母、数字和 -_=./，状态键（规则 ID、设备 ID）可能含有其他字符
func kvKey(stateKey string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(stateKey))
}

func (p *kvPersister) Load() ([]aggStateSnapshot, error) {
	keys, err := p.kv.Keys()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]aggStateSnapshot, 0, len(keys))
	for _, k := range keys {
		entry, err := p.kv.Get(k)
		if err != nil {
			continue
		}
		var sn aggStateSnapshot
		if err := json.Unmarshal(entry.Value(), &sn); err != nil {
			log.Warn().Err(err).Str("key", k).Msg("忽略无法解析的聚合状态快照")
			continue
		}
		out = append(out, sn)
	}
	return out, nil
}

func (p *kvPersister) Save(snapshots []aggStateSnapshot, removed []string) error {
	for _, sn := range snapshots {
		data, err := json.Marshal(sn)
		if err != nil {
			return err
		}
		if _, err := p.kv.Put(kvKey(sn.Key), data); err != nil {
			return err
		}
	}
	for _, key := range removed {
		if err := p.kv.Delete(kvKey(key)); err != nil && !errors.Is(err, nats.ErrKeyNotFound) {
			return err
		}
	}
	return nil
}

// closeWithFinalSave 停止后台协程并做最后一次保存
func (m *AggregateManager) closeWithFinalSave() {
	m.cancel()
	m.wg.Wait()
	if err := m.saveChanged(); err != nil {
		log.Warn().Err(err).Msg("关闭时保存聚合状态失败")
	}
}
