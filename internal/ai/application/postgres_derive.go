package application

import (
	"strings"
)

const (
	chatToolPostgresActivity      = "postgres.activity.query"
	postgresLongTransactionMetric = "oldest_transaction_duration"
	postgresEvidenceDefaultLimit  = 300
)

func isPostgresLongTransaction(namespace, metric string) bool {
	return strings.EqualFold(strings.TrimSpace(namespace), "SYS.RDS") &&
		strings.EqualFold(strings.TrimSpace(metric), postgresLongTransactionMetric)
}

func isPostgresLongTransactionAlert(alert *AlertContext) bool {
	if alert == nil {
		return false
	}
	labels := alert.Labels
	namespace := strings.TrimSpace(labels["ces_namespace"])
	if namespace == "" {
		namespace = evidenceAnnotationString(alert.Annotations, "ces_namespace")
	}
	metric := strings.TrimSpace(labels["metric_name"])
	if metric == "" {
		metric = evidenceAnnotationString(alert.Annotations, "ces_metric_name")
	}
	return isPostgresLongTransaction(namespace, metric)
}

func derivePostgresEvidenceQuery(alert *AlertContext, resource *AssetContext) (PostgresEvidenceQuery, string) {
	metricQuery, metricSkip := deriveMetricEvidenceQuery(alert)
	resourceQuery, resourceSkip := deriveResourceMetricQuery(resource)
	query := PostgresEvidenceQuery{Limit: postgresEvidenceDefaultLimit}
	if metricSkip == "" {
		query.AccountID = metricQuery.AccountID
		query.Region = metricQuery.Region
		query.ProjectID = metricQuery.ProjectID
		query.ResourceID = metricQuery.ResourceID
		query.From, query.To = metricQuery.From, metricQuery.To
		query.InstanceID = postgresInstanceIDFromLabels(alert.Labels, metricQuery.Dimensions, metricQuery.CloudResourceID)
	} else if resourceSkip == "" {
		query.AccountID = resourceQuery.AccountID
		query.Region = resourceQuery.Region
		query.ProjectID = resourceQuery.ProjectID
		query.ResourceID = resourceQuery.ResourceID
		query.From, query.To = resourceQuery.From, resourceQuery.To
		if resource != nil {
			query.InstanceID = strings.TrimSpace(resource.CloudResourceID)
		}
	} else if alert != nil {
		return query, metricSkip
	} else {
		return query, resourceSkip
	}
	if query.InstanceID == "" {
		return query, "no_rds_instance"
	}
	return query, ""
}

func postgresInstanceIDFromLabels(labels, dimensions map[string]string, cloudResourceID string) string {
	for _, key := range []string{"rds_instance_id", "rds_cluster_id", "cloud_resource_id"} {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"rds_instance_id", "rds_cluster_id", "instance_id"} {
		if value := strings.TrimSpace(dimensions[key]); value != "" {
			return value
		}
	}
	return strings.TrimSpace(cloudResourceID)
}
