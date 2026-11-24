package redis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// fakeRedis 最小的 RESP2 服务器：记录收到的命令，对 SET/PING 回复 OK/PONG，其他回复 :1
type fakeRedis struct {
	ln       net.Listener
	mu       sync.Mutex
	commands [][]string
	reads    int // 读到的"批次"数（一次 TCP 读取中处理的命令组），用于验证 Pipeline
}

func startFakeRedis(t *testing.T) *fakeRedis {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("unexpected %q", line)
	}
	n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, _ := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:size]))
	}
	return args, nil
}

func (f *fakeRedis) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		cmd, err := readCommand(r)
		if err != nil {
			return
		}
		batch := [][]string{cmd}
		for r.Buffered() > 0 { // 同一次读取中已到达的后续命令视为同一批（Pipeline）
			next, err := readCommand(r)
			if err != nil {
				return
			}
			batch = append(batch, next)
		}
		f.mu.Lock()
		f.reads++
		f.commands = append(f.commands, batch...)
		f.mu.Unlock()
		var out strings.Builder
		for _, cmd := range batch {
			switch strings.ToUpper(cmd[0]) {
			case "PING":
				out.WriteString("+PONG\r\n")
			case "SET":
				out.WriteString("+OK\r\n")
			default:
				out.WriteString(":1\r\n")
			}
		}
		c.Write([]byte(out.String()))
	}
}

func (f *fakeRedis) snapshot() ([][]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.commands...), f.reads
}

func TestRedisSinkPipelinesBatch(t *testing.T) {
	srv := startFakeRedis(t)
	s := NewRedisSink()
	cfg := fmt.Sprintf(`{"name":"r","type":"redis","params":{"address":%q,"default_expiry_sec":60,
		"points":{"status":{"format":"hash","key_format":"dev:{device_id}"},"raw":{"format":"string","key_prefix":"gw"}}}}`, srv.ln.Addr().String())
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	_, readsBefore := srv.snapshot()

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	batch := []model.Point{}
	for i := 0; i < 20; i++ {
		p := model.NewPoint("temp", fmt.Sprintf("d%d", i), int64(i), model.TypeInt)
		p.Timestamp = ts
		batch = append(batch, p)
	}
	st := model.NewPoint("status", "d1", "ok", model.TypeString)
	batch = append(batch, st, model.NewPoint("raw", "d1", 3.5, model.TypeFloat))
	if err := s.Publish(batch); err != nil {
		t.Fatal(err)
	}

	cmds, reads := srv.snapshot()
	if reads-readsBefore > 3 {
		t.Fatalf("22 个点用了 %d 次往返，应使用 Pipeline 批量发送", reads-readsBefore)
	}
	var set0 []string
	var hset, rawSet []string
	for _, c := range cmds {
		switch {
		case strings.EqualFold(c[0], "set") && c[1] == "d0:temp":
			set0 = c
		case strings.EqualFold(c[0], "hset") && c[1] == "dev:d1":
			hset = c
		case strings.EqualFold(c[0], "set") && c[1] == "gw:d1:raw":
			rawSet = c
		}
	}
	if set0 == nil {
		t.Fatalf("缺少 d0:temp 的 SET 命令: %v", cmds)
	}
	var stored map[string]interface{}
	json.Unmarshal([]byte(set0[2]), &stored)
	if stored["value"].(float64) != 0 || int64(stored["timestamp"].(float64)) != ts.UnixMilli() {
		t.Fatalf("存储内容错误（应使用采集时间戳）: %s", set0[2])
	}
	if len(set0) < 5 || strings.ToLower(set0[3]) != "ex" || set0[4] != "60" {
		t.Fatalf("应设置默认过期时间: %v", set0)
	}
	if hset == nil {
		t.Fatalf("hash 格式应使用 HSET: %v", cmds)
	}
	if rawSet == nil || rawSet[2] != "3.5" {
		t.Fatalf("string 格式 / key_prefix 错误: %v", rawSet)
	}
}

func TestRedisSinkInitToleratesUnavailableServer(t *testing.T) {
	s := NewRedisSink()
	if err := s.Init(json.RawMessage(`{"name":"r","type":"redis","params":{"address":"127.0.0.1:1"}}`)); err != nil {
		t.Fatalf("Redis 暂不可达时初始化不应失败: %v", err)
	}
	s.Start(context.Background())
	defer s.Stop()
	if err := s.Publish([]model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}); err == nil {
		t.Fatalf("不可达时写入应返回错误")
	}
}

func TestRedisWriteBatch(t *testing.T) {
	srv := startFakeRedis(t)
	s := NewRedisSink()
	if err := s.Init(json.RawMessage(fmt.Sprintf(`{"name":"r","type":"redis","params":{"address":%q}}`, srv.ln.Addr().String()))); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBatch(context.Background(), []model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}); err != nil {
		t.Fatalf("同步写入失败: %v", err)
	}
	srv.ln.Close()
	down := NewRedisSink()
	down.Init(json.RawMessage(`{"name":"r","type":"redis","params":{"address":"127.0.0.1:1"}}`))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := down.WriteBatch(ctx, []model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}); err == nil {
		t.Fatalf("Redis 不可达时应返回错误")
	}
}
