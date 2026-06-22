package link

import (
	"strings"
)

func NewShortCode(id uint64) string {
	return EncodeBase62(id)
}

func ValidateShortCode(code string) error {
	_, err := IDFromShortCode(code)
	return err
}

func IDFromShortCode(code string) (uint64, error) {
	id, err := DecodeBase62(strings.TrimSpace(code))
	if err != nil {
		return 0, ErrInvalidCode
	}
	return id, nil
}
