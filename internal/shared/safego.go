// Package shared 提供跨层共享的基础设施
// safego.go — goroutine panic 恢复辅助
package shared

import (
	"runtime/debug"

	"go.uber.org/zap"
)

// Recover 用于 goroutine 内 `defer Recover("name")`，捕获并记录 panic，
// 避免单个后台任务崩溃导致整个进程退出。
func Recover(name string) {
	if rec := recover(); rec != nil {
		GetLogger().Error("后台任务 panic 已恢复",
			zap.String("task", name),
			zap.Any("panic", rec),
			zap.String("stack", string(debug.Stack())),
		)
	}
}
