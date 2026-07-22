package main

import "openvpn-web/internal/openvpnweb"

var (
	version = "1.0.0"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "source"
)

func main() {
	openvpnweb.Run(openvpnweb.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		BuiltBy: builtBy,
	})
}
