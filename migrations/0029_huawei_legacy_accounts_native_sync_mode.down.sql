-- 回滚 0029：本迁移为数据回填迁移，无法安全自动回滚。
-- 原因：0029 up 未记录逐账号命中清单，down 阶段无法区分“由迁移回填的 native”与“迁移后用户主动配置的 native”。
-- 若在此处按 provider + extra_config={"sync_mode":"native"} 批量清空，会误伤用户主动配置的账号。
-- 需要回滚时，请 DBA 基于备份或明确账号清单定向恢复目标账号的 extra_config。
SELECT 1;
