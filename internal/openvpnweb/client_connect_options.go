package openvpnweb

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// clientConnectOptionsDir keeps options produced by successful authentication.
// The directory is keyed by the authenticated identity rather than by a global
// file, because OpenVPN may authenticate multiple clients concurrently.
const clientConnectOptionsDir = ".client-connect-options"

func clientConnectOptionsPath(dataDir, username, commonName string) string {
	sum := sha256.Sum256([]byte(username + "\x00" + commonName))
	return filepath.Join(dataDir, clientConnectOptionsDir, fmt.Sprintf("%x.conf", sum))
}

// writeClientConnectOptions atomically publishes connection options for one
// authenticated identity. The entrypoint consumes the same identity-keyed file
// from its client-connect hook, so concurrent logins cannot overwrite each
// other's fixed IP or group configuration.
func writeClientConnectOptions(username, commonName, ipAddr, config string) error {
	dataDir := strings.TrimSpace(ovData)
	if dataDir == "" {
		return fmt.Errorf("OpenVPN data directory is not configured")
	}

	var lines []string
	if ip := strings.TrimSpace(ipAddr); ip != "" {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			return fmt.Errorf("invalid fixed IPv4 address %q", ip)
		}
		lines = append(lines, fmt.Sprintf("ifconfig-push %s 255.255.255.0", parsed.String()))
	}
	if options := strings.TrimSpace(config); options != "" {
		lines = append(lines, options)
	}

	dir := filepath.Dir(clientConnectOptionsPath(dataDir, username, commonName))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create client connection options directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure client connection options directory: %w", err)
	}

	target := clientConnectOptionsPath(dataDir, username, commonName)
	temp, err := os.CreateTemp(dir, ".pending-")
	if err != nil {
		return fmt.Errorf("create client connection options: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("secure client connection options: %w", err)
	}
	contents := ""
	if len(lines) > 0 {
		contents = strings.Join(lines, "\n") + "\n"
	}
	if _, err := temp.WriteString(contents); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write client connection options: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close client connection options: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("publish client connection options: %w", err)
	}
	return nil
}
