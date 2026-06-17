package domain

import "errors"

var (
	ErrNotFound        = errors.New("runbook not found")
	ErrAlreadyExists   = errors.New("runbook already exists")
	ErrInvalidArgument = errors.New("invalid runbook argument")
	ErrStepNotFound    = errors.New("runbook step not found")
)
