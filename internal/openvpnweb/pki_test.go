package openvpnweb

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readTestCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read certificate %q: %v", path, err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatalf("decode certificate PEM %q", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse certificate %q: %v", path, err)
	}
	return cert
}

func TestGenerateClientConfigGoReplacesRevokedCertificateWhenNameIsReused(t *testing.T) {
	previousOVData := ovData
	ovData = t.TempDir()
	t.Cleanup(func() { ovData = previousOVData })

	const name = "reused-client"
	if err := generateClientConfigGo(name, "198.51.100.10", "1194", "udp", false, "", false); err != nil {
		t.Fatalf("generate initial client configuration: %v", err)
	}
	oldCert := readTestCertificate(t, clientCertPath(name))

	if err := RevokeByName(name); err != nil {
		t.Fatalf("revoke initial certificate: %v", err)
	}
	if err := os.Remove(filepath.Join(ovData, "clients", name+".ovpn")); err != nil {
		t.Fatalf("remove old client configuration: %v", err)
	}

	if err := generateClientConfigGo(name, "198.51.100.10", "1194", "udp", false, "", false); err != nil {
		t.Fatalf("generate replacement client configuration: %v", err)
	}
	newCert := readTestCertificate(t, clientCertPath(name))

	if oldCert.SerialNumber.Cmp(newCert.SerialNumber) == 0 {
		t.Fatal("replacement client certificate reused the revoked serial number")
	}
	oldRevoked, err := isCertificateRevoked(oldCert)
	if err != nil {
		t.Fatalf("check old certificate revocation status: %v", err)
	}
	if !oldRevoked {
		t.Fatal("old certificate is not present in the revocation list")
	}
	newRevoked, err := isCertificateRevoked(newCert)
	if err != nil {
		t.Fatalf("check new certificate revocation status: %v", err)
	}
	if newRevoked {
		t.Fatal("replacement client certificate is unexpectedly revoked")
	}
}

func TestNormalizeClientTopology(t *testing.T) {
	profile := "client\r\nproto udp\r\nremote vpn.example 1194\r\ndev tun\r\ntopology net30\r\ntopology net30\r\nverb 3\r\n"
	normalized := normalizeClientTopology(profile)
	if strings.Count(normalized, "topology ") != 1 {
		t.Fatalf("expected exactly one topology directive, got %q", normalized)
	}
	if !strings.Contains(normalized, "topology subnet") || strings.Contains(normalized, "topology net30") {
		t.Fatalf("profile was not normalized to subnet topology: %q", normalized)
	}
}

func TestNormalizeClientTopologyAddsDirectiveAfterTun(t *testing.T) {
	normalized := normalizeClientTopology("client\nproto udp\ndev tun\nverb 3\n")
	if !strings.Contains(normalized, "dev tun\ntopology subnet\n") {
		t.Fatalf("topology directive was not inserted after dev tun: %q", normalized)
	}
}
