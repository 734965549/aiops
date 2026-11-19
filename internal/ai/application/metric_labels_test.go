package application

import "testing"

func TestChatMetricLabel(t *testing.T) {
	cases := map[string]string{
		"rds001_cpu_util":        "CPU使用率",
		"rds002_mem_util":        "内存使用率",
		"rds039_disk_util":       "磁盘使用率",
		"rds083_conn_usage":      "连接数使用率",
		"rds046_replication_lag": "主从复制延迟",
		"some_cpu_util":          "CPU使用率",
		"db_qps":                 "每秒查询数",
		"read_iops":              "读IOPS",
		"x_read_iops":            "读IOPS",
		"x_connections_usage":    "连接数使用率",
		"unknown_metric":         "",
		"":                       "",
	}
	for metric, want := range cases {
		if got := chatMetricLabel(metric); got != want {
			t.Fatalf("chatMetricLabel(%q) = %q, want %q", metric, got, want)
		}
	}
}

func TestChatMetricDisplay(t *testing.T) {
	if got := chatMetricDisplay("rds002_mem_util"); got != "rds002_mem_util / 内存使用率" {
		t.Fatalf("mapped display = %q", got)
	}
	if got := chatMetricDisplay("custom_metric"); got != "custom_metric / 未映射" {
		t.Fatalf("unmapped display = %q", got)
	}
}
