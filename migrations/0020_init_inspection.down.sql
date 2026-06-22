-- 0020 rollback Inspection context (reference only, not auto-applied).

DELETE FROM iam_role_ai_tool_permission
WHERE tool_permission_id IN (
    SELECT tool_permission_id FROM iam_ai_tool_permission
    WHERE tool_code IN ('inspection.runs.create', 'inspection.findings.analyze')
);

DELETE FROM iam_ai_tool_permission
WHERE tool_code IN ('inspection.runs.create', 'inspection.findings.analyze');

DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission
    WHERE code IN ('app:inspections:read', 'app:inspections:write')
);

DELETE FROM iam_permission
WHERE code IN ('app:inspections:read', 'app:inspections:write');

DROP TABLE IF EXISTS inspection_recommendation;
DROP TABLE IF EXISTS inspection_finding;
DROP TABLE IF EXISTS inspection_run;
DROP TABLE IF EXISTS inspection_policy;
