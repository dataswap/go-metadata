package utils

import (
	"encoding/hex"
	"strings"
)

// ConvertToHexPrefix converts a byte slice to a hexadecimal string with "0x" prefix.
func ConvertToHexPrefix(hexBytes []byte) string {
	return "0x" + hex.EncodeToString(hexBytes)
}

// ParseHexWithPrefix parses a hexadecimal string with "0x" prefix and returns the corresponding byte slice.
func ParseHexWithPrefix(hexStr string) ([]byte, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
