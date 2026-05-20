package link

import (
	"errors"
	"strings"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var errInvalidBase62 = errors.New("invalid base62 value")

func EncodeBase62(n uint64) string {
	if n == 0 {
		return "0"
	}

	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = base62Alphabet[n%62]
		n /= 62
	}
	return string(buf[i:])
}

func DecodeBase62(s string) (uint64, error) {
	if s == "" {
		return 0, errInvalidBase62
	}

	var n uint64
	for _, r := range s {
		idx := strings.IndexRune(base62Alphabet, r)
		if idx < 0 {
			return 0, errInvalidBase62
		}
		n = n*62 + uint64(idx)
	}
	return n, nil
}
