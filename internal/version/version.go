// Package version 提供构建时注入的版本信息。
// 构建命令通过 -ldflags "-X aiporto/internal/version.Commit=abc1234" 写入 git commit。
// 未注入时显示 dev，本地开发不影响。
package version

// Commit 当前构建的 git commit 短哈希。
var Commit = "dev"

// Get 返回版本字符串。
func Get() string {
	if Commit == "" {
		return "dev"
	}
	return Commit
}
