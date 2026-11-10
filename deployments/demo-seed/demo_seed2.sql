-- 演示数据 part2：告警实例 + 事件时间线 + 历史执行任务。
-- 仅用于 aiops-demo 专属演示库。幂等：ON CONFLICT DO NOTHING，可重复执行。

-- ============ 告警实例 ============
INSERT INTO alert_alert (
  alert_id, external_id, source, source_id, source_name, fingerprint, dedup_key, lifecycle_seq,
  name, summary, description, severity, status, rule_id, rule_name, business_line, environment,
  application_id, application_name, resource_id, resource_type, resource_name,
  owner_user_id, assignee_user_id, labels, annotations, occurrence_count,
  first_seen_at, last_seen_at, recovered_at, acknowledged_at, closed_at,
  created_at, updated_at
) VALUES
('demo-alert-cpu', 'demo-ext-cpu', 'prometheus_alertmanager', 'demo-src-am', '生产 Alertmanager', 'fp-cpu-001', 'demo-dedup-cpu', 1,
 'HighCPU', '支付网关 CPU 使用率持续超过 90%', '节点 node-bj-03 上 payment-api 副本 CPU 使用率 5 分钟内平均 93.4%，超过 90% 阈值，可能导致下单请求超时。', 'p1', 'processing', 'cpu-high', 'CPU 使用率过高', '电商核心', 'prod',
 'demo-app-payment', '支付网关服务', 'demo-res-payment-1', 'pod', 'payment-api-7d9f8b-xk2m9',
 '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001',
 '{"pod":"payment-api-7d9f8b-xk2m9","namespace":"payment","severity":"p1","service":"payment-api","node":"node-bj-03"}',
 '{"summary":"CPU > 90%","runbook_url":"","dashboard_url":""}', 12,
 NOW() - INTERVAL '52 minutes', NOW() - INTERVAL '3 minutes', NULL, NOW() - INTERVAL '45 minutes', NULL,
 NOW() - INTERVAL '52 minutes', NOW() - INTERVAL '3 minutes'),

('demo-alert-latency', 'demo-ext-latency', 'prometheus_alertmanager', 'demo-src-am', '生产 Alertmanager', 'fp-lat-002', 'demo-dedup-latency', 1,
 'HighLatency', '订单中心接口 P99 响应时间超过 2s', 'order-api 接口 P99 延迟 2.34s，超过 2s 阈值，下单链路存在劣化风险。', 'p2', 'acknowledged', 'latency-high', '接口延迟过高', '电商核心', 'prod',
 'demo-app-order', '订单中心服务', 'demo-res-order-1', 'pod', 'order-api-5c7b4d-h8v2t',
 '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001',
 '{"pod":"order-api-5c7b4d-h8v2t","namespace":"order","severity":"p2","service":"order-api"}',
 '{"summary":"P99 > 2000ms"}', 5,
 NOW() - INTERVAL '28 minutes', NOW() - INTERVAL '6 minutes', NULL, NOW() - INTERVAL '20 minutes', NULL,
 NOW() - INTERVAL '28 minutes', NOW() - INTERVAL '6 minutes'),

('demo-alert-disk', 'demo-ext-disk', 'prometheus_alertmanager', 'demo-src-am', '生产 Alertmanager', 'fp-disk-003', 'demo-dedup-disk', 1,
 'DiskFull', '用户中心日志盘使用率 88%', 'user-api 节点日志分区使用率 88%，按当前写入速率预计 6 小时后写满。', 'p2', 'new', 'disk-usage-high', '磁盘使用率过高', '电商核心', 'prod',
 'demo-app-user', '用户中心服务', 'demo-res-user-1', 'pod', 'user-api-88fd6c-w3n5r',
 '', '',
 '{"pod":"user-api-88fd6c-w3n5r","namespace":"user","severity":"p2","service":"user-api"}',
 '{"summary":"disk 88%"}', 1,
 NOW() - INTERVAL '9 minutes', NOW() - INTERVAL '9 minutes', NULL, NULL, NULL,
 NOW() - INTERVAL '9 minutes', NOW() - INTERVAL '9 minutes'),

('demo-alert-gw', 'demo-ext-gw', 'prometheus_alertmanager', 'demo-src-am', '生产 Alertmanager', 'fp-gw-004', 'demo-dedup-gw', 1,
 'Gateway5xx', '接入网关 5xx 错误率突增', 'gw-nginx 上游返回 5xx 比例升至 3.2%，疑似后端订单服务异常传导。', 'p0', 'recovered', 'gateway-5xx', '网关错误率突增', '电商核心', 'prod',
 'demo-app-gateway', '接入网关服务', 'demo-res-gateway-1', 'pod', 'gw-nginx-6f4a8b-p9m3k',
 '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001',
 '{"pod":"gw-nginx-6f4a8b-p9m3k","namespace":"gateway","severity":"p0","service":"gw-nginx"}',
 '{"summary":"5xx 3.2%"}', 8,
 NOW() - INTERVAL '3 hours', NOW() - INTERVAL '2 hours 10 minutes', NOW() - INTERVAL '2 hours 10 minutes', NOW() - INTERVAL '2 hours 40 minutes', NULL,
 NOW() - INTERVAL '3 hours', NOW() - INTERVAL '2 hours 10 minutes'),

('demo-alert-oom', 'demo-ext-oom', 'custom_webhook', 'demo-src-custom', '业务自定义上报', 'fp-oom-005', 'demo-dedup-oom', 1,
 'OOMKilled', '搜索服务容器被 OOM Killer 终止', 'staging 环境 search-api 容器因内存超限被终止 2 次。', 'p3', 'closed', 'container-oom', '容器内存溢出', '电商核心', 'staging',
 'demo-app-search', '商品搜索服务', 'demo-res-search-1', 'pod', 'search-api-9b2e5f-z1c8v',
 '', '',
 '{"pod":"search-api-9b2e5f-z1c8v","namespace":"search","severity":"p3","service":"search-api"}',
 '{"summary":"OOMKilled x2"}', 2,
 NOW() - INTERVAL '1 day', NOW() - INTERVAL '23 hours', NOW() - INTERVAL '23 hours', NOW() - INTERVAL '23 hours 30 minutes', NOW() - INTERVAL '22 hours',
 NOW() - INTERVAL '1 day', NOW() - INTERVAL '22 hours')
ON CONFLICT (alert_id) DO NOTHING;

-- ============ 告警事件时间线 ============
INSERT INTO alert_event (event_id, alert_id, event_type, actor_type, actor_id, actor_name, message, payload, created_at, updated_at) VALUES
('demo-ev-cpu-1', 'demo-alert-cpu', 'created',   'system', '', 'Prometheus', '告警触发：CPU 使用率超过 90%', '{"severity":"p1"}', NOW() - INTERVAL '52 minutes', NOW() - INTERVAL '52 minutes'),
('demo-ev-cpu-2', 'demo-alert-cpu', 'acknowledged', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已认领并开始排查', '{}', NOW() - INTERVAL '45 minutes', NOW() - INTERVAL '45 minutes'),
('demo-ev-cpu-3', 'demo-alert-cpu', 'execution_created', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已创建执行任务，等待确认', '{}', NOW() - INTERVAL '40 minutes', NOW() - INTERVAL '40 minutes'),
('demo-ev-cpu-4', 'demo-alert-cpu', 'processing', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '进入处理中状态', '{}', NOW() - INTERVAL '38 minutes', NOW() - INTERVAL '38 minutes'),
('demo-ev-lat-1', 'demo-alert-latency', 'created', 'system', '', 'Prometheus', '告警触发：P99 延迟超过 2s', '{"severity":"p2"}', NOW() - INTERVAL '28 minutes', NOW() - INTERVAL '28 minutes'),
('demo-ev-lat-2', 'demo-alert-latency', 'acknowledged', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已认领', '{}', NOW() - INTERVAL '20 minutes', NOW() - INTERVAL '20 minutes'),
('demo-ev-disk-1', 'demo-alert-disk', 'created', 'system', '', 'Prometheus', '告警触发：磁盘使用率 88%', '{"severity":"p2"}', NOW() - INTERVAL '9 minutes', NOW() - INTERVAL '9 minutes'),
('demo-ev-gw-1', 'demo-alert-gw', 'created', 'system', '', 'Prometheus', '告警触发：网关 5xx 错误率突增', '{"severity":"p0"}', NOW() - INTERVAL '3 hours', NOW() - INTERVAL '3 hours'),
('demo-ev-gw-2', 'demo-alert-gw', 'acknowledged', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已认领', '{}', NOW() - INTERVAL '2 hours 40 minutes', NOW() - INTERVAL '2 hours 40 minutes'),
('demo-ev-gw-3', 'demo-alert-gw', 'execution_created', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已创建执行任务', '{}', NOW() - INTERVAL '2 hours 30 minutes', NOW() - INTERVAL '2 hours 30 minutes'),
('demo-ev-gw-4', 'demo-alert-gw', 'recovered', 'system', '', 'Prometheus', '错误率恢复正常，告警自动恢复', '{}', NOW() - INTERVAL '2 hours 10 minutes', NOW() - INTERVAL '2 hours 10 minutes'),
('demo-ev-oom-1', 'demo-alert-oom', 'created', 'system', '', 'Webhook', '业务侧上报容器 OOM', '{"severity":"p3"}', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day'),
('demo-ev-oom-2', 'demo-alert-oom', 'acknowledged', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '已认领', '{}', NOW() - INTERVAL '23 hours 30 minutes', NOW() - INTERVAL '23 hours 30 minutes'),
('demo-ev-oom-3', 'demo-alert-oom', 'recovered', 'system', '', 'Webhook', '扩容后恢复正常', '{}', NOW() - INTERVAL '23 hours', NOW() - INTERVAL '23 hours'),
('demo-ev-oom-4', 'demo-alert-oom', 'closed', 'user', '00000000-0000-0000-0004-000000000001', 'Administrator', '处理完毕，关闭告警', '{}', NOW() - INTERVAL '22 hours', NOW() - INTERVAL '22 hours')
ON CONFLICT (event_id) DO NOTHING;

-- ============ 历史执行任务 ============
INSERT INTO exec_task (
  task_id, source_type, source_id, operation_type, name, status, risk_level,
  target_type, target_id, target_name, environment, parameters, rollback_plan,
  created_by, confirmed_by, executed_by, confirmed_at, started_at, finished_at,
  result_summary, error_message, created_at, updated_at
) VALUES
('demo-task-cpu', 'alert', 'demo-alert-cpu', 'restart', '支付网关滚动重启与健康验证', 'pending_confirm', 'medium',
 'pod', 'demo-res-payment-1', 'payment-api-7d9f8b-xk2m9', 'prod', '{"replicas":1}', '{"description":"若重启后 5 分钟内指标未恢复，回滚到上一版本镜像并扩容"}',
 '00000000-0000-0000-0004-000000000001', '', '',
 NULL, NULL, NULL, '', '',
 NOW() - INTERVAL '40 minutes', NOW() - INTERVAL '40 minutes'),

('demo-task-gw', 'alert', 'demo-alert-gw', 'restart', '接入网关实例重启', 'succeeded', 'medium',
 'pod', 'demo-res-gateway-1', 'gw-nginx-6f4a8b-p9m3k', 'prod', '{"replicas":1}', '{"description":"回滚到上一版本镜像并扩容"}',
 '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001',
 NOW() - INTERVAL '2 hours 28 minutes', NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 22 minutes',
 '网关实例重启完成，5xx 错误率回落至 0.2%', '',
 NOW() - INTERVAL '2 hours 30 minutes', NOW() - INTERVAL '2 hours 22 minutes'),

('demo-task-scale', 'alert', 'demo-alert-oom', 'scale', '搜索服务扩容至 3 副本', 'succeeded', 'low',
 'pod', 'demo-res-search-1', 'search-api-9b2e5f-z1c8v', 'staging', '{"replicas":3}', '{"description":"压力缓解后逐步缩回原副本数"}',
 '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001', '00000000-0000-0000-0004-000000000001',
 NOW() - INTERVAL '23 hours 20 minutes', NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 12 minutes',
 '扩容完成，内存使用率由 94% 降至 52%，OOM 未再发生', '',
 NOW() - INTERVAL '23 hours 30 minutes', NOW() - INTERVAL '23 hours 12 minutes')
ON CONFLICT (task_id) DO NOTHING;

-- ============ 执行步骤 ============
INSERT INTO exec_step (
  step_id, task_id, step_order, name, action_type, status, output, error_message,
  started_at, finished_at, created_at, updated_at, runbook_step_id, parameters, risk_level, dry_run, rollback_plan, timeout_seconds
) VALUES
('demo-tsk-gw-s1', 'demo-task-gw', 1, '采集当前实例状态与流量', 'collect', 'succeeded', '{"replicas":3,"cpu":"12%","traffic":"normal"}', '',
 NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 27 minutes',
 'demo-rb-restart-s1', '{}', 'low', TRUE, '{}', 120),
('demo-tsk-gw-s2', 'demo-task-gw', 2, '滚动重启异常 Pod', 'restart', 'succeeded', '{"restarted":"gw-nginx-6f4a8b-p9m3k","downtime":"3s"}', '',
 NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 24 minutes', NOW() - INTERVAL '2 hours 27 minutes', NOW() - INTERVAL '2 hours 24 minutes',
 'demo-rb-restart-s2', '{"replicas":1}', 'medium', FALSE, '{"description":"回滚到上一版本镜像并扩容"}', 300),
('demo-tsk-gw-s3', 'demo-task-gw', 3, '健康检查与指标验证', 'verify', 'succeeded', '{"healthy":true,"error_rate":"0.2%"}', '',
 NOW() - INTERVAL '2 hours 24 minutes', NOW() - INTERVAL '2 hours 22 minutes', NOW() - INTERVAL '2 hours 24 minutes', NOW() - INTERVAL '2 hours 22 minutes',
 'demo-rb-restart-s3', '{}', 'low', TRUE, '{}', 180),
('demo-tsk-scale-s1', 'demo-task-scale', 1, '评估当前负载与副本数', 'collect', 'succeeded', '{"replicas":1,"mem_usage":"94%"}', '',
 NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 19 minutes',
 'demo-rb-scale-s1', '{}', 'low', TRUE, '{}', 120),
('demo-tsk-scale-s2', 'demo-task-scale', 2, '扩容副本至目标数量', 'scale', 'succeeded', '{"from":1,"to":3,"mem_usage_after":"52%"}', '',
 NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 12 minutes', NOW() - INTERVAL '23 hours 19 minutes', NOW() - INTERVAL '23 hours 12 minutes',
 'demo-rb-scale-s2', '{"replicas":3}', 'low', FALSE, '{"description":"压力缓解后逐步缩回原副本数"}', 300),
('demo-tsk-scale-s3', 'demo-task-scale', 3, '观察负载回落情况', 'verify', 'succeeded', '{"stable":true,"oom_count":0}', '',
 NOW() - INTERVAL '23 hours 12 minutes', NOW() - INTERVAL '23 hours 12 minutes', NOW() - INTERVAL '23 hours 12 minutes', NOW() - INTERVAL '23 hours 12 minutes',
 'demo-rb-scale-s3', '{}', 'low', TRUE, '{}', 180)
ON CONFLICT (step_id) DO NOTHING;
