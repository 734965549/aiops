package config

import "time"

// AlertSyncConfig CES 告警拉取后台调度参数。
type AlertSyncConfig struct {
	// WorkerEnabled 默认 false；多实例部署时只应在一个 API 实例启用。
	WorkerEnabled bool `mapstructure:"worker_enabled"`
	// WorkerScanIntervalS 扫描启用的拉取源间隔秒数；默认 30。
	WorkerScanIntervalS int `mapstructure:"worker_scan_interval_s"`
}

// ScanInterval 返回扫描间隔。
func (c AlertSyncConfig) ScanInterval() time.Duration {
	if c.WorkerScanIntervalS <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.WorkerScanIntervalS) * time.Second
}
