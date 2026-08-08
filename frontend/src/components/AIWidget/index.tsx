import { useState, useRef, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTheme } from '@/store/theme';
import { useAuth } from '@/store/auth';
import { useAsync } from '@/hooks/useAsync';
import { api } from '@/api';
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
  error?: string;
}

const HOT_TOPICS = [
  '如何查看当前在线客户端？',
  'OpenVPN 配置示例',
  '防火墙规则推荐',
];

const RECOMMENDATIONS = [
  '新增用户并导出配置文件',
  '查看服务器资源状态',
  '审计日志分析',
];

export default function AIWidget() {
  const navigate = useNavigate();
  const { theme } = useTheme();
  const { hasPermission } = useAuth();
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState('');
  const [loading, setLoading] = useState(false);
  const [sessionID, setSessionID] = useState('');
  const [deepThink, setDeepThink] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  // 健康检查：打开抽屉时或组件首次挂载时刷新
  const { data: health, loading: healthLoading } = useAsync<HealthStatus>(
    useCallback(() => api.get('/ovpn/ai/health'), []),
    [open],
  );

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
    if (!text || loading) return;

    setInput('');
    const userMsg: ChatMessage = { role: 'user', content: text };
    setMessages((prev) => [...prev, userMsg]);

    setLoading(true);
    setStreaming('');

    const controller = new AbortController();
    abortRef.current = controller;

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

      const decoder = new TextDecoder();
      let buffer = '';
      let fullText = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split('\n\n');
        buffer = events.pop() || '';

        for (const event of events) {
          let eventType = '';
          let data = '';

          for (const line of event.split('\n')) {
            if (line.startsWith('event: ')) eventType = line.slice(7);
            else if (line.startsWith('data: ')) data += line.slice(6);
          }

          switch (eventType) {
            case 'session':
              setSessionID(data);
              break;
            case 'token':
              fullText += data;
              setStreaming(fullText);
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
        }
      }
    } catch (err: any) {
      if (err.name === 'AbortError') {
        if (streaming) {
          setMessages((prev) => [
            ...prev,
            { role: 'assistant', content: streaming + ' [已中断]' },
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

  const newChat = () => {
    setMessages([]);
    setSessionID('');
    setStreaming('');
    setInput('');
    setTimeout(() => inputRef.current?.focus(), 100);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  // 如果没有 AI 聊天权限则不渲染
  if (!hasPermission('ai:chat')) {
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
                        disabled={!available || loading}
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
                        disabled={!available || loading}
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
                  disabled={loading}
                  rows={3}
                  className="min-h-[80px] resize-none border-0 bg-transparent pr-12 pb-9 focus-visible:ring-0 focus-visible:ring-offset-0 disabled:opacity-60 disabled:cursor-not-allowed"
                />
                <div className="absolute left-2.5 bottom-2 flex items-center gap-2">
                  <button
                    onClick={() => setDeepThink(!deepThink)}
                    disabled={loading}
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
                      disabled={!input.trim()}
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
