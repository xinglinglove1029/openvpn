import { useCallback, useEffect, useMemo, useState } from 'react';
import { Bell, CheckCircle2, XCircle, RefreshCw, Eye, Search } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/ui/button';
import { Badge } from '@/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/ui/dialog';
import MarkdownContent from '@/components/MarkdownContent';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import { Input } from '@/ui/input';
import { DataTable, type Column } from '@/components/DataTable';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { api } from '@/api';
import { messageOf } from '@/lib/format';
import { useNotificationHub } from '@/hooks/useNotificationHub';
import { realtimeHub } from '@/lib/notificationHub';
import type { NotifyLogRecord } from '@/types';

type EventFilter = 'all' | 'connect' | 'disconnect' | 'test' | 'user_register' | 'password_reset' | 'mfa_reset' | 'expire_reminder';
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
  if (event === 'test') return '测试';
  if (event === 'user_register') return '用户注册';
  if (event === 'user_email') return '用户邮件';
  if (event === 'password_reset') return '密码重置';
  if (event === 'mfa_reset') return 'MFA重置';
  if (event === 'expire_reminder') return '到期提醒';
  return event || '-';
}

// 渠道类型中文标签
function providerLabel(provider: string) {
  const map: Record<string, string> = {
    email: '邮件',
    dingtalk: '钉钉',
    feishu: '飞书',
    wecom: '企业微信',
    webhook: 'Webhook',
    discord: 'Discord',
    slack: 'Slack',
    telegram: 'Telegram',
    mattermost: 'Mattermost',
    system: '系统',
  };
  return map[provider] || provider || '-';
}

export default function NotificationsPage() {
  const [eventFilter, setEventFilter] = useState<EventFilter>('all');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [searchUser, setSearchUser] = useState('');
  const [searchMessage, setSearchMessage] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [detailItem, setDetailItem] = useState<NotifyLogRecord | null>(null);
  const { markRead, refresh: refreshUnread } = useNotificationHub();

  const state = useAsync(async () => {
    const result = await api.get<{ data: NotifyLogRecord[] }>('/ovpn/notify/logs?limit=500');
    return Array.isArray(result?.data) ? result.data : [];
  }, [reloadKey]);

  // 综合过滤：事件类型 + 发送结果 + 用户名 + 消息内容
  const filtered = useMemo(() => {
    const items = state.data ?? [];
    const kwUser = searchUser.trim().toLowerCase();
    const kwMsg = searchMessage.trim().toLowerCase();
    return items.filter((item) => {
      if (eventFilter !== 'all' && item.event !== eventFilter) return false;
      if (statusFilter === 'success' && !item.success) return false;
      if (statusFilter === 'failed' && item.success) return false;
      if (kwUser && !(item.username ?? '').toLowerCase().includes(kwUser)) return false;
      if (kwMsg && !(item.message ?? '').toLowerCase().includes(kwMsg)) return false;
      return true;
    });
  }, [state.data, eventFilter, statusFilter, searchUser, searchMessage]);

  const pagination = usePagination(
    filtered,
    `${eventFilter}-${statusFilter}-${searchUser}-${searchMessage}-${reloadKey}`,
    10,
  );

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
      header: '渠道类型',
      sortable: true,
      sortAccessor: (item) => item.provider ?? '',
      render: (item) =>
        item.provider ? (
          <StatusBadge status="info">{providerLabel(item.provider)}</StatusBadge>
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
    {
      key: 'channelName',
      header: '渠道名称',
      sortable: true,
      sortAccessor: (item) => item.channelName ?? '',
      render: (item) =>
        item.channelName ? (
          <span className="text-foreground">{item.channelName}</span>
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
      key: 'actions',
      header: '操作',
      render: (item) => (
        <Button
          variant="ghost"
          size="sm"
          className="h-7 px-2 text-xs"
          onClick={() => setDetailItem(item)}
        >
          <Eye className="w-3.5 h-3.5 mr-1" />
          详情
        </Button>
      ),
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
    <div className="space-y-4">
      <PageHeader eyebrow="Notification" title="站内信" description="查看 Webhook / 邮件等渠道发送的运维通知" />

      {/* 操作工具栏 */}
      <div className="space-y-2 sm:space-y-0 sm:flex sm:flex-wrap sm:items-center sm:gap-2">
        {/* 搜索行：移动端并排两个搜索框 */}
        <div className="flex items-stretch gap-2 sm:flex-1 sm:min-w-0">
          <div className="relative flex-1 sm:max-w-[170px]">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              value={searchUser}
              onChange={(e) => setSearchUser(e.target.value)}
              placeholder="用户名"
              className="pl-8 h-9 sm:h-8 text-sm"
            />
          </div>
          <div className="relative flex-1 sm:max-w-[200px]">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              value={searchMessage}
              onChange={(e) => setSearchMessage(e.target.value)}
              placeholder="消息内容"
              className="pl-8 h-9 sm:h-8 text-sm"
            />
          </div>
        </div>
        {/* 筛选+刷新行：移动端并排 */}
        <div className="flex items-center gap-2">
          <Select value={eventFilter} onValueChange={(v) => setEventFilter(v as EventFilter)}>
            <SelectTrigger className="flex-1 sm:w-[110px] h-9 sm:h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部事件</SelectItem>
              <SelectItem value="connect">上线</SelectItem>
              <SelectItem value="disconnect">下线</SelectItem>
              <SelectItem value="test">测试</SelectItem>
              <SelectItem value="user_register">用户注册</SelectItem>
              <SelectItem value="password_reset">密码重置</SelectItem>
              <SelectItem value="mfa_reset">MFA重置</SelectItem>
              <SelectItem value="expire_reminder">到期提醒</SelectItem>
            </SelectContent>
          </Select>
          <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
            <SelectTrigger className="flex-1 sm:w-[100px] h-9 sm:h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部结果</SelectItem>
              <SelectItem value="success">成功</SelectItem>
              <SelectItem value="failed">失败</SelectItem>
            </SelectContent>
          </Select>
          <Button size="sm" variant="outline" onClick={handleReload} disabled={state.loading} className="h-9 sm:h-8 shrink-0">
            <RefreshCw className={`h-3.5 w-3.5 mr-1 ${state.loading ? 'animate-spin' : ''}`} />
            <span className="hidden sm:inline">刷新</span>
          </Button>
        </div>
      </div>

      {/* 摘要 */}
      {!state.loading && filtered.length > 0 && (
        <p className="text-xs text-muted-foreground">
          共匹配 {filtered.length} 条记录。
        </p>
      )}

      {/* 表格 */}
      {!state.loading && filtered.length > 0 && (
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
      )}

      {/* 空状态 */}
      {state.loading && (
        <p className="text-sm text-muted-foreground text-center py-8">加载中...</p>
      )}
      {!state.loading && filtered.length === 0 && (
        <div className="text-center py-12">
          <Bell className="mx-auto h-10 w-10 text-muted-foreground/50" />
          <p className="mt-2 text-sm font-medium">暂无通知</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {searchUser || searchMessage || eventFilter !== 'all' || statusFilter !== 'all'
              ? '没有匹配的通知，请调整搜索条件'
              : 'Webhook / 邮件等渠道发送的运维通知会展示在这里'}
          </p>
        </div>
      )}

      <Dialog open={!!detailItem} onOpenChange={(open) => !open && setDetailItem(null)}>
        <DialogContent className="max-w-2xl w-[calc(100vw-2rem)] sm:w-auto max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>通知详情</DialogTitle>
            <DialogDescription>
              {detailItem && formatTime(detailItem.createdAt)}
            </DialogDescription>
          </DialogHeader>
          {detailItem && (
            <div className="space-y-3 sm:space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:gap-4 text-sm">
                <div>
                  <div className="text-muted-foreground text-xs sm:text-sm mb-1">事件类型</div>
                  <Badge variant="outline" className="bg-blue-500/15 text-blue-600 border-blue-500/25 text-xs">
                    {eventLabel(detailItem.event)}
                  </Badge>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs sm:text-sm mb-1">用户</div>
                  <div className="font-medium text-sm">{detailItem.username || '-'}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs sm:text-sm mb-1">渠道类型</div>
                  <div>
                    {detailItem.provider ? (
                      <StatusBadge status="info">{providerLabel(detailItem.provider)}</StatusBadge>
                    ) : (
                      <span className="text-muted-foreground text-sm">-</span>
                    )}
                  </div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs sm:text-sm mb-1">渠道名称</div>
                  <div className="font-medium text-sm break-all">{detailItem.channelName || '-'}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs sm:text-sm mb-1">发送结果</div>
                  <div>
                    {detailItem.success ? (
                      <span className="inline-flex items-center gap-1 text-emerald-500 text-sm">
                        <CheckCircle2 className="w-4 h-4" />
                        成功
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-red-500 text-sm">
                        <XCircle className="w-4 h-4" />
                        失败
                      </span>
                    )}
                  </div>
                </div>
              </div>
              <div>
                <div className="text-muted-foreground text-xs sm:text-sm mb-2">消息明细</div>
                <div className="bg-muted/40 rounded-md p-3 sm:p-4 overflow-x-auto">
                  <MarkdownContent content={detailItem.message} />
                </div>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
