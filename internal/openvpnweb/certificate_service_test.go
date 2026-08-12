package openvpnweb

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func setupCertificateServiceTest(t *testing.T) {
	t.Helper()
	previousOVData := ovData
	ovData = t.TempDir()
	viper.Reset()
	viper.Set("system.base.server_name", "server")
	viper.Set("system.base.server_cn", "test-openvpn-ca")
	t.Cleanup(func() {
		ovData = previousOVData
		viper.Reset()
	})

	if err := initPKI(); err != nil {
		t.Fatalf("initialize test PKI: %v", err)
	}
}

func writeClientArtifact(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create artifact directory for %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("test artifact"), 0600); err != nil {
		t.Fatalf("write artifact %q: %v", path, err)
	}
}

func requirePathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("artifact %q still exists or could not be checked: %v", path, err)
	}
}

func startManagementTestServer(t *testing.T, response string) *ovpn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for management test server: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 128)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte(response))
	}()
	return &ovpn{address: listener.Addr().String()}
}

func TestSynchronizePendingCRLRetriesAfterCertificateArtifactsAreGone(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "retry-pending-crl"
	if err := generateClientCert(name); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}
	result, err := DeleteClientCertificate(name)
	if err != nil || !result.Success || !result.ReloadRequired {
		t.Fatalf("delete client certificate = %#v, %v", result, err)
	}
	if !hasCRLReloadPending() {
		t.Fatal("expected durable CRL reload marker after revocation")
	}
	if err := synchronizePendingCRL(startManagementTestServer(t, "ERROR: signal rejected\r\n")); err == nil {
		t.Fatal("pending CRL reload unexpectedly succeeded after management rejection")
	}
	if !hasCRLReloadPending() {
		t.Fatal("management rejection cleared the pending CRL reload marker")
	}
	if err := synchronizePendingCRL(startManagementTestServer(t, "SUCCESS: signal\r\n")); err != nil {
		t.Fatalf("retry pending CRL reload: %v", err)
	}
	if hasCRLReloadPending() {
		t.Fatal("pending CRL reload marker was not cleared after successful retry")
	}
}

func TestRevokeByNameKeepsPendingMarkerWhenCRLGenerationFails(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "retry-after-crl-generation-failure"
	if err := generateClientCert(name); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}
	cert := readTestCertificate(t, clientCertPath(name))
	backupPath := caKeyPath() + ".backup"
	if err := os.Rename(caKeyPath(), backupPath); err != nil {
		t.Fatalf("temporarily hide CA key: %v", err)
	}
	if err := RevokeByName(name); err == nil {
		t.Fatal("RevokeByName unexpectedly succeeded without the CA key")
	}
	if !hasCRLReloadPending() {
		t.Fatal("CRL generation failure did not leave a retry marker")
	}
	if err := os.Rename(backupPath, caKeyPath()); err != nil {
		t.Fatalf("restore CA key: %v", err)
	}
	if err := synchronizePendingCRL(startManagementTestServer(t, "SUCCESS: signal\r\n")); err != nil {
		t.Fatalf("recover pending CRL reload: %v", err)
	}
	if hasCRLReloadPending() {
		t.Fatal("pending CRL marker remains after recovery")
	}
	if revoked, err := isCertificateRevoked(cert); err != nil || !revoked {
		t.Fatalf("revocation record did not survive failed CRL generation: revoked=%v err=%v", revoked, err)
	}
}

func TestReloadOpenVPNCRLRejectsManagementErrorResponse(t *testing.T) {
	err := reloadOpenVPNCRL(startManagementTestServer(t, "ERROR: signal rejected\r\n"))
	if err == nil || !strings.Contains(err.Error(), "command rejected") {
		t.Fatalf("reloadOpenVPNCRL management rejection = %v, want explicit error", err)
	}
}

func TestCertificateListProtectsIntermediateCA(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "intermediate-ca"
	data, err := os.ReadFile(caCertPath())
	if err != nil {
		t.Fatalf("read CA certificate: %v", err)
	}
	if err := os.WriteFile(clientCertPath(name), data, 0600); err != nil {
		t.Fatalf("write intermediate CA certificate: %v", err)
	}

	certificates := getCerts(ovData)
	var found *CertData
	for i := range certificates {
		cert := certificates[i]
		if cert.Name == name {
			found = &cert
			break
		}
	}
	if found == nil {
		t.Fatalf("intermediate CA %q is missing from certificate list", name)
	}
	if found.Kind != "ca" || found.Deletable || !strings.Contains(found.ProtectedReason, "CA certificate") {
		t.Fatalf("intermediate CA should be protected in UI data: %#v", found)
	}
}

func TestDeleteClientCertificateRejectsSystemCertificatesAndPathTraversal(t *testing.T) {
	setupCertificateServiceTest(t)

	protectedFiles := []string{caCertPath(), serverCertPath(), serverKeyPath(), crlPath(), tcKeyPath()}
	before := make(map[string][]byte, len(protectedFiles))
	for _, path := range protectedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read protected file %q: %v", path, err)
		}
		before[path] = data
	}

	for _, name := range []string{"ca", "server", "crl", "../server", `..\\server`} {
		result, err := DeleteClientCertificate(name)
		if err == nil {
			t.Fatalf("DeleteClientCertificate(%q) unexpectedly succeeded: %#v", name, result)
		}
		if result.Success {
			t.Fatalf("DeleteClientCertificate(%q) reported success: %#v", name, result)
		}
	}

	for _, path := range protectedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("protected file %q was removed: %v", path, err)
		}
		if string(data) != string(before[path]) {
			t.Fatalf("protected file %q was modified", path)
		}
	}
}

func TestDeleteClientCertificateCleansArtifactsForAlreadyRevokedCertificate(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "revoked-client"
	if err := generateClientCert(name); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}
	cert := readTestCertificate(t, clientCertPath(name))
	if err := RevokeByName(name); err != nil {
		t.Fatalf("revoke client certificate: %v", err)
	}
	if revoked, err := isCertificateRevoked(cert); err != nil || !revoked {
		t.Fatalf("client certificate was not recorded as revoked: revoked=%v err=%v", revoked, err)
	}

	artifacts := []string{
		clientCertPath(name),
		clientKeyPath(name),
		filepath.Join(ovData, "clients", name+".ovpn"),
		filepath.Join(ovData, "ccd", name),
		filepath.Join(pkiDir(), "reqs", name+".req"),
	}
	for _, path := range artifacts[2:] {
		writeClientArtifact(t, path)
	}

	result, err := DeleteClientCertificate(name)
	if err != nil {
		t.Fatalf("delete already revoked certificate: %v", err)
	}
	if !result.Success || result.ReloadRequired {
		t.Fatalf("unexpected deletion result: %#v", result)
	}
	if !strings.Contains(result.Message, "Removed revoked") {
		t.Fatalf("unexpected deletion message: %q", result.Message)
	}
	for _, path := range artifacts {
		requirePathAbsent(t, path)
	}
	if revoked, err := isCertificateRevoked(cert); err != nil || !revoked {
		t.Fatalf("deletion removed revocation record: revoked=%v err=%v", revoked, err)
	}
}

func TestDeleteClientCertificatesContinuesAfterProtectedCertificate(t *testing.T) {
	setupCertificateServiceTest(t)

	const clientName = "batch-client"
	if err := generateClientCert(clientName); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}
	writeClientArtifact(t, filepath.Join(ovData, "clients", clientName+".ovpn"))

	results := DeleteClientCertificates([]string{"server", clientName})
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2: %#v", len(results), results)
	}
	if results[0].Name != "server" || results[0].Success {
		t.Fatalf("protected server result is incorrect: %#v", results[0])
	}
	if !strings.Contains(results[0].Message, "protected") {
		t.Fatalf("protected server result does not explain protection: %#v", results[0])
	}
	if results[1].Name != clientName || !results[1].Success || !results[1].ReloadRequired {
		t.Fatalf("client result is incorrect: %#v", results[1])
	}
	requirePathAbsent(t, clientCertPath(clientName))
	requirePathAbsent(t, clientKeyPath(clientName))
	requirePathAbsent(t, filepath.Join(ovData, "clients", clientName+".ovpn"))
	if !fileExists(serverCertPath()) || !fileExists(serverKeyPath()) {
		t.Fatal("protected OpenVPN server credentials were changed by batch deletion")
	}
}

func TestDeleteClientCertificateRejectsNonDefaultServerAuthCertificate(t *testing.T) {
	setupCertificateServiceTest(t)

	const alias = "alternate-server-certificate"
	for _, pair := range [][2]string{
		{serverCertPath(), clientCertPath(alias)},
		{serverKeyPath(), clientKeyPath(alias)},
	} {
		data, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatalf("read source server artifact %q: %v", pair[0], err)
		}
		if err := os.WriteFile(pair[1], data, 0600); err != nil {
			t.Fatalf("write alternate server artifact %q: %v", pair[1], err)
		}
	}
	writeClientArtifact(t, filepath.Join(ovData, "clients", alias+".ovpn"))

	result, err := DeleteClientCertificate(alias)
	if err == nil || result.Success {
		t.Fatalf("DeleteClientCertificate(%q) unexpectedly succeeded: %#v", alias, result)
	}
	if !strings.Contains(result.Message, "Server certificate is protected") {
		t.Fatalf("unexpected protection result: %#v", result)
	}
	for _, path := range []string{clientCertPath(alias), clientKeyPath(alias), filepath.Join(ovData, "clients", alias+".ovpn")} {
		if !fileExists(path) {
			t.Fatalf("protected non-default server artifact %q was removed", path)
		}
	}
}

func TestDeleteClientCertificateNeverRemovesArtifactDirectories(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "directory-artifact"
	if err := os.MkdirAll(filepath.Join(ovData, "ccd", name), 0700); err != nil {
		t.Fatalf("create directory artifact: %v", err)
	}

	result, err := DeleteClientCertificate(name)
	if err == nil || result.Success {
		t.Fatalf("DeleteClientCertificate(%q) unexpectedly succeeded: %#v", name, result)
	}
	if !strings.Contains(result.Message, "refusing to remove directory") {
		t.Fatalf("unexpected directory cleanup result: %#v", result)
	}
	if info, statErr := os.Stat(filepath.Join(ovData, "ccd", name)); statErr != nil || !info.IsDir() {
		t.Fatalf("artifact directory was removed or changed: info=%#v err=%v", info, statErr)
	}
}

func TestDeleteClientCertificateKeepsReloadRequiredAfterCleanupFailure(t *testing.T) {
	setupCertificateServiceTest(t)

	const name = "revoked-before-cleanup-failure"
	if err := generateClientCert(name); err != nil {
		t.Fatalf("generate client certificate: %v", err)
	}
	cert := readTestCertificate(t, clientCertPath(name))
	if err := os.MkdirAll(filepath.Join(ovData, "ccd", name), 0700); err != nil {
		t.Fatalf("create directory artifact: %v", err)
	}

	result, err := DeleteClientCertificate(name)
	if err == nil || result.Success {
		t.Fatalf("DeleteClientCertificate(%q) unexpectedly succeeded: %#v", name, result)
	}
	if !result.ReloadRequired {
		t.Fatalf("result must request CRL reload after revocation: %#v", result)
	}
	if revoked, revocationErr := isCertificateRevoked(cert); revocationErr != nil || !revoked {
		t.Fatalf("certificate was not revoked before cleanup failure: revoked=%v err=%v", revoked, revocationErr)
	}
}

func TestMarkCertificateReloadPendingMakesRevocationIncomplete(t *testing.T) {
	reloadErr := fmt.Errorf("%s: management unavailable", crlReloadPendingMessage)
	results := []CertificateDeletionResult{
		{Name: "active-client", Success: true, Message: "Revoked", ReloadRequired: true},
		{Name: "orphan", Success: true, Message: "Removed orphaned artifacts"},
	}

	if successCount := markCertificateReloadPending(results, reloadErr); successCount != 1 {
		t.Fatalf("success count = %d, want 1", successCount)
	}
	if results[0].Success || !strings.Contains(results[0].Message, crlReloadPendingMessage) {
		t.Fatalf("active revocation was not marked pending: %#v", results[0])
	}
	if !results[1].Success {
		t.Fatalf("unrelated orphan cleanup should remain successful: %#v", results[1])
	}
}

func TestReloadOpenVPNCRLRejectsMissingManagementClient(t *testing.T) {
	if err := reloadOpenVPNCRL(nil); err == nil || !strings.Contains(err.Error(), crlReloadPendingMessage) {
		t.Fatalf("reloadOpenVPNCRL(nil) error = %v, want pending CRL error", err)
	}
}
