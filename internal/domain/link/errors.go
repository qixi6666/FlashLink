package link

import "errors"

var (
	ErrInvalidCode = errors.New("invalid short code")
	ErrExpired     = errors.New("short link expired")
	ErrNotFound    = errors.New("short link not found")
)
