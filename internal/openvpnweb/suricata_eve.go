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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const (
	suricataEVEMaxLineBytes = 1024 * 1024
	suricataEVEMaxPollSecs  = 300
)

// SuricataEVEOffset is a durable, read-only cursor for one canonical EVE file.
type SuricataEVEOffset struct {
	Path          string    `gorm:"primaryKey;size:1024"`
	Offset        int64     `gorm:"not null;default:0"`
	FileSignature string    `gorm:"size:64"`
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
	return filepath.Clean(path), nil
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
	path, err := validateSuricataEVEPath(viper.GetString("system.base.suricata_eve_path"))
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
	// Use a fixed prefix rather than file size-dependent content so appending
	// short JSONL files does not look like a rotation. Size shrink still resets
	// the cursor, while a replaced file with a different prefix is detected.
	buf := make([]byte, 64)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	sum := sha256.Sum256(buf[:n])
	return hex.EncodeToString(sum[:]), nil
}

// importSuricataEVEFile processes at most one MiB per line and advances its
// durable cursor after every consumed line, including ignored and malformed rows.
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
	var cursor SuricataEVEOffset
	result := db.Where("path = ?", path).First(&cursor)
	if result.Error != nil && !isRecordNotFound(result.Error) {
		return 0, 0, 0, 0, result.Error
	}
	if result.Error == nil && cursor.Offset <= info.Size() && (cursor.FileSignature == "" || cursor.FileSignature == signature) {
		offset = cursor.Offset
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return offset, 0, 0, 0, err
	}
	reader := bufio.NewReaderSize(file, suricataEVEMaxLineBytes+1)
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > suricataEVEMaxLineBytes {
			malformed++
			offset += int64(len(line))
			if readErr != nil {
				break
			}
			continue
		}
		if len(line) > 0 {
			// A tailing writer may leave the final JSON object incomplete. Preserve
			// its starting cursor until a terminating newline makes it immutable.
			if !strings.HasSuffix(line, "\n") && readErr == io.EOF {
				break
			}
			nextOffset := offset + int64(len(line))
			trimmed := strings.TrimSpace(line)
			var entry *SuricataNetworkEvent
			if trimmed != "" {
				parsed, ok, parseErr := parseSuricataEVELine([]byte(trimmed))
				if parseErr != nil {
					malformed++
				} else if !ok {
					dropped++
				} else if identity, found := webAuditDNS.clientIdentity(parsed.VPNIP); !found || identity.UserID == 0 || identity.Username == "" {
					dropped++
				} else {
					// Snapshot identity before the transaction; do not resolve the IP
					// again after a VPN address may have been reused.
					parsed.UserID, parsed.Username, parsed.CommonName, parsed.ConnectionID = identity.UserID, identity.Username, identity.CommonName, identity.ConnectionID
					entry = &parsed
				}
			}
			// Persist the event and its cursor in one transaction. A process crash
			// can therefore produce neither a row nor a cursor advance, never one
			// without the other that would replay a successfully imported event.
			if txErr := db.Transaction(func(tx *gorm.DB) error {
				if entry != nil {
					if err := tx.Create(entry).Error; err != nil {
						return err
					}
				}
				return tx.Save(&SuricataEVEOffset{Path: path, Offset: nextOffset, FileSignature: signature}).Error
			}); txErr != nil {
				return offset, imported, dropped, malformed, txErr
			}
			if entry != nil {
				imported++
			}
			offset = nextOffset
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return offset, imported, dropped, malformed, readErr
		}
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
		TLSSNI: event.TLS.SNI, TLSVersion: event.TLS.Version, HTTPHostname: event.HTTP.Hostname, HTTPURL: event.HTTP.URL, HTTPMethod: event.HTTP.Method,
		AlertSignature: event.Alert.Signature, AlertCategory: event.Alert.Category, AlertSeverity: event.Alert.Severity, ObservedAt: observedAt,
	}, true, nil
}
