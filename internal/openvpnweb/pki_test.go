package openvpnweb

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
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
