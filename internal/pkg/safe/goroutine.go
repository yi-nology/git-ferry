package safe

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Go 启动一个带 panic recovery 的 goroutine。
// 如果 goroutine 发生 panic,会记录错误日志并调用可选的 onError 回调,
// 而不是让整个进程崩溃。Hertz 的 recovery.Recovery() middleware
// 只保护 handler 主 goroutine,子 goroutine 需要自行 recover。
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered",
					"goroutine", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}

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
