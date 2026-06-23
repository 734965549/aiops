// Package huawei 华为云观测 Provider Adapter（infrastructure 层）。
//
// auth_type=ak_sk 时 MetricQueryPort 走真实 CES（凭据由 CredentialProvider 经 integration repo/vault 解密）。
// auth_type=none 时全部能力委托 fake，供 CI 联调。
// auth_type=ak_sk/agency 时 logs/traces/topology/assets/alerts 返回 capability unsupported，不返回 fake 样本。
// 厂商 SDK 与原始字段不得进入 observability/domain。
package huawei
