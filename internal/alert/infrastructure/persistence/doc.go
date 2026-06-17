// Package persistence 用 GORM 实现 Alert 领域仓储接口（domain/repository.go）。
//
// 映射 alert_* 四张表（ops/alert-contract.md §11）：
//   - alert_alert：告警主表，dedup_key + lifecycle_seq 唯一，active 部分唯一索引
//   - alert_event：时间线，labels/annotations/payload 使用 JSONB
//   - alert_source：接入源，secret_hash 存 Webhook token 哈希
//   - alert_silence：静默记录
//
// 建表规范：BIGSERIAL 自增主键 + VARCHAR(36) 业务 ID 唯一索引；
// 时间字段 TIMESTAMPTZ，由 pkg/database.BaseModel Hook 在 Go 侧赋值。
//
// 唯一约束冲突经 pkg/database.MapUniqueViolation 映射为 domain.ErrAlreadyExists，
// 再由 application 层 mapDomainError 转为 pkg/errors 业务码。
package persistence
