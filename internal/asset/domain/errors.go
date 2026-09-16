package domain

import "errors"

// ErrNotFound 应用或资源不存在。
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists 唯一约束冲突。
var ErrAlreadyExists = errors.New("already exists")

// ErrHasResources 应用下仍有资源，不允许删除。
var ErrHasResources = errors.New("application has resources")

// ErrHasMatchRules 应用或资源仍被匹配规则引用，不允许删除。
var ErrHasMatchRules = errors.New("asset has match rules")

// ErrHasAlertReferences 应用仍被未关闭告警引用，不允许删除。
var ErrHasAlertReferences = errors.New("application has alert references")

// ErrHasInspectionPolicyReferences 应用仍被巡检策略 scope 引用，不允许删除。
var ErrHasInspectionPolicyReferences = errors.New("application has inspection policy references")

// ErrDiscoveryUnavailable 云资源发现端口未配置。
var ErrDiscoveryUnavailable = errors.New("cloud discovery port is not configured")

// ErrLeaseLost 表示同步批次不再持有 running 租约。
var ErrLeaseLost = errors.New("sync lease lost")
