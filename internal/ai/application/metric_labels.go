package application

import "strings"

// chatMetricLabels 是常见 CES 指标代码到中文名的精确映射。
// 只登记有把握的条目；未命中时再按通用后缀匹配，仍无法确认则返回空（禁止臆造含义）。
var chatMetricLabels = map[string]string{
	"rds001_cpu_util":              "CPU使用率",
	"rds002_mem_util":              "内存使用率",
	"rds039_disk_util":             "磁盘使用率",
	"rds083_conn_usage":            "连接数使用率",
	"rds046_replication_lag":       "主从复制延迟",
	"rds046_replication_delay":     "主从复制延迟",
	"rds049_disk_read_bytes_rate":  "磁盘读速率",
	"rds050_disk_write_bytes_rate": "磁盘写速率",
}

// chatMetricSuffixLabels 是通用后缀映射；匹配时取最长命中项，声明顺序不影响结果。
var chatMetricSuffixLabels = []struct {
	suffix string
	label  string
}{
	{"disk_read_bytes_rate", "磁盘读速率"},
	{"disk_write_bytes_rate", "磁盘写速率"},
	{"replication_delay", "主从复制延迟"},
	{"replication_lag", "主从复制延迟"},
	{"network_in_rate", "网络入速率"},
	{"network_out_rate", "网络出速率"},
	{"connections_usage", "连接数使用率"},
	{"connection_usage", "连接数使用率"},
	{"active_connections", "活跃连接数"},
	{"buffer_hit_ratio", "缓存命中率"},
	{"cache_hit_ratio", "缓存命中率"},
	{"conn_usage", "连接数使用率"},
	{"conn_util", "连接数使用率"},
	{"read_iops", "读IOPS"},
	{"write_iops", "写IOPS"},
	{"cpu_util", "CPU使用率"},
	{"mem_util", "内存使用率"},
	{"disk_util", "磁盘使用率"},
	{"swap_util", "Swap使用率"},
	{"load_average", "系统负载"},
	{"slow_queries", "慢查询数"},
	{"network_in", "网络入流量"},
	{"network_out", "网络出流量"},
	{"qps", "每秒查询数"},
	{"tps", "每秒事务数"},
	{"iops", "磁盘IOPS"},
	{"pps", "包收发速率"},
	{"deadlocks", "死锁数"},
}

// chatMetricLabel 返回指标代码的中文名；空串表示未映射。
func chatMetricLabel(metric string) string {
	metric = strings.TrimSpace(metric)
	if metric == "" {
		return ""
	}
	key := strings.ToLower(metric)
	if label, ok := chatMetricLabels[key]; ok {
		return label
	}
	bestLabel := ""
	bestLen := 0
	for _, item := range chatMetricSuffixLabels {
		if key != item.suffix && !strings.HasSuffix(key, "_"+item.suffix) {
			continue
		}
		if len(item.suffix) > bestLen {
			bestLabel = item.label
			bestLen = len(item.suffix)
		}
	}
	return bestLabel
}

// chatMetricDisplay 返回观测摘要使用的指标展示文本；未映射时明确标注。
func chatMetricDisplay(metric string) string {
	metric = strings.TrimSpace(metric)
	if metric == "" {
		return "未知指标"
	}
	if label := chatMetricLabel(metric); label != "" {
		return metric + " / " + label
	}
	return metric + " / 未映射"
}
