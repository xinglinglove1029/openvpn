/**
 * 通用实时事件 Hub（WebSocket 客户端）
 *
 * 与后端 /ovpn/ws/notifications 建立连接，断线自动重连，并按 topic 派发消息。
 * 任何业务模块都可以通过 subscribe(topic, handler) 订阅自己的事件流。
 *
 * 主题命名规范：`<业务域>:<事件>`，例如：
 *   - notify:new          通知渠道产生新日志
 *   - audit:new           审计日志新增
 *   - cert:expiring       证书即将过期
 *   - system:announcement 系统公告
 *
 * 用法：
 *   const off = realtimeHub.subscribe<MyPayload>('audit:new', (data) => { ... });
 *   // 组件卸载时 off() 即可
 */

import { api } from '@/api';

export type ConnectionState = 'idle' | 'connecting' | 'open' | 'closed';

export interface IncomingNotification {
  id: number;
  event: string;
  provider: string;
  username: string;
  success: boolean;
  message: string;
  createdAt: string;
}

export interface UnreadSnapshot {
  unread: number;
  lastReadId: number;
  maxId: number;
}

interface WsEnvelope<T = unknown> {
  type: string;
  payload?: T;
}

type Listener<T> = (value: T) => void;

class RealtimeHub {
  private socket: WebSocket | null = null;
  private state: ConnectionState = 'idle';
  private listeners = new Map<string, Set<Listener<unknown>>>();
  private stateListeners = new Set<Listener<ConnectionState>>();
  private retryDelay = 1500;
  private maxRetryDelay = 15000;
  private retryTimer: number | null = null;
  private manualClose = false;
  private visibilityHandler: (() => void) | null = null;

  // 未读数 / 站内信特定数据（与 notify 模块耦合，保留为可选辅助）
  private unread: UnreadSnapshot = { unread: 0, lastReadId: 0, maxId: 0 };
  private latestNotification: IncomingNotification | null = null;
  private unreadListeners = new Set<Listener<UnreadSnapshot>>();
  private notificationListeners = new Set<Listener<IncomingNotification>>();

  getState(): ConnectionState {
    return this.state;
  }

  getUnread(): UnreadSnapshot {
    return this.unread;
  }

  getLatestNotification(): IncomingNotification | null {
    return this.latestNotification;
  }

  /** 订阅指定主题；返回取消订阅函数 */
  subscribe<T = unknown>(topic: string, fn: Listener<T>): () => void {
    if (!topic || typeof fn !== 'function') return () => {};
    const key = topic;
    let set = this.listeners.get(key);
    if (!set) {
      set = new Set();
      this.listeners.set(key, set);
    }
    set.add(fn as Listener<unknown>);
    return () => {
      const cur = this.listeners.get(key);
      if (!cur) return;
      cur.delete(fn as Listener<unknown>);
      if (cur.size === 0) this.listeners.delete(key);
    };
  }

  /** 订阅连接状态变化 */
  onState(fn: Listener<ConnectionState>): () => void {
    this.stateListeners.add(fn);
    return () => this.stateListeners.delete(fn);
  }

  /** 兼容旧 API：订阅未读数（仅供通知铃铛使用） */
  onUnread(fn: Listener<UnreadSnapshot>): () => void {
    this.unreadListeners.add(fn);
    return () => {
      this.unreadListeners.delete(fn);
    };
  }

  /** 兼容旧 API：订阅最新一条通知 */
  onNotification(fn: Listener<IncomingNotification>): () => void {
    this.notificationListeners.add(fn);
    return () => {
      this.notificationListeners.delete(fn);
    };
  }

  private emitTopic<T>(topic: string, value: T) {
    const set = this.listeners.get(topic);
    if (!set) return;
    set.forEach((fn) => {
      try {
        (fn as Listener<T>)(value);
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[realtimeHub] listener error', { topic, err });
      }
    });
  }

  private setState(next: ConnectionState) {
    if (this.state === next) return;
    this.state = next;
    this.stateListeners.forEach((fn) => {
      try {
        fn(next);
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[realtimeHub] state listener error', err);
      }
    });
  }

  private setUnread(next: UnreadSnapshot) {
    this.unread = next;
    this.unreadListeners.forEach((fn) => {
      try {
        fn(next);
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[realtimeHub] unread listener error', err);
      }
    });
  }

  private setLatestNotification(next: IncomingNotification) {
    this.latestNotification = next;
    this.notificationListeners.forEach((fn) => {
      try {
        fn(next);
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[realtimeHub] notification listener error', err);
      }
    });
  }

  /** 启动连接；若已连接则忽略 */
  connect(): void {
    if (this.socket && (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)) {
      return;
    }
    this.manualClose = false;
    this.openSocket();

    if (!this.visibilityHandler && typeof document !== 'undefined') {
      this.visibilityHandler = () => {
        if (document.visibilityState === 'visible' && (!this.socket || this.socket.readyState === WebSocket.CLOSED)) {
          this.connect();
        }
      };
      document.addEventListener('visibilitychange', this.visibilityHandler);
    }
  }

  /** 主动关闭连接（登出时调用） */
  disconnect(): void {
    this.manualClose = true;
    if (this.retryTimer !== null) {
      window.clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    if (this.visibilityHandler && typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', this.visibilityHandler);
      this.visibilityHandler = null;
    }
    if (this.socket) {
      try {
        this.socket.close();
      } catch {
        // 忽略
      }
      this.socket = null;
    }
    this.setState('closed');
  }

  private openSocket(): void {
    if (typeof window === 'undefined') return;
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${window.location.host}/ovpn/ws/notifications`;

    this.setState('connecting');
    let socket: WebSocket;
    try {
      socket = new WebSocket(url);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[realtimeHub] construct socket failed', err);
      this.scheduleRetry();
      return;
    }
    this.socket = socket;

    // 防御：连接卡在 CONNECTING 状态超过 10 秒（典型场景：反向代理未开启 ws 透传、服务端没响应）
    // 直接关掉让 onclose 触发 scheduleRetry，而不是永远卡在 connecting。
    const connectingTimer = window.setTimeout(() => {
      if (socket.readyState !== WebSocket.OPEN && socket.readyState !== WebSocket.CLOSED) {
        // eslint-disable-next-line no-console
        console.warn('[realtimeHub] ws stuck in CONNECTING > 10s, force close & retry');
        try {
          socket.close();
        } catch {
          // 忽略
        }
      }
    }, 10_000);

    socket.onopen = () => {
      window.clearTimeout(connectingTimer);
      this.retryDelay = 1500;
      this.setState('open');
      void this.refreshUnread();
    };

    socket.onmessage = (ev) => {
      try {
        const data = JSON.parse(typeof ev.data === 'string' ? ev.data : '') as WsEnvelope;
        const type = data?.type;
        if (!type) return;
        // 通用派发
        this.emitTopic(type, data.payload);
        // 兼容旧 API：notify:new
        if (type === 'notify:new' && data.payload) {
          const item = data.payload as IncomingNotification;
          this.setLatestNotification(item);
          void this.refreshUnread();
        }
      } catch (err) {
        // eslint-disable-next-line no-console
        console.warn('[realtimeHub] parse message failed', err);
      }
    };

    socket.onerror = () => {
      // onclose 通常紧跟其后
    };

    socket.onclose = () => {
      window.clearTimeout(connectingTimer);
      this.socket = null;
      this.setState('closed');
      if (!this.manualClose) {
        this.scheduleRetry();
      }
    };
  }

  private scheduleRetry(): void {
    if (this.retryTimer !== null) return;
    const delay = this.retryDelay;
    this.retryTimer = window.setTimeout(() => {
      this.retryTimer = null;
      this.retryDelay = Math.min(this.retryDelay * 2, this.maxRetryDelay);
      this.openSocket();
    }, delay);
  }

  /** 拉取未读数快照并广播 */
  async refreshUnread(): Promise<UnreadSnapshot | null> {
    try {
      const data = await api.get<UnreadSnapshot>('/ovpn/notify/unread-count');
      const snapshot: UnreadSnapshot = {
        unread: Number(data?.unread ?? 0),
        lastReadId: Number(data?.lastReadId ?? 0),
        maxId: Number(data?.maxId ?? 0),
      };
      this.setUnread(snapshot);
      return snapshot;
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[realtimeHub] fetch unread-count failed', err);
      return null;
    }
  }

  /** 标记已读；可指定 lastReadId，默认推进到最新 */
  async markRead(lastReadId?: number): Promise<UnreadSnapshot | null> {
    try {
      const data = await api.postJson<{ unread: number; lastReadId: number; maxId?: number }>(
        '/ovpn/notify/mark-read',
        { lastReadId: lastReadId ?? 0 },
      );
      const snapshot: UnreadSnapshot = {
        unread: Number(data?.unread ?? 0),
        lastReadId: Number(data?.lastReadId ?? 0),
        maxId: Number(data?.maxId ?? 0),
      };
      this.setUnread(snapshot);
      return snapshot;
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('[realtimeHub] mark-read failed', err);
      return null;
    }
  }
}

export const realtimeHub = new RealtimeHub();

// 兼容旧 import：保持 notificationHub 名字可用
export const notificationHub = realtimeHub;
