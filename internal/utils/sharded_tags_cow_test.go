package utils

import (
	"sync"
	"testing"
)

// Clone 写时复制：副本与原容器共享 map，任何一方写入都不影响另一方
func TestShardedTagsCloneCopyOnWrite(t *testing.T) {
	orig := NewShardedTagsFromMap(map[string]string{"a": "1", "b": "2"})
	c1 := orig.Clone()
	c2 := orig.Clone()
	c1.Set("a", "x")
	c2.Delete("b")
	orig.SetMultiple(map[string]string{"c": "3"})
	if v, _ := orig.Get("a"); v != "1" || orig.Len() != 3 {
		t.Fatalf("原容器被副本修改: %v", orig.GetAll())
	}
	if v, _ := c1.Get("a"); v != "x" || c1.Len() != 2 {
		t.Fatalf("c1 = %v", c1.GetAll())
	}
	if _, ok := c2.Get("b"); ok || c2.Len() != 1 {
		t.Fatalf("c2 = %v", c2.GetAll())
	}
	c2.Delete("missing") // 删除不存在的键不需要复制
	c2.Clear()
	c2.Set("z", "1")
	if c2.Len() != 1 || orig.Len() != 3 {
		t.Fatal("Clear 后写入不应影响原容器")
	}
	cc := c1.Clone()
	if err := cc.UnmarshalJSON([]byte(`{"q":"1"}`)); err != nil || c1.Len() != 2 {
		t.Fatal("反序列化不应影响共享的容器")
	}
	if (&ShardedTags{}).Clone().Len() != 0 {
		t.Fatal("空容器的副本")
	}
	var nilTags *ShardedTags
	if nilTags.Clone() != nil {
		t.Fatal("nil 的副本应为 nil")
	}
}

// 多个连接器各自持有副本：一边读一边写，没有数据竞争
func TestShardedTagsCloneConcurrent(t *testing.T) {
	orig := NewShardedTagsFromMap(map[string]string{"a": "1"})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		c := orig.Clone()
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.Set("k", "v")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = orig.GetAll()
				orig.Set("a", "1")
			}
		}()
	}
	wg.Wait()
	if v, _ := orig.Get("a"); v != "1" {
		t.Fatal("原容器值错误")
	}
}
