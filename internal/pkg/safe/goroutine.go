// Package safe 收纳 oekit concurrency/async 没有覆盖的 panic 处理原语。
// goroutine 的启动一律走 async.GoSafe(进程级 PanicHandler 由 core pkg/proclog 接到 slog);
// 这里只保留"panic 转 error 回传调用方"的 defer 辅助。
package safe

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// RecoverToErr 用于 defer recover,将 panic 转为 error 写回调用方变量。
// 适用于需要把 panic 信息传递给调用方的场景(如并行 DB 查询)。
//
//nolint:gocritic // errPtr 必须为指针以写回调用方 error
func RecoverToErr(name string, errPtr *error) {
	if r := recover(); r != nil {
		*errPtr = fmt.Errorf("goroutine %s panic: %v", name, r)
		slog.Error("goroutine panic recovered",
			"goroutine", name,
			"panic", fmt.Sprintf("%v", r),
			"stack", string(debug.Stack()),
		)
	}
}
