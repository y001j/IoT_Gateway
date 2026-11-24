package modbus

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goburrow/modbus"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// fakeClient 模拟一个 Modbus 从站：寄存器和线圈保存在内存中，并记录请求次数
type fakeClient struct {
	modbus.Client // 未实现的方法调用时会 panic
	mu            sync.Mutex
	holding       map[uint16]uint16
	coils         map[uint16]bool
	requests      int
	failWith      error
}

func newFakeClient() *fakeClient {
	return &fakeClient{holding: map[uint16]uint16{}, coils: map[uint16]bool{}}
}

func (f *fakeClient) readRegs(addr, qty uint16) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]byte, int(qty)*2)
	for i := uint16(0); i < qty; i++ {
		binary.BigEndian.PutUint16(out[i*2:], f.holding[addr+i])
	}
	return out, nil
}
func (f *fakeClient) ReadHoldingRegisters(a, q uint16) ([]byte, error) { return f.readRegs(a, q) }
func (f *fakeClient) ReadInputRegisters(a, q uint16) ([]byte, error)   { return f.readRegs(a, q) }
func (f *fakeClient) readBits(addr, qty uint16) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]byte, (qty+7)/8)
	for i := uint16(0); i < qty; i++ {
		if f.coils[addr+i] {
			out[i/8] |= 1 << (i % 8)
		}
	}
	return out, nil
}
func (f *fakeClient) ReadCoils(a, q uint16) ([]byte, error)          { return f.readBits(a, q) }
func (f *fakeClient) ReadDiscreteInputs(a, q uint16) ([]byte, error) { return f.readBits(a, q) }

type fakeHandler struct {
	connectErr error
	connects   int32
	closes     int32
}

func (h *fakeHandler) Connect() error { atomic.AddInt32(&h.connects, 1); return h.connectErr }
func (h *fakeHandler) Close() error   { atomic.AddInt32(&h.closes, 1); return nil }

func reg(addr uint16, typ, dt, key string) config.ModbusRegister {
	return config.ModbusRegister{Address: addr, Type: typ, DataType: dt, DeviceID: "plc", Key: key, Scale: 1, ByteOrder: "ABCD"}
}

func TestParseRegisterDataScaling(t *testing.T) {
	be16 := func(v uint16) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, v); return b }

	// 回归：scale 未配置（0）时以前所有读数都等于 offset
	r := config.ModbusRegister{DataType: "uint16"}
	v, typ, err := parseRegisterData(be16(1234), r)
	if err != nil || v.(int64) != 1234 || typ != model.TypeInt {
		t.Fatalf("未配置 scale 时应返回原值: %v %v %v", v, typ, err)
	}

	// 回归：整数乘小数系数以前被截断（235*0.1 = 23）
	r = config.ModbusRegister{DataType: "int16", Scale: 0.1}
	v, typ, _ = parseRegisterData(be16(235), r)
	if typ != model.TypeFloat || math.Abs(v.(float64)-23.5) > 1e-9 {
		t.Fatalf("带小数系数应输出浮点数 23.5, got %v (%s)", v, typ)
	}

	// 负数与 offset
	r = config.ModbusRegister{DataType: "int16", Scale: 1, Offset: -40}
	v, _, _ = parseRegisterData(be16(uint16(0xFFFF)), r) // -1
	if v.(float64) != -41 {
		t.Fatalf("int16 -1 + offset -40 = %v", v)
	}

	// 32/64 位与浮点
	b32 := make([]byte, 4)
	binary.BigEndian.PutUint32(b32, math.Float32bits(12.5))
	v, typ, _ = parseRegisterData(b32, config.ModbusRegister{DataType: "float32"})
	if typ != model.TypeFloat || v.(float64) != 12.5 {
		t.Fatalf("float32 = %v", v)
	}
	binary.BigEndian.PutUint32(b32, uint32(0xFFFFFFFE))
	v, _, _ = parseRegisterData(b32, config.ModbusRegister{DataType: "int32"})
	if v.(int64) != -2 {
		t.Fatalf("int32 = %v", v)
	}
	v, _, _ = parseRegisterData(b32, config.ModbusRegister{DataType: "uint32"})
	if v.(int64) != 0xFFFFFFFE {
		t.Fatalf("uint32 = %v", v)
	}
	b64 := make([]byte, 8)
	binary.BigEndian.PutUint64(b64, math.Float64bits(-3.25))
	v, _, _ = parseRegisterData(b64, config.ModbusRegister{DataType: "float64"})
	if v.(float64) != -3.25 {
		t.Fatalf("float64 = %v", v)
	}
	binary.BigEndian.PutUint64(b64, math.MaxUint64)
	v, typ, _ = parseRegisterData(b64, config.ModbusRegister{DataType: "uint64"})
	if typ != model.TypeFloat {
		t.Fatalf("超出 int64 范围的 uint64 应输出浮点数, got %v %s", v, typ)
	}
	v, _, _ = parseRegisterData(b64, config.ModbusRegister{DataType: "int64"})
	if v.(int64) != -1 {
		t.Fatalf("int64 = %v", v)
	}

	if _, _, err := parseRegisterData([]byte{1}, config.ModbusRegister{DataType: "uint16"}); err == nil {
		t.Fatalf("数据不足应报错")
	}
	if _, _, err := parseRegisterData(be16(1), config.ModbusRegister{DataType: "int128"}); err == nil {
		t.Fatalf("不支持的数据类型应报错")
	}
}

func TestByteOrders(t *testing.T) {
	// 0x41480000 = float32(12.5)，按不同字节序排列
	cases := map[string][]byte{
		"ABCD": {0x41, 0x48, 0x00, 0x00},
		"CDAB": {0x00, 0x00, 0x41, 0x48},
		"BADC": {0x48, 0x41, 0x00, 0x00},
		"DCBA": {0x00, 0x00, 0x48, 0x41},
	}
	for order, raw := range cases {
		v, _, err := parseRegisterData(raw, config.ModbusRegister{DataType: "float32", ByteOrder: order})
		if err != nil || v.(float64) != 12.5 {
			t.Errorf("字节序 %s: got %v %v", order, v, err)
		}
	}
	// 16 位：BADC/DCBA 交换字节，CDAB 不变
	v, _, _ := parseRegisterData([]byte{0x34, 0x12}, config.ModbusRegister{DataType: "uint16", ByteOrder: "DCBA"})
	if v.(int64) != 0x1234 {
		t.Errorf("16 位小端 = %#x", v)
	}
	v, _, _ = parseRegisterData([]byte{0x12, 0x34}, config.ModbusRegister{DataType: "uint16", ByteOrder: "CDAB"})
	if v.(int64) != 0x1234 {
		t.Errorf("16 位 CDAB = %#x", v)
	}
}

func TestBuildReadGroupsMergesContiguous(t *testing.T) {
	regs := []config.ModbusRegister{
		reg(10, "holding_register", "float32", "a"), // 10-11
		reg(0, "holding_register", "uint16", "b"),   // 0
		reg(2, "holding_register", "uint32", "c"),   // 2-3
		reg(500, "holding_register", "uint16", "far"),
		reg(0, "coil", "", "c0"),
		reg(5, "coil", "", "c5"),
		reg(1, "input_register", "uint16", "ir"),
	}
	groups := buildReadGroups(regs, true, 10)
	// coil 1 组、holding 2 组（0-11 合并，500 单独）、input 1 组
	if len(groups) != 4 {
		t.Fatalf("分组数量 = %d: %+v", len(groups), groups)
	}
	var hold []readGroup
	for _, g := range groups {
		if g.regType == "holding_register" {
			hold = append(hold, g)
		}
	}
	if len(hold) != 2 || hold[0].start != 0 || hold[0].count != 12 || len(hold[0].members) != 3 {
		t.Fatalf("holding 合并错误: %+v", hold)
	}
	if hold[1].start != 500 || hold[1].count != 1 {
		t.Fatalf("远处寄存器应单独读取: %+v", hold[1])
	}

	// 关闭合并后每个寄存器一组
	if n := len(buildReadGroups(regs, false, 10)); n != len(regs) {
		t.Fatalf("关闭合并后分组数 = %d", n)
	}

	// 单次读取不能超过 125 个寄存器
	var many []config.ModbusRegister
	for i := 0; i < 300; i++ {
		many = append(many, reg(uint16(i), "holding_register", "uint16", "k"))
	}
	for _, g := range buildReadGroups(many, true, 10) {
		if g.count > maxRegistersPerRead {
			t.Fatalf("单次读取 %d 个寄存器，超过协议上限", g.count)
		}
	}
}

func newTestAdapter(t *testing.T, regs []config.ModbusRegister) (*ModbusAdapter, *fakeClient, *fakeHandler) {
	t.Helper()
	fc := newFakeClient()
	fh := &fakeHandler{}
	a := &ModbusAdapter{
		BaseAdapter: southbound.NewBaseAdapter("plc1", "modbus"),
		mode:        "tcp",
		interval:    20 * time.Millisecond,
		client:      fc,
		handler:     fh,
		registers:   regs,
		groups:      buildReadGroups(regs, true, 10),
		minBackoff:  30 * time.Millisecond,
		maxBackoff:  120 * time.Millisecond,
	}
	return a, fc, fh
}

func collect(ch chan model.Point) map[string]model.Point {
	out := map[string]model.Point{}
	for {
		select {
		case p := <-ch:
			out[p.Key] = p
		default:
			return out
		}
	}
}

func TestPollOnceReadsMergedGroup(t *testing.T) {
	regs := []config.ModbusRegister{
		reg(0, "holding_register", "uint16", "speed"),
		reg(1, "holding_register", "int16", "temp"),
		reg(2, "holding_register", "float32", "flow"),
		reg(3, "coil", "", "running"),
	}
	regs[1].Scale = 0.1
	a, fc, _ := newTestAdapter(t, regs)
	fc.holding[0] = 1500
	fc.holding[1] = 235
	bits := math.Float32bits(3.5)
	fc.holding[2] = uint16(bits >> 16)
	fc.holding[3] = uint16(bits)
	fc.coils[3] = true

	ch := make(chan model.Point, 10)
	if err := a.pollOnce(ch); err != nil {
		t.Fatal(err)
	}
	if fc.requests != 2 {
		t.Fatalf("4 个寄存器应合并为 2 次请求（寄存器 + 线圈），实际 %d 次", fc.requests)
	}
	got := collect(ch)
	if got["speed"].Value.(int64) != 1500 || math.Abs(got["temp"].Value.(float64)-23.5) > 1e-9 ||
		got["flow"].Value.(float64) != 3.5 || got["running"].Value != true {
		t.Fatalf("读数错误: %+v", got)
	}
	temp := got["temp"]
	if v, _ := temp.GetTag("address"); v != "1" {
		t.Fatalf("address 标签 = %q", v)
	}
	if got["speed"].Timestamp.IsZero() {
		t.Fatalf("数据点应带采集时间")
	}
}

func TestPollOnceReportsConnectionErrors(t *testing.T) {
	a, fc, _ := newTestAdapter(t, []config.ModbusRegister{reg(0, "holding_register", "uint16", "x")})
	fc.failWith = errors.New("read tcp: connection reset by peer")
	err := a.pollOnce(make(chan model.Point, 1))
	if err == nil || !isConnectionError(err) {
		t.Fatalf("应返回连接错误: %v", err)
	}
	fc.failWith = errors.New("modbus: exception '2' (illegal data address)")
	if err := a.pollOnce(make(chan model.Point, 1)); err == nil || isConnectionError(err) {
		t.Fatalf("协议异常不应视为连接错误: %v", err)
	}
}

func TestStartDoesNotBlockAndReconnectsWithBackoff(t *testing.T) {
	a, _, fh := newTestAdapter(t, []config.ModbusRegister{reg(0, "holding_register", "uint16", "x")})
	fh.connectErr = errors.New("dial tcp: connection refused")

	start := time.Now()
	if err := a.Start(context.Background(), make(chan model.Point, 10)); err != nil {
		t.Fatalf("设备不可达时 Start 不应失败: %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("Start 被连接重试阻塞")
	}
	time.Sleep(300 * time.Millisecond)
	n := atomic.LoadInt32(&fh.connects)
	if n < 2 {
		t.Fatalf("应在后台重试连接, connects=%d", n)
	}
	// 退避：退避时间 30ms→60ms→120ms，300ms 内不应像采集间隔（20ms）那样每次都重试
	if n > 6 {
		t.Fatalf("重连没有退避, connects=%d", n)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	// 可以再次启动（以前重复 Start/Stop 会关闭已关闭的通道而 panic）
	fh.connectErr = nil
	ch := make(chan model.Point, 100)
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	select {
	case p := <-ch:
		if p.Key != "x" {
			t.Fatalf("unexpected point %+v", p)
		}
	case <-deadline:
		t.Fatalf("恢复连接后应采集到数据")
	}
	a.Stop()
	a.Stop() // 重复 Stop 安全
	if atomic.LoadInt32(&fh.closes) == 0 {
		t.Fatalf("停止时应关闭连接")
	}
}

func TestInitConfig(t *testing.T) {
	a := &ModbusAdapter{}
	err := a.Init(json.RawMessage(`{"name":"m","type":"modbus","host":"127.0.0.1","port":1502,
		"byte_order":"CDAB","registers":[{"address":1,"type":"holding_register","device_id":"d","key":"k"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.registers[0].Scale != 1 || a.registers[0].DataType != "uint16" || a.registers[0].ByteOrder != "CDAB" {
		t.Fatalf("寄存器默认值错误: %+v", a.registers[0])
	}
	if a.Name() != "m" || len(a.groups) != 1 {
		t.Fatalf("初始化结果错误")
	}

	rtu := &ModbusAdapter{}
	if err := rtu.Init(json.RawMessage(`{"name":"r","type":"modbus","protocol":"rtu","serial_port":"/dev/ttyUSB0",
		"registers":[{"address":1,"type":"coil","device_id":"d","key":"k"}]}`)); err != nil {
		t.Fatalf("RTU 模式应可初始化: %v", err)
	}
	bad := &ModbusAdapter{}
	if err := bad.Init(json.RawMessage(`{"name":"r","type":"modbus","protocol":"rtu",
		"registers":[{"address":1,"type":"coil","device_id":"d","key":"k"}]}`)); err == nil {
		t.Fatalf("RTU 缺少 serial_port 应报错")
	}
	noHost := &ModbusAdapter{}
	if err := noHost.Init(json.RawMessage(`{"name":"t","type":"modbus",
		"registers":[{"address":1,"type":"coil","device_id":"d","key":"k"}]}`)); err == nil {
		t.Fatalf("TCP 缺少 host 应报错")
	}
}

func TestIsConnectionError(t *testing.T) {
	for _, msg := range []string{"i/o timeout", "connection refused", "EOF", "broken pipe", "use of closed network connection"} {
		if !isConnectionError(errors.New(msg)) {
			t.Errorf("%q 应视为连接错误", msg)
		}
	}
	if isConnectionError(nil) || isConnectionError(errors.New("illegal data value")) {
		t.Errorf("误判为连接错误")
	}
}
