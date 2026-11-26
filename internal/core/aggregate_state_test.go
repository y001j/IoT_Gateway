package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/y001j/iot-gateway/internal/rules/actions"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// rule_engine.aggregate_state.persist（默认开启）时为聚合状态创建 JetStream KV 存储桶，关闭时不创建
func TestRuntimeAggregateStatePersistenceConfig(t *testing.T) {
	for _, persist := range []bool{true, false} {
		srv, nc := testutil.StartNATS(t)
		dir := t.TempDir()
		persistLine := ""
		if !persist {
			persistLine = "  aggregate_state:\n    persist: false\n"
		}
		cfg := fmt.Sprintf("gateway:\n  log_level: error\n  nats_url: %q\n  http_port: %d\n  plugins_dir: %q\nrule_engine:\n  enabled: true\n  rules_dir: %q\n  hot_reload:\n    enabled: false\n%sweb_ui:\n  enabled: false\n",
			srv.ClientURL(), freePort(t), filepath.Join(dir, "plugins"), filepath.Join(dir, "rules"), persistLine)
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
		rt, err := NewRuntime(path)
		if err != nil {
			t.Fatal(err)
		}
		js, _ := nc.JetStream()
		_, kvErr := js.KeyValue(actions.AggStateKVBucket)
		rt.Bus.Close()
		if persist && kvErr != nil {
			t.Fatalf("默认应创建聚合状态存储桶: %v", kvErr)
		}
		if !persist && kvErr == nil {
			t.Fatal("关闭持久化时不应创建存储桶")
		}
	}
}
