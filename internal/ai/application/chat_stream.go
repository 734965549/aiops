package application

import (
	"context"
	"strings"
)

// 对话事件类型（ops/ai-contract.md §10.17）。
//
// 说明：回答是结构化 JSON（summary/actions/references），token 级流式对渲染没有价值，
// 因此这里做「事件级流式」：阶段、工具观测、最终答案与错误分别推送，前端可即时反馈。
const (
	ChatEventStage  = "stage"
	ChatEventTool   = "tool"
	ChatEventAnswer = "answer"
	ChatEventError  = "error"

	// ChatStageGathering / ChatStageThinking / ChatStageAnswering 是阶段事件取值。
	ChatStageGathering = "gathering"
	ChatStageThinking  = "thinking"
	ChatStageAnswering = "answering"
)

// ChatToolEvent 是一次工具校验/执行的流式摘要（不含凭据与 prompt）。
type ChatToolEvent struct {
	ToolCode string `json:"tool_code"`
	Allowed  bool   `json:"allowed"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// ChatEvent 是流式推送的单条事件。
type ChatEvent struct {
	Type    string                 `json:"type"`
	Stage   string                 `json:"stage,omitempty"`
	Tool    *ChatToolEvent         `json:"tool,omitempty"`
	Result  *SendChatMessageResult `json:"result,omitempty"`
	Code    string                 `json:"code,omitempty"`
	Message string                 `json:"message,omitempty"`
}

// ChatEventSink 是事件推送回调；返回错误表示客户端已断开，调用方应停止后续工作。
type ChatEventSink func(ChatEvent) error

type ChatEventSinkKey struct{}

// withChatEventSink 把事件通道挂到 ctx 上，供 appendToolResult 等深层调用点推送。
func withChatEventSink(ctx context.Context, sink ChatEventSink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, ChatEventSinkKey{}, sink)
}

// emitChatEvent 推送事件；没有通道时是 no-op。
// 推送失败只返回错误给调用方判断是否中断，不写日志（避免日志刷屏）。
func emitChatEvent(ctx context.Context, event ChatEvent) error {
	sink, ok := ctx.Value(ChatEventSinkKey{}).(ChatEventSink)
	if !ok || sink == nil {
		return nil
	}
	return sink(event)
}

// emitChatStage 推送阶段事件。
func emitChatStage(ctx context.Context, stage string) error {
	return emitChatEvent(ctx, ChatEvent{Type: ChatEventStage, Stage: stage})
}

// toolEventFromObservation 把观测转成流式工具事件。
func toolEventFromObservation(obs ChatObservation) ChatEvent {
	return ChatEvent{Type: ChatEventTool, Tool: &ChatToolEvent{
		ToolCode: obs.ToolCode, Allowed: obs.Allowed, Status: obs.Status,
		Reason: obs.Reason, Summary: strings.TrimSpace(obs.Summary),
	}}
}
