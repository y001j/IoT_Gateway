package bus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestNatsBusPublishSubscribePoints(t *testing.T) {
	srv, _ := testutil.StartNATS(t)
	b, err := NewNatsBus(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	var mu sync.Mutex
	var got []model.Point
	sub, err := SubscribePoints(context.Background(), b, "plc.1", func(p model.Point) error {
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	time.Sleep(50 * time.Millisecond)

	// 设备 ID 含点号：发布和订阅必须使用相同的主题编码
	if err := PublishPoint(b, model.NewPoint("temp", "plc.1", 1.5, model.TypeFloat)); err != nil {
		t.Fatal(err)
	}
	if err := PublishPoint(b, model.NewPoint("temp", "plc", 9.9, model.TypeFloat)); err != nil {
		t.Fatal(err)
	}
	testutil.WaitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1
	}, "应收到订阅设备的数据")
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].DeviceID != "plc.1" || got[0].Value.(float64) != 1.5 {
		t.Fatalf("收到的数据错误（不应收到其他设备的数据）: %+v", got)
	}
}

func TestNatsBusBatchAndAsync(t *testing.T) {
	srv, nc := testutil.StartNATS(t)
	b, err := NewNatsBus(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	sub, _ := nc.SubscribeSync("t.>")
	nc.Flush()

	if err := b.PublishBatch([]string{"t.a", "t.b"}, []interface{}{map[string]int{"x": 1}, "y"}); err != nil {
		t.Fatal(err)
	}
	if err := b.PublishBatch([]string{"t.a"}, nil); err == nil {
		t.Fatalf("主题与消息数量不一致应报错")
	}
	if err := b.PublishAsync("t.c", 3); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for len(seen) < 3 {
		msg, err := sub.NextMsg(3 * time.Second)
		if err != nil {
			t.Fatalf("消息未全部到达: %v (已收到 %v)", err, seen)
		}
		seen[msg.Subject] = true
	}
}
