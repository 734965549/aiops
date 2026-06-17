package toolgateway

import (
	"context"
	"fmt"
	"strings"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"go.uber.org/zap"
)

// ToolRequest 表示一次 AI 工具调用。
type ToolRequest struct {
	UserID    string
	ToolCode  string
	Resource  string
	Action    string
	OwnerID   string
	Dept      string
	Team      string
	Region    string
	Tags      []string
	Confirmed bool
	Payload   map[string]any
}

// ToolResponse 表示一次工具调用结果。
type ToolResponse struct {
	Allowed  bool           `json:"allowed"`
	Reason   string         `json:"reason,omitempty"`
	Mode     string         `json:"mode,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// Authorizer 定义统一授权能力。
type Authorizer interface {
	Authorize(ctx context.Context, input identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error)
}

// Gateway 是 AI 工具调用网关的标准实现。
type Gateway struct {
	authorizer Authorizer
	registry   *ProviderRegistry
}

func NewGateway(authorizer Authorizer, registry *ProviderRegistry) *Gateway {
	return &Gateway{authorizer: authorizer, registry: registry}
}

func (g *Gateway) Validate(ctx context.Context, req ToolRequest) (*ToolResponse, error) {
	if g == nil || g.authorizer == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "tool gateway is not configured")
	}
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.ToolCode) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "user_id and tool_code are required")
	}
	res, err := g.authorizer.Authorize(ctx, identityapp.AuthorizationInput{UserID: strings.TrimSpace(req.UserID), Resource: strings.TrimSpace(req.Resource), Action: strings.TrimSpace(req.Action), ObjectOwner: strings.TrimSpace(req.OwnerID), ObjectDept: strings.TrimSpace(req.Dept), ObjectTeam: strings.TrimSpace(req.Team), ObjectRegion: strings.TrimSpace(req.Region), ObjectTags: req.Tags, ToolCode: strings.TrimSpace(req.ToolCode), UserConfirmed: req.Confirmed})
	if err != nil {
		logger.From(ctx).Warn("tool gateway authorization failed", zap.Error(err))
		reason := "authorization failed"
		if ae := apperr.FromError(err); ae != nil && strings.TrimSpace(ae.Message) != "" {
			reason = ae.Message
			if ae.Code == apperr.CodePermissionDenied {
				return &ToolResponse{Allowed: false, Reason: reason}, nil
			}
		}
		return &ToolResponse{Allowed: false, Reason: reason}, err
	}
	if !res.Allowed {
		return &ToolResponse{Allowed: false, Reason: res.Reason, Mode: res.ToolMode}, nil
	}
	return &ToolResponse{Allowed: true, Mode: res.ToolMode}, nil
}

func (g *Gateway) Invoke(ctx context.Context, providerID string, req ToolRequest) (*ToolResponse, error) {
	resp, err := g.Validate(ctx, req)
	if err != nil {
		return resp, err
	}
	if resp == nil || !resp.Allowed {
		return resp, nil
	}
	if g.registry == nil {
		return resp, apperr.New(apperr.CodeUnavailable, "tool provider registry is not configured")
	}
	out, err := g.registry.Invoke(ctx, providerID, req)
	if err != nil {
		return nil, err
	}
	if out != nil {
		resp.Provider = providerID
		resp.Data = out.Data
	}
	return resp, nil
}

func (r *ToolRequest) String() string {
	return fmt.Sprintf("tool=%s resource=%s action=%s", r.ToolCode, r.Resource, r.Action)
}
