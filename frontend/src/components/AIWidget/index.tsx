import { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTheme } from '@/store/theme';
import { useAuth } from '@/store/auth';
import { api } from '@/api';
import { realtimeHub } from '@/lib/notificationHub';
import { SSEParser } from '@/lib/sse';
import { Button } from '@/ui/button';
import { Textarea } from '@/ui/textarea';
import { Card, CardContent } from '@/ui/card';
import MarkdownContent from '@/components/MarkdownContent';
import { toast } from 'sonner';
import {
  Bot,
  Send,
  Loader2,
  X,
  Plus,
  Maximize2,
  Minimize2,
  Sparkles,
  Flame,
  Lightbulb,
  Settings as SettingsIcon,
} from 'lucide-react';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

interface HealthStatus {
  available: boolean;
  model: string;
  provider?: string;
  error?: string;
  checkedAt?: string;
}


interface PersistedChat {
  messages: ChatMessage[];
  sessionID: string;
}

interface HistoryResponse {
  session_id: string;
  messages: ChatMessage[];
}

const CHAT_STORAGE_KEY_PREFIX = 'openvpn-ai-widget-chat';

function chatStorageKey(username?: string): string {
  return username ? `${CHAT_STORAGE_KEY_PREFIX}:${username}` : '';
}

function loadPersistedChat(storageKey: string): PersistedChat {
  if (!storageKey) return { messages: [], sessionID: '' };
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return { messages: [], sessionID: '' };
    const parsed = JSON.parse(raw) as PersistedChat;
    if (!Array.isArray(parsed.messages)) return { messages: [], sessionID: '' };
    return { messages: parsed.messages, sessionID: parsed.sessionID || '' };
  } catch {
    return { messages: [], sessionID: '' };
  }
}

const HOT_TOPICS = [
  '查看当前在线客户端',
  '查看当前防火墙规则',
  '查看系统运行概况',
];

const RECOMMENDATIONS = [
  '查看服务器实时资源状态',
  '查询近期审计日志',
];

export default function AIWidget() {
  const navigate = useNavigate();
  const { theme } = useTheme();
  const { hasPermission, aiEnabled, user } = useAuth();
  const storageKey = chatStorageKey(user?.username);
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionID, setSessionID] = useState('');
  const [deepThink, setDeepThink] = useState(false);
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const [historyReadyFor, setHistoryReadyFor] = useState('');
  // 记录上一次 WS 推送的 available 状态，用于"仅从可用→不可用时弹出提示"
  const prevAvailableRef = useRef<boolean | null>(null);

  // 健康状态订阅：
  // 1. 组件挂载时拉取一次 /ovpn/ai/health 作为初始值（避免 WS 未连接时显示空白）
  // Restore the current user's fallback cache first, then replace it with SQLite history.
  // Do not write to localStorage until this restoration completes, or an empty initial
  // React state could overwrite an existing fallback before it is read.
  useEffect(() => {
    let cancelled = false;
    if (!storageKey) {
      setMessages([]);
      setSessionID('');
      setHistoryReadyFor('');
      return () => {
        cancelled = true;
      };
    }

    const fallback = loadPersistedChat(storageKey);
    setMessages(fallback.messages);
    setSessionID(fallback.sessionID);
    setHistoryReadyFor('');

    const endpoint = fallback.sessionID
      ? `/ovpn/ai/history?session_id=${encodeURIComponent(fallback.sessionID)}`
      : '/ovpn/ai/history';
    api
      .get<HistoryResponse>(endpoint)
      .then((history) => {
        if (cancelled) return;
        // A successful server response is authoritative, including an empty history.
        setSessionID(history.session_id || '');
        setMessages(history.messages || []);
      })
      .catch(() => {
        // Keep the per-user local fallback if the durable history request fails.
      })
      .finally(() => {
        if (!cancelled) setHistoryReadyFor(storageKey);
      });

    return () => {
      cancelled = true;
    };
  }, [storageKey]);

  // LocalStorage is only an immediate, per-user fallback for unavailable history requests.
  useEffect(() => {
    if (!storageKey || historyReadyFor !== storageKey) return;
    try {
      if (!sessionID && messages.length === 0) {
        localStorage.removeItem(storageKey);
        return;
      }
      localStorage.setItem(
        storageKey,
        JSON.stringify({ messages, sessionID } satisfies PersistedChat),
      );
    } catch {
      // Browser storage may be disabled or full; SQLite remains authoritative.
    }
  }, [historyReadyFor, messages, sessionID, storageKey]);

  useEffect(() => {
    let cancelled = false;
    // 初始拉取（静默，不弹提示）
    api
      .get<HealthStatus>('/ovpn/ai/health')
      .then((data) => {
        if (cancelled) return;
        setHealth(data);
        prevAvailableRef.current = data?.available ?? false;
      })
      .catch(() => {
        // 静默失败，等待 WS 推送
      });

    // 订阅 WS ai:health 事件（后台周期自检 + 状态变更时推送）
    const off = realtimeHub.subscribe<HealthStatus>('ai:health', (status) => {
      if (!status) return;
      setHealth(status);
      const prev = prevAvailableRef.current;
      const current = !!status.available;
      // 仅在 从可用 → 不可用 时弹出提示（避免启动时初始不可用就弹提示）
      if (prev === true && current === false) {
        toast.error(`AI 服务不可用：${status.error || '请检查 AI 配置'}`);
      }
      prevAvailableRef.current = current;
    });

    // 订阅 WS ai:session_reset 事件（AI 热切换/禁用后旧 sessionID 失效，前端清空会话）
    const offReset = realtimeHub.subscribe<{ reason?: string; message?: string }>(
      'ai:session_reset',
      (payload) => {
        setMessages([]);
        setSessionID('');
        setStreaming('');
        setInput('');
        try {
          if (storageKey) localStorage.removeItem(storageKey);
        } catch {
          // SQLite remains authoritative when browser storage is unavailable.
        }
        toast.info(payload?.message || 'AI configuration updated; the next message will start a new session');
      },
    );

    // 订阅 WS ws:reconnected 事件（断连重连后重新拉取 health 状态，避免丢失断连期间变更）
    const offReconnect = realtimeHub.subscribe<null>('ws:reconnected', () => {
      api
        .get<HealthStatus>('/ovpn/ai/health')
        .then((data) => {
          if (cancelled) return;
          setHealth(data);
          prevAvailableRef.current = data?.available ?? false;
        })
        .catch(() => {
          // 静默失败
        });
    });

    return () => {
      cancelled = true;
      off();
      offReset();
      offReconnect();
    };
  }, [storageKey]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streaming]);

  // 打开时聚焦输入框
  useEffect(() => {
    if (open) {
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [open]);

  const sendMessage = async (textOverride?: string) => {
    const text = (textOverride ?? input).trim();
    if (!text || loading || historyReadyFor !== storageKey) return;

    setInput('');
    const userMsg: ChatMessage = { role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);

    setLoading(true);
    setStreaming('');

    const controller = new AbortController();
    abortRef.current = controller;

    // fullText 提升到 try 块外，使 catch 块也能访问（用于 AbortError 中断时保存已接收内容）
    let fullText = '';

    try {
      const response = await fetch('/ovpn/ai/chat', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json; charset=UTF-8' },
        body: JSON.stringify({ message: text, session_id: sessionID }),
        signal: controller.signal,
      });

      if (!response.ok) {
        const ct = response.headers.get('content-type') || '';
        if (ct.includes('application/json')) {
          const err = await response.json();
          throw new Error(err.message || '请求失败');
        }
        throw new Error(`AI 服务异常 (${response.status})`);
      }

      const reader = response.body?.getReader();
      if (!reader) throw new Error('不支持流式响应');

      const parser = new SSEParser();

      const handleEvent = ({ event: eventType, data }: { event: string; data: string }) => {
        switch (eventType) {
          case 'session':
            setSessionID(data);
            break;
          case 'token':
            fullText += data;
            setStreaming(fullText);
            break;
          case 'tool_call':
            // 工具调用开始：展示"正在执行XX操作..."
            setStreaming(`正在执行：${data}...`);
            break;
          case 'tool_result':
            // 工具调用结果：展示结果摘要
            setStreaming(`操作结果：${data}`);
            break;
          case 'done':
            if (fullText) {
              setMessages((prev) => [...prev, { role: 'assistant', content: fullText }]);
            }
            setStreaming('');
            break;
          case 'error':
            toast.error(data || 'AI 服务返回错误');
            break;
        }
      };

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        for (const event of parser.push(value)) {
          handleEvent(event);
        }
      }

      for (const event of parser.finish()) {
        handleEvent(event);
      }
    } catch (err: any) {
      if (err.name === 'AbortError') {
        // 用 fullText 局部变量而非 streaming state（避免闭包捕获过期值）
        if (fullText) {
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: fullText + ' [已中断]' },
          ]);
        }
      } else {
        toast.error(err.message || 'AI 服务不可用');
        setMessages((prev) => {
          const msgs = [...prev];
          if (
            msgs.length > 0 &&
            msgs[msgs.length - 1].role === 'user' &&
            msgs[msgs.length - 1].content === text
          ) {
            msgs.pop();
          }
          return msgs;
        });
        setStreaming('');
      }
    } finally {
      setLoading(false);
      abortRef.current = null;
    }
  };

  const cancelStream = () => {
    abortRef.current?.abort();
    abortRef.current = null;
  };

  const newChat = async () => {
    if (loading || historyReadyFor !== storageKey) return;
    const currentSessionID = sessionID;
    if (currentSessionID) {
      try {
        await api.delete(`/ovpn/ai/history?session_id=${encodeURIComponent(currentSessionID)}`);
      } catch (err: any) {
        toast.error(err.message || 'Failed to clear AI chat history');
        return;
      }
    }
    setMessages([]);
    setSessionID('');
    setStreaming('');
    setInput('');
    try {
      if (storageKey) localStorage.removeItem(storageKey);
    } catch {
      // SQLite remains authoritative when browser storage is unavailable.
    }
    setTimeout(() => inputRef.current?.focus(), 100);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  // AI 助手未启用或用户无 AI 聊天权限则不渲染
  if (!aiEnabled || !hasPermission('ai:chat')) {
    return null;
  }

  const available = health?.available ?? false;
  const isDark = theme !== 'daylight';

  return (
    <>
      {/* 悬浮入口 */}
      {!open && (
        <button
          onClick={() => setOpen(true)}
          className={`
            fixed bottom-6 right-6 z-50
            flex items-center gap-2
            px-4 py-3 rounded-full
            shadow-lg shadow-black/20
            transition-all duration-200
            hover:scale-105 hover:shadow-xl
            active:scale-95
            ${isDark ? 'bg-white text-black' : 'bg-[#4b70e2] text-white'}
          `}
          aria-label="打开 AI 助手"
        >
          <Sparkles className="h-5 w-5" />
          <span className="font-medium text-sm">AI 助理</span>
        </button>
      )}

      {/* 遮罩层：防止点击穿透背景页面，在抽屉面板打开时始终显示 */}
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm animate-in fade-in duration-200"
          onClick={() => setOpen(false)}
        />
      )}
      {/* 抽屉面板 */}
      {open && (
        <div
          className={`
            fixed top-0 bottom-0 z-50
            transition-all duration-300 ease-out
            ${expanded ? 'right-0 w-full sm:w-[720px]' : 'right-0 w-full sm:w-[440px]'}
            flex flex-col
            bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80
            border-l shadow-2xl
          `}
        >
          {/* 头部 */}
          <header className="flex items-center justify-between px-4 py-3 border-b shrink-0">
            <div className="flex items-center gap-2">
              <div className="h-8 w-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                <Bot className="h-5 w-5" />
              </div>
              <span className="font-semibold text-base">AI 助理</span>
              {health && (
                <span
                  className={`ml-1 inline-block h-2 w-2 rounded-full ${
                    available ? 'bg-emerald-500' : 'bg-destructive'
                  }`}
                  title={available ? 'AI 服务正常' : health.error || 'AI 服务不可用'}
                />
              )}
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={newChat}
                title="新会话"
              >
                <Plus className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 hidden sm:flex"
                onClick={() => setExpanded(!expanded)}
                title={expanded ? '缩小' : '扩大'}
              >
                {expanded ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
              </Button>
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => setOpen(false)}
                title="关闭"
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          </header>

          {/* 消息区域 */}
          <div
            ref={scrollRef}
            className="flex-1 overflow-y-auto p-4 space-y-4 scroll-smooth"
          >
            {messages.length === 0 && !streaming && (
              <div className="space-y-5 animate-in fade-in duration-300">
                {/* 欢迎语 */}
                <div className="flex flex-col items-start gap-3 pt-2">
                  <div className="h-12 w-12 rounded-2xl bg-gradient-to-br from-violet-500 to-blue-500 flex items-center justify-center text-white shadow-lg">
                    <Sparkles className="h-7 w-7" />
                  </div>
                  <h2 className="text-2xl font-bold leading-tight">
                    你好，我是 <span className="text-transparent bg-clip-text bg-gradient-to-r from-violet-500 to-blue-500">AI 运维助手</span>
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    基于大模型的 OpenVPN 智能运维问答助手，可协助配置、排障与分析。
                  </p>
                </div>

                {/* 近期热点 */}
                <div>
                  <div className="flex items-center gap-2 mb-3 text-sm font-medium text-foreground">
                    <Flame className="h-4 w-4 text-orange-500" />
                    <span>近期热点</span>
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {HOT_TOPICS.map((q) => (
                      <button
                        key={q}
                        onClick={() => sendMessage(q)}
                        disabled={!available || loading || historyReadyFor !== storageKey}
                        className="text-left px-3 py-2.5 rounded-lg border bg-card hover:bg-accent transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-card"
                      >
                        {q}
                      </button>
                    ))}
                  </div>
                </div>

                {/* 推荐解决方案 */}
                <div>
                  <div className="flex items-center gap-2 mb-3 text-sm font-medium text-foreground">
                    <Lightbulb className="h-4 w-4 text-amber-500" />
                    <span>推荐解决方案</span>
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {RECOMMENDATIONS.map((q) => (
                      <button
                        key={q}
                        onClick={() => sendMessage(q)}
                        disabled={!available || loading || historyReadyFor !== storageKey}
                        className="text-left px-3 py-2.5 rounded-lg border bg-card hover:bg-accent transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-card"
                      >
                        {q}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {/* 消息列表 */}
            {messages.map((msg, i) => (
              <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <Card
                  className={`max-w-[90%] ${
                    msg.role === 'user'
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-card border shadow-sm'
                  }`}
                >
                  <CardContent className="p-3">
                    {msg.role === 'user' ? (
                      <p className="text-sm whitespace-pre-wrap">{msg.content}</p>
                    ) : (
                      <div className="prose prose-sm dark:prose-invert max-w-none">
                        <MarkdownContent content={msg.content} />
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            ))}

            {/* 流式消息 */}
            {streaming && (
              <div className="flex justify-start">
                <Card className="max-w-[90%] bg-card border shadow-sm">
                  <CardContent className="p-3">
                    <div className="prose prose-sm dark:prose-invert max-w-none">
                      <MarkdownContent content={streaming} />
                    </div>
                    <span className="inline-block w-1.5 h-4 bg-primary animate-pulse ml-1 rounded-sm" />
                  </CardContent>
                </Card>
              </div>
            )}

            {/* 思考中 */}
            {loading && !streaming && (
              <div className="flex justify-start">
                <Card className="bg-card border shadow-sm">
                  <CardContent className="p-3 flex items-center gap-2">
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                    <span className="text-sm text-muted-foreground">
                      {deepThink ? 'AI 正在深度思考...' : 'AI 正在思考...'}
                    </span>
                  </CardContent>
                </Card>
              </div>
            )}
          </div>

          {/* 输入区域 */}
          <div className="shrink-0 p-4 border-t bg-card/50">
            {!available ? (
              <div className="rounded-xl border bg-destructive/5 p-4 space-y-3">
                <div className="flex items-start gap-3">
                  <div className="h-8 w-8 rounded-lg bg-destructive/10 flex items-center justify-center text-destructive shrink-0">
                    <SettingsIcon className="h-4 w-4" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground">
                      AI 服务尚未配置或不可用
                    </p>
                    <p className="text-xs text-muted-foreground mt-1">
                      {health?.error || '请先在系统设置中配置 AI 服务的 API 密钥与模型参数。'}
                    </p>
                  </div>
                </div>
                {hasPermission('settings:ai') && (
                  <Button
                    size="sm"
                    className="w-full"
                    onClick={() => {
                      setOpen(false);
                      setTimeout(() => navigate('/settings'), 150);
                    }}
                  >
                    <SettingsIcon className="h-4 w-4 mr-2" />
                    前往 AI 助手设置
                  </Button>
                )}
              </div>
            ) : (
              <div className="relative rounded-xl border bg-background shadow-sm">
                <Textarea
                  ref={inputRef}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="请您遇到的问题告诉我，使用 Shift + Enter 换行"
                  disabled={loading || historyReadyFor !== storageKey}
                  rows={3}
                  className="min-h-[80px] resize-none border-0 bg-transparent pr-12 pb-9 focus-visible:ring-0 focus-visible:ring-offset-0 disabled:opacity-60 disabled:cursor-not-allowed"
                />
                <div className="absolute left-2.5 bottom-2 flex items-center gap-2">
                  <button
                    onClick={() => setDeepThink(!deepThink)}
                    disabled={loading || historyReadyFor !== storageKey}
                    className={`
                      flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium transition-colors
                      ${
                        deepThink
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-muted text-muted-foreground hover:bg-accent'
                      }
                      disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-muted
                    `}
                  >
                    <Sparkles className="h-3 w-3" />
                    深度思考
                  </button>
                </div>
                <div className="absolute right-2.5 bottom-2">
                  {loading ? (
                    <Button size="icon" variant="outline" className="h-8 w-8" onClick={cancelStream}>
                      <Loader2 className="h-4 w-4 animate-spin" />
                    </Button>
                  ) : (
                    <Button
                      size="icon"
                      className="h-8 w-8"
                      disabled={!input.trim() || historyReadyFor !== storageKey}
                      onClick={() => sendMessage()}
                    >
                      <Send className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            )}
            <p className="text-[10px] text-muted-foreground text-center mt-2">
              内容由 AI 生成，仅供参考，请据此所作判断及操作均由您自行承担责任。
            </p>
          </div>
        </div>
      )}
    </>
  );
}
