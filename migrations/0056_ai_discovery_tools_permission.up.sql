-- 0056: AI 上下文发现只读工具权限（accounts.list / resource.analyze / resources.list）
-- 工具权限走 iam_ai_tool_permission（与 alarm.analyze / metrics.query / alert.rules.list 同一套校验），
-- 仅绑定内置 admin；无表结构变更。
INSERT INTO iam_ai_tool_permission (tool_permission_id, tool_code, tool_name, permission_mode, allow_confirm, description, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0003-000000000006', 'accounts.list', 'Accounts list tool', 'read_only', FALSE,
     'List integration accounts visible to the current user for chat context discovery (ops/ai-contract.md 10.12.1)', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000007', 'resource.analyze', 'Resource analyze tool', 'read_only', FALSE,
     'Read the selected asset resource context for chat analysis (ops/ai-contract.md 10.12.1)', NOW(), NOW()),
    ('00000000-0000-0000-0003-000000000008', 'resources.list', 'Cloud resources list tool', 'read_only', FALSE,
     'List cloud-synced resources for an integration account (ops/ai-contract.md 10.12.1)', NOW(), NOW())
ON CONFLICT (tool_code) DO UPDATE SET
    tool_name = EXCLUDED.tool_name,
    permission_mode = EXCLUDED.permission_mode,
    allow_confirm = EXCLUDED.allow_confirm,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO iam_role_ai_tool_permission (role_id, tool_permission_id, created_at, updated_at)
SELECT r.role_id, tp.tool_permission_id, NOW(), NOW()
FROM iam_role r
JOIN iam_ai_tool_permission tp
    ON tp.tool_code IN ('accounts.list', 'resource.analyze', 'resources.list')
WHERE r.code = 'admin'
ON CONFLICT (role_id, tool_permission_id) DO UPDATE SET updated_at = NOW();
