package logging

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
)

var Log *zap.SugaredLogger
var cfg *config.ExportConfig

func init() {
	cfg = config.ExportCfg

	logger := zap.NewExample()
	Log = logger.Sugar()
}

func Logger(l *zap.SugaredLogger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				l.Infow("",
					zap.String("protocol", r.Proto),
					zap.String("request", r.Method),
					zap.String("path", r.URL.Path),
					zap.Duration("latency", time.Since(t1)),
					zap.Int("status", ww.Status()),
					zap.Int("size", ww.BytesWritten()),
					zap.String("reqId", request_id.GetReqID(r.Context())))
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}
