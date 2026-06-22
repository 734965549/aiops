// Package huawei 华为云观测 Provider Adapter（infrastructure 层）。
//
// 阶段 1.5 使用 fake 实现跑通 QueryService 与 HTTP 契约；阶段 3 按能力拆分为：
//   - huawei_ces：指标（MetricQueryPort）
//   - huawei_aom：日志（LogSearchPort）
//   - huawei_apm：链路（TraceQueryPort）
//
// 厂商 SDK 与原始字段不得进入 observability/domain。
package huawei
