import { useEffect, useMemo, useState } from 'react';
import { Search, RefreshCw, Download, Globe2, ChevronLeft, ChevronRight } from 'lucide-react';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { formatBytes, getClientBytes } from '@/lib/format';
import { formatDateTime, formatDuration } from '@/lib/utils';
import { PageHeader } from '@/components/PageHeader';
import { TimeRangePicker } from '@/components/TimeRangePicker';
import { DataTable, type Column } from '@/components/DataTable';
import { Card, CardContent } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/ui/dialog';
import { useAuth } from '@/store/auth';
import type {
  HistoryRecord,
  HistoryResponse,
  HistoryWebsiteAuditResponse,
  OnlineClient,
} from '../../types';

const auditPageSize = 20;

function historyStartSeconds(record: HistoryRecord): number {
  if (typeof record.time_unix === 'number') return record.time_unix;
  if (typeof record.time_unix === 'string') {
    const parsed = Date.parse(record.time_unix);
    return Number.isNaN(parsed) ? 0 : Math.floor(parsed / 1000);
  }
  return 0;
}

function historyDurationSeconds(record: HistoryRecord): number {
  if (typeof record.time_duration === 'number') return record.time_duration;
  if (typeof record.time_duration === 'string') {
    const match = record.time_duration.match(/(?:(\d+)h)?\s*(?:(\d+)m)?\s*(?:(\d+)s)?/i);
    if (match && (match[1] || match[2] || match[3])) {
      return Number(match[1] || 0) * 3600 + Number(match[2] || 0) * 60 + Number(match[3] || 0);
    }
    const numeric = Number(record.time_duration);
    return Number.isFinite(numeric) ? numeric : 0;
  }
  return 0;
}

function auditResponseLabel(responseCode: string) {
  return responseCode === 'RCodeSuccess' ? '成功' : responseCode || '-';
}

export default function HistoryPage() {
  const { hasPermission } = useAuth();
  const canViewWebsiteAudit = hasPermission('web-audit:view');
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
  const [selectedHistory, setSelectedHistory] = useState<HistoryRecord | null>(null);
  const [auditPage, setAuditPage] = useState(0);
  const [auditState, setAuditState] = useState<{
    loading: boolean;
    error?: string;
    data?: HistoryWebsiteAuditResponse;
  }>({ loading: false });

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

  useEffect(() => {
    let active = true;
    const historyID = selectedHistory?.id;
    if (!historyID) {
      setAuditState({ loading: false });
      return () => {
        active = false;
      };
    }

    setAuditState({ loading: true });
    api
      .get<HistoryWebsiteAuditResponse>(
        `/ovpn/history/${historyID}/web-audit?offset=${auditPage * auditPageSize}&limit=${auditPageSize}`,
      )
      .then((data) => active && setAuditState({ loading: false, data }))
      .catch((error) => active && setAuditState({ loading: false, error: error instanceof Error ? error.message : String(error) }));

    return () => {
      active = false;
    };
  }, [selectedHistory?.id, auditPage]);

  const openWebsiteAudit = (record: HistoryRecord) => {
    setAuditPage(0);
    setAuditState({ loading: false });
    setSelectedHistory(record);
  };

  const closeWebsiteAudit = () => {
    setSelectedHistory(null);
    setAuditPage(0);
    setAuditState({ loading: false });
  };

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
        key: 'ripRegion',
        header: 'IP 归属地',
        sortable: true,
        sortAccessor: (item) => item.ripRegion ?? item.rip6Region ?? '',
        render: (item) => item.ripRegion || item.rip6Region || '-',
      },
      {
        key: 'bytes_received',
        header: '下载',
        sortable: true,
        sortAccessor: (item) => getClientBytes(item as unknown as OnlineClient, 'received'),
        render: (item) => formatBytes(getClientBytes(item as unknown as OnlineClient, 'received')),
      },
      {
        key: 'bytes_sent',
        header: '上传',
        sortable: true,
        sortAccessor: (item) => getClientBytes(item as unknown as OnlineClient, 'sent'),
        render: (item) => formatBytes(getClientBytes(item as unknown as OnlineClient, 'sent')),
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
        sortAccessor: (item) => historyDurationSeconds(item),
        render: (item) => {
          const duration = historyDurationSeconds(item);
          return duration > 0 ? formatDuration(duration) : '-';
        },
      },
      ...(canViewWebsiteAudit
        ? [
            {
              key: 'website_audit',
              header: '访问记录',
              sortable: false,
              render: (item: HistoryRecord) => (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="gap-1.5 whitespace-nowrap"
                  onClick={() => openWebsiteAudit(item)}
                  disabled={!item.id}
                >
                  <Globe2 className="h-3.5 w-3.5" />
                  <span>访问记录</span>
                </Button>
              ),
            } as Column<HistoryRecord>,
          ]
        : []),
    ],
    [canViewWebsiteAudit],
  );

  const auditData = auditState.data;
  const auditTotalPages = auditData ? Math.max(1, Math.ceil(auditData.total / auditPageSize)) : 1;
  const selectedStart = selectedHistory ? historyStartSeconds(selectedHistory) : 0;
  const selectedEnd = selectedHistory
    ? selectedStart + historyDurationSeconds(selectedHistory)
    : 0;

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Telemetry" title="连接历史" description="查看 VPN 客户端连接历史记录" />

      <Card>
        <CardContent className="p-6">
          <div className="flex flex-col sm:flex-row flex-wrap items-stretch sm:items-center gap-3 mb-6">
            <TimeRangePicker
              value={dateRange}
              onChange={setDateRange}
              placeholder="选择时间范围"
            />
            <div className="relative">
              <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-8 w-full sm:w-[200px]"
                value={search}
                placeholder="搜索用户/IP"
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <Button size="sm" onClick={() => setReloadKey((v) => v + 1)} className="w-full sm:w-auto">
              <RefreshCw className="h-4 w-4 mr-1" />
              查询
            </Button>
            <Button variant="outline" size="sm" asChild className="w-full sm:w-auto">
              <a href={`/ovpn/history/export?qt=${qt}`}>
                <Download className="h-4 w-4 mr-1" />
                导出
              </a>
            </Button>
          </div>

          {state.loading && (
            <p className="text-sm text-muted-foreground text-center py-8">加载中...</p>
          )}

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

      <Dialog open={Boolean(selectedHistory)} onOpenChange={(open) => !open && closeWebsiteAudit()}>
        <DialogContent className="w-[calc(100vw-1rem)] max-w-[min(96vw,1280px)] sm:w-[min(96vw,1280px)] sm:max-w-[min(96vw,1280px)]">
          <DialogHeader>
            <DialogTitle>连接期间的网站访问记录</DialogTitle>
            <DialogDescription>
              仅展示 DNS 查询到的域名元数据，不包含 URL 路径、页面内容、Cookie 或账号密码。
            </DialogDescription>
          </DialogHeader>

          {selectedHistory && (
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-3 rounded-lg border bg-muted/20 p-4 sm:grid-cols-2 xl:grid-cols-4">
                <div>
                  <p className="text-xs text-muted-foreground">用户</p>
                  <p className="mt-1 font-medium">{selectedHistory.username || selectedHistory.common_name || '-'}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">连接时间</p>
                  <p className="mt-1 text-sm">
                    {selectedStart ? formatDateTime(new Date(selectedStart * 1000)) : '-'}
                    {' → '}
                    {selectedEnd ? formatDateTime(new Date(selectedEnd * 1000)) : '未知'}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">连接 ID</p>
                  <p className="mt-1 truncate font-mono text-xs" title={selectedHistory.connection_id || selectedHistory.connectionId || ''}>
                    {selectedHistory.connection_id || selectedHistory.connectionId || '旧记录：按用户和时间匹配'}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">匹配方式</p>
                  <p className="mt-1 text-sm">
                    {auditData?.matchedBy === 'connection_id' ? '连接 ID 精确匹配' : '用户 + 时间范围匹配'}
                  </p>
                </div>
              </div>

              {auditState.loading && (
                <p className="py-10 text-center text-sm text-muted-foreground">正在加载访问记录...</p>
              )}
              {auditState.error && (
                <p className="py-10 text-center text-sm text-destructive">{auditState.error}</p>
              )}
              {!auditState.loading && !auditState.error && auditData && auditData.data.length === 0 && (
                <div className="rounded-lg border border-dashed py-10 text-center">
                  <Globe2 className="mx-auto h-8 w-8 text-muted-foreground/60" />
                  <p className="mt-3 font-medium">该连接期间没有记录到 DNS 访问</p>
                  <p className="mt-1 text-sm text-muted-foreground">可能未启用网站访问审计，或客户端没有经过 VPN DNS。</p>
                </div>
              )}
              {!auditState.loading && !auditState.error && auditData && auditData.data.length > 0 && (
                <>
                  <div className="overflow-x-auto rounded-lg border">
                    <table className="w-full min-w-[880px] text-sm">
                      <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
                        <tr>
                          <th className="px-4 py-3 font-medium">查询时间</th>
                          <th className="px-4 py-3 font-medium">域名</th>
                          <th className="px-4 py-3 font-medium">DNS 类型</th>
                          <th className="px-4 py-3 font-medium">响应状态</th>
                          <th className="px-4 py-3 font-medium">VPN IP</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y">
                        {auditData.data.map((record) => (
                          <tr key={record.id} className="hover:bg-muted/20">
                            <td className="whitespace-nowrap px-4 py-3 text-muted-foreground">
                              {record.queriedAt ? formatDateTime(new Date(record.queriedAt * 1000)) : '-'}
                            </td>
                            <td className="px-4 py-3 font-mono text-xs">{record.domain || '-'}</td>
                            <td className="px-4 py-3">{record.queryType || '-'}</td>
                            <td className="px-4 py-3">{auditResponseLabel(record.responseCode)}</td>
                            <td className="px-4 py-3 text-muted-foreground">{record.vpnIp || '-'}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <div className="flex flex-col gap-2 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
                    <span>
                      共 {auditData.total} 条记录，每页 {auditPageSize} 条{auditData.total > auditPageSize ? `，第 ${auditPage + 1}/${auditTotalPages} 页` : ''}
                    </span>
                    {auditTotalPages > 1 && (
                      <div className="flex items-center gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => setAuditPage((page) => Math.max(0, page - 1))}
                          disabled={auditPage === 0 || auditState.loading}
                        >
                          <ChevronLeft className="mr-1 h-4 w-4" />上一页
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => setAuditPage((page) => Math.min(auditTotalPages - 1, page + 1))}
                          disabled={auditPage >= auditTotalPages - 1 || auditState.loading}
                        >
                          下一页<ChevronRight className="ml-1 h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </div>
                </>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}
