import { useCallback, useEffect, useMemo, useState } from 'react';
import { Bell, CheckCircle2, XCircle, RefreshCw, Filter } from 'lucide-react';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Badge } from '@/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import { DataTable, type Column } from '@/components/DataTable';
import { StatusBadge } from '@/components/StatusBadge';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { api } from '@/api';
import { messageOf } from '@/lib/format';
import { useNotificationHub } from '@/hooks/useNotificationHub';
import { realtimeHub } from '@/lib/notificationHub';
import type { NotifyLogRecord } from '@/types';

type EventFilter = 'all' | 'connect' | 'disconnect';
type StatusFilter = 'all' | 'success' | 'failed';

function formatTime(value?: string) {
  if (!value) return '-';
  // 后端返回的是 UTC 时间（Go time.Time RFC3339），需要转换到本地时区显示
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour12: false });
}

function eventLabel(event: string) {
  if (event === 'connect') return '上线';
  if (event === 'disconnect') return '下线';
  return event || '-';
}

export default function NotificationsPage() {
  const [eventFilter, setEventFilter] = useState<EventFilter>('all');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [reloadKey, setReloadKey] = useState(0);
  const { markRead, refresh: refreshUnread } = useNotificationHub();

  const state = useAsync(async () => {
    const result = await api.get<{ data: NotifyLogRecord[] }>('/ovpn/notify/logs?limit=200');
    return Array.isArray(result?.data) ? result.data : [];
  }, [reloadKey]);

  const filtered = useMemo(() => {
    const items = state.data ?? [];
    return items.filter((item) => {
      if (eventFilter !== 'all' && item.event !== eventFilter) return false;
      if (statusFilter === 'success' && !item.success) return false;
      if (statusFilter === 'failed' && item.success) return false;
      return true;
    });
  }, [state.data, eventFilter, statusFilter]);

  const pagination = usePagination(filtered, `${eventFilter}-${statusFilter}-${reloadKey}`, 10);

  const handleReload = useCallback(() => {
    setReloadKey((key) => key + 1);
  }, []);

  // 进入通知中心即视为已读：把已读进度推进到当前最新
  useEffect(() => {
    void markRead();
  }, [markRead]);

  // 订阅新通知：收到推送后自动刷新列表
  useEffect(() => {
    const off = realtimeHub.subscribe('notify:new', () => {
      setReloadKey((key) => key + 1);
      void refreshUnread();
    });
    return () => {
      off();
    };
  }, [refreshUnread]);

  useEffect(() => {
    if (state.error) {
      toast.error(`加载通知失败：${messageOf(state.error)}`);
    }
  }, [state.error]);

  const columns: Column<NotifyLogRecord>[] = [
    {
      key: 'event',
      header: '事件',
      sortable: true,
      sortAccessor: (item) => item.event ?? '',
      render: (item) => (
        <Badge variant="outline" className="bg-blue-500/15 text-blue-600 border-blue-500/25">
          {eventLabel(item.event)}
        </Badge>
      ),
    },
    {
      key: 'username',
      header: '用户',
      sortable: true,
      sortAccessor: (item) => item.username ?? '',
      render: (item) => item.username || '-',
    },
    {
      key: 'provider',
      header: '通道',
      sortable: true,
      sortAccessor: (item) => item.provider ?? '',
      render: (item) =>
        item.provider ? (
          <StatusBadge status="info">{item.provider}</StatusBadge>
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
    {
      key: 'success',
      header: '结果',
      sortable: true,
      sortAccessor: (item) => (item.success ? 1 : 0),
      render: (item) =>
        item.success ? (
          <span className="inline-flex items-center gap-1 text-emerald-500">
            <CheckCircle2 className="w-4 h-4" />
            成功
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 text-red-500">
            <XCircle className="w-4 h-4" />
            失败
          </span>
        ),
    },
    {
      key: 'message',
      header: '消息',
      sortable: true,
      sortAccessor: (item) => item.message ?? '',
      render: (item) => (
        <span className={item.success ? 'text-muted-foreground' : 'text-red-500/90'}>
          {item.message || '-'}
        </span>
      ),
      className: 'max-w-[420px]',
    },
    {
      key: 'createdAt',
      header: '时间',
      sortable: true,
      sortAccessor: (item) => (item.createdAt ? new Date(item.createdAt).getTime() : 0),
      render: (item) => formatTime(item.createdAt),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold flex items-center gap-2">
            <Bell className="w-7 h-7" />
            站内信
          </h1>
          <p className="text-muted-foreground mt-1">查看 Webhook / 邮件等渠道发送的运维通知</p>
        </div>
        <Button variant="outline" onClick={handleReload} disabled={state.loading}>
          <RefreshCw className={`w-4 h-4 mr-2 ${state.loading ? 'animate-spin' : ''}`} />
          刷新
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Filter className="w-4 h-4" />
            筛选
          </CardTitle>
          <CardDescription>按事件类型和发送结果过滤最近 200 条通知</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-end gap-4">
            <div className="space-y-1.5">
              <label className="text-sm font-medium" htmlFor="notify-event-filter">事件类型</label>
              <Select value={eventFilter} onValueChange={(v) => setEventFilter(v as EventFilter)}>
                <SelectTrigger id="notify-event-filter" className="w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部事件</SelectItem>
                  <SelectItem value="connect">上线</SelectItem>
                  <SelectItem value="disconnect">下线</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium" htmlFor="notify-status-filter">发送结果</label>
              <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
                <SelectTrigger id="notify-status-filter" className="w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部结果</SelectItem>
                  <SelectItem value="success">成功</SelectItem>
                  <SelectItem value="failed">失败</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="text-sm text-muted-foreground pb-2">
              共 {filtered.length} 条
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <DataTable<NotifyLogRecord>
            columns={columns}
            data={pagination.pagedItems}
            fullData={filtered}
            page={pagination.page}
            pageSize={pagination.pageSize}
            pageCount={pagination.pageCount}
            total={pagination.total}
            start={pagination.start}
            end={pagination.end}
            onPageChange={pagination.setPage}
            onPageSizeChange={pagination.setPageSize}
            emptyTitle={state.loading ? '加载中...' : '暂无通知'}
            emptyDescription={state.loading ? '正在拉取最近的运维通知' : '当前筛选条件下还没有任何通知记录'}
            keyFn={(item) => item.id}
          />
        </CardContent>
      </Card>
    </div>
  );
}
