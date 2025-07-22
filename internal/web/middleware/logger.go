package middleware

import (
	"log"
	"net/http"
	"time"
)

// Logger 日志中间件
func Logger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 创建一个响应写入器来捕获状态码
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// 处理请求
			next.ServeHTTP(rw, r)

			// 记录日志
			duration := time.Since(start)
			log.Printf("[%s] %s %s %d %v %s",
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
				rw.statusCode,
				duration,
				r.UserAgent(),
			)
		})
	}
}

// responseWriter 包装http.ResponseWriter以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}
