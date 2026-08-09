import { useState, useRef, useEffect } from 'react';
import { Bot, Send, Loader2, WifiOff, Wifi, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/ui/button';
import { Textarea } from '@/ui/textarea';
import { Badge } from '@/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/ui/card';
import MarkdownContent from '@/components/MarkdownContent';
import { PageHeader } from '@/components/PageHeader';
import { api } from '@/api';
import { realtimeHub } from '@/lib/notificationHub';
import { SSEParser } from '@/lib/sse';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

interface HealthStatus {
  available: boolean;
  model: string;
  error?: string;
}

const CHAT_STORAGE_KEY = 'openvpn-ai-chat';

interface PersistedChat {
  messages: ChatMessage[];
  sessionID: string;
}

/** 从 localStorage 恢复聊天记录 */
function loadPersistedChat(): PersistedChat {
  try {
    const raw = localStorage.getItem(CHAT_STORAGE_KEY);
    if (!raw) return { messages: [], sessionID: '' };
    const parsed = JSON.parse(raw) as PersistedChat;
    if (!Array.isArray(parsed.messages)) return { messages: [], sessionID: '' };
    return { messages: parsed.messages, sessionID: parsed.sessionID || '' };
  } catch {
    return { messages: [], sessionID: '' };
  }
}

export default function AIAssistant() {
  const [initial] = useState(loadPersistedChat);
  const [messages, setMessages] = useState<ChatMessage[]>(initial.messages);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [sessionID, setSessionID] = useState<string>(initial.sessionID);
  const [health, setHealth] = useState<HealthStatus | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  // 持久化聊天记录到 localStorage（刷新页面后可恢复）
  useEffect(() => {
    try {
      localStorage.setItem(
        CHAT_STORAGE_KEY,
        JSON.stringify({ messages, sessionID } satisfies PersistedChat),
      );
    } catch {
      // localStorage 满或不可用时静默忽略
    }
  }, [messages, sessionID]);

  // 健康状态订阅：
  // 1. 组件挂载时拉取一次 /ovpn/ai/health 作为初始值（避免 WS 未连接时显示空白）
  // 2. 订阅 ai:health WS topic，状态变化时实时更新
  useEffect(() => {
    let cancelled = false;
    api
      .get<HealthStatus>('/ovpn/ai/health')
      .then((data) => {
        if (cancelled) return;
        setHealth(data);
      })
      .catch(() => {
        // 静默失败，等待 WS 推送
      });

    const off = realtimeHub.subscribe<HealthStatus>('ai:health', (status) => {
      if (!status) return;
      setHealth(status);
    });

    // 订阅 WS ai:session_reset 事件（AI 热切换/禁用后旧 sessionID 失效，前端清空会话）
    const offReset = realtimeHub.subscribe<{ reason?: string; message?: string }>(
      'ai:session_reset',
      (payload) => {
        setSessionID('');
        setMessages([]);
        setStreaming('');
        toast.info(payload?.message || 'AI 配置已更新，会话已重置');
      },
    );

    // 订阅 WS ws:reconnected 事件（断连重连后重新拉取 health 状态，避免丢失断连期间变更）
    const offReconnect = realtimeHub.subscribe<null>('ws:reconnected', () => {
      api
        .get<HealthStatus>('/ovpn/ai/health')
        .then((data) => {
          if (cancelled) return;
          setHealth(data);
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
  }, []);

  // 自动滚动到底部
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streaming]);

  // 发送消息
  const sendMessage = async () => {
    const text = input.trim();
    if (!text || loading) return;

    setInput('');

    // 添加用户消息
    const userMsg: ChatMessage = { role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);

    setLoading(true);
    setStreaming('');

    // 创建 AbortController 用于中断
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

      // 检查 HTTP 错误
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
            // 流结束，添加 assistant 消息
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
        // 用户主动中断：用 fullText 局部变量而非 streaming state（避免闭包捕获过期值）
        if (fullText) {
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: fullText + ' [已中断]' },
          ]);
        }
      } else {
        toast.error(err.message || 'AI 服务不可用');
        // 移除刚添加的用户消息（发送失败）
        setMessages((prev) => {
          const msgs = [...prev];
          if (msgs.length > 0 && msgs[msgs.length - 1].role === 'user' &&
              msgs[msgs.length - 1].content === text) {
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

  // 中断生成
  const cancelStream = () => {
    abortRef.current?.abort();
    abortRef.current = null;
  };

  // 清空对话
  const clearChat = () => {
    setMessages([]);
    setSessionID('');
    setStreaming('');
  };

  // 键盘快捷键
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const available = health?.available ?? false;

  return (
    <div className="flex flex-col h-[calc(100vh-6rem)] max-w-4xl mx-auto">
      {/* 顶部标题栏 */}
      <section className="shrink-0 px-4 sm:px-6 pt-4 sm:pt-6">
        <PageHeader
          eyebrow="AI"
          title="AI 运维助手"
          description="基于 Ollama 本地大模型的智能运维问答，支持流式对话"
        >
          <div className="flex items-center gap-2">
            {/* 健康状态 */}
            <Badge
              variant={available ? 'default' : 'destructive'}
              className="gap-1"
            >
              {available ? (
                <>
                  <Wifi className="h-3 w-3" />
                  {health?.model ?? '已连接'}
                </>
              ) : (
                <>
                  <WifiOff className="h-3 w-3" />
                  {health?.error ? health.error.substring(0, 30) + '...' : '不可用'}
                </>
              )}
            </Badge>
            {/* 清空按钮 */}
            {messages.length > 0 && (
              <Button variant="ghost" size="sm" onClick={clearChat}>
                <Trash2 className="h-4 w-4 mr-1" />
                清空对话
              </Button>
            )}
          </div>
        </PageHeader>
      </section>

      {/* 消息区域 */}
      <section className="flex-1 overflow-y-auto px-4 sm:px-6 py-4 space-y-4" ref={scrollRef}>
        {/* 空状态 */}
        {messages.length === 0 && !streaming && (
          <div className="flex flex-col items-center justify-center h-full text-center text-muted-foreground">
            <Bot className="h-16 w-16 mb-4 opacity-30" />
            <p className="text-lg font-medium">有什么可以帮助你的？</p>
            <p className="text-sm mt-2 max-w-md">
              你可以询问 OpenVPN 配置、防火墙规则、用户管理、日志分析等运维相关问题。
            </p>
            <div className="flex flex-wrap gap-2 mt-4 justify-center">
              {['如何添加防火墙规则？', '查看在线客户端', '如何导出用户列表？', '服务器状态怎么看？'].map(
                (q) => (
                  <Button
                    key={q}
                    variant="outline"
                    size="sm"
                    disabled={!available}
                    onClick={() => {
                      setInput(q);
                      inputRef.current?.focus();
                    }}
                  >
                    {q}
                  </Button>
                ),
              )}
            </div>
          </div>
        )}

        {/* 消息列表 */}
        {messages.map((msg, i) => (
          <div
            key={i}
            className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            <Card
              className={`max-w-[85%] ${
                msg.role === 'user'
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-card'
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

        {/* 流式渲染中的消息 */}
        {streaming && (
          <div className="flex justify-start">
            <Card className="max-w-[85%] bg-card">
              <CardContent className="p-3">
                <div className="prose prose-sm dark:prose-invert max-w-none">
                  <MarkdownContent content={streaming} />
                </div>
                <span className="inline-block w-2 h-4 bg-primary animate-pulse ml-1" />
              </CardContent>
            </Card>
          </div>
        )}

        {/* 加载中（等待第一个 token） */}
        {loading && !streaming && (
          <div className="flex justify-start">
            <Card className="bg-card">
              <CardContent className="p-3 flex items-center gap-2">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span className="text-sm text-muted-foreground">AI 正在思考...</span>
              </CardContent>
            </Card>
          </div>
        )}
      </section>

      {/* 输入区域 */}
      <section className="shrink-0 px-4 sm:px-6 pb-4">
        <Card>
          <CardContent className="p-3">
            <div className="flex gap-2 items-end">
              <Textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={available ? '输入消息，Enter 发送，Shift+Enter 换行' : 'AI 服务不可用'}
                disabled={!available || loading}
                rows={1}
                className="min-h-[40px] max-h-[120px] resize-none"
              />
              {loading ? (
                <Button size="icon" variant="outline" onClick={cancelStream}>
                  <Loader2 className="h-4 w-4 animate-spin" />
                </Button>
              ) : (
                <Button
                  size="icon"
                  disabled={!input.trim() || !available}
                  onClick={sendMessage}
                >
                  <Send className="h-4 w-4" />
                </Button>
              )}
            </div>
            {/* 不健康提示 */}
            {health && !health.available && health.error && (
              <p className="text-xs text-destructive mt-2">
                提示：{health.error}
              </p>
            )}
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
