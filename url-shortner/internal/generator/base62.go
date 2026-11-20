package generator

import (
	"errors"
	"strings"
)

const (
	alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base     = uint64(len(alphabet)) // 62
)

var (
	ErrEmptyString     = errors.New("cannot decode empty string")
	ErrInvalidCharacter = errors.New("string contains character outside Base62 alphabet")
)

// Encode converts a 64-bit unsigned integer into a Base62 string.
func Encode(num uint64) string {
	if num == 0 {
		return string(alphabet[0])
	}

	var sb strings.Builder
	for num > 0 {
		rem := num % base
		sb.WriteByte(alphabet[rem])
		num /= base
	}

	// Reverse the string builder content so most significant digit comes first
	runes := []byte(sb.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

// Decode converts a Base62 string back into a 64-bit unsigned integer.
func Decode(str string) (uint64, error) {
	if len(str) == 0 {
		return 0, ErrEmptyString
	}

	var num uint64
	for i := 0; i < len(str); i++ {
		char := str[i]
		idx := strings.IndexByte(alphabet, char)
		if idx == -1 {
			return 0, ErrInvalidCharacter
		}
		num = num*base + uint64(idx)
	}

	return num, nil
}
