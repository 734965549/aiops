package domain

import "errors"

var (
	ErrNotFound           = errors.New("execution task not found")
	ErrAlreadyExists      = errors.New("execution task already exists")
	ErrInvalidTransition  = errors.New("invalid execution status transition")
	ErrInvalidArgument    = errors.New("invalid execution argument")
	ErrFailedPrecondition = errors.New("execution failed precondition")
)
