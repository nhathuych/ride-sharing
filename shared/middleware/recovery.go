package middleware

import (
	"net/http"
	"ride-sharing/shared/logger"
	"runtime/debug"

	"go.uber.org/zap"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.WithTrace(r.Context()).Error(
					"panic recovered",
					zap.Any("panic", recovered),
					zap.ByteString("stack", debug.Stack()),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)

				http.Error(
					w,
					http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError,
				)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
