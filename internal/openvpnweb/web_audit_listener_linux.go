//go:build linux

package openvpnweb

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// webAuditListenConfig binds the DNS proxy socket to tun0 itself. This keeps
// port 5353 off host, Docker bridge, and management interfaces without adding
// a firewall rule that would match any non-tunnel interface.
func webAuditListenConfig() net.ListenConfig {
	return net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var bindErr error
		if err := raw.Control(func(fd uintptr) {
			bindErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, "tun0")
		}); err != nil {
			return fmt.Errorf("访问 DNS 监听套接字失败: %w", err)
		}
		if bindErr != nil {
			return fmt.Errorf("将 DNS 监听绑定到 tun0 失败: %w", bindErr)
		}
		return nil
	}}
}

func listenWebAuditUDP(ctx context.Context, network, address string) (*net.UDPConn, error) {
	config := webAuditListenConfig()
	packet, err := config.ListenPacket(ctx, network, address)
	if err != nil {
		return nil, err
	}
	udp, ok := packet.(*net.UDPConn)
	if !ok {
		_ = packet.Close()
		return nil, fmt.Errorf("DNS %s 监听未返回 UDP 套接字", network)
	}
	return udp, nil
}

func listenWebAuditTCP(ctx context.Context, network, address string) (net.Listener, error) {
	config := webAuditListenConfig()
	return config.Listen(ctx, network, address)
}
