-- 演示数据：构造一套连贯的电商业务运维场景。
-- 仅用于 aiops-demo 专属演示库，不属于仓库正式迁移，不得在生产执行。
-- 幂等：全部 ON CONFLICT DO NOTHING，可重复执行。

-- ============ 资产：应用 + 资源 ============
INSERT INTO asset_application (application_id, name, environment, namespace, description, created_at, updated_at) VALUES
('demo-app-payment',  '支付网关服务',   'prod', 'payment',  '核心交易链路，承载下单/支付回调', NOW(), NOW()),
('demo-app-order',    '订单中心服务',   'prod', 'order',    '订单创建与履约调度',           NOW(), NOW()),
('demo-app-user',     '用户中心服务',   'prod', 'user',     '用户注册/登录/资料',            NOW(), NOW()),
('demo-app-search',   '商品搜索服务',   'staging', 'search', '搜索与推荐实验环境',           NOW(), NOW()),
('demo-app-gateway',  '接入网关服务',   'prod', 'gateway',  '南北向流量入口',               NOW(), NOW())
ON CONFLICT (application_id) DO NOTHING;

INSERT INTO asset_resource (resource_id, application_id, name, resource_type, namespace, pod, node, instance, created_at, updated_at) VALUES
('demo-res-payment-1', 'demo-app-payment', 'payment-api-7d9f8b-xk2m9', 'pod', 'payment',  'payment-api-7d9f8b-xk2m9',  'node-bj-03', 'payment-api', NOW(), NOW()),
('demo-res-payment-2', 'demo-app-payment', 'payment-api-7d9f8b-q4t7p', 'pod', 'payment',  'payment-api-7d9f8b-q4t7p',  'node-bj-04', 'payment-api', NOW(), NOW()),
('demo-res-order-1',   'demo-app-order',   'order-api-5c7b4d-h8v2t',   'pod', 'order',    'order-api-5c7b4d-h8v2t',    'node-bj-02', 'order-api',   NOW(), NOW()),
('demo-res-user-1',    'demo-app-user',    'user-api-88fd6c-w3n5r',   'pod', 'user',     'user-api-88fd6c-w3n5r',    'node-sh-01', 'user-api',    NOW(), NOW()),
('demo-res-search-1',  'demo-app-search',  'search-api-9b2e5f-z1c8v',  'pod', 'search',   'search-api-9b2e5f-z1c8v',   'node-bj-01', 'search-api',  NOW(), NOW()),
('demo-res-gateway-1', 'demo-app-gateway', 'gw-nginx-6f4a8b-p9m3k',   'pod', 'gateway',  'gw-nginx-6f4a8b-p9m3k',    'node-bj-05', 'gw-nginx',    NOW(), NOW())
ON CONFLICT (resource_id) DO NOTHING;

-- ============ 资产匹配规则 ============
INSERT INTO asset_match_rule (rule_id, name, enabled, priority, target_type, source_type, label_key, label_value_pattern, application_id, resource_id, created_at, updated_at) VALUES
('demo-rule-payment', '支付服务按 pod 标签匹配',   TRUE, 100, 'resource', 'all', 'pod', 'payment-api-*',   'demo-app-payment', 'demo-res-payment-1', NOW(), NOW()),
('demo-rule-order',   '订单服务按 pod 标签匹配',   TRUE,  90, 'resource', 'all', 'pod', 'order-api-*',     'demo-app-order',   'demo-res-order-1',   NOW(), NOW()),
('demo-rule-user',    '用户服务按 pod 标签匹配',   TRUE,  80, 'resource', 'all', 'pod', 'user-api-*',      'demo-app-user',    'demo-res-user-1',    NOW(), NOW()),
('demo-rule-gateway', '网关服务按 pod 标签匹配',   TRUE,  70, 'resource', 'all', 'pod', 'gw-nginx-*',      'demo-app-gateway', 'demo-res-gateway-1', NOW(), NOW())
ON CONFLICT (rule_id) DO NOTHING;

-- ============ Runbook 模板 ============
INSERT INTO runbook_template (template_id, name, description, enabled, operation_type, risk_level, match_alert_name, match_resource_type, match_environment, parameter_schema, rollback_plan, created_by, created_at, updated_at) VALUES
('demo-rb-restart', '服务重启与健康验证', '滚动重启异常 Pod，等待健康检查通过并确认业务指标回升', TRUE, 'restart', 'medium', 'HighCPU', 'pod', 'prod',
 '{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}',
 '{"description":"若重启后 5 分钟内指标未恢复，回滚到上一版本镜像并扩容"}', '00000000-0000-0000-0000-000000000001', NOW(), NOW()),
('demo-rb-scale',   '扩容实例以缓解压力', '在 CPU/内存持续超阈值时扩容副本数并观察负载', TRUE, 'scale', 'low', 'CPUThrottling', 'pod', 'prod',
 '{"type":"object","properties":{"replicas":{"type":"integer","default":3}}}',
 '{"description":"压力缓解后逐步缩回原副本数"}', '00000000-0000-0000-0000-000000000001', NOW(), NOW()),
('demo-rb-rollback','版本回滚', '发布后异常时回滚至上一稳定版本', TRUE, 'rollback', 'high', 'ErrorRate', 'pod', 'prod',
 '{"type":"object","properties":{"version":{"type":"string","default":"previous"}}}',
 '{"description":"回滚失败需人工介入"}', '00000000-0000-0000-0000-000000000001', NOW(), NOW()),
('demo-rb-diagnose','慢查询诊断', '定位并优化慢 SQL，采集执行计划与索引使用情况', TRUE, 'diagnose', 'low', 'SlowQuery', 'pod', 'prod',
 '{"type":"object","properties":{"thresholdMs":{"type":"integer","default":1000}}}',
 '{"description":"仅诊断不改动线上数据"}', '00000000-0000-0000-0000-000000000001', NOW(), NOW())
ON CONFLICT (template_id) DO NOTHING;

-- ============ 告警源 ============
INSERT INTO alert_source (source_id, name, type, enabled, secret_hash, environment, business_line, description, created_at, updated_at) VALUES
('demo-src-am', '生产 Alertmanager', 'prometheus_alertmanager', TRUE, '', 'prod', '电商核心', '生产集群 Prometheus 告警入口', NOW(), NOW()),
('demo-src-custom', '业务自定义上报', 'custom_webhook', TRUE, '', 'prod', '电商核心', '业务侧主动上报的异常事件', NOW(), NOW())
ON CONFLICT (source_id) DO NOTHING;

-- ============ Runbook 步骤 ============
INSERT INTO runbook_step (step_id, template_id, step_order, name, action_type, risk_level, dry_run_supported, default_dry_run, parameter_schema, default_parameters, rollback_plan, timeout_seconds, created_at, updated_at) VALUES
('demo-rb-restart-s1', 'demo-rb-restart', 1, '采集当前实例状态与流量',   'collect',  'low',     TRUE,  TRUE,  '{}', '{}', '{}', 120, NOW(), NOW()),
('demo-rb-restart-s2', 'demo-rb-restart', 2, '滚动重启异常 Pod',        'restart',  'medium', TRUE,  FALSE, '{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}', '{"replicas":1}', '{"description":"回滚到上一版本镜像并扩容"}', 300, NOW(), NOW()),
('demo-rb-restart-s3', 'demo-rb-restart', 3, '健康检查与指标验证',      'verify',   'low',     TRUE,  TRUE,  '{}', '{}', '{}', 180, NOW(), NOW()),
('demo-rb-scale-s1',   'demo-rb-scale',   1, '评估当前负载与副本数',     'collect',  'low',     TRUE,  TRUE,  '{}', '{}', '{}', 120, NOW(), NOW()),
('demo-rb-scale-s2',   'demo-rb-scale',   2, '扩容副本至目标数量',       'scale',    'low',     TRUE,  FALSE, '{"type":"object","properties":{"replicas":{"type":"integer","default":3}}}', '{"replicas":3}', '{"description":"压力缓解后逐步缩回原副本数"}', 300, NOW(), NOW()),
('demo-rb-scale-s3',   'demo-rb-scale',   3, '观察负载回落情况',         'verify',   'low',     TRUE,  TRUE,  '{}', '{}', '{}', 180, NOW(), NOW()),
('demo-rb-rollback-s1','demo-rb-rollback',1, '确认当前与目标版本差异',   'collect',  'medium', TRUE,  TRUE,  '{}', '{}', '{}', 120, NOW(), NOW()),
('demo-rb-rollback-s2','demo-rb-rollback',2, '执行版本回滚',             'rollback', 'high',    TRUE,  FALSE, '{"type":"object","properties":{"version":{"type":"string","default":"previous"}}}', '{"version":"previous"}', '{"description":"回滚失败需人工介入"}', 600, NOW(), NOW()),
('demo-rb-rollback-s3','demo-rb-rollback',3, '业务可用性验证',           'verify',   'medium', TRUE,  TRUE,  '{}', '{}', '{}', 240, NOW(), NOW()),
('demo-rb-diagnose-s1','demo-rb-diagnose',1, '采集慢查询执行计划',       'collect',  'low',     TRUE,  TRUE,  '{}', '{}', '{}', 120, NOW(), NOW()),
('demo-rb-diagnose-s2','demo-rb-diagnose',2, '分析索引命中与锁等待',     'diagnose', 'low',     TRUE,  TRUE,  '{"type":"object","properties":{"thresholdMs":{"type":"integer","default":1000}}}', '{"thresholdMs":1000}', '{"description":"仅诊断不改动线上数据"}', 300, NOW(), NOW()),
('demo-rb-diagnose-s3','demo-rb-diagnose',3, '输出优化建议',             'report',   'low',     TRUE,  TRUE,  '{}', '{}', '{}', 180, NOW(), NOW())
ON CONFLICT (step_id) DO NOTHING;
