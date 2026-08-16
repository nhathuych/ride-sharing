package middleware

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"ride-sharing/shared/logger"
	"time"

	"go.uber.org/zap"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	bytes       int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}

	rw.status = code
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += int64(n)

	return n, err
}

// ReadFrom preserves io.Copy optimizations when the underlying
// ResponseWriter implements io.ReaderFrom.
func (rw *responseWriter) ReadFrom(src io.Reader) (int64, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	if readerFrom, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(src)
		rw.bytes += n
		return n, err
	}

	n, err := io.Copy(rw.ResponseWriter, src)
	rw.bytes += n

	return n, err
}

// Unwrap allows http.ResponseController to reach the underlying
// ResponseWriter and preserve support for Flush, Hijack,
// deadlines, full duplex, etc.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Flush implements http.Flusher.
func (rw *responseWriter) Flush() {
	_ = rw.FlushError()
}

// FlushError preserves the newer FlushError capability when available.
func (rw *responseWriter) FlushError() error {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	if flusher, ok := rw.ResponseWriter.(interface {
		FlushError() error
	}); ok {
		return flusher.FlushError()
	}

	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
		return nil
	}

	return http.ErrNotSupported
}

// Hijack implements http.Hijacker.
// This is required by WebSocket implementations such as gorilla/websocket.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	return hijacker.Hijack()
}

// Push preserves HTTP/2 server push support when available.
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := rw.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}

	return pusher.Push(target, opts)
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		logger.WithTraceNoCaller(r.Context()).Info(
			"HTTP request",
			zap.String("method", r.Method),
			zap.String("path", requestPath(r)),
			zap.String("route", requestRoute(r)),
			zap.Int("status", rw.status),
			zap.Int64("bytes", rw.bytes),
			zap.Duration("duration", time.Since(start)),
			zap.String("client_ip", clientIP(r)),
		)
	})
}

func requestPath(r *http.Request) string {
	if r.URL == nil {
		return ""
	}

	return r.URL.Path
}

func requestRoute(r *http.Request) string {
	// net/http ServeMux populates Request.Pattern.
	// For third-party routers such as chi/gin/echo, this may be empty.
	// In that case fall back to the actual URL path.
	if r.Pattern != "" {
		return r.Pattern
	}

	return requestPath(r)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}
