package openvpnweb

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	suricataBuiltInEVEPath  = "/data/suricata/eve.json"
	suricataEVEMaxLineBytes = 1024 * 1024
	suricataEVEMaxPollSecs  = 300
)

// SuricataEVEOffset is a durable, read-only cursor for one canonical EVE file.
type SuricataEVEOffset struct {
	Path          string    `gorm:"primaryKey;size:1024"`
	Offset        int64     `gorm:"not null;default:0"`
	FileSignature string    `gorm:"size:64"`
	FileIdentity  string    `gorm:"size:255"`
	FileSize      int64     `gorm:"not null;default:0"`
	FileModUnixNS int64     `gorm:"not null;default:0"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (SuricataEVEOffset) TableName() string { return "suricata_eve_offsets" }

type SuricataEVEStatus struct {
	Enabled        bool   `json:"enabled"`
	Path           string `json:"path,omitempty"`
	Running        bool   `json:"running"`
	Offset         int64  `json:"offset"`
	Imported       uint64 `json:"imported"`
	Dropped        uint64 `json:"dropped"`
	Malformed      uint64 `json:"malformed"`
	LastError      string `json:"lastError,omitempty"`
	LastImportTime int64  `json:"lastImportTime,omitempty"`
}

type suricataEVEEvent struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	SrcIP     string `json:"src_ip"`
	DestIP    string `json:"dest_ip"`
	Proto     string `json:"proto"`
	AppProto  string `json:"app_proto"`
	SrcPort   uint16 `json:"src_port"`
	DestPort  uint16 `json:"dest_port"`
	Flow      struct {
		BytesToServer uint64 `json:"bytes_toserver"`
		BytesToClient uint64 `json:"bytes_toclient"`
		PktsToServer  uint64 `json:"pkts_toserver"`
		PktsToClient  uint64 `json:"pkts_toclient"`
	} `json:"flow"`
	DNS struct {
		RRName string `json:"rrname"`
		RRType string `json:"rrtype"`
	} `json:"dns"`
	TLS struct {
		SNI     string `json:"sni"`
		Version string `json:"version"`
	} `json:"tls"`
	HTTP struct {
		Hostname string `json:"hostname"`
		URL      string `json:"url"`
		Method   string `json:"http_method"`
	} `json:"http"`
	Alert struct {
		Signature string `json:"signature"`
		Category  string `json:"category"`
		Severity  int    `json:"severity"`
	} `json:"alert"`
}

type suricataEVEService struct {
	mu     sync.RWMutex
	pollMu sync.Mutex
	cancel context.CancelFunc
	status SuricataEVEStatus
}

var suricataEVE = &suricataEVEService{}

func suricataEVEEnabled() bool { return viper.GetBool("system.base.suricata_eve_enabled") }

func suricataEVEPollInterval() time.Duration {
	seconds := viper.GetInt("system.base.suricata_eve_poll_seconds")
	if seconds < 1 {
		seconds = 5
	}
	if seconds > suricataEVEMaxPollSecs {
		seconds = suricataEVEMaxPollSecs
	}
	return time.Duration(seconds) * time.Second
}

func suricataEVEMaxDays() int {
	if days := viper.GetInt("system.base.suricata_eve_max_days"); days > 0 {
		return days
	}
	return configHistoryMaxDays()
}

func validateSuricataEVEPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("Suricata EVE 文件路径不能为空")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Suricata EVE 文件路径必须为绝对路径")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("解析 Suricata EVE 文件路径失败: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("读取 Suricata EVE 文件状态失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Suricata EVE 路径必须指向普通文件")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("Suricata EVE 文件不可读: %w", err)
	}
	return resolved, file.Close()
}

func sanitizeSuricataHTTPPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	// Suricata emits an origin-form request target here. ParseRequestURI accepts
	// that relative form while rejecting malformed targets; only Path is retained.
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.User != nil || parsed.Host != "" || parsed.Scheme != "" {
		return ""
	}
	path := parsed.EscapedPath()
	if path == "" {
		return "/"
	}
	if len(path) > 2048 {
		return path[:2048]
	}
	return path
}

func getSuricataEVEStatus() SuricataEVEStatus {
	suricataEVE.mu.RLock()
	defer suricataEVE.mu.RUnlock()
	return suricataEVE.status
}

func (s *suricataEVEService) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.status.LastError = ""
		return
	}
	s.status.LastError = err.Error()
}

func ensureBuiltInSuricataEVEFile(path string) error {
	if strings.TrimSpace(path) != suricataBuiltInEVEPath {
		return nil
	}
	return ensureSuricataEVEFile(path)
}

func ensureSuricataEVEFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("创建内置 Suricata EVE 目录失败: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("内置 Suricata EVE 路径必须是普通文件，不能是符号链接")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取内置 Suricata EVE 文件状态失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("创建内置 Suricata EVE 文件失败: %w", err)
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return fmt.Errorf("收紧内置 Suricata EVE 文件权限失败: %w", err)
	}
	return file.Close()
}

func (s *suricataEVEService) reconcile() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.status = SuricataEVEStatus{Enabled: suricataEVEEnabled()}
	if !s.status.Enabled {
		s.mu.Unlock()
		return
	}
	configuredPath := viper.GetString("system.base.suricata_eve_path")
	if err := ensureBuiltInSuricataEVEFile(configuredPath); err != nil {
		s.status.LastError = err.Error()
		s.mu.Unlock()
		return
	}
	path, err := validateSuricataEVEPath(configuredPath)
	if err != nil {
		s.status.LastError = err.Error()
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status.Path, s.status.Running = path, true
	s.mu.Unlock()
	go s.loop(ctx, path)
}

func (s *suricataEVEService) loop(ctx context.Context, path string) {
	s.poll(path)
	ticker := time.NewTicker(suricataEVEPollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(path)
		}
	}
}

func (s *suricataEVEService) poll(path string) {
	// A settings save can replace the loop while an import is still in flight.
	// Keep file reads and cursor writes serialized to avoid duplicate inserts.
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	offset, imported, dropped, malformed, err := importSuricataEVEFile(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Path != path {
		return
	}
	s.status.Offset = offset
	s.status.Imported += imported
	s.status.Dropped += dropped
	s.status.Malformed += malformed
	if imported > 0 {
		s.status.LastImportTime = time.Now().Unix()
	}
	if err != nil {
		s.status.LastError = err.Error()
	} else {
		s.status.LastError = ""
	}
}

func suricataEVEFileSignature(file *os.File) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	buf := make([]byte, 64)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	sum := sha256.Sum256(buf[:n])
	return hex.EncodeToString(sum[:]), nil
}

// suricataEVEFileIdentity uses the platform file object where available. Linux
// stat values carry device/inode; other systems still get a stable object string
// when exposed, otherwise the persisted size/mtime/prefix fallback is used.
func suricataEVEFileIdentity(info os.FileInfo) string {
	if info.Sys() == nil {
		return ""
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return ""
	}
	return fmt.Sprintf("%T:%+v", info.Sys(), info.Sys())
}

func readSuricataEVELine(reader *bufio.Reader) (line []byte, consumed int64, complete, tooLong bool, err error) {
	for {
		chunk, readErr := reader.ReadSlice('\n')
		consumed += int64(len(chunk))
		if !tooLong {
			if len(line)+len(chunk) > suricataEVEMaxLineBytes {
				tooLong = true
				line = nil
			} else {
				line = append(line, chunk...)
			}
		}
		switch readErr {
		case nil:
			return line, consumed, true, tooLong, nil
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			return line, consumed, false, tooLong, io.EOF
		default:
			return line, consumed, false, tooLong, readErr
		}
	}
}

func saveSuricataEVECursor(tx *gorm.DB, path string, offset int64, signature, identity string, info os.FileInfo) error {
	return tx.Save(&SuricataEVEOffset{Path: path, Offset: offset, FileSignature: signature, FileIdentity: identity, FileSize: info.Size(), FileModUnixNS: info.ModTime().UnixNano()}).Error
}

// importSuricataEVEFile has bounded line memory and atomically persists every
// accepted, skipped, malformed, or oversized complete line with its cursor.
func importSuricataEVEFile(path string) (offset int64, imported, dropped, malformed uint64, err error) {
	path, err = validateSuricataEVEPath(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	signature, err := suricataEVEFileSignature(file)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	identity := suricataEVEFileIdentity(info)
	var cursor SuricataEVEOffset
	result := db.Where("path = ?", path).First(&cursor)
	if result.Error != nil && !isRecordNotFound(result.Error) {
		return 0, 0, 0, 0, result.Error
	}
	if result.Error == nil && cursor.Offset <= info.Size() {
		// Ordinary appends can change both timestamps and content prefixes. Retain
		// the durable position unless the file shrank; that is the conservative,
		// portable truncation signal required for the bounded JSONL tailer.
		offset = cursor.Offset
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return offset, 0, 0, 0, err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	for {
		line, consumed, complete, tooLong, readErr := readSuricataEVELine(reader)
		if consumed == 0 && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return offset, imported, dropped, malformed, readErr
		}
		// Never checkpoint an unterminated tail; it may still be in flight.
		if !complete {
			break
		}
		nextOffset := offset + consumed
		var entry *SuricataNetworkEvent
		if tooLong {
			malformed++
		} else if trimmed := strings.TrimSpace(string(line)); trimmed != "" {
			parsed, ok, parseErr := parseSuricataEVELine([]byte(trimmed))
			if parseErr != nil {
				malformed++
			} else if !ok {
				dropped++
			} else if client, found := webAuditDNS.clientIdentity(parsed.VPNIP); !found || client.UserID == 0 || client.Username == "" {
				dropped++
			} else {
				parsed.UserID, parsed.Username, parsed.CommonName, parsed.ConnectionID = client.UserID, client.Username, client.CommonName, client.ConnectionID
				entry = &parsed
			}
		}
		if txErr := db.Transaction(func(tx *gorm.DB) error {
			if entry != nil {
				if err := tx.Create(entry).Error; err != nil {
					return err
				}
			}
			return saveSuricataEVECursor(tx, path, nextOffset, signature, identity, info)
		}); txErr != nil {
			return offset, imported, dropped, malformed, txErr
		}
		if entry != nil {
			imported++
		}
		offset = nextOffset
	}
	return offset, imported, dropped, malformed, nil
}

func isRecordNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "record not found")
}

func parseSuricataEVELine(line []byte) (SuricataNetworkEvent, bool, error) {
	var event suricataEVEEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return SuricataNetworkEvent{}, false, err
	}
	event.EventType = strings.ToLower(strings.TrimSpace(event.EventType))
	switch event.EventType {
	case "flow", "dns", "tls", "http", "alert":
	default:
		return SuricataNetworkEvent{}, false, nil
	}
	if net.ParseIP(event.SrcIP) == nil || net.ParseIP(event.DestIP) == nil {
		return SuricataNetworkEvent{}, false, nil
	}
	observedAt := time.Now().Unix()
	if event.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil {
			return SuricataNetworkEvent{}, false, err
		}
		observedAt = parsed.Unix()
	}
	return SuricataNetworkEvent{
		EventType: event.EventType, VPNIP: event.SrcIP, DestinationIP: event.DestIP, Protocol: event.Proto, AppProtocol: event.AppProto,
		SourcePort: event.SrcPort, DestinationPort: event.DestPort, BytesToServer: event.Flow.BytesToServer, BytesToClient: event.Flow.BytesToClient,
		PacketsToServer: event.Flow.PktsToServer, PacketsToClient: event.Flow.PktsToClient, DNSName: event.DNS.RRName, DNSRecordType: event.DNS.RRType,
		TLSSNI: event.TLS.SNI, TLSVersion: event.TLS.Version, HTTPHostname: event.HTTP.Hostname, HTTPURL: sanitizeSuricataHTTPPath(event.HTTP.URL), HTTPMethod: event.HTTP.Method,
		AlertSignature: event.Alert.Signature, AlertCategory: event.Alert.Category, AlertSeverity: event.Alert.Severity, ObservedAt: observedAt,
	}, true, nil
}
