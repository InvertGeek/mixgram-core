package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

// RandomString 生成指定长度、指定字符集的随机字符串
func RandomString(length int, charset string) string {
	if len(charset) == 0 {
		return ""
	}

	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return ""
		}
		result[i] = charset[n.Int64()]
	}

	return string(result)
}

func RandomHexString(length int) string {
	rBytes := make([]byte, length/2)
	rand.Read(rBytes)
	return hex.EncodeToString(rBytes)
}

// RandomStringDefault 使用默认字符集（数字+大小写字母）
func RandomStringDefault(length int) string {
	const defaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	return RandomString(length, defaultCharset)
}
