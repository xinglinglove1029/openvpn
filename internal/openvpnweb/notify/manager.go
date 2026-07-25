package notify

import (
	"context"
	"encoding/json"
	"sync"
)

// Manager 渠道管理器：负责注册、查找、并发派发
type Manager struct {
	mu        sync.RWMutex
	notifiers map[string]Notifier
}

var globalManager = &Manager{
	notifiers: make(map[string]Notifier),
}

// Register 注册一个渠道实现
func (m *Manager) Register(n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers[n.Type()] = n
}

// Get 取出指定渠道的实现
func (m *Manager) Get(channelType string) (Notifier, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, ok := m.notifiers[channelType]
	return n, ok
}

// Types 返回所有已注册渠道类型
func (m *Manager) Types() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.notifiers))
	for k := range m.notifiers {
		out = append(out, k)
	}
	return out
}

// DispatchItem 单条渠道派发结果
type DispatchItem struct {
	ChannelID   uint
	ChannelName string
	ChannelType string
	Success     bool
	Error       string
}

// Dispatch 把消息并发派发到若干渠道，返回每条渠道的发送结果
// 不返回 error；调用方遍历 results 决定日志
func (m *Manager) Dispatch(
	ctx context.Context,
	channels []Channel,
	msg Message,
) []DispatchItem {
	items := make([]DispatchItem, 0, len(channels))
	if len(channels) == 0 {
		return items
	}
	type result struct {
		idx   int
		succ  bool
		err   string
	}
	resCh := make(chan result, len(channels))
	var wg sync.WaitGroup
	for i, ch := range channels {
		if !ch.Enabled {
			continue
		}
		n, ok := m.Get(ch.Type)
		if !ok {
			items = append(items, DispatchItem{
				ChannelID: ch.ID, ChannelName: ch.Name, ChannelType: ch.Type,
				Success: false, Error: "未注册的渠道类型：" + ch.Type,
			})
			continue
		}
		wg.Add(1)
		go func(i int, ch Channel) {
			defer wg.Done()
			var raw json.RawMessage
			if len(ch.Config) > 0 {
				raw = ch.Config
			} else {
				raw = json.RawMessage("{}")
			}
			err := n.Send(ctx, msg, raw)
			if err != nil {
				resCh <- result{idx: i, succ: false, err: err.Error()}
			} else {
				resCh <- result{idx: i, succ: true}
			}
		}(i, ch)
	}
	wg.Wait()
	close(resCh)
	for r := range resCh {
		ch := channels[r.idx]
		items = append(items, DispatchItem{
			ChannelID:   ch.ID,
			ChannelName: ch.Name,
			ChannelType: ch.Type,
			Success:     r.succ,
			Error:       r.err,
		})
	}
	return items
}

// Global 返回全局 Manager（应用启动时 Register 一次）
func Global() *Manager { return globalManager }

// Channel 派发所需的最小字段（避免 import 循环）
type Channel struct {
	ID      uint
	Name    string
	Type    string
	Enabled bool
	Config  json.RawMessage
}
