package middleware

import (
	"errors"
	"net/http"

	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// SecurityHeaders 返回为每个响应注入基础 HTTP 安全头的中间件 (F-05)。
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Next()
	}
}

// RequestBodyLimit 返回限制请求体最大字节数的中间件 (F-04)。
// 如果请求体超过限制，c.ShouldBindJSON 会返回 *http.MaxBytesError。
func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()

		if len(c.Errors) > 0 {
			for _, ginErr := range c.Errors {
				var maxBytesErr *http.MaxBytesError
				if errors.As(ginErr.Err, &maxBytesErr) || ginErr.Err.Error() == "http: request body too large" {
					if !c.Writer.Written() {
						c.Abort()
						httpx.Fail(c, apperr.New(apperr.CodePayloadTooLarge, "request body too large"))
					}
					return
				}
			}
		}
	}
}
