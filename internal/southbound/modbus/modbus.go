package modbus

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goburrow/modbus"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	southbound.Register("modbus", NewAdapter)
}

const (
	// 单次读取寄存器数量上限（Modbus 协议规定 FC03/FC04 最多 125 个寄存器）
	maxRegistersPerRead = 125
	// 单次读取线圈数量上限（协议规定最多 2000 个）
	maxBitsPerRead     = 2000
	defaultMaxReadGap  = 10
	defaultMaxBackoff  = 60 * time.Second
	initialBackoff     = 1 * time.Second
	defaultPollTimeout = 3 * time.Second
)

// connHandler 抽象 TCP/RTU 连接处理器，便于测试时替换
type connHandler interface {
	Connect() error
	Close() error
}

// ModbusAdapter Modbus 适配器，支持 TCP 和 RTU 模式
type ModbusAdapter struct {
	*southbound.BaseAdapter
	mode       string // "tcp" 或 "rtu"
	interval   time.Duration
	client     modbus.Client
	handler    connHandler
	registers  []config.ModbusRegister
	groups     []readGroup
	byteOrder  string
	minBackoff time.Duration
	maxBackoff time.Duration

	mutex     sync.Mutex
	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	connected atomic.Bool

	parser *config.ConfigParser[config.ModbusConfig]
}

// readGroup 一次 Modbus 请求读取的一组寄存器（同类型、地址连续或间隔很小）
type readGroup struct {
	regType string
	start   uint16
	count   uint16 // 寄存器数量（或线圈数量）
	members []config.ModbusRegister
}

// Name 返回适配器名称
func (a *ModbusAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *ModbusAdapter) Init(cfg json.RawMessage) error {
	a.parser = config.NewParserWithDefaults(config.GetDefaultModbusConfig())
	modbusConfig, err := a.parser.Parse(cfg)
	if err != nil {
		return fmt.Errorf("解析Modbus配置失败: %w", err)
	}
	return a.initWithConfig(modbusConfig)
}

func (a *ModbusAdapter) initWithConfig(cfg *config.ModbusConfig) error {
	a.BaseAdapter = southbound.NewBaseAdapter(cfg.Name, "modbus")
	a.mode = strings.ToLower(cfg.Protocol)
	if a.mode == "" {
		a.mode = "tcp"
	}
	a.interval = cfg.Interval.Duration()
	if a.interval <= 0 {
		a.interval = 5 * time.Second
	}
	a.byteOrder = strings.ToUpper(cfg.ByteOrder)
	if a.byteOrder == "" {
		a.byteOrder = "ABCD"
	}
	a.minBackoff = initialBackoff
	a.maxBackoff = cfg.ReconnectMaxBackoff.Duration()
	if a.maxBackoff <= 0 {
		a.maxBackoff = defaultMaxBackoff
	}

	a.registers = make([]config.ModbusRegister, len(cfg.Registers))
	for i, reg := range cfg.Registers {
		if reg.Scale == 0 {
			// scale 未配置时按 1 处理（以前为 0，导致所有读数都等于 offset）
			reg.Scale = 1
		}
		if reg.DataType == "" {
			reg.DataType = "uint16"
		}
		if reg.ByteOrder == "" {
			reg.ByteOrder = a.byteOrder
		}
		a.registers[i] = reg
	}

	maxGap := cfg.MaxReadGap
	if maxGap == 0 {
		maxGap = defaultMaxReadGap
	}
	a.groups = buildReadGroups(a.registers, !cfg.DisableMerge, maxGap)

	timeout := cfg.Timeout.Duration()
	if timeout <= 0 {
		timeout = defaultPollTimeout
	}

	switch a.mode {
	case "tcp":
		if cfg.Host == "" {
			return fmt.Errorf("Modbus TCP 模式必须配置 host")
		}
		h := modbus.NewTCPClientHandler(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
		h.Timeout = timeout
		h.SlaveId = cfg.SlaveID
		a.handler = h
		a.client = modbus.NewClient(h)
	case "rtu":
		if cfg.SerialPort == "" {
			return fmt.Errorf("Modbus RTU 模式必须配置 serial_port")
		}
		h := modbus.NewRTUClientHandler(cfg.SerialPort)
		h.BaudRate = orDefault(cfg.BaudRate, 9600)
		h.DataBits = orDefault(cfg.DataBits, 8)
		h.StopBits = orDefault(cfg.StopBits, 1)
		h.Parity = cfg.Parity
		if h.Parity == "" {
			h.Parity = "N"
		}
		h.Timeout = timeout
		h.SlaveId = cfg.SlaveID
		a.handler = h
		a.client = modbus.NewClient(h)
	default:
		return fmt.Errorf("不支持的Modbus模式: %s", a.mode)
	}

	log.Info().
		Str("name", a.Name()).
		Str("mode", a.mode).
		Int("registers", len(a.registers)).
		Int("read_requests_per_poll", len(a.groups)).
		Dur("interval", a.interval).
		Msg("Modbus适配器初始化完成")
	return nil
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// registerWidth 返回数据类型占用的寄存器数量
func registerWidth(reg config.ModbusRegister) uint16 {
	switch reg.Type {
	case "coil", "discrete_input":
		return 1
	}
	return getRegisterCount(reg.DataType)
}

// getRegisterCount 返回数据类型需要的寄存器数量
func getRegisterCount(dataType string) uint16 {
	switch dataType {
	case "uint32", "int32", "float32":
		return 2
	case "uint64", "int64", "float64":
		return 4
	default:
		return 1
	}
}

// buildReadGroups 把寄存器按类型分组，并把地址相邻（间隔不超过 maxGap）的寄存器合并为一次读取。
// 以前每个寄存器单独发一次请求，100 个寄存器就要 100 次往返。
func buildReadGroups(regs []config.ModbusRegister, merge bool, maxGap uint16) []readGroup {
	byType := map[string][]config.ModbusRegister{}
	var types []string
	for _, r := range regs {
		if _, ok := byType[r.Type]; !ok {
			types = append(types, r.Type)
		}
		byType[r.Type] = append(byType[r.Type], r)
	}
	sort.Strings(types)

	var groups []readGroup
	for _, t := range types {
		list := byType[t]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Address < list[j].Address })
		limit := uint32(maxRegistersPerRead)
		if t == "coil" || t == "discrete_input" {
			limit = maxBitsPerRead
		}
		var cur *readGroup
		for _, r := range list {
			w := uint32(registerWidth(r))
			end := uint32(r.Address) + w // 不含
			if cur != nil && merge {
				curEnd := uint32(cur.start) + uint32(cur.count)
				newEnd := end
				if curEnd > newEnd {
					newEnd = curEnd
				}
				if uint32(r.Address) <= curEnd+uint32(maxGap) && newEnd-uint32(cur.start) <= limit {
					cur.count = uint16(newEnd - uint32(cur.start)) // #nosec G115 -- 受 limit（≤2000）约束
					cur.members = append(cur.members, r)
					continue
				}
			}
			groups = append(groups, readGroup{regType: t, start: r.Address, count: uint16(w), members: []config.ModbusRegister{r}}) // #nosec G115 -- 寄存器宽度最多 4
			cur = &groups[len(groups)-1]
		}
	}
	return groups
}

// Start 启动适配器。连接在后台建立，设备暂时不可达不会阻塞网关启动。
func (a *ModbusAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.running {
		return nil
	}
	a.running = true
	a.stopCh = make(chan struct{})
	a.doneCh = make(chan struct{})
	a.SetRunning(true)

	go a.loop(ctx, ch, a.stopCh, a.doneCh)
	log.Info().Str("name", a.Name()).Msg("Modbus适配器启动")
	return nil
}

func (a *ModbusAdapter) loop(ctx context.Context, ch chan<- model.Point, stopCh, doneCh chan struct{}) {
	defer close(doneCh)
	defer a.disconnect()

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	minBackoff := a.minBackoff
	if minBackoff <= 0 {
		minBackoff = initialBackoff
	}
	backoff := minBackoff
	var nextConnect time.Time

	poll := func() {
		if !a.connected.Load() {
			if time.Now().Before(nextConnect) {
				return
			}
			if err := a.connect(); err != nil {
				a.SetLastError(err)
				log.Warn().Err(err).Str("name", a.Name()).Dur("retry_in", backoff).Msg("Modbus连接失败，稍后重试")
				nextConnect = time.Now().Add(backoff)
				backoff *= 2
				if backoff > a.maxBackoff {
					backoff = a.maxBackoff
				}
				return
			}
			backoff = minBackoff
		}
		if err := a.pollOnce(ch); err != nil && isConnectionError(err) {
			a.disconnect()
		}
	}

	poll()
	for {
		select {
		case <-ticker.C:
			poll()
		case <-stopCh:
			log.Info().Str("name", a.Name()).Msg("Modbus适配器停止")
			return
		case <-ctx.Done():
			return
		}
	}
}

func (a *ModbusAdapter) connect() error {
	if a.handler == nil {
		return fmt.Errorf("Modbus处理器未初始化")
	}
	if err := a.handler.Connect(); err != nil {
		return err
	}
	a.connected.Store(true)
	log.Info().Str("name", a.Name()).Str("mode", a.mode).Msg("Modbus设备连接成功")
	return nil
}

func (a *ModbusAdapter) disconnect() {
	if !a.connected.Swap(false) {
		return
	}
	if a.handler != nil {
		_ = a.handler.Close()
	}
	log.Info().Str("name", a.Name()).Msg("Modbus设备连接已断开")
}

// pollOnce 执行一轮采集。返回遇到的最后一个错误（用于判断是否需要重连）。
func (a *ModbusAdapter) pollOnce(ch chan<- model.Point) error {
	pollStart := time.Now()
	var lastErr error
	for _, g := range a.groups {
		raw, err := a.readGroup(g)
		if err != nil {
			lastErr = err
			a.SetLastError(err)
			log.Error().Err(err).Str("name", a.Name()).Str("type", g.regType).
				Uint16("start", g.start).Uint16("count", g.count).Msg("读取寄存器失败")
			if isConnectionError(err) {
				return err
			}
			continue
		}
		for _, reg := range g.members {
			value, dataType, err := extractValue(raw, g, reg)
			if err != nil {
				lastErr = err
				log.Error().Err(err).Str("name", a.Name()).Str("key", reg.Key).Msg("解析寄存器数据失败")
				continue
			}
			point := model.NewPoint(reg.Key, reg.DeviceID, value, dataType)
			point.Timestamp = pollStart
			point.AddTag("source", "modbus")
			point.AddTag("mode", a.mode)
			point.AddTag("address", fmt.Sprintf("%d", reg.Address))
			point.AddTag("type", reg.Type)
			a.SafeSendDataPoint(ch, point, pollStart)
		}
	}
	return lastErr
}

func (a *ModbusAdapter) readGroup(g readGroup) ([]byte, error) {
	switch g.regType {
	case "coil":
		return a.client.ReadCoils(g.start, g.count)
	case "discrete_input":
		return a.client.ReadDiscreteInputs(g.start, g.count)
	case "input_register":
		return a.client.ReadInputRegisters(g.start, g.count)
	case "holding_register":
		return a.client.ReadHoldingRegisters(g.start, g.count)
	default:
		return nil, fmt.Errorf("不支持的寄存器类型: %s", g.regType)
	}
}

// extractValue 从一次合并读取的原始数据中取出单个寄存器的值
func extractValue(raw []byte, g readGroup, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	offset := int(reg.Address - g.start)
	switch reg.Type {
	case "coil", "discrete_input":
		byteIdx := offset / 8
		if byteIdx >= len(raw) {
			return nil, "", fmt.Errorf("数据长度不足")
		}
		return raw[byteIdx]&(1<<(uint(offset)%8)) != 0, model.TypeBool, nil // #nosec G115 -- offset 为非负的位偏移
	}
	start := offset * 2
	width := int(getRegisterCount(reg.DataType)) * 2
	if start+width > len(raw) {
		return nil, "", fmt.Errorf("数据长度不足，需要%d字节", width)
	}
	return parseRegisterData(raw[start:start+width], reg)
}

// reorder 按字节序把原始数据转换为大端（ABCD）顺序。
//
//	ABCD: 大端（默认）
//	CDAB: 字交换（低字在前），常见于很多 PLC 的 32 位数
//	BADC: 字节交换
//	DCBA: 小端
func reorder(b []byte, order string) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	if len(b) < 4 {
		if order == "BADC" || order == "DCBA" {
			if len(out) == 2 {
				out[0], out[1] = out[1], out[0]
			}
		}
		return out
	}
	switch order {
	case "CDAB":
		// 交换字的顺序
		words := len(b) / 2
		for i := 0; i < words; i++ {
			copy(out[i*2:i*2+2], b[(words-1-i)*2:(words-i)*2])
		}
	case "BADC":
		for i := 0; i+1 < len(b); i += 2 {
			out[i], out[i+1] = b[i+1], b[i]
		}
	case "DCBA":
		for i := range b {
			out[i] = b[len(b)-1-i]
		}
	}
	return out
}

// parseRegisterData 解析寄存器数据并做线性换算
func parseRegisterData(data []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	need := int(getRegisterCount(reg.DataType)) * 2
	if len(data) < need {
		return nil, "", fmt.Errorf("数据长度不足，需要%d字节", need)
	}
	b := reorder(data[:need], reg.ByteOrder)

	var raw float64
	var intVal int64
	var uintVal uint64
	isFloat, isUnsigned := false, false

	switch reg.DataType {
	case "uint16", "":
		uintVal = uint64(binary.BigEndian.Uint16(b))
		isUnsigned = true
	case "int16":
		intVal = int64(int16(binary.BigEndian.Uint16(b))) // #nosec G115 -- 按补码解释有符号寄存器
	case "uint32":
		uintVal = uint64(binary.BigEndian.Uint32(b))
		isUnsigned = true
	case "int32":
		intVal = int64(int32(binary.BigEndian.Uint32(b))) // #nosec G115 -- 按补码解释有符号寄存器
	case "uint64":
		uintVal = binary.BigEndian.Uint64(b)
		isUnsigned = true
	case "int64":
		intVal = int64(binary.BigEndian.Uint64(b)) // #nosec G115 -- 按补码解释有符号寄存器
	case "float32":
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
		isFloat = true
	case "float64":
		raw = math.Float64frombits(binary.BigEndian.Uint64(b))
		isFloat = true
	default:
		return nil, "", fmt.Errorf("不支持的数据类型: %s", reg.DataType)
	}

	scale := reg.Scale
	if scale == 0 {
		scale = 1
	}
	if isFloat {
		return raw*scale + reg.Offset, model.TypeFloat, nil
	}
	// 整数：没有换算时保持整数类型；有换算时输出浮点数，避免 23.5 被截断为 23
	if scale == 1 && reg.Offset == 0 {
		if isUnsigned {
			if uintVal > math.MaxInt64 {
				return float64(uintVal), model.TypeFloat, nil
			}
			return int64(uintVal), model.TypeInt, nil
		}
		return intVal, model.TypeInt, nil
	}
	if isUnsigned {
		return float64(uintVal)*scale + reg.Offset, model.TypeFloat, nil
	}
	return float64(intVal)*scale + reg.Offset, model.TypeFloat, nil
}

// isConnectionError 判断是否为连接错误（需要断开重连）
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, k := range []string{"connection", "timeout", "refused", "broken pipe", "reset by peer", "eof", "no such file", "closed"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// Stop 停止适配器并等待采集协程退出
func (a *ModbusAdapter) Stop() error {
	a.mutex.Lock()
	if !a.running {
		a.mutex.Unlock()
		return nil
	}
	a.running = false
	close(a.stopCh)
	done := a.doneCh
	a.mutex.Unlock()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		log.Warn().Str("name", a.Name()).Msg("等待Modbus采集协程退出超时")
	}
	if a.BaseAdapter != nil {
		a.SetRunning(false)
	}
	return nil
}

// NewAdapter 创建一个新的Modbus适配器实例
func NewAdapter() southbound.Adapter {
	return &ModbusAdapter{}
}
