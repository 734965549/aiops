package domain

import "errors"

var (
	ErrNotFound              = errors.New("not found")
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrAccountDisabled       = errors.New("account disabled")
	ErrCapabilityUnsupported = errors.New("capability unsupported")
	ErrUnsupportedProvider   = errors.New("unsupported provider")
	ErrProviderUnavailable   = errors.New("provider unavailable")
)
