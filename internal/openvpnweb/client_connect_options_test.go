package openvpnweb

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestWriteClientConnectOptionsKeepsConcurrentIdentitiesIsolated(t *testing.T) {
	originalDataDir := ovData
	ovData = t.TempDir()
	t.Cleanup(func() { ovData = originalDataDir })

	identities := []struct {
		username   string
		commonName string
		ip         string
		config     string
	}{
		{username: "nas", commonName: "nas-client", ip: "10.8.0.2", config: "push \"route 10.10.0.0 255.255.0.0\""},
		{username: "luxin", commonName: "luxin-client", ip: "10.8.0.3", config: "push \"route 10.20.0.0 255.255.0.0\""},
	}

	var wg sync.WaitGroup
	for _, identity := range identities {
		identity := identity
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := writeClientConnectOptions(identity.username, identity.commonName, identity.ip, identity.config); err != nil {
				t.Errorf("write %s options: %v", identity.username, err)
			}
		}()
	}
	wg.Wait()

	for _, identity := range identities {
		contents, err := os.ReadFile(clientConnectOptionsPath(ovData, identity.username, identity.commonName))
		if err != nil {
			t.Fatalf("read %s options: %v", identity.username, err)
		}
		text := string(contents)
		if !strings.Contains(text, "ifconfig-push "+identity.ip+" 255.255.255.0") || !strings.Contains(text, identity.config) {
			t.Fatalf("unexpected %s options: %q", identity.username, text)
		}
		for _, other := range identities {
			if other.username != identity.username && strings.Contains(text, other.ip) {
				t.Fatalf("%s options leaked %s address: %q", identity.username, other.username, text)
			}
		}
	}
}

func TestWriteClientConnectOptionsRejectsInvalidFixedIP(t *testing.T) {
	originalDataDir := ovData
	ovData = t.TempDir()
	t.Cleanup(func() { ovData = originalDataDir })

	if err := writeClientConnectOptions("alice", "alice-client", "not-an-ip", ""); err == nil {
		t.Fatal("expected invalid IP to be rejected")
	}
}
