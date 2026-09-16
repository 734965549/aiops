package middleware

import (
	"time"

	"github.com/734965549/aiops/pkg/config"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS 根据配置生成跨域中间件。
//
// 配置应已通过 config.normalizeCORSConfig 与 Validate 处理；
// 此处再做一层防御，禁止 allow_credentials=true 与 * 同时生效。
func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	origins := cfg.AllowOrigins
	if cfg.AllowCredentials {
		origins = filterWildcardOrigins(origins)
		if len(origins) == 0 {
			// 同源反代部署无需 CORS 头；gin-contrib/cors 在 credentials=true 且 origins 为空时会 panic。
			return func(c *gin.Context) { c.Next() }
		}
	} else if len(origins) == 0 {
		origins = []string{"*"}
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", httpx.HeaderTraceID},
		ExposeHeaders:    []string{httpx.HeaderTraceID},
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           12 * time.Hour,
	})
}

func filterWildcardOrigins(origins []string) []string {
	out := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin != "*" {
			out = append(out, origin)
		}
	}
	return out
}
