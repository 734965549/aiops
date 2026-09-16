package domain

import "errors"

// 领域层仅定义标准库哨兵错误，不含 HTTP 业务码。
// application 层通过 pkg/errors.MapSentinels 映射为 NOT_FOUND / ALREADY_EXISTS / INVALID_ARGUMENT（§10）。

// ErrNotFound 告警、接入源或 active dedup 记录不存在。
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists 唯一约束冲突（alert_id、dedup_key+lifecycle_seq 等）。
var ErrAlreadyExists = errors.New("already exists")

// ErrInvalidTransition 表示状态机不允许的流转（由 application 层映射为 INVALID_ARGUMENT）。
var ErrInvalidTransition = errors.New("invalid status transition")
