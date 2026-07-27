package openvpnweb

import (
	"github.com/pquerna/otp/totp"
)

// GenMfa 生成 TOTP 密钥和 otpauth URL
// 返回值: secret（base32 密钥）、url（otpauth:// URI，用于生成二维码）、error
func GenMfa(user string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "openvpn-web",
		AccountName: user,
	})
	if err != nil {
		return "", "", err
	}

	return key.Secret(), key.URL(), nil
}

func ValidateMfa(passcode, key string) bool {
	return totp.Validate(passcode, key)
}
