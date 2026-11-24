# internal/integration 模块说明

Integration 是云厂商与观测平台的账号接入上下文，负责账号注册、凭据加密引用、默认能力声明和字段级连接检查。它不负责执行指标、日志、链路、拓扑或资源查询；真实能力由 Observability、Asset 等模块的 Provider Adapter 单独实现。稳定契约见 `ops/cloud-observability-contract.md` §4。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 账号管理 | 列表、详情、创建、部分更新、软删除 | 只有静态 RBAC，无 owner_team 数据范围过滤；无删除后恢复接口 |
| Provider | `huawei_cloud`、`signoz`、`prometheus` | `custom` 和其他 provider 会被拒绝 |
| 认证方式 | `none`、`ak_sk`、`agency`、`api_token` 按 provider 组合 | 前端缺少 agency 凭据字段，agency 目前只能通过 API 完整配置 |
| 凭据保存 | AES-GCM 加密 JSON、账号引用、指纹 | 不返回明文；外部 Secret 引用只是模型预留，无外部 Vault 接入 |
| 能力声明 | 创建和检查时写入 provider 默认 capabilities | 是声明/门控元数据，不是在线能力实测结果 |
| 连通性检查 | 字段检查、结果历史、最近状态、审计 | 当前所有 checker 都不访问真实云端或观测平台 |
| 前端 | 列表筛选、创建/编辑、启停、删除、检查、资源同步入口 | 无关键字框、检查历史、凭据清空、provider 编辑或 agency 表单 |
| 密钥轮换 | 密文有版本字节 | 仅持有一个当前密钥；版本不匹配直接拒绝，没有多版本解密和在线重加密流程 |

## 2. 路由与权限

所有路由要求 Bearer Token，并使用 `AuthorizeStatic`，只检查固定 RBAC 权限，不执行对象级数据范围过滤。

| 方法与路径 | 权限 |
| --- | --- |
| `GET /api/integrations/accounts` | `app:integrations:read` |
| `GET /api/integrations/accounts/:account_id` | `app:integrations:read` |
| `POST /api/integrations/accounts` | `app:integrations:create` |
| `PUT /api/integrations/accounts/:account_id` | `app:integrations:update` |
| `DELETE /api/integrations/accounts/:account_id` | `app:integrations:delete` |
| `POST /api/integrations/accounts/:account_id/check` | `app:integrations:check` |

migration `0018_init_integration` 创建五类权限并默认绑定 admin，同时将旧 `app:integrations:write` 的角色绑定迁移到五类新权限后删除旧权限。`owner_team` 不会自动过滤 Integration 列表或详情；其他模块是否使用该字段做数据范围校验属于各自调用点。

## 3. Provider、认证与凭据校验

| Provider | 支持的 `auth_type` | 非 none 的主要凭据要求 |
| --- | --- | --- |
| `huawei_cloud` | `none`、`ak_sk`、`agency` | AK/SK 需要 `access_key + secret_key`；agency 需要 `agency_name + domain_name` |
| `signoz` | `none`、`api_token` | `api_token` 或 `access_token` 至少一个 |
| `prometheus` | `none`、`api_token`、`ak_sk` | 除相应认证字段外，还必须提供 `credential.base_url` |

通用规则：

- 创建非 none 账号必须提供相应凭据；provider/auth_type 不兼容返回 `INVALID_ARGUMENT`。
- `auth_type=none` 允许省略凭据；如果仍传入 `base_url` 等配置，非空内容也会加密保存并产生 `has_credential=true`。
- 凭据 map 会去除空键和空值；API 响应只返回 `has_credential`，不返回 credential、ciphertext、fingerprint 或 credential_ref_id。
- 更新省略 `credential` 或传空对象都会保留原凭据；当前没有通过更新清空凭据的语义。
- 切换到 `auth_type=none` 不会自动删除旧密文；只有删除账号时才会硬删除凭据。需要清理旧凭据时应按这一边界验收。
- provider 或 auth_type 变化时会重新校验新凭据或已存凭据；provider 变化还会刷新默认 capabilities。后端允许 provider 更新，但前端编辑弹窗禁用了 provider。
- regions 会去除空项、去重并排序；华为 region 只允许小写字母、数字和合法连字符。创建时的区域校验不代替 checker 对非 none 华为账号“至少一个 region”的检查。

## 4. 凭据加密与事务

```text
credential map
  -> Vault：SHA-256 派生当前密钥 -> AES-GCM + 随机 nonce
  -> 密文首字节写 key version
  -> integration_credential_ref
  -> integration_account.credential_ref_id
```

- 当前配置为 `integration.credential_encryption_key` 和 `credential_encryption_key_version`，与 JWT secret 独立。
- dev 允许占位密钥；非 dev 会校验强度，并要求与 JWT secret 不同。
- 密文版本用于识别当前密钥，不等于已完成轮换：Decrypt 只接受当前版本，没有旧密钥集合或批量重加密工具。
- `external_ref` / `store_type=external_ref` 是预留模型，当前写入路径固定使用 `local_encrypted`。
- 创建时账号、凭据和默认 capabilities 在同一 UnitOfWork 事务提交。
- 更新凭据或 provider 时使用事务；删除时账号软删除与凭据硬删除在同一事务提交。
- 检查时 check history 与 capabilities 替换在同一事务提交。
- 审计在上述事务之外 best-effort 写入，不在同一事务中。

Observability 的账号摘要端口不携带明文；真实华为 Provider 另有服务端 CredentialProvider 在 infrastructure 边界读取密文并解密。明文不得进入前端、DTO、日志或审计，但不能描述成“只有 Integration application 会接触明文”。

## 5. `extra_config` 与历史账号

`extra_config` 保存 provider 专属非敏感 JSON。服务端递归拒绝明显敏感键，读取旧数据时也会移除这些键；普通未知非敏感键并非全部拒绝。

华为已识别字段：

- `sync_mode`：`ces` / `hybrid` / `native`。
- `resource_group_name`、`resource_group_id`、`enterprise_project_id`。
- `max_resources`：1–20000 的整数。
- `region_projects`：按 region 唯一的 `region + project_id` 映射，可选资源组字段；数组项的未知字段会被拒绝。

新建华为账号传空或省略 `extra_config` 时显式保存 `sync_mode=ces`；其他 provider 为 `{}`。更新省略该字段会保留原值，传 `{}` 会恢复上述默认配置。

migration `0024` 增加该列，`0029` 将仍为空配置的历史华为账号回填为 `sync_mode=native`，避免升级后历史账号无意切到 CES。新账号默认 CES 与历史账号保留 native 是两条不同路径。

前端会保留编辑前存在但表单未识别的非敏感未知 key；资源组、sync_mode 和 region_projects 的具体同步语义见 `ops/huawei-ces-sync-contract.md`。

## 6. 连通性检查的真实语义

当前 `HuaweiCloudChecker`、`SigNozChecker`、`PrometheusChecker` 都只检查认证方式、必要字段和部分 region/base_url 条件，不发起网络请求：

- `status=ok` 只表示字段级检查通过，不证明账号有效、网络可达、云端权限足够或每项 capability 可用。
- checker 出错时，服务生成 `status=failed` 的检查结果并脱敏 message；只要结果和 capabilities 成功落库，HTTP 仍返回统一成功响应，调用方必须检查 `data.status`。
- 账号不存在返回 404，disabled 返回 412；结果持久化失败才作为请求错误返回。
- 检查结果为空 capabilities 时也会补 provider 默认 capabilities，包括 failed 结果，因此 capabilities 不能当作实测通过清单。
- 每次检查保存到 `integration_check_result`，账号 DTO 只暴露最近一次的 `last_check_status`；当前没有检查历史查询 API。

默认声明：

| Provider | 默认 capabilities |
| --- | --- |
| `huawei_cloud` | `assets`、`metrics`、`logs`、`traces`、`topology`、`alerts` |
| `signoz` | `metrics`、`logs`、`traces`、`topology`、`alerts` |
| `prometheus` | `metrics`、`alerts` |

这些默认值用于后续模块的能力门控，不表示真实 Adapter 已全部实现。尤其真实凭据下不支持的 Observability 能力应返回 unsupported，而不是用 fake 数据冒充。

## 7. 前端页面

页面为 `web/src/views/integrations/index.vue`，API 为 `web/src/api/integration.ts`，路由和静态侧栏入口为 `/integrations`。

- 支持 Provider/启用状态筛选、分页、新建、编辑、启停、删除确认、检查和触发 Asset Sync。
- 列表 API 另支持 `keyword`，按 name/account_id/owner_team 做 ILIKE，但页面没有关键字输入。
- 编辑不回显凭据，留空即保留；页面没有清空凭据入口，也禁用了 provider 修改。
- 认证下拉展示全部选项，最终组合由后端校验；页面缺少 `agency_name/domain_name`，无法新建 agency 账号或新填/替换 agency 凭据。已有 agency 账号仍可在保留原凭据时编辑其他字段。
- 检查接口 HTTP 成功后页面统一使用成功 Toast，即使 `data.status=failed`；默认 capabilities 非空时还可能不展示失败 message。验收必须以 status 和最近检查结果为准，不能仅看 Toast 颜色。
- “同步资源”入口对各行显示，真正 enabled/assets capability/RBAC 校验由 Asset Sync 后端执行；按钮存在不代表该账号一定可同步。
- 当前只有 `huaweiConfig.spec.ts` 覆盖 region/region_projects 解析、转义和未知 extra_config 保留，没有完整页面 Vitest。

## 8. 审计

创建、更新、删除和检查写：

```text
resource_type = integration_account
resource_id   = account_id
action        = create / update / delete / check
```

Payload 只含名称、provider、enabled、检查状态等业务摘要，不包含凭据。审计在业务事务提交后 best-effort 写入，失败不回滚账号或检查结果；业务成功不能单独证明审计已落库。

## 9. 迁移与验证覆盖

- `0018_init_integration`：四张 `integration_*` 表、五类权限及 admin 绑定。
- `0024_integration_account_extra_config`：provider 扩展配置。
- `0029_huawei_legacy_accounts_native_sync_mode`：历史空配置华为账号保留 native。

仓库已有覆盖：

- domain：provider/auth_type 兼容、capability 合法值和默认声明。
- application：删除清密文、none 凭据、extra_config、provider/auth_type 校验、更新、region 和错误脱敏等。
- infrastructure：AES-GCM 往返/错误版本、华为 checker 字段校验。
- HTTP：创建成功、none/非 none 凭据、参数错误、未认证和权限不足。
- 前端：华为配置纯函数 Vitest。
- `e2e-integration.ps1`：使用 `huawei_cloud + none` 覆盖创建、列表/详情、检查 ok、更新、viewer 创建 403 和删除。

E2E 不使用真实凭据，不覆盖 AK/SK/agency/api_token、加密/解密、凭据替换/保留/清空、检查 failed、keyword、软删除后的详情/列表、审计断言、真实外部网络或完整 UI。本轮仅静态复核，未重新执行测试或脚本。
