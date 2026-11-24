package utils

import (
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestShardedTagsBasic(t *testing.T) {
	st := NewShardedTags(16)
	if !st.IsEmpty() || st.Len() != 0 {
		t.Fatalf("新容器应为空")
	}
	if _, ok := st.Get("missing"); ok {
		t.Fatalf("不存在的键不应返回 ok")
	}
	st.Set("a", "1")
	st.Set("b", "2")
	if v, ok := st.Get("a"); !ok || v != "1" {
		t.Fatalf("Get(a) = %q,%v", v, ok)
	}
	if st.Len() != 2 || st.IsEmpty() {
		t.Fatalf("Len = %d", st.Len())
	}
	st.Delete("a")
	if _, ok := st.Get("a"); ok {
		t.Fatalf("删除后不应存在")
	}
	st.SetMultiple(map[string]string{"c": "3", "d": "4"})
	if st.Len() != 3 {
		t.Fatalf("SetMultiple 后 Len = %d", st.Len())
	}
	keys := st.GetKeys()
	if len(keys) != 3 {
		t.Fatalf("GetKeys = %v", keys)
	}
	st.Clear()
	if !st.IsEmpty() {
		t.Fatalf("Clear 后应为空")
	}
	if st.GetStats()["current_size"] != 0 {
		t.Fatalf("GetStats current_size 应为 0")
	}
}

func TestShardedTagsGetAllIsCopy(t *testing.T) {
	st := NewShardedTagsFromMap(map[string]string{"k": "v"})
	all := st.GetAll()
	all["k"] = "changed"
	all["new"] = "x"
	if v, _ := st.Get("k"); v != "v" {
		t.Fatalf("修改 GetAll 返回值不应影响容器")
	}
	if st.Len() != 1 {
		t.Fatalf("Len = %d", st.Len())
	}
}

func TestShardedTagsFromMapCopiesSource(t *testing.T) {
	src := map[string]string{"a": "1"}
	st := NewShardedTagsFromMap(src)
	src["a"] = "2"
	if v, _ := st.Get("a"); v != "1" {
		t.Fatalf("修改源 map 不应影响容器")
	}
}

func TestShardedTagsCloneAndMerge(t *testing.T) {
	st := NewShardedTagsFromMap(map[string]string{"a": "1"})
	c := st.Clone()
	c.Set("a", "2")
	c.Set("b", "3")
	if v, _ := st.Get("a"); v != "1" || st.Len() != 1 {
		t.Fatalf("Clone 应为深拷贝")
	}
	st.Merge(c)
	if v, _ := st.Get("b"); v != "3" {
		t.Fatalf("Merge 失败")
	}
	st.Merge(nil)
	st.Merge(st) // 自合并不应死锁
	var nilTags *ShardedTags
	if nilTags.Clone() != nil {
		t.Fatalf("nil Clone 应返回 nil")
	}
}

func TestShardedTagsJSON(t *testing.T) {
	st := NewShardedTagsFromMap(map[string]string{"x": "1", "y": "2"})
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var back ShardedTags
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if v, _ := back.Get("y"); v != "2" || back.Len() != 2 {
		t.Fatalf("JSON 往返失败: %v", back.GetAll())
	}
	if err := back.UnmarshalJSON([]byte("not json")); err == nil {
		t.Fatalf("非法 JSON 应返回错误")
	}
}

func TestShardedTagsConcurrent(t *testing.T) {
	st := NewShardedTags(0)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				st.Set("k", "v")
				st.Get("k")
				st.GetAll()
				st.Len()
			}
		}(i)
	}
	wg.Wait()
}

func TestShardedTagsAdapter(t *testing.T) {
	a := NewShardedTagsAdapter()
	a.FromMap(map[string]string{"a": "1"})
	if a.ToMap()["a"] != "1" {
		t.Fatalf("适配器转换失败")
	}
}

func TestCheckOrigin(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		origin  string
		allowed []string
		want    bool
	}{
		{"无Origin（非浏览器客户端）", "gw:8081", "", nil, true},
		{"同源", "gw:8081", "http://gw:8081", nil, true},
		{"同源大小写不敏感", "GW:8081", "http://gw:8081", nil, true},
		{"跨域默认拒绝", "gw:8081", "http://evil.com", nil, false},
		{"白名单允许", "gw:8081", "http://ui.local:3000", []string{"http://ui.local:3000"}, true},
		{"白名单末尾斜杠", "gw:8081", "http://ui.local:3000", []string{"http://ui.local:3000/"}, true},
		{"通配符", "gw:8081", "http://any.com", []string{"*"}, true},
		{"非法Origin", "gw:8081", "::bad", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+c.host+"/ws", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if got := CheckOrigin(r, c.allowed); got != c.want {
				t.Fatalf("CheckOrigin = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	if ok, wc := OriginAllowed("http://a.com", nil); ok || wc {
		t.Fatalf("空白名单不应允许")
	}
	if ok, wc := OriginAllowed("http://a.com", []string{"http://a.com"}); !ok || wc {
		t.Fatalf("白名单匹配应允许且非通配")
	}
	if ok, wc := OriginAllowed("http://b.com", []string{"*"}); !ok || !wc {
		t.Fatalf("通配符应允许")
	}
	if ok, _ := OriginAllowed("", []string{"http://a.com"}); ok {
		t.Fatalf("空 Origin 不应匹配白名单")
	}
}
