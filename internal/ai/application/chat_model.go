package application

import (
	"strings"

	"github.com/734965549/aiops/internal/ai/toolgateway"
)

// ChatModelOption 是前端可选的对话模型摘要（不含 base_url、api_key、headers 等运维字段）。
type ChatModelOption struct {
	ProviderID string `json:"provider_id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
}

// ListChatModels 返回启用中的类型 b 模型；实现未提供模型目录时返回空列表。
func (s *ChatService) ListChatModels() []ChatModelOption {
	if s == nil || s.chat == nil {
		return []ChatModelOption{}
	}
	catalog, ok := s.chat.(toolgateway.ChatModelCatalog)
	if !ok {
		return []ChatModelOption{}
	}
	models := catalog.ChatModels()
	out := make([]ChatModelOption, 0, len(models))
	for _, model := range models {
		out = append(out, ChatModelOption{
			ProviderID: model.ProviderID,
			Name:       model.Name,
			Model:      model.Model,
		})
	}
	return out
}

// resolveChatProvider 解析本次对话使用的模型：
// 请求/会话指定的 provider 可用时直接使用；不可用/禁用时回退默认模型并返回提示文案。
func (s *ChatService) resolveChatProvider(requested string) (providerID, fallbackNote string, ok bool) {
	if s == nil || s.chat == nil {
		return "", "", false
	}
	requested = strings.TrimSpace(requested)
	catalog, hasCatalog := s.chat.(toolgateway.ChatModelCatalog)
	if requested != "" && hasCatalog {
		if model, found := catalog.ChatProviderByID(requested); found {
			return model.ProviderID, "", true
		}
	}
	defaultID, found := s.chat.ChatProviderID()
	if !found {
		return "", "", false
	}
	if requested == "" || strings.EqualFold(requested, defaultID) {
		return defaultID, "", true
	}
	fallbackName := defaultID
	if hasCatalog {
		if model, found := catalog.ChatProviderByID(defaultID); found && strings.TrimSpace(model.Name) != "" {
			fallbackName = model.Name
		}
	}
	return defaultID, "所选模型 " + requested + " 不可用，已回退 " + fallbackName, true
}
