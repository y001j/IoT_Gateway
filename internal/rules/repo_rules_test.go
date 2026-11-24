package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepoRuleFilesAreValid 仓库 rules/ 目录中的每个规则文件都必须能通过校验并被加载
// （规则加载现在会真正拒绝校验失败的规则，这里防止示例规则被静默丢弃）
func TestRepoRuleFilesAreValid(t *testing.T) {
	root := "../../rules"
	if _, err := os.Stat(root); err != nil {
		t.Skip("rules 目录不存在")
	}
	m := NewManager(t.TempDir())
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}
		if strings.Contains(path, "test_data") {
			return nil
		}
		rules, err := m.loadRuleFile(path)
		if err != nil {
			t.Errorf("%s: 解析失败: %v", path, err)
			return nil
		}
		if len(rules) == 0 {
			t.Errorf("%s: 没有加载出任何合法规则", path)
		}
		return nil
	})
}
