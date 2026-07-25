import { useEffect, useRef } from 'react';
import { Link } from 'react-router-dom';
import { Bell, CheckCircle2, XCircle, ChevronRight } from 'lucide-react';
import { Button } from '@/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/ui/popover';
import { Separator } from '@/ui/separator';
import { useNotificationHub } from '@/hooks/useNotificationHub';
import { cn } from '@/lib/utils';

function eventLabel(event: string) {
  if (event === 'connect' || event === 'online') return '上线';
  if (event === 'disconnect' || event === 'offline') return '下线';
  return event || '事件';
}

function formatTime(value?: string) {
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('zh-CN', { hour12: false });
}

export function NotificationBell() {
  const { snapshot, latest, state, markRead } = useNotificationHub();
  const popoverOpenRef = useRef(false);

  const unread = snapshot.unread;

  // 打开 popover 时自动标记已读
  function handleOpenChange(open: boolean) {
    popoverOpenRef.current = open;
    if (open && unread > 0) {
      void markRead();
    }
  }

  // 首次出现新消息时（不打开 popover 也提示），用原生 title 即可，
  // 顶部红点的角标 + 数字变化本身已经能起到提示作用。

  // 连接异常时给一个轻量提示
  useEffect(() => {
    if (state === 'closed') {
      // 静默：自动重连中
    }
  }, [state]);

  return (
    <Popover onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="relative"
          title={
            unread > 0
              ? `${unread} 条未读通知`
              : state === 'open'
                ? '通知（已连接）'
                : '通知（连接中…）'
          }
          aria-label="通知"
        >
          <Bell className="w-5 h-5" />
          {unread > 0 ? (
            <span
              className={cn(
                'absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 rounded-full',
                'bg-red-600 text-white text-[10px] font-semibold leading-[18px] text-center',
                'ring-2 ring-card',
              )}
            >
              {unread > 99 ? '99+' : unread}
            </span>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0">
        <div className="flex items-center justify-between px-4 py-3">
          <div>
            <p className="text-sm font-semibold">站内信</p>
            <p className="text-xs text-muted-foreground">
              {unread > 0 ? `未读 ${unread} 条` : '暂无未读'}
            </p>
          </div>
          <span
            className={cn(
              'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
              state === 'open' && 'bg-emerald-500/15 text-emerald-600',
              state === 'connecting' && 'bg-amber-500/15 text-amber-600',
              state === 'closed' && 'bg-muted text-muted-foreground',
              state === 'idle' && 'bg-muted text-muted-foreground',
            )}
            title={
              state === 'open'
                ? '实时连接已建立'
                : state === 'connecting'
                  ? '正在连接…'
                  : '连接已断开，后台自动重连中'
            }
          >
            <span
              className={cn(
                'inline-block h-1.5 w-1.5 rounded-full',
                state === 'open' && 'bg-emerald-500',
                state === 'connecting' && 'bg-amber-500 animate-pulse',
                (state === 'closed' || state === 'idle') && 'bg-muted-foreground',
              )}
            />
            {state === 'open' ? '实时' : state === 'connecting' ? '连接中' : '离线'}
          </span>
        </div>
        <Separator />
        <div className="max-h-80 overflow-y-auto">
          {latest ? (
            <div className="px-4 py-3 space-y-1.5">
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1 rounded bg-blue-500/10 text-blue-600 px-1.5 py-0.5">
                  {eventLabel(latest.event)}
                </span>
                {latest.provider ? (
                  <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5">
                    {latest.provider}
                  </span>
                ) : null}
                <span>{formatTime(latest.createdAt)}</span>
              </div>
              <p
                className={cn(
                  'text-sm leading-relaxed line-clamp-3',
                  latest.success ? 'text-foreground' : 'text-red-500/90',
                )}
              >
                {latest.message || `${latest.username || ''} ${eventLabel(latest.event)}`}
              </p>
              <div className="flex items-center gap-1 text-xs text-muted-foreground">
                {latest.success ? (
                  <span className="inline-flex items-center gap-1 text-emerald-500">
                    <CheckCircle2 className="h-3 w-3" /> 发送成功
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-red-500">
                    <XCircle className="h-3 w-3" /> 发送失败
                  </span>
                )}
                {latest.username ? <span>· {latest.username}</span> : null}
              </div>
            </div>
          ) : (
            <div className="px-4 py-8 text-center text-xs text-muted-foreground">
              暂无最新通知
            </div>
          )}
        </div>
        <Separator />
        <div className="px-2 py-2">
          <Button
            asChild
            variant="ghost"
            className="w-full justify-between"
            size="sm"
          >
            <Link to="/notifications">
              查看全部通知
              <ChevronRight className="h-4 w-4" />
            </Link>
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
