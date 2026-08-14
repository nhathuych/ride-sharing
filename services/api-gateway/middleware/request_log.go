package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"ride-sharing/shared/logger"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker.
// This is REQUIRED for WebSocket upgrades.
// gorilla/websocket needs to hijack the underlying TCP connection to switch from HTTP to WebSocket protocol.
// If we wrap ResponseWriter without this method, Upgrade() will fail with "websocket: response does not implement http.Hijacker"
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Flush implements http.Flusher.
// It proxies flush calls to the underlying ResponseWriter if it supports it.
// This is important for streaming responses and ensures compatibility
// with middlewares that expect flushing capability (e.g., SSE, chunked responses).
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.WithTrace(r.Context()).Info(
			fmt.Sprintf("[HTTP] INCOMING -> Method: %s | Path: %s | IP: %s",
				r.Method,
				r.URL.Path,
				r.RemoteAddr,
			),
		)

		start := time.Now()
		wrappedWriter := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrappedWriter, r)
		logger.WithTrace(r.Context()).Info(
			fmt.Sprintf("[HTTP] COMPLETED <- Method: %s | Path: %s | Status: %d | Duration: %v",
				r.Method,
				r.URL.Path,
				wrappedWriter.status,
				time.Since(start),
			),
		)
	})
}
