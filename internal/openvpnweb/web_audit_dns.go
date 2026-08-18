package openvpnweb

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/net/dns/dnsmessage"
)

const (
	webAuditDNSListenAddress4 = "0.0.0.0:5353"
	webAuditDNSListenAddress6 = "[::]:5353"
	webAuditDNSPort           = 5353
	webAuditRequestQueueSize  = 256
	webAuditAuditQueueSize    = 4096
	webAuditRequestWorkers    = 24
	webAuditAuditWorkers      = 2
	webAuditTCPMaxConnections = 64
	// DNS forwarding is protected separately from audit persistence. A single
	// VPN peer cannot fill the shared forwarding queue/workers for every user.
	webAuditForwardPerClientRate  = 20.0 // requests per second
	webAuditForwardPerClientBurst = 40.0
	webAuditForwardGlobalRate     = 200.0
	webAuditForwardGlobalBurst    = 400.0
	// Persistence is intentionally much lower than forwarding capacity. Audit
	// loss must never affect DNS availability or consume unbounded DB storage.
	webAuditPerClientEventsPerMinute       = 60
	webAuditGlobalEventsPerMinute          = 300
	webAuditMaxStoredRows            int64 = 1000000
	webAuditHookMappingGrace               = 15 * time.Second
	webAuditRecoveryDelay                  = 30 * time.Second
)

type WebAuditDNSStatus struct {
	Enabled               bool     `json:"enabled"`
	ListenerReady         bool     `json:"listenerReady"`     // IPv4 compatibility field.
	RedirectInstalled     bool     `json:"redirectInstalled"` // IPv4 compatibility field.
	IPv4ListenerReady     bool     `json:"ipv4ListenerReady"`
	IPv6ListenerReady     bool     `json:"ipv6ListenerReady"`
	IPv4RedirectInstalled bool     `json:"ipv4RedirectInstalled"`
	IPv6RedirectInstalled bool     `json:"ipv6RedirectInstalled"`
	ListenAddress         string   `json:"listenAddress"`
	UpstreamDNS           []string `json:"upstreamDns"`
	DroppedAuditEvents    uint64   `json:"droppedAuditEvents"`
	DroppedDNSRequests    uint64   `json:"droppedDnsRequests"`
	StorageLimitReached   bool     `json:"storageLimitReached"`
	IngressRestricted     bool     `json:"ingressRestricted"`
	LastError             string   `json:"lastError,omitempty"`
	CoverageNote          string   `json:"coverageNote"`
}

type auditClientIdentity struct {
	UserID       uint
	Username     string
	CommonName   string
	ConnectionID string
	// UpdatedAt is supplied by the local OpenVPN lifecycle hook. It prevents a
	// delayed older hook from replacing a newer owner of a recycled VPN address.
	UpdatedAt  int64
	HookSource bool
}

type webAuditEvent struct {
	VPNIP        string
	Identity     auditClientIdentity
	Domain       string
	QueryType    string
	ResponseCode string
	QueriedAt    int64
}

type webAuditUDPRequest struct {
	conn     *net.UDPConn
	addr     *net.UDPAddr
	request  []byte
	identity auditClientIdentity // snapshot taken on receipt; never re-resolve after IP reuse.
}

type auditRateBucket struct {
	minute int64
	count  int
}

type forwardRateBucket struct {
	tokens  float64
	updated time.Time
}

type webAuditDNSRun struct {
	ctx         context.Context
	cancel      context.CancelFunc
	udp4        *net.UDPConn
	udp6        *net.UDPConn
	tcp4        net.Listener
	tcp6        net.Listener
	requests    chan webAuditUDPRequest
	audits      chan webAuditEvent
	tcpSem      chan struct{}
	failureOnce sync.Once
}

type webAuditDNSService struct {
	ov *ovpn
	mu sync.RWMutex

	run                  *webAuditDNSRun
	clients              map[string]auditClientIdentity
	status               WebAuditDNSStatus
	rate                 map[string]auditRateBucket
	forwardRate          map[string]forwardRateBucket
	forwardGlobal        forwardRateBucket
	auditMinute          int64
	auditCount           int
	storedAuditRows      int64
	storageCheckedMinute int64
	storageInitialized   bool
	forwardFailureStreak int
	lastErrorIsForward   bool
	recoveryScheduled    bool
}

var webAuditDNSLifecycleMu sync.Mutex

var webAuditDNS = &webAuditDNSService{
	clients:     map[string]auditClientIdentity{},
	rate:        map[string]auditRateBucket{},
	forwardRate: map[string]forwardRateBucket{},
	status: WebAuditDNSStatus{
		ListenAddress: webAuditDNSListenAddress4 + "；IPv6 " + webAuditDNSListenAddress6 + "（仅接受 tun0 经 DNS 重定向送入的请求）",
		CoverageNote:  "仅记录经 VPN 隧道的普通 DNS 查询；不会解密 HTTPS 或记录 URL、网页内容、Cookie、凭据或 DNS 响应内容。DoH、DoT、QUIC DNS、应用内解析及绕过 VPN 的请求不会被记录。IPv6 DNS 仅在本机 IPv6 UDP/TCP 监听和 ip6tables REDIRECT 都就绪时审计。",
	},
}

func normalizeDNSName(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
}

func dnsQueryInfo(packet []byte) (string, string, error) {
	var parser dnsmessage.Parser
	if _, err := parser.Start(packet); err != nil {
		return "", "", err
	}
	q, err := parser.Question()
	if err != nil {
		return "", "", err
	}
	return normalizeDNSName(q.Name.String()), q.Type.String(), nil
}

func dnsResponseCode(packet []byte) string {
	var parser dnsmessage.Parser
	h, err := parser.Start(packet)
	if err != nil {
		return "MALFORMED"
	}
	return h.RCode.String()
}

func getWebAuditDNSStatus() WebAuditDNSStatus {
	webAuditDNS.mu.RLock()
	defer webAuditDNS.mu.RUnlock()
	s := webAuditDNS.status
	s.UpstreamDNS = append([]string(nil), s.UpstreamDNS...)
	return s
}

func (s *webAuditDNSService) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.status.LastError = err.Error()
	s.lastErrorIsForward = false
	s.mu.Unlock()
}

func (s *webAuditDNSService) updateCoverageLocked() {
	base := "仅记录经 VPN 隧道的普通 DNS 查询；不会解密 HTTPS 或记录 URL、网页内容、Cookie、凭据或 DNS 响应内容。DoH、DoT、QUIC DNS、应用内解析及绕过 VPN 的请求不会被记录。"
	if !s.status.IngressRestricted {
		s.status.CoverageNote = base + " DNS 监听入口防护未就绪，因此不会截获任何 DNS 流量。"
		return
	}
	if s.status.IPv6ListenerReady && s.status.IPv6RedirectInstalled {
		s.status.CoverageNote = base + " IPv4 与 IPv6 的普通 UDP/TCP DNS 重定向均已就绪；5353 端口仅允许 tun0 入站。"
		return
	}
	if s.status.IPv6ListenerReady {
		s.status.CoverageNote = base + " IPv6 本地监听已就绪，但 ip6tables REDIRECT 未就绪；IPv6 普通 DNS 当前不审计。5353 端口仅允许 tun0 入站。"
		return
	}
	s.status.CoverageNote = base + " IPv6 UDP/TCP DNS 截获未启用或不可用；IPv6 普通 DNS 当前不审计。5353 端口仅允许 tun0 入站。"
}

func webAuditEnabled() bool { return viper.GetBool("system.base.web_audit_enabled") }

func configuredDNSUpstreams() []string {
	values := []string{strings.TrimSpace(viper.GetString("openvpn.ovpn_push_dns1")), strings.TrimSpace(viper.GetString("openvpn.ovpn_push_dns2"))}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if net.ParseIP(value) == nil {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// syncWebAuditDNS is the only lifecycle entry point after startup. It makes a
// saved enable/disable setting take effect immediately; stopping always removes
// both IPv4 and IPv6 interception rules before returning.
func syncWebAuditDNS(ctx context.Context, ov *ovpn) {
	// Settings saves and fsnotify reloads may arrive together. Serialize the
	// stop/start pair so they can never leave a listener running without its
	// matching redirect state.
	webAuditDNSLifecycleMu.Lock()
	defer webAuditDNSLifecycleMu.Unlock()
	stopWebAuditDNS()
	if webAuditEnabled() {
		startWebAuditDNS(ctx, ov)
	}
}

// reconcileWebAuditDNSConfig applies direct config-file edits as well as UI
// saves. It only restarts the proxy when the enabled state or upstream DNS
// values actually differ, avoiding an avoidable DNS gap for unrelated config
// changes observed by fsnotify.
func reconcileWebAuditDNSConfig() {
	webAuditDNS.mu.RLock()
	ov := webAuditDNS.ov
	run := webAuditDNS.run
	status := webAuditDNS.status
	webAuditDNS.mu.RUnlock()
	if ov == nil {
		return
	}
	wantedEnabled := webAuditEnabled()
	wantedUpstreams := configuredDNSUpstreams()
	if wantedEnabled && run != nil && status.Enabled && status.RedirectInstalled && equalStringSlices(status.UpstreamDNS, wantedUpstreams) {
		return
	}
	if !wantedEnabled && run == nil && !status.Enabled {
		return
	}
	syncWebAuditDNS(context.Background(), ov)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func startWebAuditDNS(ctx context.Context, ov *ovpn) {
	if !webAuditEnabled() {
		stopWebAuditDNS()
		return
	}

	upstreams := configuredDNSUpstreams()
	if len(upstreams) == 0 {
		webAuditDNS.mu.Lock()
		webAuditDNS.status.Enabled = true
		webAuditDNS.status.UpstreamDNS = nil
		webAuditDNS.status.LastError = "未配置有效上游 DNS，DNS 审计不会截获流量"
		webAuditDNS.updateCoverageLocked()
		webAuditDNS.mu.Unlock()
		_ = webAuditDNS.ensureRedirect(false)
		_ = webAuditDNS.ensureIngressGuard(false)
		return
	}

	child, cancel := context.WithCancel(ctx)
	run := &webAuditDNSRun{
		ctx: child, cancel: cancel,
		requests: make(chan webAuditUDPRequest, webAuditRequestQueueSize),
		audits:   make(chan webAuditEvent, webAuditAuditQueueSize),
		tcpSem:   make(chan struct{}, webAuditTCPMaxConnections),
	}

	udp4, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: webAuditDNSPort})
	if err != nil {
		cancel()
		webAuditDNS.setStartFailure(fmt.Errorf("DNS IPv4 UDP 监听失败: %w", err), upstreams)
		_ = webAuditDNS.ensureRedirect(false)
		_ = webAuditDNS.ensureIngressGuard(false)
		return
	}
	tcp4, err := net.Listen("tcp4", webAuditDNSListenAddress4)
	if err != nil {
		_ = udp4.Close()
		cancel()
		webAuditDNS.setStartFailure(fmt.Errorf("DNS IPv4 TCP 监听失败: %w", err), upstreams)
		_ = webAuditDNS.ensureRedirect(false)
		_ = webAuditDNS.ensureIngressGuard(false)
		return
	}
	run.udp4, run.tcp4 = udp4, tcp4

	// IPv6 is optional and independently fail-open. A host without IPv6 or
	// ip6tables still receives working IPv4 DNS and explicitly reports the gap.
	var ipv6Err error
	if udp6, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: webAuditDNSPort}); err == nil {
		run.udp6 = udp6
		if tcp6, err := net.Listen("tcp6", webAuditDNSListenAddress6); err == nil {
			run.tcp6 = tcp6
		} else {
			ipv6Err = fmt.Errorf("DNS IPv6 TCP 监听失败: %w", err)
			_ = udp6.Close()
			run.udp6 = nil
		}
	} else {
		ipv6Err = fmt.Errorf("DNS IPv6 UDP 监听失败: %w", err)
	}

	webAuditDNS.mu.Lock()
	if webAuditDNS.run != nil {
		webAuditDNS.mu.Unlock()
		closeWebAuditDNSRun(run)
		return
	}
	webAuditDNS.ov = ov
	webAuditDNS.run = run
	webAuditDNS.clients = make(map[string]auditClientIdentity)
	webAuditDNS.rate = make(map[string]auditRateBucket)
	webAuditDNS.forwardRate = make(map[string]forwardRateBucket)
	webAuditDNS.forwardGlobal = forwardRateBucket{}
	webAuditDNS.auditMinute = 0
	webAuditDNS.auditCount = 0
	webAuditDNS.storedAuditRows = 0
	webAuditDNS.storageCheckedMinute = 0
	webAuditDNS.storageInitialized = false
	webAuditDNS.recoveryScheduled = false
	webAuditDNS.status.Enabled = true
	webAuditDNS.status.UpstreamDNS = append([]string(nil), upstreams...)
	webAuditDNS.status.IPv4ListenerReady = true
	webAuditDNS.status.ListenerReady = true
	webAuditDNS.status.IPv6ListenerReady = run.udp6 != nil && run.tcp6 != nil
	webAuditDNS.status.IPv4RedirectInstalled = false
	webAuditDNS.status.RedirectInstalled = false
	webAuditDNS.status.IPv6RedirectInstalled = false
	webAuditDNS.status.DroppedAuditEvents = 0
	webAuditDNS.status.DroppedDNSRequests = 0
	webAuditDNS.status.StorageLimitReached = false
	webAuditDNS.status.IngressRestricted = false
	if ipv6Err != nil {
		webAuditDNS.status.LastError = ipv6Err.Error()
	} else {
		webAuditDNS.status.LastError = ""
	}
	webAuditDNS.updateCoverageLocked()
	webAuditDNS.mu.Unlock()

	// Linux REDIRECT retains the local tunnel destination, so the proxy must
	// bind all local addresses instead of loopback. Install INPUT protection
	// before serving or redirecting so port 5353 never becomes an open relay.
	if err := webAuditDNS.ensureIngressGuard(true); err != nil {
		webAuditDNS.listenerUnavailable(run, false, fmt.Errorf("DNS 监听入口防护安装失败: %w", err))
		return
	}
	for i := 0; i < webAuditRequestWorkers; i++ {
		go webAuditDNS.udpWorker(run)
	}
	for i := 0; i < webAuditAuditWorkers; i++ {
		go webAuditDNS.auditWorker(run)
	}
	go webAuditDNS.refreshClientCacheLoop(run, ov)
	go webAuditDNS.serveUDP(run, run.udp4, false)
	go webAuditDNS.serveTCP(run, run.tcp4, false)
	if run.udp6 != nil {
		go webAuditDNS.serveUDP(run, run.udp6, true)
		go webAuditDNS.serveTCP(run, run.tcp6, true)
	}
	if err := webAuditDNS.ensureRedirect(true); err != nil {
		webAuditDNS.setError(err)
		webAuditDNS.scheduleRedirectRecovery(run)
	}
}

func (s *webAuditDNSService) setStartFailure(err error, upstreams []string) {
	s.mu.Lock()
	s.status.Enabled = true
	s.status.ListenerReady = false
	s.status.RedirectInstalled = false
	s.status.IPv4ListenerReady = false
	s.status.IPv4RedirectInstalled = false
	s.status.IPv6ListenerReady = false
	s.status.IPv6RedirectInstalled = false
	s.status.UpstreamDNS = append([]string(nil), upstreams...)
	s.status.LastError = err.Error()
	s.updateCoverageLocked()
	s.mu.Unlock()
}

func closeWebAuditDNSRun(run *webAuditDNSRun) {
	if run == nil {
		return
	}
	run.cancel()
	if run.udp4 != nil {
		_ = run.udp4.Close()
	}
	if run.udp6 != nil {
		_ = run.udp6.Close()
	}
	if run.tcp4 != nil {
		_ = run.tcp4.Close()
	}
	if run.tcp6 != nil {
		_ = run.tcp6.Close()
	}
}

func stopWebAuditDNS() {
	webAuditDNS.mu.Lock()
	run := webAuditDNS.run
	webAuditDNS.run = nil
	webAuditDNS.clients = make(map[string]auditClientIdentity)
	webAuditDNS.rate = make(map[string]auditRateBucket)
	webAuditDNS.forwardRate = make(map[string]forwardRateBucket)
	webAuditDNS.forwardGlobal = forwardRateBucket{}
	webAuditDNS.auditMinute = 0
	webAuditDNS.auditCount = 0
	webAuditDNS.storedAuditRows = 0
	webAuditDNS.storageCheckedMinute = 0
	webAuditDNS.storageInitialized = false
	webAuditDNS.forwardFailureStreak = 0
	webAuditDNS.lastErrorIsForward = false
	webAuditDNS.recoveryScheduled = false
	upstreams := append([]string(nil), webAuditDNS.status.UpstreamDNS...)
	webAuditDNS.status.Enabled = false
	webAuditDNS.status.ListenerReady = false
	webAuditDNS.status.RedirectInstalled = false
	webAuditDNS.status.IPv4ListenerReady = false
	webAuditDNS.status.IPv4RedirectInstalled = false
	webAuditDNS.status.IPv6ListenerReady = false
	webAuditDNS.status.IPv6RedirectInstalled = false
	webAuditDNS.status.DroppedAuditEvents = 0
	webAuditDNS.status.DroppedDNSRequests = 0
	webAuditDNS.status.StorageLimitReached = false
	webAuditDNS.status.IngressRestricted = false
	webAuditDNS.status.LastError = ""
	webAuditDNS.status.UpstreamDNS = nil
	webAuditDNS.updateCoverageLocked()
	webAuditDNS.mu.Unlock()
	closeWebAuditDNSRun(run)
	// Always remove rules, including rules for the previous DNS servers. Snapshot
	// them before clearing status so a config change cannot leave old interception.
	_ = webAuditDNS.ensureRedirectWithUpstreams(false, upstreams)
	_ = webAuditDNS.ensureIngressGuard(false)
}

func (s *webAuditDNSService) isCurrentRun(run *webAuditDNSRun) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.run == run
}

func (s *webAuditDNSService) listenerUnavailable(run *webAuditDNSRun, ipv6 bool, err error) {
	run.failureOnce.Do(func() {
		s.mu.Lock()
		if s.run != run {
			s.mu.Unlock()
			return
		}
		upstreams := append([]string(nil), s.status.UpstreamDNS...)
		s.run = nil
		s.clients = make(map[string]auditClientIdentity)
		s.rate = make(map[string]auditRateBucket)
		s.forwardRate = make(map[string]forwardRateBucket)
		s.forwardGlobal = forwardRateBucket{}
		s.recoveryScheduled = false
		s.status.IPv4ListenerReady = false
		s.status.ListenerReady = false
		s.status.IPv4RedirectInstalled = false
		s.status.RedirectInstalled = false
		s.status.IPv6ListenerReady = false
		s.status.IPv6RedirectInstalled = false
		s.status.IngressRestricted = false
		s.status.LastError = err.Error()
		s.updateCoverageLocked()
		s.mu.Unlock()

		closeWebAuditDNSRun(run)
		_ = s.ensureRedirectWithUpstreams(false, upstreams)
		_ = s.ensureIngressGuard(false)
		// Never retain a broken interception path. Retry asynchronously so a transient
		// listener failure can self-heal without blocking OpenVPN or DNS clients.
		go func() {
			// closeWebAuditDNSRun cancels run.ctx immediately, so retry must not
			// wait on that cancelled context. Lifecycle serialization prevents a
			// stale retry from replacing a newer configured instance.
			time.Sleep(5 * time.Second)
			webAuditDNSLifecycleMu.Lock()
			defer webAuditDNSLifecycleMu.Unlock()
			if webAuditEnabled() && webAuditDNS.run == nil && webAuditDNS.ov != nil {
				startWebAuditDNS(context.Background(), webAuditDNS.ov)
			}
		}()
	})
}

func (s *webAuditDNSService) serveUDP(run *webAuditDNSRun, conn *net.UDPConn, ipv6 bool) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if run.ctx.Err() == nil {
				s.listenerUnavailable(run, ipv6, fmt.Errorf("DNS UDP 服务异常: %w", err))
			}
			return
		}
		identity, _ := s.clientIdentity(addr.IP.String())
		req := webAuditUDPRequest{conn: conn, addr: addr, request: append([]byte(nil), buf[:n]...), identity: identity}
		if !s.allowForward(run, addr.IP.String()) {
			if response := dnsFailureResponse(req.request); len(response) > 0 {
				_, _ = conn.WriteToUDP(response, addr)
			}
			continue
		}
		select {
		case run.requests <- req:
		case <-run.ctx.Done():
			return
		default:
			// Request work is deliberately bounded. Under overload return a DNS
			// error immediately rather than growing goroutines or blocking VPN DNS.
			if response := dnsFailureResponse(req.request); len(response) > 0 {
				_, _ = conn.WriteToUDP(response, addr)
			}
			s.noteDroppedDNSRequest(run)
		}
	}
}

func (s *webAuditDNSService) udpWorker(run *webAuditDNSRun) {
	for {
		select {
		case <-run.ctx.Done():
			return
		case req := <-run.requests:
			s.processUDPRequest(run, req)
		}
	}
}

func (s *webAuditDNSService) processUDPRequest(run *webAuditDNSRun, req webAuditUDPRequest) {
	response, code, err := s.forwardUDP(req.request)
	if err != nil {
		s.noteForwardFailure(run, err)
		response, code = dnsFailureResponse(req.request), "SERVFAIL"
	} else {
		s.noteForwardSuccess()
	}
	if len(response) > 0 {
		_, _ = req.conn.WriteToUDP(response, req.addr)
	}
	// The response is intentionally written before any parse, ownership lookup,
	// queue operation, or database write. Audit loss can never block DNS.
	if domain, qtype, parseErr := dnsQueryInfo(req.request); parseErr == nil {
		s.enqueueAudit(run, webAuditEvent{VPNIP: req.addr.IP.String(), Identity: req.identity, Domain: domain, QueryType: qtype, ResponseCode: code, QueriedAt: time.Now().Unix()})
	}
}

func (s *webAuditDNSService) serveTCP(run *webAuditDNSRun, listener net.Listener, ipv6 bool) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if run.ctx.Err() == nil {
				s.listenerUnavailable(run, ipv6, fmt.Errorf("DNS TCP 服务异常: %w", err))
			}
			return
		}
		select {
		case run.tcpSem <- struct{}{}:
			go func() { defer func() { <-run.tcpSem }(); s.handleTCP(run, conn) }()
		case <-run.ctx.Done():
			_ = conn.Close()
			return
		default:
			// Keep the number of TCP DNS connections finite as well.
			_ = conn.Close()
		}
	}
}

type dnsForwardFunc func([]byte) ([]byte, string, error)
type dnsResponseObserver func([]byte, string)

// serveTCPDNSFrames processes every length-prefixed request on a DNS-over-TCP
// connection. Keeping the frame loop separate makes the multi-request protocol
// behavior testable without a live port-53 upstream.
func serveTCPDNSFrames(client net.Conn, forward dnsForwardFunc, observed dnsResponseObserver) {
	defer client.Close()
	var length [2]byte
	for {
		_ = client.SetDeadline(time.Now().Add(12 * time.Second))
		if _, err := io.ReadFull(client, length[:]); err != nil {
			return // EOF after any number of DNS messages is normal.
		}
		size := int(binary.BigEndian.Uint16(length[:]))
		if size < 1 || size > 65535 {
			return
		}
		request := make([]byte, size)
		if _, err := io.ReadFull(client, request); err != nil {
			return
		}
		response, code, err := forward(request)
		if err != nil {
			response, code = dnsFailureResponse(request), "SERVFAIL"
		}
		if len(response) == 0 || len(response) > 65535 {
			return
		}
		binary.BigEndian.PutUint16(length[:], uint16(len(response)))
		if _, err := client.Write(append(length[:], response...)); err != nil {
			return
		}
		// Observers run strictly after the client response. They may enqueue a
		// bounded audit event, but must never participate in DNS forwarding.
		if observed != nil {
			observed(request, code)
		}
	}
}

func (s *webAuditDNSService) handleTCP(run *webAuditDNSRun, client net.Conn) {
	host, _, splitErr := net.SplitHostPort(client.RemoteAddr().String())
	if splitErr != nil {
		_ = client.Close()
		return
	}
	identity, _ := s.clientIdentity(host)
	serveTCPDNSFrames(client, func(request []byte) ([]byte, string, error) {
		if !s.allowForward(run, host) {
			return nil, "SERVFAIL", fmt.Errorf("DNS 请求速率超过保护阈值")
		}
		response, code, err := s.forwardTCP(request)
		if err != nil {
			s.noteForwardFailure(run, err)
		} else {
			s.noteForwardSuccess()
		}
		return response, code, err
	}, func(request []byte, code string) {
		if domain, qtype, parseErr := dnsQueryInfo(request); parseErr == nil {
			s.enqueueAudit(run, webAuditEvent{VPNIP: host, Identity: identity, Domain: domain, QueryType: qtype, ResponseCode: code, QueriedAt: time.Now().Unix()})
		}
	})
}

func consumeForwardToken(bucket *forwardRateBucket, rate, burst float64, now time.Time) bool {
	if bucket.updated.IsZero() {
		bucket.tokens, bucket.updated = burst, now
	} else {
		bucket.tokens = min(burst, bucket.tokens+now.Sub(bucket.updated).Seconds()*rate)
		bucket.updated = now
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (s *webAuditDNSService) allowForward(run *webAuditDNSRun, vpnIP string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != run {
		return false
	}
	if s.forwardRate == nil {
		s.forwardRate = make(map[string]forwardRateBucket)
	}
	client := s.forwardRate[vpnIP]
	// Refill both buckets before deciding. Do not consume either one unless both
	// limits allow the request, so a noisy peer cannot drain shared capacity.
	if client.updated.IsZero() {
		client.tokens, client.updated = webAuditForwardPerClientBurst, now
	} else {
		client.tokens = min(webAuditForwardPerClientBurst, client.tokens+now.Sub(client.updated).Seconds()*webAuditForwardPerClientRate)
		client.updated = now
	}
	global := s.forwardGlobal
	if global.updated.IsZero() {
		global.tokens, global.updated = webAuditForwardGlobalBurst, now
	} else {
		global.tokens = min(webAuditForwardGlobalBurst, global.tokens+now.Sub(global.updated).Seconds()*webAuditForwardGlobalRate)
		global.updated = now
	}
	if client.tokens < 1 || global.tokens < 1 {
		s.forwardRate[vpnIP], s.forwardGlobal = client, global
		s.status.DroppedDNSRequests++
		return false
	}
	client.tokens--
	global.tokens--
	s.forwardRate[vpnIP], s.forwardGlobal = client, global
	return true
}

func (s *webAuditDNSService) noteDroppedDNSRequest(run *webAuditDNSRun) {
	s.mu.Lock()
	if s.run == run {
		s.status.DroppedDNSRequests++
	}
	s.mu.Unlock()
}

// reserveAuditStorage periodically refreshes an exact row count and reserves one
// slot before insert. Reaching the quota drops only audit metadata, never DNS.
func (s *webAuditDNSService) reserveAuditStorage(run *webAuditDNSRun) bool {
	minute := time.Now().Unix() / 60
	s.mu.Lock()
	if s.run != run {
		s.mu.Unlock()
		return false
	}
	refresh := !s.storageInitialized || s.storageCheckedMinute != minute
	s.mu.Unlock()
	if refresh {
		var count int64
		if err := db.Model(&WebsiteAccessLog{}).Count(&count).Error; err != nil {
			s.setError(fmt.Errorf("检查 DNS 审计存储额度失败: %w", err))
			return false
		}
		s.mu.Lock()
		if s.run == run {
			s.storedAuditRows, s.storageCheckedMinute, s.storageInitialized = count, minute, true
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != run || s.storedAuditRows >= webAuditMaxStoredRows {
		if s.run == run {
			s.status.StorageLimitReached = true
			s.status.DroppedAuditEvents++
		}
		return false
	}
	s.storedAuditRows++
	s.status.StorageLimitReached = false
	return true
}

func (s *webAuditDNSService) scheduleRedirectRecovery(run *webAuditDNSRun) {
	s.mu.Lock()
	if s.run != run || s.recoveryScheduled {
		s.mu.Unlock()
		return
	}
	s.recoveryScheduled = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			if s.run == run {
				s.recoveryScheduled = false
			}
			s.mu.Unlock()
		}()
		for {
			select {
			case <-run.ctx.Done():
				return
			case <-time.After(webAuditRecoveryDelay):
			}
			if !s.isCurrentRun(run) || !webAuditEnabled() {
				return
			}
			upstreams := configuredDNSUpstreams()
			if err := probeDNSUpstreams(upstreams); err != nil {
				s.setForwardError(err)
				continue
			}
			if err := s.ensureRedirectWithUpstreams(true, upstreams); err != nil {
				s.setError(fmt.Errorf("恢复 DNS 审计重定向失败: %w", err))
				continue
			}
			s.noteForwardSuccess()
			return
		}
	}()
}

func (s *webAuditDNSService) setForwardError(err error) {
	s.mu.Lock()
	s.status.LastError = err.Error()
	s.lastErrorIsForward = true
	s.mu.Unlock()
}

func probeDNSUpstreams(upstreams []string) error {
	if len(upstreams) == 0 {
		return fmt.Errorf("未配置有效上游 DNS")
	}
	name, _ := dnsmessage.NewName("example.com.")
	message := dnsmessage.Message{Header: dnsmessage.Header{ID: 1, RecursionDesired: true}, Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}}}
	packet, err := message.Pack()
	if err != nil {
		return err
	}
	var lastErr error
	for _, upstream := range upstreams {
		conn, err := net.DialTimeout("udp", net.JoinHostPort(upstream, "53"), 2*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, err = conn.Write(packet)
		if err == nil {
			buf := make([]byte, 4096)
			_, err = conn.Read(buf)
		}
		_ = conn.Close()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("上游 DNS 健康探测失败: %w", lastErr)
}

func (s *webAuditDNSService) noteForwardSuccess() {
	s.mu.Lock()
	s.forwardFailureStreak = 0
	// Only clear an error known to be a forwarding failure. Database/cache or
	// listener errors remain observable until their own operation succeeds.
	if s.lastErrorIsForward {
		s.status.LastError = ""
		s.lastErrorIsForward = false
	}
	s.mu.Unlock()
}

func (s *webAuditDNSService) noteForwardFailure(run *webAuditDNSRun, err error) {
	s.mu.Lock()
	current := s.run == run
	if current {
		s.forwardFailureStreak++
		s.status.LastError = err.Error()
		s.lastErrorIsForward = true
	}
	// A few retryable failures are expected on unreliable WAN links. Once the
	// threshold is exceeded, remove REDIRECT so clients return to their configured
	// DNS resolver instead of being held behind a failing proxy. This counter is
	// deliberately independent from the audit event rate limiter.
	remove := current && s.forwardFailureStreak >= 3
	upstreams := append([]string(nil), s.status.UpstreamDNS...)
	s.mu.Unlock()
	if remove {
		_ = s.ensureRedirectWithUpstreams(false, upstreams)
		s.scheduleRedirectRecovery(run)
	}
}

func (s *webAuditDNSService) allowAuditEvent(run *webAuditDNSRun, vpnIP string) bool {
	minute := time.Now().Unix() / 60
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != run {
		return false
	}
	if s.auditMinute != minute {
		s.auditMinute, s.auditCount = minute, 0
	}
	if s.rate == nil {
		s.rate = make(map[string]auditRateBucket)
	}
	bucket := s.rate[vpnIP]
	if bucket.minute != minute {
		bucket = auditRateBucket{minute: minute}
	}
	if s.auditCount >= webAuditGlobalEventsPerMinute || bucket.count >= webAuditPerClientEventsPerMinute {
		s.status.DroppedAuditEvents++
		return false
	}
	s.auditCount++
	bucket.count++
	s.rate[vpnIP] = bucket
	return true
}

func (s *webAuditDNSService) enqueueAudit(run *webAuditDNSRun, event webAuditEvent) {
	// The owner is snapshotted as the packet arrives. Resolving it in the async
	// worker would let a recycled VPN IP be written under the next user.
	if event.Domain == "" || event.Identity.UserID == 0 || !webAuditEnabled() || !s.isCurrentRun(run) || !s.allowAuditEvent(run, event.VPNIP) {
		return
	}
	select {
	case run.audits <- event:
	default:
		s.mu.Lock()
		if s.run == run {
			s.status.DroppedAuditEvents++
		}
		s.mu.Unlock()
	}
}

func (s *webAuditDNSService) auditWorker(run *webAuditDNSRun) {
	for {
		select {
		case <-run.ctx.Done():
			return
		case event := <-run.audits:
			if !s.isCurrentRun(run) || !webAuditEnabled() || event.Identity.UserID == 0 || !s.reserveAuditStorage(run) {
				continue
			}
			entry := WebsiteAccessLog{UserID: event.Identity.UserID, Username: event.Identity.Username, CommonName: event.Identity.CommonName, ConnectionID: event.Identity.ConnectionID, VPNIP: event.VPNIP, Domain: event.Domain, QueryType: event.QueryType, ResponseCode: event.ResponseCode, QueriedAt: event.QueriedAt}
			if err := db.Create(&entry).Error; err != nil {
				s.setError(fmt.Errorf("保存 DNS 审计失败: %w", err))
			}
		}
	}
}

func (s *webAuditDNSService) clientIdentity(vpnIP string) (auditClientIdentity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity, ok := s.clients[vpnIP]
	return identity, ok
}

// updateClientIdentity is fed by local OpenVPN hooks. Delete checks the supplied
// connection identity so a late disconnect for user A cannot erase a newly reused
// VPN IP already mapped to user B.
func (s *webAuditDNSService) updateClientIdentity(action string, identity auditClientIdentity, ips ...string) {
	if identity.UpdatedAt == 0 {
		identity.UpdatedAt = time.Now().UnixNano()
	}
	identity.HookSource = true
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = make(map[string]auditClientIdentity)
	}
	for _, rawIP := range ips {
		ip := strings.TrimSpace(rawIP)
		if net.ParseIP(ip) == nil {
			continue
		}
		current, found := s.clients[ip]
		if action == "delete" {
			if !found || (identity.ConnectionID != "" && current.ConnectionID != identity.ConnectionID) ||
				(identity.ConnectionID == "" && identity.Username != "" && current.Username != identity.Username) ||
				(identity.UpdatedAt != 0 && current.UpdatedAt > identity.UpdatedAt) {
				continue
			}
			delete(s.clients, ip)
			continue
		}
		if identity.UserID == 0 || identity.Username == "" {
			continue
		}
		// A late hook from an older connection must never overwrite the owner that
		// already claimed this address. In a timestamp tie, retain the current
		// different connection rather than guessing and leaking attribution.
		if found && current.ConnectionID != identity.ConnectionID && current.UpdatedAt >= identity.UpdatedAt {
			continue
		}
		s.clients[ip] = identity
	}
}

func (s *webAuditDNSService) refreshClientCacheLoop(run *webAuditDNSRun, ov *ovpn) {
	s.refreshClientCache(run, ov)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			s.refreshClientCache(run, ov)
		}
	}
}

func (s *webAuditDNSService) refreshClientCache(run *webAuditDNSRun, ov *ovpn) {
	if ov == nil || !s.isCurrentRun(run) {
		return
	}
	clients, ok := ov.safeOnlineClients()
	if !ok {
		s.setError(fmt.Errorf("刷新 DNS 审计客户端归属失败: OpenVPN 管理接口暂不可用"))
		return
	}
	usernames := make([]string, 0, len(clients))
	seen := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		if client.Username != "" {
			if _, ok := seen[client.Username]; !ok {
				seen[client.Username] = struct{}{}
				usernames = append(usernames, client.Username)
			}
		}
	}
	userIDs := make(map[string]uint, len(usernames))
	if len(usernames) > 0 {
		lookupCtx, cancel := context.WithTimeout(run.ctx, 3*time.Second)
		defer cancel()
		var users []User
		if err := db.WithContext(lookupCtx).Select("id", "username").Where("username IN ?", usernames).Find(&users).Error; err != nil {
			s.setError(fmt.Errorf("刷新 DNS 审计用户缓存失败: %w", err))
			return
		}
		for _, user := range users {
			userIDs[user.Username] = user.ID
		}
	}
	identities := make(map[string]auditClientIdentity, len(clients)*2)
	for _, client := range clients {
		identity := auditClientIdentity{UserID: userIDs[client.Username], Username: client.Username, CommonName: client.CommonName, ConnectionID: client.ID, UpdatedAt: time.Now().UnixNano()}
		if identity.UserID == 0 {
			continue
		}
		if client.Vip != "" {
			identities[client.Vip] = identity
		}
		if client.Vip6 != "" {
			identities[client.Vip6] = identity
		}
	}
	// Management is a reconciliation source, but a just-arrived lifecycle hook
	// is more precise during address reuse. Preserve a conflicting hook owner for
	// a short grace window; the next fresh management snapshot then takes over.
	now := time.Now()
	s.mu.Lock()
	if s.run == run {
		merged := identities
		for ip, current := range s.clients {
			observed, exists := identities[ip]
			if current.HookSource && (!exists || observed.ConnectionID != current.ConnectionID) && now.Sub(time.Unix(0, current.UpdatedAt)) < webAuditHookMappingGrace {
				merged[ip] = current
				continue
			}
			if current.HookSource && exists && observed.ConnectionID == current.ConnectionID {
				merged[ip] = current
			}
		}
		s.clients = merged
	}
	s.mu.Unlock()
}

func dnsFailureResponse(request []byte) []byte {
	var parser dnsmessage.Parser
	header, err := parser.Start(request)
	if err != nil {
		return nil
	}
	questions := make([]dnsmessage.Question, 0, 1)
	for {
		question, err := parser.Question()
		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			return nil
		}
		questions = append(questions, question)
	}
	message := dnsmessage.Message{Header: dnsmessage.Header{ID: header.ID, Response: true, RecursionDesired: header.RecursionDesired, RecursionAvailable: true, RCode: dnsmessage.RCodeServerFailure}, Questions: questions}
	response, err := message.Pack()
	if err != nil {
		return nil
	}
	return response
}

func (s *webAuditDNSService) upstreams() ([]string, error) {
	upstreams := configuredDNSUpstreams()
	if len(upstreams) == 0 {
		return nil, fmt.Errorf("未配置有效上游 DNS")
	}
	s.mu.Lock()
	s.status.UpstreamDNS = append([]string(nil), upstreams...)
	s.mu.Unlock()
	return upstreams, nil
}

func (s *webAuditDNSService) forwardUDP(request []byte) ([]byte, string, error) {
	upstreams, err := s.upstreams()
	if err != nil {
		return nil, "SERVFAIL", err
	}
	var lastErr error
	for _, upstream := range upstreams {
		conn, err := net.DialTimeout("udp", net.JoinHostPort(upstream, "53"), 4*time.Second)
		if err != nil {
			lastErr = fmt.Errorf("连接上游 DNS %s 失败: %w", upstream, err)
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
		_, writeErr := conn.Write(request)
		if writeErr == nil {
			buf := make([]byte, 65535)
			n, readErr := conn.Read(buf)
			if readErr == nil {
				_ = conn.Close()
				response := append([]byte(nil), buf[:n]...)
				return response, dnsResponseCode(response), nil
			}
			lastErr = fmt.Errorf("上游 DNS %s 无响应: %w", upstream, readErr)
		} else {
			lastErr = fmt.Errorf("向上游 DNS %s 发送失败: %w", upstream, writeErr)
		}
		_ = conn.Close()
	}
	return nil, "SERVFAIL", lastErr
}

func (s *webAuditDNSService) forwardTCP(request []byte) ([]byte, string, error) {
	upstreams, err := s.upstreams()
	if err != nil {
		return nil, "SERVFAIL", err
	}
	var lastErr error
	for _, upstream := range upstreams {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(upstream, "53"), 4*time.Second)
		if err != nil {
			lastErr = fmt.Errorf("连接上游 DNS %s 失败: %w", upstream, err)
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
		var lenbuf [2]byte
		binary.BigEndian.PutUint16(lenbuf[:], uint16(len(request)))
		_, writeErr := conn.Write(append(lenbuf[:], request...))
		if writeErr == nil {
			if _, readErr := io.ReadFull(conn, lenbuf[:]); readErr == nil {
				n := int(binary.BigEndian.Uint16(lenbuf[:]))
				if n > 0 {
					response := make([]byte, n)
					if _, readErr := io.ReadFull(conn, response); readErr == nil {
						_ = conn.Close()
						return response, dnsResponseCode(response), nil
					} else {
						lastErr = fmt.Errorf("读取上游 DNS %s 响应失败: %w", upstream, readErr)
					}
				} else {
					lastErr = fmt.Errorf("上游 DNS %s 返回空响应", upstream)
				}
			} else {
				lastErr = fmt.Errorf("读取上游 DNS %s 响应长度失败: %w", upstream, readErr)
			}
		} else {
			lastErr = fmt.Errorf("向上游 DNS %s 发送失败: %w", upstream, writeErr)
		}
		_ = conn.Close()
	}
	return nil, "SERVFAIL", lastErr
}

func findWebAuditClientByIP(clients []ClientData, vpnIP string) (ClientData, bool) {
	for _, candidate := range clients {
		if candidate.Vip == vpnIP || candidate.Vip6 == vpnIP {
			return candidate, true
		}
	}
	return ClientData{}, false
}

func preferredAuditIPTables(ipv6 bool) (string, error) {
	base := "iptables"
	if ipv6 {
		base = "ip6tables"
	}
	if legacy, err := exec.LookPath(base + "-legacy"); err == nil && exec.Command(legacy, "-L", "-n", "-t", "nat").Run() == nil {
		return legacy, nil
	}
	if nft, err := exec.LookPath(base + "-nft"); err == nil && exec.Command(nft, "-L", "-n", "-t", "nat").Run() == nil {
		return nft, nil
	}
	if binary, err := exec.LookPath(base); err == nil && exec.Command(binary, "-L", "-n", "-t", "nat").Run() == nil {
		return binary, nil
	}
	return "", fmt.Errorf("未找到可用的 %s，DNS 审计不会截获该协议族流量", base)
}

func auditIngressGuardRules() [][]string {
	return [][]string{
		{"INPUT", "!", "-i", "tun0", "-p", "udp", "--dport", "5353", "-j", "DROP"},
		{"INPUT", "!", "-i", "tun0", "-p", "tcp", "--dport", "5353", "-j", "DROP"},
	}
}

func (s *webAuditDNSService) ensureIngressGuardFamily(enable, ipv6 bool) error {
	ipt, err := preferredAuditIPTables(ipv6)
	if err != nil {
		if !enable {
			return nil
		}
		return err
	}
	for _, rule := range auditIngressGuardRules() {
		check := append([]string{"-t", "filter", "-C"}, rule...)
		exists := exec.Command(ipt, check...).Run() == nil
		if enable && !exists {
			add := append([]string{"-t", "filter", "-I"}, rule...)
			if out, err := exec.Command(ipt, add...).CombinedOutput(); err != nil {
				return fmt.Errorf("安装 DNS %s 入口防护失败: %s", map[bool]string{false: "IPv4", true: "IPv6"}[ipv6], strings.TrimSpace(string(out)))
			}
		}
		if !enable && exists {
			del := append([]string{"-t", "filter", "-D"}, rule...)
			_ = exec.Command(ipt, del...).Run()
		}
	}
	return nil
}

func (s *webAuditDNSService) ensureIngressGuard(enable bool) error {
	s.mu.RLock()
	v6Ready := s.status.IPv6ListenerReady
	s.mu.RUnlock()
	v4Err := s.ensureIngressGuardFamily(enable, false)
	v6Err := s.ensureIngressGuardFamily(enable && v6Ready, true)
	s.mu.Lock()
	s.status.IngressRestricted = enable && v4Err == nil && (!v6Ready || v6Err == nil)
	s.updateCoverageLocked()
	s.mu.Unlock()
	if v4Err != nil {
		return v4Err
	}
	return v6Err
}

func auditRedirectRules(ipv6 bool, upstreams []string) [][]string {
	rules := make([][]string, 0, len(upstreams)*2)
	for _, upstream := range upstreams {
		ip := net.ParseIP(upstream)
		if ip == nil || (ip.To4() == nil) == !ipv6 {
			continue
		}
		for _, proto := range []string{"udp", "tcp"} {
			rules = append(rules, []string{"-t", "nat", "PREROUTING", "-i", "tun0", "-p", proto, "-d", upstream, "--dport", "53", "-j", "REDIRECT", "--to-ports", "5353"})
		}
	}
	return rules
}

func legacyAuditRedirectRules() [][]string {
	return [][]string{{"-t", "nat", "PREROUTING", "-i", "tun0", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "5353"}, {"-t", "nat", "PREROUTING", "-i", "tun0", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", "5353"}}
}

// auditRedirectRuleArgs returns a removable PREROUTING rule only when it has
// the exact ownership signature used by this feature. It intentionally accepts
// any destination address so a process restart can remove an orphaned redirect
// after the configured upstream DNS servers have changed or the audit switch
// was turned off while the web process was unavailable.
func auditRedirectRuleArgs(line string) ([]string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "-A" || fields[1] != "PREROUTING" {
		return nil, false
	}
	values := make(map[string]string, 5)
	for i := 2; i+1 < len(fields); i++ {
		switch fields[i] {
		case "-i", "-p", "--dport", "-j", "--to-ports":
			values[fields[i]] = fields[i+1]
			i++
		}
	}
	if values["-i"] != "tun0" || (values["-p"] != "udp" && values["-p"] != "tcp") || values["--dport"] != "53" || values["-j"] != "REDIRECT" || values["--to-ports"] != "5353" {
		return nil, false
	}
	return append([]string{"-t", "nat"}, fields[1:]...), true
}

// discoverAuditRedirectRules finds stale audit redirect rules without relying
// on the current config. This is necessary because NAT state survives a web
// process restart, while in-memory status does not.
func discoverAuditRedirectRules(ipt string) [][]string {
	out, err := exec.Command(ipt, "-t", "nat", "-S", "PREROUTING").Output()
	if err != nil {
		return nil
	}
	rules := make([][]string, 0)
	for _, line := range strings.Split(string(out), "\n") {
		if rule, ok := auditRedirectRuleArgs(line); ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func (s *webAuditDNSService) ensureRedirectFamily(enable, ipv6 bool, upstreams []string) error {
	ipt, err := preferredAuditIPTables(ipv6)
	if err != nil {
		if !enable {
			return nil
		}
		return err
	}
	rules := auditRedirectRules(ipv6, upstreams)
	if !enable {
		// Remove both the rules we can derive from the old status snapshot and
		// any matching orphan that survived a previous web process restart.
		// The latter covers disabling the feature through a direct config-file
		// edit before a restart, when status has no old upstream list yet.
		rules = append(rules, legacyAuditRedirectRules()...)
		rules = append(rules, discoverAuditRedirectRules(ipt)...)
	}
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		key := strings.Join(rule, "\x00")
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		check := append([]string{"-t", "nat", "-C"}, rule[2:]...)
		exists := exec.Command(ipt, check...).Run() == nil
		if enable && !exists {
			add := append([]string{"-t", "nat", "-A"}, rule[2:]...)
			if out, err := exec.Command(ipt, add...).CombinedOutput(); err != nil {
				_ = s.ensureRedirectFamily(false, ipv6, upstreams)
				return fmt.Errorf("安装 %s DNS 重定向失败: %s", map[bool]string{false: "IPv4", true: "IPv6"}[ipv6], strings.TrimSpace(string(out)))
			}
		}
		if !enable && exists {
			del := append([]string{"-t", "nat", "-D"}, rule[2:]...)
			_ = exec.Command(ipt, del...).Run()
		}
	}
	return nil
}

func (s *webAuditDNSService) ensureRedirectWithUpstreams(enable bool, upstreams []string) error {
	s.mu.RLock()
	v6Ready := s.status.IPv6ListenerReady
	s.mu.RUnlock()
	v4Err := s.ensureRedirectFamily(enable, false, upstreams)
	v6Err := s.ensureRedirectFamily(enable && v6Ready, true, upstreams)
	s.mu.Lock()
	s.status.IPv4RedirectInstalled = enable && v4Err == nil && len(auditRedirectRules(false, upstreams)) > 0
	s.status.RedirectInstalled = s.status.IPv4RedirectInstalled
	s.status.IPv6RedirectInstalled = enable && v6Ready && v6Err == nil && len(auditRedirectRules(true, upstreams)) > 0
	s.updateCoverageLocked()
	s.mu.Unlock()
	if v4Err != nil {
		return v4Err
	}
	if v6Err != nil {
		return v6Err
	}
	return nil
}

func (s *webAuditDNSService) ensureRedirect(enable bool) error {
	s.mu.RLock()
	upstreams := append([]string(nil), s.status.UpstreamDNS...)
	s.mu.RUnlock()
	return s.ensureRedirectWithUpstreams(enable, upstreams)
}
