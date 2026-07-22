package openvpnweb

import "time"

func setLoginFail(ip string) int {
	if ip == "" {
		return 0
	}

	key := "lf:" + ip
	if v, ok := cc.Get(key); ok {
		if n, ok := v.(int); ok {
			n++
			cc.Set(key, n, 30*time.Minute)
			return n
		}
	}

	cc.Set(key, 1, 30*time.Minute)

	return 1
}

func resetLoginFail(ip string) {
	if ip == "" {
		return
	}

	cc.Delete("lf:" + ip)
}
