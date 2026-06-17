// Package application 实现 Alert 告警中心第一阶段用例。
//
// 范围（ops/alert-contract.md §1）：
//   - 接收外部告警 Webhook，优先 Prometheus Alertmanager，归一化为平台 Alert
//   - 按稳定 dedup_key 生成或更新告警记录（IngestService）
//   - 告警列表、详情、时间线（AlertService）
//   - 认领、转派、开始处理、手动恢复、关闭、静默、取消静默、备注（AlertService）
//   - 接入源管理（SourceService）
//   - AI 分析、执行任务、审计日志通过 AuditRecorder 与 DTO 关联字段预留
//
// §9 跨模块集成：
//   - Asset：IngestService 经 AssetMatcher port 自动写入 application_id / resource_id
//   - Audit：AlertService 经 AuditRecorder adapter 写入 audit_operation
//   - AI：POST /api/alerts/:id/ai-analysis 写时间线；实际分析见 POST /api/ai/analyze-alert
//
// 第一阶段暂唔做：告警规则 UI、降噪编排、Incident 聚合、通知模板、云厂商主动拉取。
//
// HTTP 对外响应格式见 ops/alert-contract.md §2（httpx.OK / httpx.Fail，列表 PageData）。
// 鉴权见 §3；Webhook X-Request-ID 幂等生产须走 RedisStore（多 Pod 共享），
// redis.required=false 时可降级 MemoryStore（单实例开发）。
package application
