package link

import "errors"

var (
	ErrInvalidCode = errors.New("invalid short code")
	ErrInvalidURL  = errors.New("invalid long url")
	ErrExpired     = errors.New("short link expired")
	ErrNotFound    = errors.New("short link not found")
	ErrQueueFull   = errors.New("visit log queue is full")
)
