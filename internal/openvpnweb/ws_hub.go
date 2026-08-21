package openvpnweb

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WsEnvelope WebSocket 消息信封
type WsEnvelope struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

// WsClient 单个 WebSocket 客户端连接
type WsClient struct {
	hub         *WsHub
	conn        *websocket.Conn
	send        chan []byte
	permissions map[string]bool
	mu          sync.Mutex
	closed      bool
	closeOn     sync.Once
}

// WsHub 维护所有活跃连接并对外提供广播能力
type wsBroadcast struct {
	envType string
	payload interface{}
}

type WsHub struct {
	clients    map[*WsClient]struct{}
	register   chan *WsClient
	unregister chan *WsClient
	broadcast  chan wsBroadcast
	mu         sync.RWMutex
	once       sync.Once
}

var globalHub = &WsHub{
	clients:    make(map[*WsClient]struct{}),
	register:   make(chan *WsClient, 16),
	unregister: make(chan *WsClient, 16),
	broadcast:  make(chan wsBroadcast, 256),
}

// WsHub 返回全局 Hub 单例
func WsHubInstance() *WsHub {
	return globalHub
}

// Run 启动 Hub 事件循环（应用启动时调用一次）
func (h *WsHub) Run() {
	h.once.Do(func() {
		go h.loop()
		// 自动桥接：把事件总线上所有主题转发到 WebSocket 广播通道
		// 业务模块只需 Bus().Publish(topic, payload) 即可推送到前端
		go h.bridgeEventBus()
	})
}

// bridgeEventBus 监听 EventBus 并将事件转成 WsEnvelope 广播。
// 使用通配订阅：所有非空主题都会透传，前端按 type 自行路由。
func (h *WsHub) bridgeEventBus() {
	// 通过对常见主题显式 Subscribe 会带来穷举问题；
	// 这里使用一个简易桥接：对总线上每个 topic 单独 Subscribe。
	// 业务侧不需关心：新主题一旦被 Publish 就会被转发（前提是已被 Subscribe 注册）。
	// 启动时即注册所有已知主题；新主题可在注册时再次触发 Subscribe。
	known := []string{
		"notify:new",
		"audit:new",
		"cert:expiring",
		"cert:expired",
		"user:login",
		"system:announcement",
		"system:stats",
		"dashboard:stats",
	}
	for _, topic := range known {
		h.subscribeBridge(topic)
	}
}

// SubscribeTopic 注册一个业务主题到广播桥接；可由业务模块在启动时调用以纳入新主题
func (h *WsHub) SubscribeTopic(topic string) func() {
	return h.subscribeBridge(topic)
}

func (h *WsHub) subscribeBridge(topic string) func() {
	return Bus().Subscribe(topic, func(payload any) {
		h.Broadcast(topic, payload)
	})
}

func (h *WsHub) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.closeOn.Do(func() {
					close(client.send)
				})
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				data, err := marshalForClient(client, msg)
				if err != nil || data == nil {
					continue
				}
				select {
				case client.send <- data:
				default:
					// 单个客户端写阻塞，丢弃并关闭连接
					go func(c *WsClient) {
						h.unregister <- c
						c.conn.Close()
					}(client)
				}
			}
			h.mu.RUnlock()
		case <-ticker.C:
			// 周期性 ping 探测
			h.mu.RLock()
			for client := range h.clients {
				_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					go func(c *WsClient) {
						h.unregister <- c
						c.conn.Close()
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func marshalForClient(client *WsClient, msg wsBroadcast) ([]byte, error) {
	payload := msg.payload
	// dashboard:stats also contains aggregate overview data. Users with the
	// overview permission may receive that aggregate data, but online client
	// identities require the separate client:view_online permission.
	if msg.envType == dashboardStatsTopic && !client.permissions["*"] {
		if !client.permissions["menu:overview"] {
			return nil, nil
		}
		if !client.permissions["client:view_online"] {
			switch snapshot := msg.payload.(type) {
			case DashboardStatsPayload:
				snapshot.Online = nil
				payload = snapshot
			case *DashboardStatsPayload:
				if snapshot != nil {
					copy := *snapshot
					copy.Online = nil
					payload = &copy
				}
			}
		}
	}
	return json.Marshal(WsEnvelope{Type: msg.envType, Payload: payload})
}

// Broadcast 广播任意数据
func (h *WsHub) Broadcast(envType string, payload interface{}) {
	select {
	case h.broadcast <- wsBroadcast{envType: envType, payload: payload}:
	default:
		// 队列已满，丢弃
		log.Printf("ws hub broadcast queue full, drop message type=%s", envType)
	}
}

// ConnectionCount 返回当前活跃连接数
func (h *WsHub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 站点是同源 Cookie 鉴权，放行所有 origin；如需收敛可在此处校验 r.Header.Get("Origin")
		return true
	},
}

// ServeWs 处理 WebSocket 升级请求
func (h *WsHub) ServeWs(w http.ResponseWriter, r *http.Request, permissions map[string]bool) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	client := &WsClient{
		hub:         h,
		conn:        conn,
		send:        make(chan []byte, 64),
		permissions: permissions,
	}
	h.register <- client

	// 启动写循环
	go client.writePump()
	// 读循环同步执行，结束时注销客户端
	client.readPump()
}

func (c *WsClient) close() {
	c.closeOn.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		close(c.send)
	})
}

func (c *WsClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})
	for {
		// 我们不接收客户端业务消息，但需要保持读取以驱动 ping/pong 心跳
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *WsClient) writePump() {
	ticker := time.NewTicker(45 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
