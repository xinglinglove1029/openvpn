package openvpnweb

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CertificateDeletionResult is the per-certificate outcome returned by the web and AI APIs.
// Only client certificates can ever be deleted; CA, server, CRL and other PKI files stay protected.
type CertificateDeletionResult struct {
	Name           string `json:"name"`
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	ReloadRequired bool   `json:"reloadRequired,omitempty"`
}

var certificateNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validateCertificateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !certificateNamePattern.MatchString(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid certificate name")
	}
	return name, nil
}

func certificateProtectionReason(name string, cert *x509.Certificate) string {
	serverName := viperGetString("system.base.server_name", "server")
	switch name {
	case "ca":
		return "CA certificate is protected and cannot be deleted"
	case "server":
		return "OpenVPN Server certificate is protected and cannot be deleted"
	case "crl":
		return "CRL is required for OpenVPN and cannot be deleted"
	}
	if name == serverName {
		return "OpenVPN Server certificate is protected and cannot be deleted"
	}
	if cert != nil {
		if cert.IsCA {
			return "CA certificate is protected and cannot be deleted"
		}
		for _, usage := range cert.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth {
				return "OpenVPN Server certificate is protected and cannot be deleted"
			}
		}
	}
	return ""
}

func parseCertificateFile(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func removeFileIfPresent(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("refusing to remove directory")
	}
	return os.Remove(path)
}

// cleanupClientArtifacts deletes only files that belong to a validated client name.  It uses
// os.Remove rather than RemoveAll so an unexpected directory cannot recursively delete data.
func cleanupClientArtifacts(name string) error {
	for _, path := range []string{
		clientCertPath(name),
		clientKeyPath(name),
		filepath.Join(ovData, "clients", name+".ovpn"),
		filepath.Join(ovData, "ccd", name),
		filepath.Join(pkiDir(), "reqs", name+".req"),
	} {
		if err := removeFileIfPresent(path); err != nil {
			return fmt.Errorf("remove client artifact %q: %w", path, err)
		}
	}
	return nil
}

const crlReloadPendingMessage = "client certificate revocation is recorded but has not been applied by the running OpenVPN service"

// reloadOpenVPNCRL applies the updated CRL to the running daemon. Callers must treat an
// error as an incomplete security operation: the revocation record is durable, but the
// current OpenVPN process can still be using its previously loaded CRL.
func reloadOpenVPNCRL(ov *ovpn) error {
	if ov == nil {
		return fmt.Errorf("%s: OpenVPN management client is not initialized", crlReloadPendingMessage)
	}
	if _, err := ov.sendCommand("signal SIGHUP"); err != nil {
		return fmt.Errorf("%s: %w", crlReloadPendingMessage, err)
	}
	return nil
}

// synchronizePendingCRL regenerates the CRL from revoked.json and then reloads
// OpenVPN. The durable marker makes a failed reload retryable even after the
// original certificate files have been removed.
func synchronizePendingCRL(ov *ovpn) error {
	if !hasCRLReloadPending() {
		return nil
	}
	if err := generateCRL(); err != nil {
		return fmt.Errorf("%s: regenerate CRL: %w", crlReloadPendingMessage, err)
	}
	if err := reloadOpenVPNCRL(ov); err != nil {
		return err
	}
	if err := clearCRLReloadPending(); err != nil {
		return fmt.Errorf("%s: clear pending reload marker: %w", crlReloadPendingMessage, err)
	}
	return nil
}

func markCertificateReloadPending(results []CertificateDeletionResult, reloadErr error) int {
	successCount := 0
	for i := range results {
		if results[i].ReloadRequired && results[i].Success {
			results[i].Success = false
			results[i].Message = fmt.Sprintf("%s; %v", results[i].Message, reloadErr)
		}
		if results[i].Success {
			successCount++
		}
	}
	return successCount
}

// DeleteClientCertificate revokes an active client certificate before removing its client-only
// artifacts. It intentionally never touches CA/server/CRL/tls-crypt or OpenVPN server config.
// The name is validated before any path is built, preventing path traversal from UI or AI calls.
func DeleteClientCertificate(rawName string) (CertificateDeletionResult, error) {
	result := CertificateDeletionResult{Name: strings.TrimSpace(rawName)}
	name, err := validateCertificateName(rawName)
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	result.Name = name

	if reason := certificateProtectionReason(name, nil); reason != "" {
		result.Message = reason
		return result, fmt.Errorf("%s", reason)
	}

	certPath := clientCertPath(name)
	cert, certErr := parseCertificateFile(certPath)
	if certErr != nil && !os.IsNotExist(certErr) {
		result.Message = fmt.Sprintf("read client certificate: %v", certErr)
		return result, certErr
	}

	if certErr == nil {
		if reason := certificateProtectionReason(name, cert); reason != "" {
			result.Message = reason
			return result, fmt.Errorf("%s", reason)
		}
		revoked, err := isCertificateRevoked(cert)
		if err != nil {
			result.Message = fmt.Sprintf("check revocation status: %v", err)
			return result, err
		}
		if !revoked {
			if err := RevokeByName(name); err != nil {
				// RevokeByName records a durable pending marker before rebuilding the
				// CRL. Preserve that fact so callers never report this partial state as
				// an ordinary failed delete and can retry the CRL reload safely.
				result.ReloadRequired = hasCRLReloadPending()
				result.Message = fmt.Sprintf("revoke client certificate: %v", err)
				return result, err
			}
			result.ReloadRequired = true
		}

		if err := cleanupClientArtifacts(name); err != nil {
			result.Message = fmt.Sprintf("remove client artifacts: %v", err)
			return result, err
		}

		result.Success = true
		if revoked {
			result.Message = "Removed revoked client certificate and related files"
		} else {
			result.Message = "Revoked and removed client certificate and related files"
		}
		return result, nil
	}

	// A historical user deletion can leave a key, .ovpn, CCD, or request after its
	// certificate was manually removed. No revocation is possible without a certificate,
	// but removing these strictly name-scoped artifacts is safe and prevents stale reuse.
	artifacts := []string{
		clientKeyPath(name),
		filepath.Join(ovData, "clients", name+".ovpn"),
		filepath.Join(ovData, "ccd", name),
		filepath.Join(pkiDir(), "reqs", name+".req"),
	}
	foundArtifact := false
	for _, path := range artifacts {
		if fileExists(path) {
			foundArtifact = true
			break
		}
	}
	if !foundArtifact {
		err := fmt.Errorf("client certificate %q and related artifacts do not exist", name)
		result.Message = err.Error()
		return result, err
	}
	if err := cleanupClientArtifacts(name); err != nil {
		result.Message = fmt.Sprintf("remove orphaned client artifacts: %v", err)
		return result, err
	}
	result.Success = true
	result.Message = "Removed orphaned client certificate artifacts"
	return result, nil
}

// DeleteClientCertificates processes each requested client certificate independently so a
// protected/system certificate cannot block auditing of the remaining requested targets.
func DeleteClientCertificates(names []string) []CertificateDeletionResult {
	results := make([]CertificateDeletionResult, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		normalized := strings.TrimSpace(rawName)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result, _ := DeleteClientCertificate(normalized)
		results = append(results, result)
	}
	return results
}
