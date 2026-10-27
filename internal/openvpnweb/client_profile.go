package openvpnweb

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Every download path shares this migration, so old files and files edited by
// an older client cannot reintroduce unsupported options on the next download.
func readCompatibleClientProfile(root *os.Root, filename string) ([]byte, error) {
	// Account creation historically persisted either a client CN or its full
	// filename. Both refer to the same managed profile on disk.
	if !strings.HasSuffix(filename, ".ovpn") {
		filename += ".ovpn"
	}
	data, err := root.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	compatible := normalizeClientProfile(string(data))
	if compatible != string(data) {
		if err := root.WriteFile(filename, []byte(compatible), 0600); err != nil {
			return nil, fmt.Errorf("migrate client profile: %w", err)
		}
	}
	return []byte(compatible), nil
}

func downloadClientProfile(c *gin.Context) {
	filename := strings.TrimPrefix(c.Param("filepath"), "/")
	if filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\") || !strings.HasSuffix(filename, ".ovpn") {
		c.Status(http.StatusNotFound)
		return
	}
	isAdmin, _ := c.Get("isAdmin")
	username, _ := c.Get("user")
	user, _ := username.(string)
	if isAdmin != true && !CanAccessClientConfig(user, strings.TrimSuffix(filename, ".ovpn")) {
		c.Status(http.StatusForbidden)
		return
	}
	root, err := os.OpenRoot(filepath.Join(ovData, "clients"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer root.Close()
	data, err := readCompatibleClientProfile(root, filename)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Data(http.StatusOK, "application/x-openvpn-profile", data)
}
