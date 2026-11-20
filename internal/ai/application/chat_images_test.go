package application

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const testImageDataURL = "data:image/png;base64,iVBORw0KGgo="

type fakeVisionChat struct {
	*fakeChatCatalog
	visionImages []string
	visionCalls  int
}

func (f *fakeVisionChat) CompleteVision(ctx context.Context, providerID, systemPrompt, userPrompt string, images []string) (*toolgateway.ChatCompletionResult, error) {
	f.visionCalls++
	f.visionImages = images
	return f.Complete(ctx, providerID, systemPrompt, userPrompt)
}

func (f *fakeVisionChat) CompleteStreamVision(ctx context.Context, providerID, systemPrompt, userPrompt string, images []string, onDelta toolgateway.ChatModelDelta) (*toolgateway.ChatCompletionResult, error) {
	f.visionCalls++
	f.visionImages = images
	return f.CompleteStream(ctx, providerID, systemPrompt, userPrompt, onDelta)
}

func newVisionChat(chat *fakeChatClient, visionEnabled bool) *fakeVisionChat {
	return &fakeVisionChat{fakeChatCatalog: &fakeChatCatalog{fakeChatClient: chat, models: []toolgateway.ChatModel{
		{ProviderID: "provider-1", Name: "默认模型", Model: "model-1", VisionEnabled: visionEnabled},
	}}}
}

func TestNormalizeChatImages(t *testing.T) {
	if out, err := normalizeChatImages(nil); err != nil || out != nil {
		t.Fatalf("nil images = %v, %v", out, err)
	}
	if out, err := normalizeChatImages([]string{testImageDataURL}); err != nil || len(out) != 1 {
		t.Fatalf("valid image = %v, %v", out, err)
	}
	if _, err := normalizeChatImages([]string{testImageDataURL, testImageDataURL, testImageDataURL, testImageDataURL}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("4 images should be rejected, err = %v", err)
	}
	if _, err := normalizeChatImages([]string{"data:image/gif;base64,R0lGODlh"}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("gif should be rejected, err = %v", err)
	}
	if _, err := normalizeChatImages([]string{"https://example.com/a.png"}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("remote url should be rejected, err = %v", err)
	}
	if _, err := normalizeChatImages([]string{"data:image/png;base64,!!!"}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("invalid base64 should be rejected, err = %v", err)
	}
	if _, err := normalizeChatImages([]string{"data:image/png;base64,"}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("empty payload should be rejected, err = %v", err)
	}
	oversize := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, chatMaxImageBytes+1))
	if _, err := normalizeChatImages([]string{oversize}); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("oversize image should be rejected, err = %v", err)
	}
	if !strings.HasPrefix(oversize, "data:image/png;base64,") {
		t.Fatal("sanity: oversize data url prefix")
	}
}

func TestSendMessageWithImagesUsesVisionChannel(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	vision := newVisionChat(chat, true)
	svc.chat = vision
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析截图中的告警", Images: []string{testImageDataURL},
	})
	if err != nil {
		t.Fatalf("send message with images: %v", err)
	}
	if result.Status != chatStatusOK || vision.visionCalls != 1 || len(vision.visionImages) != 1 {
		t.Fatalf("vision channel not used: calls=%d images=%v", vision.visionCalls, vision.visionImages)
	}
	messages := repo.messages["s-1"]
	if len(messages) == 0 || len(messages[0].Images) != 1 || messages[0].Images[0] != testImageDataURL {
		t.Fatalf("user message images not persisted: %+v", messages)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["image_count"] != 1 {
		t.Fatalf("image_count missing in audit: %+v", chatAudit)
	}
}

func TestSendMessageImageOnlyUsesDefaultPrompt(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	vision := newVisionChat(chat, true)
	svc.chat = vision
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "  ", Images: []string{testImageDataURL},
	}); err != nil {
		t.Fatalf("image-only message: %v", err)
	}
	messages := repo.messages["s-1"]
	if len(messages) == 0 || messages[0].Content != chatImageOnlyPrompt {
		t.Fatalf("image-only content = %+v", messages)
	}
}

func TestSendMessageImagesRejectedWhenModelNotVision(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	svc.chat = newVisionChat(chat, false)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	_, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析截图", Images: []string{testImageDataURL},
	})
	if apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if len(repo.messages["s-1"]) != 0 {
		t.Fatalf("rejected message must not be persisted: %+v", repo.messages["s-1"])
	}
	if chat.calls != 0 {
		t.Fatalf("rejected message must not call model, calls=%d", chat.calls)
	}
}

func TestSendMessageImagesRejectedWithoutCatalog(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	_, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析截图", Images: []string{testImageDataURL},
	})
	if apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("provider without vision catalog must reject images, got %v", err)
	}
}
