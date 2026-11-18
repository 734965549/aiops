package http

import (
	"bytes"
	"io"
	"unicode/utf8"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

// BindJSON 读取原始请求体并校验 UTF-8 有效性后再绑定到 obj。
//
// encoding/json 遇到非法 UTF-8 字节会静默替换为 U+FFFD，绑定后再检查字符串
// 永远通过，因此校验必须发生在解码前的原始字节上（如 GBK 控制台编码污染）。
// 非法字节直接拒绝，不做转码猜测修复。
func BindJSON(c *gin.Context, obj any) error {
	raw, err := c.GetRawData()
	if err != nil {
		return apperr.New(apperr.CodeInvalidArgument, "read request body failed")
	}
	if !utf8.Valid(raw) {
		return apperr.New(apperr.CodeInvalidArgument, "request body is not valid utf-8")
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return c.ShouldBindJSON(obj)
}
