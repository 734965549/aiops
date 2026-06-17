// Package domain 定义 Alert 告警中心限界上下文的领域模型、状态机规则与仓储接口。
//
// 契约来源：ops/alert-contract.md
//
// 第一阶段核心实体（§5）：
//   - Alert：告警主记录，一轮可处理的生命周期
//   - AlertEvent：时间线事件
//   - AlertSource：外部接入源配置
//   - AlertSilence：静默记录
//
// 领域枚举（§4）：
//   - AlertSeverity：p0/p1/p2/p3/info，含外部级别归一化
//   - AlertStatus：new/acknowledged/processing/recovered/closed/silenced
//   - AlertEventType：triggered/updated/recovered/acknowledged 等
//   - StatusAction + TransitionStatus：人工与 external_recover 状态机（§4.2）
//
// 错误约定：
//   - 本层仅定义标准库哨兵（errors.go），不含 HTTP 业务码
//   - application 层经 pkg/errors.MapSentinels 映射为 NOT_FOUND / INVALID_ARGUMENT 等（§10）
//   - 本层不依赖 pkg/logger，日志由 application / interfaces 层负责
//
// 仓储接口定义在 repository.go，由 infrastructure/persistence 用 GORM 实现（依赖倒置）。
// 表结构与索引见 migrations/0007_init_alert.up.sql、契约 §11 / §7.3。
package domain
