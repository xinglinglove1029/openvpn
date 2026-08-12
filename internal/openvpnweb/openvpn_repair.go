package openvpnweb

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	serverRepairRestoreRequiredDirectives = "restore_required_directives"
	serverRepairCRLReference              = "repair_crl_reference"
	serverRepairReload                    = "reload"
)

// OpenVPNServerIssue describes one observable configuration or runtime problem.  It never
// exposes certificate/key material or the raw server.conf content.
type OpenVPNServerIssue struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	RepairAction string `json:"repairAction,omitempty"`
}

// OpenVPNServerDiagnosis is intentionally a compact, safe-to-display server health report.
type OpenVPNServerDiagnosis struct {
	ConfigFound  bool                 `json:"configFound"`
	ManagementOK bool                 `json:"managementOk"`
	ServerStatus string               `json:"serverStatus,omitempty"`
	Issues       []OpenVPNServerIssue `json:"issues"`
}

// OpenVPNServerRepairResult reports a constrained repair and its post-repair diagnosis.
type OpenVPNServerRepairResult struct {
	Success           bool                   `json:"success"`
	Action            string                 `json:"action"`
	Message           string                 `json:"message"`
	BackupPath        string                 `json:"backupPath,omitempty"`
	ChangedDirectives []string               `json:"changedDirectives,omitempty"`
	Reloaded          bool                   `json:"reloaded"`
	Diagnosis         OpenVPNServerDiagnosis `json:"diagnosis"`
}

func openVPNServerConfigPath() string {
	return filepath.Join(ovData, "server.conf")
}

func appendServerIssue(d *OpenVPNServerDiagnosis, code, severity, message, repairAction string) {
	d.Issues = append(d.Issues, OpenVPNServerIssue{
		Code: code, Severity: severity, Message: message, RepairAction: repairAction,
	})
}

func readOpenVPNDirectives(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	directives := make(map[string][]string)
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(fields[0])
		directives[key] = append(directives[key], strings.TrimSpace(strings.TrimPrefix(line, fields[0])))
	}
	return directives, nil
}

func firstDirective(directives map[string][]string, key string) string {
	values := directives[strings.ToLower(key)]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func managedServerName() string {
	return viperGetString("system.base.server_name", "server")
}

// managedServerCertPath and managedServerKeyPath match the paths emitted by the
// container entrypoint. The Go PKI helpers retain their legacy server.crt/server.key
// paths, so the self-healing code must not use them for a deployed server.conf.
func managedServerCertPath() string {
	return filepath.Join(pkiIssuedDir(), managedServerName()+".crt")
}

func managedServerKeyPath() string {
	return filepath.Join(pkiPrivateDir(), managedServerName()+".key")
}

func expectedServerDirectiveValues() (map[string]string, error) {
	subnet := viperGetString("openvpn.ovpn_subnet", "10.8.0.0/24")
	ip, network, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid configured VPN subnet %q: %w", subnet, err)
	}
	port := viperGetString("openvpn.ovpn_port", "1194")
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("invalid configured OpenVPN port %q", port)
	}
	proto := strings.ToLower(viperGetString("openvpn.ovpn_proto", "udp"))
	if proto == "" {
		proto = "udp"
	}
	management := viperGetString("openvpn.ovpn_management", "127.0.0.1:7505")
	host, managementPort, err := net.SplitHostPort(management)
	if err != nil || host == "" || managementPort == "" {
		return nil, fmt.Errorf("invalid configured management address %q", management)
	}

	return map[string]string{
		"port":       port,
		"proto":      proto,
		"server":     fmt.Sprintf("%s %s", ip.Mask(network.Mask), net.IP(network.Mask)),
		"ca":         caCertPath(),
		"cert":       managedServerCertPath(),
		"key":        managedServerKeyPath(),
		"crl-verify": crlPath(),
		"tls-crypt":  tcKeyPath(),
		"management": fmt.Sprintf("%s %s", host, managementPort),
	}, nil
}
func validOpenVPNProto(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "udp", "udp4", "udp6", "tcp", "tcp4", "tcp6", "tcp-server", "tcp4-server", "tcp6-server":
		return true
	default:
		return false
	}
}

func validOpenVPNServerNetwork(value string) bool {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return false
	}
	ip := net.ParseIP(fields[0])
	mask := net.ParseIP(fields[1])
	return ip != nil && ip.To4() != nil && mask != nil && mask.To4() != nil
}

func (ov *ovpn) DiagnoseOpenVPNServer() OpenVPNServerDiagnosis {
	diagnosis := OpenVPNServerDiagnosis{Issues: make([]OpenVPNServerIssue, 0)}
	configPath := openVPNServerConfigPath()
	directives, err := readOpenVPNDirectives(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			appendServerIssue(&diagnosis, "missing_server_config", "critical", "server.conf is missing", "")
		} else {
			appendServerIssue(&diagnosis, "unreadable_server_config", "critical", "server.conf cannot be read: "+err.Error(), "")
		}
		return diagnosis
	}
	diagnosis.ConfigFound = true

	expected, expectedErr := expectedServerDirectiveValues()
	if expectedErr != nil {
		appendServerIssue(&diagnosis, "invalid_application_openvpn_settings", "critical", expectedErr.Error(), "")
	} else {
		// Only the application's PKI directives are eligible for a fixed repair. A
		// valid custom path is surfaced as a warning and never overwritten by AI.
		for _, key := range []string{"ca", "cert", "key", "crl-verify", "tls-crypt"} {
			actual := firstDirective(directives, key)
			repair := serverRepairRestoreRequiredDirectives
			if key == "crl-verify" {
				repair = serverRepairCRLReference
			}
			if actual == "" {
				appendServerIssue(&diagnosis, "missing_directive_"+key, "critical", key+" directive is missing", repair)
			} else if filepath.Clean(actual) != filepath.Clean(expected[key]) {
				if fileExists(actual) {
					appendServerIssue(&diagnosis, "custom_"+key+"_path", "warning", key+" uses an existing custom path and will not be changed automatically", "")
				} else {
					appendServerIssue(&diagnosis, "invalid_"+key+"_path", "critical", key+" does not point to an existing file", repair)
				}
			}
		}
		for pathName, path := range map[string]string{
			"CA certificate": caCertPath(), "server certificate": managedServerCertPath(), "server key": managedServerKeyPath(), "CRL": crlPath(), "tls-crypt key": tcKeyPath(),
		} {
			if !fileExists(path) {
				appendServerIssue(&diagnosis, "missing_"+strings.ReplaceAll(strings.ToLower(pathName), " ", "_"), "critical", pathName+" is missing from the managed PKI", "")
			}
		}

		port := firstDirective(directives, "port")
		if parsed, err := strconv.Atoi(port); err != nil || parsed < 1 || parsed > 65535 {
			appendServerIssue(&diagnosis, "invalid_port", "critical", "port must be an integer from 1 to 65535; automatic repair is intentionally disabled", "")
		}
		proto := firstDirective(directives, "proto")
		if !validOpenVPNProto(proto) {
			appendServerIssue(&diagnosis, "invalid_proto", "critical", "proto must be a supported OpenVPN transport; automatic repair is intentionally disabled", "")
		}
		if !validOpenVPNServerNetwork(firstDirective(directives, "server")) {
			appendServerIssue(&diagnosis, "invalid_server_network", "critical", "server must contain an IPv4 network address and netmask; automatic repair is intentionally disabled", "")
		}
		if firstDirective(directives, "management") == "" {
			appendServerIssue(&diagnosis, "missing_management", "warning", "management directive is missing; the web console cannot verify runtime state", "")
		} else if firstDirective(directives, "management") != expected["management"] {
			appendServerIssue(&diagnosis, "management_drift", "warning", "management directive differs from the application setting and will not be changed automatically", "")
		}
	}

	if ov == nil {
		appendServerIssue(&diagnosis, "management_unavailable", "warning", "OpenVPN management client is not initialized", "")
		return diagnosis
	}
	server, managementOK := ov.safeServerData()
	diagnosis.ManagementOK = managementOK
	diagnosis.ServerStatus = strings.TrimSpace(server.Status)
	if !managementOK {
		appendServerIssue(&diagnosis, "management_unreachable", "warning", "OpenVPN management interface did not return a server state; inspect the service or container logs before attempting a reload", "")
	}
	return diagnosis
}

// ensureManagedDirective appends a missing active directive without changing an
// administrator-supplied existing value.
func ensureManagedDirective(lines []string, key, value string) ([]string, bool) {
	if firstDirectiveFromLines(lines, key) != "" {
		return lines, false
	}
	return append(lines, key+" "+value), true
}

func firstDirectiveFromLines(lines []string, key string) string {
	prefix := strings.ToLower(key)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && strings.EqualFold(fields[0], prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		}
	}
	return ""
}

func setManagedDirective(lines []string, key, value string) []string {
	prefix := key + " "
	replacement := prefix + value
	found := false
	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") || !strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(prefix)) {
			out = append(out, line)
			continue
		}
		if !found {
			out = append(out, replacement)
			found = true
		}
	}
	if !found {
		out = append(out, replacement)
	}
	return out
}

func writeServerConfigAtomically(path string, lines []string) (string, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	backupDir := filepath.Join(ovData, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}
	backupPath := filepath.Join(backupDir, "server.conf."+time.Now().UTC().Format("20060102T150405.000000000Z")+".bak")
	if err := os.WriteFile(backupPath, original, 0600); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".server.conf-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(info.Mode()); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", err
	}
	return backupPath, nil
}

func hasCriticalServerConfigIssue(diagnosis OpenVPNServerDiagnosis) bool {
	for _, issue := range diagnosis.Issues {
		if issue.Severity == "critical" {
			return true
		}
	}
	return false
}

// hasBlockingCriticalIssue ensures a constrained repair does not alter a partially
// broken server configuration. A repair may proceed only when every critical issue is
// explicitly assigned to that exact fixed action; all other critical issues require
// operator intervention first.
func hasBlockingCriticalIssue(diagnosis OpenVPNServerDiagnosis, action string) bool {
	for _, issue := range diagnosis.Issues {
		if issue.Severity != "critical" {
			continue
		}
		if strings.TrimSpace(issue.RepairAction) != action {
			return true
		}
	}
	return false
}

// RepairOpenVPNServer applies only fixed, reviewed repairs. It never accepts raw config,
// shell commands, paths, or certificate/key data from the AI model.
func (ov *ovpn) RepairOpenVPNServer(action string) (OpenVPNServerRepairResult, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	result := OpenVPNServerRepairResult{Action: action}
	if action != serverRepairRestoreRequiredDirectives && action != serverRepairCRLReference && action != serverRepairReload {
		return result, fmt.Errorf("unsupported repair action %q", action)
	}

	// Refuse before probing/reloading when application-managed PKI is incomplete.
	// This is also safe in an offline test or a stopped server: no config is written.
	if action != serverRepairReload {
		for _, path := range []string{caCertPath(), managedServerCertPath(), managedServerKeyPath(), crlPath(), tcKeyPath()} {
			if !fileExists(path) {
				return result, fmt.Errorf("refusing repair because required PKI file is missing: %s", filepath.Base(path))
			}
		}
	}

	diagnosis := ov.DiagnoseOpenVPNServer()
	result.Diagnosis = diagnosis
	if hasBlockingCriticalIssue(diagnosis, action) {
		return result, fmt.Errorf("refusing %s because unrelated critical server.conf or PKI issues remain", action)
	}
	if ov == nil || !diagnosis.ManagementOK {
		return result, fmt.Errorf("OpenVPN management interface is unavailable; no configuration was changed")
	}
	if action == serverRepairReload {
		if hasCriticalServerConfigIssue(diagnosis) {
			return result, fmt.Errorf("refusing reload while critical server.conf or PKI issues remain")
		}
		if _, err := ov.sendCommand("signal SIGHUP"); err != nil {
			return result, fmt.Errorf("reload OpenVPN: %w", err)
		}
		result.Reloaded = true
		result.Diagnosis = ov.DiagnoseOpenVPNServer()
		result.Success = result.Diagnosis.ManagementOK && !hasCriticalServerConfigIssue(result.Diagnosis)
		if !result.Success {
			return result, fmt.Errorf("reload signal was sent but post-reload diagnostics are not healthy")
		}
		result.Message = "OpenVPN reload signal sent and post-reload diagnostics passed"
		return result, nil
	}

	configPath := openVPNServerConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("read server.conf: %w", err)
	}
	expected, err := expectedServerDirectiveValues()
	if err != nil {
		return result, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	changed := make([]string, 0, 5)
	updatePKIDirective := func(key string) {
		actual := firstDirectiveFromLines(lines, key)
		if actual == "" {
			var didChange bool
			lines, didChange = ensureManagedDirective(lines, key, expected[key])
			if didChange {
				changed = append(changed, key)
			}
			return
		}
		if filepath.Clean(actual) != filepath.Clean(expected[key]) && !fileExists(actual) {
			lines = setManagedDirective(lines, key, expected[key])
			changed = append(changed, key)
		}
	}
	if action == serverRepairCRLReference {
		updatePKIDirective("crl-verify")
	} else {
		// Do not rewrite port/proto/server/management or valid custom PKI paths.
		// This action only restores missing/broken application-managed PKI directives.
		for _, key := range []string{"ca", "cert", "key", "crl-verify", "tls-crypt"} {
			updatePKIDirective(key)
		}
	}
	if len(changed) == 0 {
		return result, fmt.Errorf("no safe automatic change is required for action %q", action)
	}
	backupPath, err := writeServerConfigAtomically(configPath, lines)
	if err != nil {
		return result, fmt.Errorf("write repaired server.conf: %w", err)
	}
	result.BackupPath = backupPath
	result.ChangedDirectives = changed

	if _, err := ov.sendCommand("signal SIGHUP"); err != nil {
		return result, fmt.Errorf("server.conf repaired and backed up, but reload failed: %w", err)
	}
	result.Reloaded = true
	result.Diagnosis = ov.DiagnoseOpenVPNServer()
	if !result.Diagnosis.ManagementOK || hasCriticalServerConfigIssue(result.Diagnosis) {
		return result, fmt.Errorf("repair completed but post-reload diagnostics remain unhealthy; backup retained at %s", backupPath)
	}
	result.Success = true
	result.Message = "OpenVPN server.conf repaired, backed up, reloaded, and verified"
	return result, nil
}
