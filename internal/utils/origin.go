package utils

import (
	"net/http"
	"net/url"
	"strings"
)

// CheckOrigin 校验 WebSocket / 跨域请求的 Origin。
//
//   - 没有 Origin 头（非浏览器客户端，如脚本、设备）：允许；
//   - Origin 与请求的 Host 相同（同源访问，例如通过 Vite 代理或同域部署）：允许；
//   - Origin 在 allowed 白名单中，或白名单包含 "*"：允许；
//   - 其他情况：拒绝。
//
// 以前 WebSocket 的 CheckOrigin 恒返回 true，任何网站都可以借用户浏览器
// 发起跨站 WebSocket 连接（CSWSH）。
func CheckOrigin(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "*" || strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// OriginAllowed 用于 CORS：判断 origin 是否在白名单中（不含同源判断）。
// 返回值 wildcard 表示白名单包含 "*"。
func OriginAllowed(origin string, allowed []string) (ok bool, wildcard bool) {
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "*" {
			return true, true
		}
		if origin != "" && strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(origin, "/")) {
			ok = true
		}
	}
	return ok, false
}
