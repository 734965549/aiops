// Package provider 实现各 Provider 的占位连通性探测与默认能力声明。
//
// 第一阶段仅校验凭据字段完整性并返回声明能力，不调用真实云/观测 API。
// 支持：huawei_cloud、signoz、prometheus。
package provider
