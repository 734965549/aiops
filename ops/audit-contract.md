# 审计中心接口契约 (Audit Contract)

本文档定义 AI 运维平台操作审计中心接口规范。

## 1. 概述

操作审计中心记录平台内所有业务操作（如告警关闭/认领/静音、执行任务确认/拒绝/触发、Runbook 修改、资产调整、AI 工具调用等）。
审计中心通过统一的只读接口向管理员与运维团队提供审计日志检索、链路追踪聚合、资源操作聚合以及 CSV 导出能力。

## 2. 鉴权与权限码

- 认证方式：Bearer Token (JWT)
- 权限码：`app:audits:read`（通过 `iam_permission` 与各角色绑定）
- 审计记录不可由外部通过 API 修改或删除，保证不可篡改性与审计追溯。

## 3. 接口列表

### 3.1 获取操作审计列表

- **Method**: `GET`
- **Path**: `/api/audits`
- **Query 参数**:
  - `page`: 页码（默认 1）
  - `page_size`: 每页条数（默认 20，最大 100）
  - `resource_type`: 资源类型筛选（如 `alert`、`asset`、`execution`、`runbook`、`ai`、`auth` 等）
  - `resource_id`: 资源 ID 筛选
  - `user_id`: 操作人用户业务 ID 筛选
  - `action`: 操作动作筛选（如 `close`、`silence`、`confirm`、`execute` 等）
  - `trace_id`: 全链路跟踪 ID 筛选（精准匹配 context 或 payload 中的 trace_id）

- **响应格式**:
  ```json
  {
    "code": "OK",
    "message": "ok",
    "trace_id": "c1f7b8...",
    "data": {
      "items": [
        {
          "id": "00000000-0000-0000-0000-000000000001",
          "user_id": "user-admin-001",
          "resource_type": "alert",
          "resource_id": "alt-prod-cpu-high-01",
          "action": "close",
          "payload": {
            "comment": "已完成扩容处置",
            "trace_id": "req-123456"
          },
          "ip": "192.168.1.50",
          "user_agent": "Mozilla/5.0 ...",
          "created_at": 1773300000
        }
      ],
      "total": 1,
      "page": 1,
      "page_size": 20
    }
  }
  ```

## 4. 前端视图与聚合能力

前端视图位于 `web/src/views/audits/index.vue`，实现：
1. **多维筛选与统计**：支持按资源类型、资源ID、操作人、动作、TraceID 组合筛选，提供按类型聚合的快捷统计卡片。
2. **多视图聚合查看**：
   - **明细列表**：可展开查看结构化 Payload，复制 TraceID/资源ID，支持详情抽屉。
   - **按 Trace 聚合**：按全链路调用链跟踪步骤数、涉及资源数及操作时间窗。
   - **按 Resource 聚合**：汇总单资源的变更频率、涉及操作人及动作分布。
   - **按 User 聚合**：统计操作人行为画像、资源覆盖范围及最近活跃时间。
3. **安全数据导出**：支持根据当前检索条件导出 CSV 文件（含 UTF-8 BOM 兼容 Excel）。
