package domain

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrInvalidProvider     = errors.New("invalid provider")
	ErrInvalidAuthType     = errors.New("invalid auth type")
	ErrInvalidCapability   = errors.New("invalid capability")
	ErrCredentialRequired  = errors.New("credential required")
	ErrAccountDisabled     = errors.New("account disabled")
	ErrConnectivityFailed  = errors.New("connectivity check failed")
	ErrUnsupportedProvider = errors.New("unsupported provider")
)
