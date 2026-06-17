DELETE FROM iam_role_permission
WHERE permission_id IN (
    SELECT permission_id FROM iam_permission WHERE code LIKE 'app:alerts:%'
);

DELETE FROM iam_permission WHERE code LIKE 'app:alerts:%';

DROP TABLE IF EXISTS alert_silence;
DROP TABLE IF EXISTS alert_source;
DROP TABLE IF EXISTS alert_event;
DROP TABLE IF EXISTS alert_alert;
