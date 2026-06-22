package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrPolicyDisabled    = errors.New("policy disabled")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrRunNotFinished    = errors.New("run not finished")
	ErrScopeIncomplete   = errors.New("scope incomplete")
	ErrUnsupportedCheck  = errors.New("unsupported inspection check")
)
