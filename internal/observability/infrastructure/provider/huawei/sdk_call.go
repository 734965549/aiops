package huawei

import "context"

// callWithContext 执行不接收 ctx 的华为 SDK 同步调用：
// ctx 超时/取消立即返回，不等 SDK 自身 HTTP 超时；结果通道带缓冲，被放弃的调用完成后 goroutine 可退出。
func callWithContext[T any](ctx context.Context, call func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := call()
		done <- result{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-done:
		return r.value, r.err
	}
}
