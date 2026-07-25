import { useMemo, useState } from 'react';
import { Search, RefreshCw, Download } from 'lucide-react';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { formatBytes, getClientBytes, normalizeList } from '@/lib/format';
import { formatDateTime, formatDuration } from '@/lib/utils';
import { PageHeader } from '@/components/PageHeader';
import { TimeRangePicker } from '@/components/TimeRangePicker';
import { DataTable, type Column } from '@/components/DataTable';
import { Card, CardContent } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import type { HistoryRecord, HistoryResponse, OnlineClient } from '../../types';

export default function HistoryPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [dateRange, setDateRange] = useState(() => {
    const end = new Date();
    const start = new Date();
    start.setMonth(start.getMonth() - 1);
    return {
      from: start.toISOString().slice(0, 10),
      to: end.toISOString().slice(0, 10),
    };
  });

  const qt = useMemo(() => {
    const startTime = new Date(`${dateRange.from}T00:00:00`).getTime() / 1000;
    const endTime = new Date(`${dateRange.to}T23:59:59`).getTime() / 1000;
    return `${startTime},${endTime}`;
  }, [dateRange]);

  const state = useAsync(
    () =>
      api.get<HistoryResponse>(
        `/ovpn/history?draw=1&offset=0&limit=50&orderColumn=time_unix&order=desc&search=${encodeURIComponent(search)}&qt=${qt}`,
      ),
    [reloadKey, qt],
  );

  const rows = state.data?.data || [];
  const pagination = usePagination(rows, `${search}|${qt}|${reloadKey}`);

  const columns: Column<HistoryRecord>[] = useMemo(
    () => [
      {
        key: 'username',
        header: '用户',
        sortable: true,
        sortAccessor: (item) => item.username ?? '',
        render: (item) => item.username || '-',
      },
      {
        key: 'common_name',
        header: '客户端',
        sortable: true,
        sortAccessor: (item) => item.common_name ?? item.commonName ?? '',
        render: (item) => item.common_name || item.commonName || '-',
      },
      {
        key: 'vip',
        header: 'VPN IP',
        sortable: true,
        sortAccessor: (item) => item.vip ?? item.vip6 ?? '',
        render: (item) => item.vip || item.vip6 || '-',
      },
      {
        key: 'rip',
        header: '来源 IP',
        sortable: true,
        sortAccessor: (item) => item.rip ?? item.rip6 ?? '',
        render: (item) => item.rip || item.rip6 || '-',
      },
      {
        key: 'bytes_received',
        header: '下载',
        sortable: true,
        sortAccessor: (item) =>
          getClientBytes(item as unknown as OnlineClient, 'received') ||
          Number(item.bytes_received ?? item.bytesReceived ?? 0),
        render: (item) =>
          formatBytes(
            getClientBytes(item as unknown as OnlineClient, 'received') ||
              Number(item.bytes_received ?? item.bytesReceived ?? 0),
          ),
      },
      {
        key: 'bytes_sent',
        header: '上传',
        sortable: true,
        sortAccessor: (item) =>
          getClientBytes(item as unknown as OnlineClient, 'sent') ||
          Number(item.bytes_sent ?? item.bytesSent ?? 0),
        render: (item) =>
          formatBytes(
            getClientBytes(item as unknown as OnlineClient, 'sent') ||
              Number(item.bytes_sent ?? item.bytesSent ?? 0),
          ),
      },
      {
        key: 'time_unix',
        header: '上线时间',
        sortable: true,
        sortAccessor: (item) =>
          typeof item.time_unix === 'string'
            ? new Date(item.time_unix).getTime() || 0
            : Number(item.time_unix ?? 0),
        render: (item) =>
          typeof item.time_unix === 'string'
            ? item.time_unix
            : item.time_unix
              ? formatDateTime(new Date(item.time_unix * 1000))
              : '-',
      },
      {
        key: 'time_duration',
        header: '在线时长',
        sortable: true,
        sortAccessor: (item) => {
          if (item.time_duration == null) return 0;
          if (typeof item.time_duration === 'number') return item.time_duration;
          // 解析形如 "1h 20m 30s" 这类字符串
          const match = String(item.time_duration).match(/(\d+)/);
          return match ? Number(match[1]) : 0;
        },
        render: (item) =>
          item.time_duration
            ? typeof item.time_duration === 'number'
              ? formatDuration(item.time_duration)
              : item.time_duration
            : '-',
      },
    ],
    [],
  );

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Telemetry" title="连接历史" description="查看 VPN 客户端连接历史记录" />

      <Card>
        <CardContent className="p-6">
          {/* 过滤栏 */}
          <div className="flex flex-wrap items-center gap-3 mb-6">
            <TimeRangePicker
              value={dateRange}
              onChange={setDateRange}
              placeholder="选择时间范围"
            />
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-8 w-[200px]"
                value={search}
                placeholder="搜索用户/IP"
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <Button size="sm" onClick={() => setReloadKey((v) => v + 1)}>
              <RefreshCw className="h-4 w-4 mr-1" />
              查询
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href={`/ovpn/history/export?qt=${qt}`}>
                <Download className="h-4 w-4 mr-1" />
                导出
              </a>
            </Button>
          </div>

          {/* 加载中 */}
          {state.loading && (
            <p className="text-sm text-muted-foreground text-center py-8">
              加载中...
            </p>
          )}

          {/* 表格 */}
          {!state.loading && rows.length > 0 && (
            <DataTable
              columns={columns}
              data={pagination.pagedItems}
              fullData={rows}
              page={pagination.page}
              pageSize={pagination.pageSize}
              pageCount={pagination.pageCount}
              total={pagination.total}
              start={pagination.start}
              end={pagination.end}
              onPageChange={pagination.setPage}
              onPageSizeChange={pagination.setPageSize}
              keyFn={(item, index) => item.id ?? pagination.start + index}
            />
          )}

          {/* 空状态 */}
          {!state.loading && rows.length === 0 && (
            <div className="text-center py-12">
              <p className="text-muted-foreground font-medium">暂无历史记录</p>
              <p className="text-sm text-muted-foreground mt-1">
                客户端下线后，OpenVPN hook 会写入这里并触发通知。
              </p>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
