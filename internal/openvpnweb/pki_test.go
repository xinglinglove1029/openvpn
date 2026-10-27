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
	normalized := normalizeClientProfile(profile)
	if strings.Contains(normalized, "topology ") {
		t.Fatalf("topology must come from the server push, got %q", normalized)
	}
}

func TestNormalizeClientProfilePreservesCompatibleProfile(t *testing.T) {
	profile := "client\nproto udp\ndev tun\nverb 3\n"
	if got := normalizeClientProfile(profile); got != profile {
		t.Fatal("compatible profile was unexpectedly modified")
	}
}

func TestNormalizeClientProfilePreservesInlineSecretsAndCustomRoutes(t *testing.T) {
	kept := "client\r\n# topology explanation\r\n; persist-tun comment\r\nroute 172.16.0.0 255.240.0.0\r\nstatic-challenge \"Enter MFA code\" 1\r\n<key>\r\npersist-key\r\nopaque-secret\r\n</key>\r\n<tls-crypt>\r\nsecret-material\r\n</tls-crypt>\r\n"
	profile := "  topology subnet\r\nresolv-retry infinite\r\npersist-key\r\npersist-tun\r\nexplicit-exit-notify 1\r\n--topology subnet\r\n" + kept
	got := normalizeClientProfile(profile)
	if got != kept {
		t.Fatal("normalization did not remove only unsupported directives")
	}
	if normalizeClientProfile(got) != got {
		t.Fatal("normalization is not idempotent")
	}
}

func TestGenerateClientConfigGoConnectCompatibility(t *testing.T) {
	setupCertificateServiceTest(t)
	if err := os.WriteFile(filepath.Join(ovData, "server.conf"), []byte("auth-user-pass-verify /auth via-env\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, proto := range []string{"udp", "tcp"} {
		name := "connect-" + proto
		if err := generateClientConfigGo(name, "198.51.100.10", "1194", proto, false, "route 172.16.0.0 255.240.0.0\ntopology net30\npersist-tun", true); err != nil {
			t.Fatal(err)
		}
		profile, err := os.ReadFile(filepath.Join(ovData, "clients", name+".ovpn"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(profile), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "topology", "resolv-retry", "persist-key", "persist-tun", "explicit-exit-notify":
				t.Fatalf("Go generation still emits %s", fields[0])
			}
		}
		for _, required := range []string{"remote-cert-tls server", "verify-x509-name server name", "auth-user-pass", "static-challenge", "route 172.16.0.0 255.240.0.0", "<ca>", "<cert>", "<key>", "<tls-crypt>"} {
			if !strings.Contains(string(profile), required) {
				t.Fatalf("missing %s", required)
			}
		}
		originalCert := readTestCertificate(t, clientCertPath(name))
		oldProfile := string(profile) + "topology subnet\npersist-key\n"
		if err := os.WriteFile(filepath.Join(ovData, "clients", name+".ovpn"), []byte(oldProfile), 0600); err != nil {
			t.Fatal(err)
		}
		if err := generateClientConfigGo(name, "198.51.100.10", "1194", proto, false, "", true); err != nil {
			t.Fatal(err)
		}
		migrated, _ := os.ReadFile(filepath.Join(ovData, "clients", name+".ovpn"))
		if string(migrated) != string(profile) {
			t.Fatal("existing profile was not repaired in place")
		}
		if originalCert.SerialNumber.Cmp(readTestCertificate(t, clientCertPath(name)).SerialNumber) != 0 {
			t.Fatal("profile repair rotated the client certificate")
		}
	}
}
