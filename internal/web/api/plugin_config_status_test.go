package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/y001j/iot-gateway/internal/plugin"
	"github.com/y001j/iot-gateway/internal/web/services"
)

type stubConfigService struct {
	services.PluginService
	err error
}

func (s *stubConfigService) UpdatePluginConfig(string, map[string]interface{}) error { return s.err }

// 保存插件配置：校验失败返回 400、实例不存在 404、其他错误 500（以前一律 500）
func TestUpdatePluginConfigStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	invalid := fmt.Errorf("包装: %w", plugin.ErrInvalidInstanceConfig)
	for name, c := range map[string]struct {
		err  error
		want int
	}{
		"成功":    {nil, http.StatusOK},
		"校验失败":  {invalid, http.StatusBadRequest},
		"实例不存在": {plugin.ErrInstanceNotConfigured, http.StatusNotFound},
		"写文件失败": {errors.New("disk full"), http.StatusInternalServerError},
	} {
		h := NewPluginHandler(&stubConfigService{err: c.err})
		r := gin.New()
		r.PUT("/plugins/:id/config", h.UpdatePluginConfig)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/plugins/m1/config", strings.NewReader(`{"config":{}}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("%s: 状态码 %d, want %d", name, w.Code, c.want)
		}
	}
}
