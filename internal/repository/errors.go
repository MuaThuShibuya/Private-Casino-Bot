package repository

import "errors"

var (
	ErrInsufficientBalance  = errors.New("insufficient balance")
	ErrUserNotFound         = errors.New("user not found")
	ErrSelfTransfer         = errors.New("cannot transfer to self")
	ErrInvalidAmount        = errors.New("amount must be greater than zero")
	ErrRoundAlreadyResolved = errors.New("round already resolved")
)
