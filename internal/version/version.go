// Package version 通过 -ldflags 注入版本信息，供 /version 接口与启动日志使用。
package version

var (
	// Version 由构建脚本在编译时通过 -ldflags 注入。
	Version = "dev"
	// Commit 注入 Git 提交 hash 短码。
	Commit = "none"
	// BuildAt 注入构建时间（UTC ISO8601）。
	BuildAt = "unknown"
)

// Info 表示服务版本信息。
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuildAt string `json:"build_at"`
}

// Get 返回当前版本信息。
func Get() Info {
	return Info{Version: Version, Commit: Commit, BuildAt: BuildAt}
}
