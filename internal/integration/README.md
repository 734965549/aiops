# Integration 限界上下文

云厂商与可观测系统账号只读接入底座。契约：`ops/cloud-observability-contract.md` §4；演进设计：`docs/cloud-observability-agent-roadmap.md` §4.1。

## 目录

```text
internal/integration/
  domain/           账号、凭据引用、能力、连通性模型与仓储端口
  application/      账号 CRUD、凭据加密写入、连通性测试、审计
  infrastructure/
    persistence/    GORM 仓储（integration_* 表，迁移 0018）
    credential/     AES-GCM 凭据加密（仅存引用/密文）
    provider/       huawei_cloud / signoz / prometheus 占位 checker
    audit/          审计适配
  interfaces/http/  /api/integrations/accounts
```

## 安全边界

- API 响应不返回明文凭据，仅 `has_credential: true/false`。
- 凭据写入时加密存入 `integration_credential_ref.ciphertext`；加密密钥使用独立配置 `integration.credential_encryption_key`（与 JWT 分离），密文首字节为密钥版本号。
- `auth_type=none` 时可无凭据创建；若传入 `base_url` 等非密钥配置仍会加密存储。
- 账号 + 凭据引用 + 能力声明通过 `UnitOfWork` 事务原子提交。
- 能力字符串见 `domain/capability.go`（含独立 `topology`，不隐含于 `traces`）；契约 §4.6。
- 连通性失败时 `message` 脱敏，不暴露 AK/SK/Token。
- 写操作写审计：`integration_account` + `create/update/delete/check`。

## 权限

| 权限码 | 说明 |
| --- | --- |
| `app:integrations:read` | 列表、详情 |
| `app:integrations:create` | 创建账号 |
| `app:integrations:update` | 更新账号 |
| `app:integrations:delete` | 删除/禁用账号 |
| `app:integrations:check` | 连通性测试 |

## 验证

```powershell
go test ./internal/integration/...
make migrate   # 或 go run ./cmd/migrate -config configs/config.yaml
```
