package errors

import stderrors "errors"

// Sentinel 将领域哨兵错误映射到平台错误码。
type Sentinel struct {
	Err  error
	Code Code
}

// MapSentinels 按顺序匹配哨兵错误并转换为 *Error；未匹配则 Wrap 为 INTERNAL。
func MapSentinels(err error, internalMessage string, sentinels ...Sentinel) error {
	if err == nil {
		return nil
	}
	for _, s := range sentinels {
		if s.Err != nil && stderrors.Is(err, s.Err) {
			return New(s.Code, err.Error())
		}
	}
	msg := internalMessage
	if msg == "" {
		msg = "operation failed"
	}
	return Wrap(err, CodeInternal, msg)
}
