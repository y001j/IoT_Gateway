// Package storage 实现基于内嵌 JetStream 的本地暂存（断网续传）。
//
// 设计见 docs/design_local_storage.md：所有采集数据（iot.data.>）写入持久化流 IOT_RAW，
// 每个开启缓冲的 Sink 对应一个 JetStream 持久消费者（游标），Sink 确认写入成功后才 ACK，
// 目标不可用时数据留在本地磁盘，恢复后按原顺序补发；网关重启后从断点继续。
package storage

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Config 本地暂存配置（config.yaml 中的 storage 段）
type Config struct {
	Enabled bool
	// Stream 持久化流名称，默认 IOT_RAW
	Stream string
	// Subjects 写入流的主题，默认 iot.data.>
	Subjects []string
	// MaxAge 数据最长保留时间，默认 24h
	MaxAge time.Duration
	// MaxBytes 流最大占用磁盘字节数，默认 2GB；超出后丢弃最旧数据
	MaxBytes int64
	// MaxMsgs 流最大消息数（0 表示不限制）
	MaxMsgs int64
	// Compression 是否开启 S2 压缩，默认开启
	Compression bool
	// MaxDisk 内嵌 NATS 的 JetStream 总磁盘上限（JetStreamMaxStore），默认 4GB
	MaxDisk int64
	// SyncInterval 刷盘间隔（内嵌 NATS），默认 10s
	SyncInterval time.Duration
	// Query 近期数据查询的资源限制
	Query QueryConfig
	// Rollup 汇总层（长期趋势）
	Rollup RollupConfig
	// Alerts 积压/磁盘告警
	Alerts AlertConfig
	// StoreDir JetStream 数据目录（用于检查所在磁盘的使用率）
	StoreDir string
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Stream:       "IOT_RAW",
		Subjects:     []string{"iot.data.>"},
		MaxAge:       24 * time.Hour,
		MaxBytes:     2 << 30,
		Compression:  true,
		MaxDisk:      4 << 30,
		SyncInterval: 10 * time.Second,
		Query:        DefaultQueryConfig(),
		Rollup:       DefaultRollupConfig(),
		Alerts:       DefaultAlertConfig(),
		StoreDir:     "./data/jetstream",
	}
}

// ConfigFromViper 从 viper 读取 storage 配置
func ConfigFromViper(v *viper.Viper) (Config, error) {
	cfg := DefaultConfig()
	if v == nil {
		return cfg, nil
	}
	cfg.Enabled = v.GetBool("storage.enabled")
	if s := v.GetString("storage.raw.stream"); s != "" {
		cfg.Stream = s
	}
	if subs := v.GetStringSlice("storage.raw.subjects"); len(subs) > 0 {
		cfg.Subjects = subs
	}
	if d := v.GetDuration("storage.raw.max_age"); d > 0 {
		cfg.MaxAge = d
	}
	if s := v.GetString("storage.raw.max_bytes"); s != "" {
		n, err := ParseSize(s)
		if err != nil {
			return cfg, fmt.Errorf("storage.raw.max_bytes: %w", err)
		}
		cfg.MaxBytes = n
	}
	if n := v.GetInt64("storage.raw.max_msgs"); n > 0 {
		cfg.MaxMsgs = n
	}
	if v.IsSet("storage.raw.compression") {
		c := strings.ToLower(v.GetString("storage.raw.compression"))
		cfg.Compression = c != "" && c != "none" && c != "false"
	}
	if s := v.GetString("storage.max_disk"); s != "" {
		n, err := ParseSize(s)
		if err != nil {
			return cfg, fmt.Errorf("storage.max_disk: %w", err)
		}
		cfg.MaxDisk = n
	}
	if d := v.GetDuration("storage.sync_interval"); d > 0 {
		cfg.SyncInterval = d
	}
	if n := v.GetInt("storage.query.max_concurrent"); n > 0 {
		cfg.Query.MaxConcurrent = n
	}
	if n := v.GetInt("storage.query.max_scan"); n > 0 {
		cfg.Query.MaxScan = n
	}
	if n := v.GetInt("storage.query.default_limit"); n > 0 {
		cfg.Query.DefaultLimit = n
	}
	if n := v.GetInt("storage.query.max_limit"); n > 0 {
		cfg.Query.MaxLimit = n
	}
	if d := v.GetDuration("storage.query.timeout"); d > 0 {
		cfg.Query.Timeout = d
	}
	if v.IsSet("storage.query.tolerance") {
		cfg.Query.Tolerance = v.GetDuration("storage.query.tolerance")
	}
	if d := v.GetString("storage.store_dir"); d != "" {
		cfg.StoreDir = d
	}
	if v.IsSet("storage.alerts.enabled") {
		cfg.Alerts.Enabled = v.GetBool("storage.alerts.enabled")
	}
	if d := v.GetDuration("storage.alerts.interval"); d > 0 {
		cfg.Alerts.Interval = d
	}
	if d := v.GetDuration("storage.alerts.throttle"); d > 0 {
		cfg.Alerts.Throttle = d
	}
	for key, dst := range map[string]*float64{
		"storage.alerts.lag_warning_ratio":   &cfg.Alerts.LagWarningRatio,
		"storage.alerts.lag_critical_ratio":  &cfg.Alerts.LagCriticalRatio,
		"storage.alerts.disk_warning_ratio":  &cfg.Alerts.DiskWarningRatio,
		"storage.alerts.disk_critical_ratio": &cfg.Alerts.DiskCriticalRatio,
	} {
		if f := v.GetFloat64(key); f > 0 {
			*dst = f
		}
	}
	cfg.Rollup.Enabled = v.GetBool("storage.rollup.enabled")
	if s := v.GetString("storage.rollup.stream"); s != "" {
		cfg.Rollup.Stream = s
	}
	if d := v.GetDuration("storage.rollup.interval"); d > 0 {
		cfg.Rollup.Interval = d
	}
	if v.IsSet("storage.rollup.lateness") {
		cfg.Rollup.Lateness = v.GetDuration("storage.rollup.lateness")
	}
	if d := v.GetDuration("storage.rollup.max_age"); d > 0 {
		cfg.Rollup.MaxAge = d
	}
	if s := v.GetString("storage.rollup.max_bytes"); s != "" {
		n, err := ParseSize(s)
		if err != nil {
			return cfg, fmt.Errorf("storage.rollup.max_bytes: %w", err)
		}
		cfg.Rollup.MaxBytes = n
	}
	if cfg.Rollup.Enabled && cfg.MaxBytes+cfg.Rollup.MaxBytes > cfg.MaxDisk {
		return cfg, fmt.Errorf("storage.raw.max_bytes + storage.rollup.max_bytes (%d) 不能大于 storage.max_disk (%d)", cfg.MaxBytes+cfg.Rollup.MaxBytes, cfg.MaxDisk)
	}
	if cfg.MaxBytes > cfg.MaxDisk {
		return cfg, fmt.Errorf("storage.raw.max_bytes (%d) 不能大于 storage.max_disk (%d)", cfg.MaxBytes, cfg.MaxDisk)
	}
	return cfg, nil
}

// ParseSize 解析 "512MB"、"2GB"、"1048576" 等容量字符串
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("容量为空")
	}
	units := []struct {
		suffix string
		mul    int64
	}{{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("无效的容量: %q", s)
			}
			return int64(n * float64(u.mul)), nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("无效的容量: %q", s)
	}
	return n, nil
}

// Store 本地暂存服务
type Store struct {
	nc       *nats.Conn
	js       nats.JetStreamContext
	cfg      Config
	querySem chan struct{}
	rollup   atomic.Value // *Rollup
	monitor  atomic.Value // *AlertMonitor
	seriesCacheState
}

// New 检查 JetStream 可用性并创建/更新持久化流
func New(nc *nats.Conn, cfg Config) (*Store, error) {
	if nc == nil {
		return nil, fmt.Errorf("NATS 连接未初始化")
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("创建 JetStream 上下文失败: %w", err)
	}
	if _, err := js.AccountInfo(); err != nil {
		return nil, fmt.Errorf("JetStream 不可用（外部 NATS 需开启 JetStream）: %w", err)
	}
	if cfg.Stream == "" {
		cfg.Stream = "IOT_RAW"
	}
	if len(cfg.Subjects) == 0 {
		cfg.Subjects = []string{"iot.data.>"}
	}
	cfg.Query.applyDefaults()
	s := &Store{nc: nc, js: js, cfg: cfg, querySem: make(chan struct{}, cfg.Query.MaxConcurrent)}
	s.seriesEntries = make(map[string]seriesCacheEntry)
	if err := s.ensureStream(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) streamConfig() *nats.StreamConfig {
	sc := &nats.StreamConfig{
		Name:      s.cfg.Stream,
		Subjects:  s.cfg.Subjects,
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		Discard:   nats.DiscardOld,
		MaxAge:    s.cfg.MaxAge,
		MaxBytes:  s.cfg.MaxBytes,
		Replicas:  1,
	}
	if s.cfg.MaxMsgs > 0 {
		sc.MaxMsgs = s.cfg.MaxMsgs
	}
	if s.cfg.Compression {
		sc.Compression = nats.S2Compression
	}
	return sc
}

func (s *Store) ensureStream() error {
	sc := s.streamConfig()
	if _, err := s.js.StreamInfo(sc.Name); err == nil {
		if _, err := s.js.UpdateStream(sc); err != nil {
			return fmt.Errorf("更新持久化流 %s 失败: %w", sc.Name, err)
		}
	} else {
		if _, err := s.js.AddStream(sc); err != nil {
			return fmt.Errorf("创建持久化流 %s 失败: %w", sc.Name, err)
		}
	}
	log.Info().Str("stream", sc.Name).Strs("subjects", sc.Subjects).Dur("max_age", sc.MaxAge).
		Int64("max_bytes", sc.MaxBytes).Msg("本地暂存流已就绪")
	return nil
}

// JS 返回 JetStream 上下文
func (s *Store) JS() nats.JetStreamContext { return s.js }

// Config 返回配置
func (s *Store) Config() Config { return s.cfg }

// StreamStatus 持久化流状态
type StreamStatus struct {
	Stream    string    `json:"stream"`
	Messages  uint64    `json:"messages"`
	Bytes     uint64    `json:"bytes"`
	FirstSeq  uint64    `json:"first_seq"`
	LastSeq   uint64    `json:"last_seq"`
	FirstTime time.Time `json:"first_time"`
	LastTime  time.Time `json:"last_time"`
	MaxBytes  int64     `json:"max_bytes"`
	MaxAge    string    `json:"max_age"`
	Usage     float64   `json:"usage_ratio"` // Bytes / MaxBytes
}

// Status 返回持久化流状态
func (s *Store) Status() (*StreamStatus, error) {
	si, err := s.js.StreamInfo(s.cfg.Stream)
	if err != nil {
		return nil, err
	}
	st := &StreamStatus{
		Stream:    si.Config.Name,
		Messages:  si.State.Msgs,
		Bytes:     si.State.Bytes,
		FirstSeq:  si.State.FirstSeq,
		LastSeq:   si.State.LastSeq,
		FirstTime: si.State.FirstTime,
		LastTime:  si.State.LastTime,
		MaxBytes:  si.Config.MaxBytes,
		MaxAge:    si.Config.MaxAge.String(),
	}
	if si.Config.MaxBytes > 0 {
		st.Usage = float64(si.State.Bytes) / float64(si.Config.MaxBytes)
	}
	return st, nil
}
