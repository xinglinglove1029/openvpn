//go:build !linux

package openvpnweb

import (
	"context"
	"fmt"
	"net"
)

// Domain interception depends on Linux netfilter and a tun0-bound socket. On
// other platforms it intentionally stays unavailable so enabling auditing can
// never expose port 5353 or change host traffic.
func listenWebAuditUDP(context.Context, string, string) (*net.UDPConn, error) {
	return nil, fmt.Errorf("网站域名审计仅支持 Linux tun0 环境")
}

func listenWebAuditTCP(context.Context, string, string) (net.Listener, error) {
	return nil, fmt.Errorf("网站域名审计仅支持 Linux tun0 环境")
}
