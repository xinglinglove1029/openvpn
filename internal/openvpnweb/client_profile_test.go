package openvpnweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDownloadClientProfileMigratesOldFilesAndPreservesCredentials(t *testing.T) {
	prev := ovData
	ovData = t.TempDir()
	t.Cleanup(func() { ovData = prev })
	dir := filepath.Join(ovData, "clients")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	kept := "client\nremote 198.51.100.10 1194\nauth-user-pass\nstatic-challenge \"Enter MFA code\" 1\n<cert>\ncert-bytes\n</cert>\n<key>\nkey-bytes\n</key>\n<tls-crypt>\nshared-key-bytes\n</tls-crypt>\n"
	old := "topology subnet\nresolv-retry infinite\npersist-key\npersist-tun\nexplicit-exit-notify\n" + kept
	if err := os.WriteFile(filepath.Join(dir, "alice.ovpn"), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("isAdmin", true) })
	r.GET("/ovpn/download/*filepath", downloadClientProfile)
	for range 2 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ovpn/download/alice.ovpn", nil))
		if w.Code != http.StatusOK || w.Body.String() != kept {
			t.Fatalf("download did not return compatible credentials: status=%d", w.Code)
		}
		if !strings.Contains(w.Header().Get("Content-Disposition"), "alice.ovpn") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing attachment or cache headers")
		}
	}
	persisted, err := os.ReadFile(filepath.Join(dir, "alice.ovpn"))
	if err != nil {
		t.Fatal(err)
	}
	if string(persisted) != kept {
		t.Fatal("old on-disk file was not migrated")
	}
	for _, path := range []string{"/ovpn/download/", "/ovpn/download/../config.json", "/ovpn/download/alice.txt"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code == http.StatusOK {
			t.Fatalf("unexpected file served at %s", path)
		}
	}
}
