package openvpnweb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func setupOpenVPNRepairTest(t *testing.T) {
	t.Helper()
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	viper.Set("system.base.server_name", "server")
	viper.Set("openvpn.ovpn_port", 1194)
	viper.Set("openvpn.ovpn_proto", "udp")
	viper.Set("openvpn.ovpn_subnet", "10.8.0.0/24")
	viper.Set("openvpn.ovpn_management", "127.0.0.1:7505")
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})
}

func TestSetManagedDirectiveReplacesOnlyActiveDirectives(t *testing.T) {
	lines := []string{
		"# port 1194",
		"; Port 1194",
		"  PORT 1194",
		"port 443",
		"port-share 127.0.0.1 8443",
		"proto udp",
	}

	got := setManagedDirective(lines, "port", "1194")
	want := []string{
		"# port 1194",
		"; Port 1194",
		"port 1194",
		"port-share 127.0.0.1 8443",
		"proto udp",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("setManagedDirective() = %q, want %q", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	activePorts := 0
	for _, line := range got {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "port ") {
			activePorts++
		}
	}
	if activePorts != 1 {
		t.Fatalf("active port directive count = %d, want 1", activePorts)
	}
}

func TestRepairCRLReferenceRefusesWhenManagedPKIIsMissing(t *testing.T) {
	setupOpenVPNRepairTest(t)

	configPath := openVPNServerConfigPath()
	original := []byte("port 1194\nproto udp\nserver 10.8.0.0 255.255.255.0\ncrl-verify /tmp/stale-crl.pem\n")
	if err := os.WriteFile(configPath, original, 0600); err != nil {
		t.Fatalf("write server.conf: %v", err)
	}

	result, err := (&ovpn{}).RepairOpenVPNServer(serverRepairCRLReference)
	if err == nil {
		t.Fatalf("RepairOpenVPNServer unexpectedly succeeded: %#v", result)
	}
	if !strings.Contains(err.Error(), "required PKI file is missing") {
		t.Fatalf("unexpected repair error: %v", err)
	}
	if result.BackupPath != "" || result.Reloaded || result.Success {
		t.Fatalf("repair result claims a change despite missing PKI: %#v", result)
	}

	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read server.conf after rejected repair: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("server.conf changed despite rejected repair:\n got %q\nwant %q", got, original)
	}
	if _, statErr := os.Stat(filepath.Join(ovData, "backups")); !os.IsNotExist(statErr) {
		t.Fatalf("backup directory was created despite rejected repair: %v", statErr)
	}
}

func TestRepairCRLReferenceDoesNotTouchConfigWhenAnotherCriticalIssueExists(t *testing.T) {
	setupOpenVPNRepairTest(t)
	if err := initPKI(); err != nil {
		t.Fatalf("initialize managed PKI: %v", err)
	}

	configPath := openVPNServerConfigPath()
	original := []byte("port invalid\nproto udp\nserver 10.8.0.0 255.255.255.0\nca " + caCertPath() + "\ncert " + managedServerCertPath() + "\nkey " + managedServerKeyPath() + "\ncrl-verify /tmp/stale-crl.pem\ntls-crypt " + tcKeyPath() + "\nmanagement 127.0.0.1 7505\n")
	if err := os.WriteFile(configPath, original, 0600); err != nil {
		t.Fatalf("write server.conf: %v", err)
	}

	result, err := (&ovpn{address: "127.0.0.1:1"}).RepairOpenVPNServer(serverRepairCRLReference)
	if err == nil || !strings.Contains(err.Error(), "unrelated critical") {
		t.Fatalf("repair error = %v, want unrelated critical refusal; result=%#v", err, result)
	}
	if result.BackupPath != "" || result.Reloaded || result.Success {
		t.Fatalf("repair result claims a change despite unrelated critical issue: %#v", result)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read server.conf: %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("server.conf changed despite blocked repair:\n got %q\nwant %q", got, original)
	}
	if _, statErr := os.Stat(filepath.Join(ovData, "backups")); !os.IsNotExist(statErr) {
		t.Fatalf("backup directory was created despite blocked repair: %v", statErr)
	}
}

func TestHasBlockingCriticalIssueRequiresExactRepairAction(t *testing.T) {
	diagnosis := OpenVPNServerDiagnosis{Issues: []OpenVPNServerIssue{
		{Code: "invalid_crl_verify_path", Severity: "critical", RepairAction: serverRepairCRLReference},
		{Code: "invalid_port", Severity: "critical"},
	}}
	if !hasBlockingCriticalIssue(diagnosis, serverRepairCRLReference) {
		t.Fatal("unassigned critical issue must block constrained CRL repair")
	}
	if hasBlockingCriticalIssue(OpenVPNServerDiagnosis{Issues: diagnosis.Issues[:1]}, serverRepairCRLReference) {
		t.Fatal("matching critical issue should be repairable by its assigned action")
	}
}
