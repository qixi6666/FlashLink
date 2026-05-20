package link

import (
	"hash/fnv"
	"strings"
)

const checksumSalt = "flashlink:v1"

func NewShortCode(id uint64) string {
	body := EncodeBase62(id)
	return body + string(checksumFor(body))
}

func ValidateShortCode(code string) error {
	body, checksum, ok := splitCode(code)
	if !ok {
		return ErrInvalidCode
	}
	if _, err := DecodeBase62(body); err != nil {
		return ErrInvalidCode
	}
	if checksumFor(body) != checksum {
		return ErrInvalidCode
	}
	return nil
}

func IDFromShortCode(code string) (uint64, error) {
	body, _, ok := splitCode(code)
	if !ok {
		return 0, ErrInvalidCode
	}
	if checksumFor(body) != code[len(code)-1] {
		return 0, ErrInvalidCode
	}

	id, err := DecodeBase62(body)
	if err != nil {
		return 0, ErrInvalidCode
	}
	return id, nil
}

func splitCode(code string) (string, byte, bool) {
	code = strings.TrimSpace(code)
	if len(code) < 2 {
		return "", 0, false
	}
	return code[:len(code)-1], code[len(code)-1], true
}

func checksumFor(body string) byte {
	h := fnv.New32a()
	_, _ = h.Write([]byte(checksumSalt))
	_, _ = h.Write([]byte(body))
	return base62Alphabet[h.Sum32()%62]
}
