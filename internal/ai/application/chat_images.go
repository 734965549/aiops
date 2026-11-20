package application

import (
	"encoding/base64"
	"strings"

	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const (
	// chatMaxImages 单条用户消息最多附图数量。
	chatMaxImages = 3
	// chatMaxImageBytes 单张图片 base64 解码后的字节上限（2MB）。
	chatMaxImageBytes = 2 << 20
	// chatImageOnlyPrompt 是用户只发图片未填文字时的兜底提问。
	chatImageOnlyPrompt = "请分析这张图片。"
)

// chatImageMimes 是允许的图片 data URL MIME 前缀。
var chatImageMimes = []string{"image/png", "image/jpeg", "image/webp"}

// normalizeChatImages 校验并规范化用户消息附图（data URL 数组）。
// 规则：最多 3 张；仅 png/jpeg/webp；base64 解码后单张不超过 2MB。
// 图片只作为模型输入与消息留档，不写入日志、审计或 Prompt 文本。
func normalizeChatImages(images []string) ([]string, error) {
	if len(images) == 0 {
		return nil, nil
	}
	if len(images) > chatMaxImages {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "images exceed %d per message", chatMaxImages)
	}
	out := make([]string, 0, len(images))
	for _, raw := range images {
		image := strings.TrimSpace(raw)
		if image == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "image data url is empty")
		}
		mime := ""
		for _, allowed := range chatImageMimes {
			if strings.HasPrefix(image, "data:"+allowed+";base64,") {
				mime = allowed
				break
			}
		}
		if mime == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "image must be a data url of png, jpeg or webp")
		}
		payload := image[len("data:"+mime+";base64,"):]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "image base64 payload is invalid")
		}
		if len(decoded) == 0 {
			return nil, apperr.New(apperr.CodeInvalidArgument, "image payload is empty")
		}
		if len(decoded) > chatMaxImageBytes {
			return nil, apperr.Newf(apperr.CodeInvalidArgument, "image exceeds %d bytes after base64 decoding", chatMaxImageBytes)
		}
		out = append(out, image)
	}
	return out, nil
}

// chatVisionSupported 按 resolveChatProvider 的同一优先级判断本轮实际模型是否接受图片：
// 请求/会话指定的 provider 可用时看它，否则看默认模型。
func (s *ChatService) chatVisionSupported(requestedProviderID string) bool {
	if s == nil || s.chat == nil {
		return false
	}
	catalog, ok := s.chat.(toolgateway.ChatModelCatalog)
	if !ok {
		return false
	}
	requested := strings.TrimSpace(requestedProviderID)
	if requested != "" {
		if model, found := catalog.ChatProviderByID(requested); found {
			return model.VisionEnabled
		}
	}
	defaultID, found := s.chat.ChatProviderID()
	if !found {
		return false
	}
	model, found := catalog.ChatProviderByID(defaultID)
	return found && model.VisionEnabled
}
