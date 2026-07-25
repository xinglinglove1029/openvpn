package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// hmacSHA256Hex 计算 HMAC-SHA256 摘要并以 hex 返回
func hmacSHA256Hex(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
