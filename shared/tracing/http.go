package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func Middleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "",
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			return r.Pattern // r.Method + " " + r.URL.Path
		}),
	)
}
