package openvpnweb

import (
	"log"
	"reflect"
	"sync"
	"sync/atomic"
)

// EventBus 全局应用事件总线。
//
// 业务模块通过 Bus().Publish("topic", payload) 解耦地发出事件，
// 由 WsHub 订阅后转发给 WebSocket 客户端。
//
// 主题命名规范：`<业务域>:<事件>`，例如：
//   - notify:new          通知渠道产生新日志
//   - audit:new           审计日志新增
//   - cert:expiring       证书即将过期
//   - cert:expired        证书已过期
//   - user:login          用户登录
//   - system:announcement 系统公告
//
// payload 必须是可被 encoding/json 序列化的值。
type EventBus struct {
	mu        sync.RWMutex
	listeners map[string][]*busHandler
	nextID    atomic.Uint64
}

type busHandler struct {
	id      uint64
	fn      func(payload any)
	hash    uintptr
	topic   string
}

var globalBus = &EventBus{
	listeners: make(map[string][]*busHandler),
}

// Bus 返回全局事件总线单例
func Bus() *EventBus {
	return globalBus
}

// Subscribe 订阅一个主题；返回取消订阅的函数
func (b *EventBus) Subscribe(topic string, handler func(payload any)) func() {
	if topic == "" || handler == nil {
		return func() {}
	}
	id := b.nextID.Add(1)
	h := &busHandler{
		id:    id,
		fn:    handler,
		hash:  reflect.ValueOf(handler).Pointer(),
		topic: topic,
	}
	b.mu.Lock()
	b.listeners[topic] = append(b.listeners[topic], h)
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			list := b.listeners[topic]
			for i, item := range list {
				if item.id == id {
					b.listeners[topic] = append(list[:i], list[i+1:]...)
					if len(b.listeners[topic]) == 0 {
						delete(b.listeners, topic)
					}
					return
				}
			}
		})
	}
}

// Publish 异步分发事件给所有订阅者
func (b *EventBus) Publish(topic string, payload any) {
	if topic == "" {
		return
	}
	b.mu.RLock()
	list := append([]*busHandler{}, b.listeners[topic]...)
	b.mu.RUnlock()

	for _, h := range list {
		go func(handler *busHandler) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event bus handler panic topic=%s id=%d: %v", topic, handler.id, r)
				}
			}()
			handler.fn(payload)
		}(h)
	}
}

// TopicCount 调试用，返回当前主题数
func (b *EventBus) TopicCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.listeners)
}

// HandlerCount 调试用，返回某主题订阅者数量
func (b *EventBus) HandlerCount(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.listeners[topic])
}
