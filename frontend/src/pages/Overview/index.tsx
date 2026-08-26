import { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Activity,
  Users,
  FileCode2,
  LogIn,
  AlertTriangle,
  Server,
  XCircle,
  Gauge,
  ShieldBan,
  ShieldCheck,
  TrendingUp,
  Wifi,
  MonitorUp,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Badge } from '@/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/ui/dialog';
import { Label } from '@/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { DataTable, type Column } from '@/components/DataTable';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { CardGlow } from '@/components/CardGlow';
import { DashboardTrafficUsers } from '@/components/DashboardTrafficUsers';
import SystemMonitor from '@/components/SystemMonitor';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { api } from '@/api';
import { useAuth } from '@/store/auth';
import { HasPermission } from '@/components/HasPermission';
import {
  formatBytes,
  getClientName,
  getClientBytes,
  clientVips,
  messageOf,
} from '@/lib/format';
import { cn } from '@/lib/utils';
import { realtimeHub } from '@/lib/notificationHub';
import type {
  OnlineClient,
  OnlineResponse,
  DashboardSummary,
  DashboardStatsPayload,
} from '@/types';

/* ---------- types ---------- */

type FieldErrors = Record<string, string>;

type ModalState =
  | { type: 'none' }
  | { type: 'rate-limit'; client: OnlineClient; upload: string; uploadUnit: string; download: string; downloadUnit: string };

/* ---------- main component ---------- */

export default function OverviewPage() {
  const navigate = useNavigate();
  const { executiveDashboardEnabled } = useAuth();
  const [confirmState, setConfirmState] = useState<ConfirmState>();
  const [modal, setModal] = useState<ModalState>({ type: 'none' });

  const notify = useCallback((type: 'success' | 'error' | 'info', message: string) => {
    if (type === 'success') toast.success(message);
    else if (type === 'error') toast.error(message);
    else toast.info(message);
  }, []);

  /* ---------- data loading ---------- */
  // 首屏通过 HTTP 拿一次初始数据，后续全部由 WebSocket 实时推送更新
  const onlineState = useAsync(() => api.get<OnlineResponse>('/ovpn/online-client'), []);
  const dashboardState = useAsync(() => api.get<DashboardSummary>('/ovpn/dashboard/summary'), []);

  // 实时数据状态（WS 推送覆盖首屏初始数据）
  const [dashboard, setDashboard] = useState<DashboardSummary | undefined>(undefined);
  const [online, setOnline] = useState<OnlineClient[]>([]);
  const [serverRuntime, setServerRuntime] = useState<OnlineResponse['server'] | undefined>(undefined);
  const [wsConnected, setWsConnected] = useState(false);

  // 首屏初始数据填充
  useEffect(() => {
    if (onlineState.data) {
      setOnline(onlineState.data.clients ?? []);
      setServerRuntime(onlineState.data.server);
    }
  }, [onlineState.data]);

  useEffect(() => {
    if (dashboardState.data) {
      setDashboard(dashboardState.data);
    }
  }, [dashboardState.data]);

  /* ---------- WebSocket 实时订阅：dashboard:stats ---------- */
  useEffect(() => {
    // 订阅连接状态，用于在卡片标题旁显示实时连接标识
    const offState = realtimeHub.onState((state) => {
      setWsConnected(state === 'open');
    });
    setWsConnected(realtimeHub.getState() === 'open');

    // 订阅 dashboard:stats topic，后端每 5s 推送一次概览全量数据
    const offSub = realtimeHub.subscribe<DashboardStatsPayload>('dashboard:stats', (payload) => {
      if (!payload) return;
      setDashboard(payload.summary);
      setOnline(payload.online ?? []);
      setServerRuntime(payload.server);
    });

    return () => {
      offState();
      offSub();
    };
  }, []);

  const stats = dashboard?.stats;
  // 过滤掉 management 不可用风险 —— 已提升到顶部导航呼吸灯提醒，避免在概览页重复展示
  const risks = (dashboard?.risks ?? []).filter((r) => !(r.level === 'danger' && /Management/i.test(r.title)));

  /* ---------- online table search & pagination ---------- */

  const [onlineSearch, setOnlineSearch] = useState('');

  const filteredOnline = online.filter((client) => {
    const keyword = onlineSearch.toLowerCase().trim();
    if (!keyword) return true;
    return [getClientName(client), client.vip, client.vip6, client.rip, client.rip6, client.commonName, client.common_name]
      .filter(Boolean)
      .some((v) => String(v).toLowerCase().includes(keyword));
  });

  const onlinePagination = usePagination(filteredOnline, onlineSearch);

  /* ---------- server actions ---------- */
  // 重启 OpenVPN / 编辑 server.conf 已迁至【系统设置 → 服务管理】

  async function killClient(client: OnlineClient) {
    setConfirmState({
      title: '断开在线连接',
      message: `确认断开 ${getClientName(client)} 吗？该用户需要重新连接 VPN。`,
      danger: true,
      onConfirm: async () => {
        await api.postForm('/ovpn/kill', { cid: client.id || client.cid });
        notify('success', '客户端已断开');
        // 无需手动刷新，后端采集器会在下一周期（≤5s）通过 WebSocket 推送最新数据
      },
    });
  }

  async function setBlacklist(client: OnlineClient, action: 'add_blacklist' | 'remove_blacklist') {
    try {
      const result = await api.postForm<{ message: string }>(`/ovpn/firewall?a=${action}`, { vip: clientVips(client) });
      notify('success', result.message || '操作成功');
      // 无需手动刷新，后端采集器会在下一周期（≤5s）通过 WebSocket 推送最新数据
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function openRateLimit(client: OnlineClient) {
    try {
      const rate = await api.get<{
        upQos?: { rate?: string; unit?: string };
        downQos?: { rate?: string; unit?: string };
      }>(`/ovpn/firewall?a=get_rateLimit&vip=${encodeURIComponent(client.vip || client.vip6 || '')}`);
      setModal({
        type: 'rate-limit',
        client,
        upload: rate.upQos?.rate || '',
        uploadUnit: rate.upQos?.unit || 'mbytes/second',
        download: rate.downQos?.rate || '',
        downloadUnit: rate.downQos?.unit || 'mbytes/second',
      });
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  /* ---------- online table columns ---------- */

  const onlineColumns: Column<OnlineClient>[] = [
    {
      key: 'name',
      header: '用户/客户端',
      render: (c) => getClientName(c),
    },
    {
      key: 'vip',
      header: 'VPN IP',
      render: (c) => c.vip || c.vip6 || '-',
    },
    {
      key: 'rip',
      header: '来源',
      render: (c) => c.rip || c.rip6 || '-',
    },
    {
      key: 'received',
      header: '接收',
      render: (c) => formatBytes(getClientBytes(c, 'received')),
    },
    {
      key: 'sent',
      header: '发送',
      render: (c) => formatBytes(getClientBytes(c, 'sent')),
    },
    {
      key: 'connDate',
      header: '上线时间',
      render: (c) => c.connDate || c.connectedSince || c.connected_since || '-',
    },
    {
      key: 'onlineTime',
      header: '在线时长',
      render: (c) => c.onlineTime || '-',
    },
    {
      key: 'actions',
      header: '操作',
      render: (c) => (
        <div className="flex items-center gap-1">
          <HasPermission code="client:kill">
            <Button variant="ghost" size="sm" className="h-7 text-destructive" onClick={() => killClient(c)}>
              <XCircle className="h-3.5 w-3.5 mr-1" />
              断开
            </Button>
          </HasPermission>
          <HasPermission code="firewall:update">
            <Button variant="ghost" size="sm" className="h-7" onClick={() => openRateLimit(c)}>
              <Gauge className="h-3.5 w-3.5 mr-1" />
              限速
            </Button>
          </HasPermission>
          <HasPermission code="firewall:create">
            <Button
              variant="ghost"
              size="sm"
              className="h-7"
              onClick={() => setBlacklist(c, c.isNftBlacklist || c.isNftBlackList ? 'remove_blacklist' : 'add_blacklist')}
            >
              {c.isNftBlacklist || c.isNftBlackList ? (
                <><ShieldCheck className="h-3.5 w-3.5 mr-1" />解网</>
              ) : (
                <><ShieldBan className="h-3.5 w-3.5 mr-1" />禁网</>
              )}
            </Button>
          </HasPermission>
        </div>
      ),
    },
  ];

  /* ---------- risk level → status mapping ---------- */

  function riskStatus(level: string): 'danger' | 'warning' | 'info' | 'neutral' {
    if (level === 'danger') return 'danger';
    if (level === 'warning') return 'warning';
    if (level === 'info') return 'info';
    return 'neutral';
  }

  /* ---------- render ---------- */

  return (
    <div className="space-y-6">
      {/* ---- Hero ---- */}
      <CardGlow>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5 text-primary transition-colors group-hover:text-[var(--accent)]" />
              OpenVPN 统一运维控制台
              <Wifi className={cn('h-4 w-4 ml-1 transition-colors', wsConnected ? 'text-emerald-500' : 'text-muted-foreground/50')} />
              <span className={cn('text-xs font-normal', wsConnected ? 'text-emerald-500' : 'text-muted-foreground/60')}>
                {wsConnected ? '实时' : '连接中'}
              </span>
            </CardTitle>
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
              <CardDescription>
                账号、客户端、防火墙、连接历史、证书与系统设置已统一接入，日常 VPN 管理都可以在这里完成。
              </CardDescription>
              {executiveDashboardEnabled && (
                <Button type="button" variant="outline" className="shrink-0" onClick={() => navigate('/screen')}>
                  <MonitorUp className="mr-2 h-4 w-4" />
                  运营大屏
                </Button>
              )}
            </div>
          </CardHeader>
        </Card>
      </CardGlow>

      {/* ---- System Real-time Monitor (WebSocket, no polling) ---- */}
      <SystemMonitor />

      {/* ---- Stats ---- */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="在线连接"
          icon={Activity}
          loading={!dashboard}
          value={stats?.onlineClients ?? online.length}
          description={onlineState.error ? '本地未连接 management' : 'WebSocket 实时推送'}
        />

        <StatCard
          title="账号总数"
          icon={Users}
          loading={!dashboard}
          value={stats?.totalUsers ?? 0}
          description={`${stats?.enabledUsers ?? 0} 个启用账号`}
        />

        <StatCard
          title="客户端配置"
          icon={FileCode2}
          loading={!dashboard}
          value={stats?.clientConfigs ?? 0}
          description="证书与 CCD 管理"
        />

        <StatCard
          title="今日上线"
          icon={LogIn}
          loading={!dashboard}
          value={stats?.todayConnections ?? 0}
          description="当天成功建立的连接次数"
        />
      </div>

      {/* ---- Risks ---- */}
      {risks.length > 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {risks.map((risk, idx) => (
            <CardGlow
              key={`${risk.title}-${idx}`}
              className={cn(
                'group transition-all duration-300',
                risk.level === 'danger'
                  ? 'border-l-4 border-l-destructive/70 hover:border-l-destructive'
                  : risk.level === 'warning'
                    ? 'border-l-4 border-l-amber-500/70 hover:border-l-amber-500'
                    : 'border-l-4 border-l-blue-500/70 hover:border-l-blue-500',
              )}
            >
              <Card>
                <CardContent className="pt-4 flex items-start gap-3">
                  <AlertTriangle className={`h-5 w-5 mt-0.5 shrink-0 ${risk.level === 'danger' ? 'text-destructive' : risk.level === 'warning' ? 'text-amber-500' : 'text-blue-500'}`} />
                  <div>
                    <p className="font-medium">{risk.title}</p>
                    <p className="text-sm text-muted-foreground">{risk.message}</p>
                  </div>
                  <StatusBadge status={riskStatus(risk.level)} className="ml-auto shrink-0">
                    {risk.level === 'danger' ? '高危' : risk.level === 'warning' ? '警告' : '提示'}
                  </StatusBadge>
                </CardContent>
              </Card>
            </CardGlow>
          ))}
        </div>
      )}

      {/* ---- Time-range user traffic ---- */}
      {executiveDashboardEnabled && dashboard && (
        <CardGlow>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <TrendingUp className="h-5 w-5" />
                用户流量分析
              </CardTitle>
              <CardDescription>在选定时间段内按用户汇总下载、上传和在线时长</CardDescription>
            </CardHeader>
            <CardContent>
              <DashboardTrafficUsers />
            </CardContent>
          </Card>
        </CardGlow>
      )}

      {/* ---- Server Runtime ---- */}
      {serverRuntime && (
        <CardGlow>
          <Card>
            <CardHeader>
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Server className="h-5 w-5" />
                  服务状态
                </CardTitle>
                <CardDescription>Server Runtime · WebSocket 实时</CardDescription>
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
                {[
                  { label: '地址', value: serverRuntime.Address },
                  { label: '状态', value: serverRuntime.Status },
                  { label: '入站', value: serverRuntime.BytesIn },
                  { label: '出站', value: serverRuntime.BytesOut },
                  { label: '运行时间', value: serverRuntime.RunDate },
                ].map((item) => (
                  <div key={item.label} className="space-y-1">
                    <p className="text-xs text-muted-foreground">{item.label}</p>
                    <p className="text-sm font-medium">{item.value || '-'}</p>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </CardGlow>
      )}

      {/* ---- Online Connections Table ---- */}
      <CardGlow>
        <Card>
          <CardHeader>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <PageHeader eyebrow="Live Tunnel" title="在线连接" />
              <div className="flex items-center gap-2">
                <Input
                  className="h-9 w-full sm:w-[240px]"
                  placeholder="搜索用户 / VPN IP / 来源 IP"
                  value={onlineSearch}
                  onChange={(e) => setOnlineSearch(e.target.value)}
                />
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {online.length ? (
              filteredOnline.length ? (
                <DataTable<OnlineClient>
                  columns={onlineColumns}
                  data={onlinePagination.pagedItems}
                  page={onlinePagination.page}
                  pageSize={onlinePagination.pageSize}
                  pageCount={onlinePagination.pageCount}
                  total={onlinePagination.total}
                  start={onlinePagination.start}
                  end={onlinePagination.end}
                  onPageChange={onlinePagination.setPage}
                  onPageSizeChange={onlinePagination.setPageSize}
                  keyFn={(c, idx) => String(c.id ?? c.cid ?? onlinePagination.start + idx)}
                  emptyTitle="没有匹配的在线连接"
                  emptyDescription="换个用户名、VPN IP 或来源 IP 再试试。"
                />
              ) : (
                <div className="flex flex-col items-center justify-center py-12 text-center">
                  <p className="text-lg font-medium">没有匹配的在线连接</p>
                  <p className="text-sm text-muted-foreground mt-1">换个用户名、VPN IP 或来源 IP 再试试。</p>
                </div>
              )
            ) : (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <p className="text-lg font-medium">暂无在线客户端</p>
                <p className="text-sm text-muted-foreground mt-1">
                  本地只启动 Web 服务时，这是正常现象；Docker 完整环境会显示真实连接。
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </CardGlow>

      {/* ---- Confirm Dialog ---- */}
      {confirmState && (
        <ConfirmDialog state={confirmState} onClose={() => setConfirmState(undefined)} />
      )}

      {/* ---- Rate Limit Modal ---- */}
      {modal.type === 'rate-limit' && (
        <RateLimitModal
          client={modal.client}
          upload={modal.upload}
          uploadUnit={modal.uploadUnit}
          download={modal.download}
          downloadUnit={modal.downloadUnit}
          notify={notify}
          onClose={() => setModal({ type: 'none' })}
        />
      )}

    </div>
  );
}

/* ========== Stat Card with default glow + hover spotlight ========== */

function StatCard({
  title,
  icon: Icon,
  value,
  description,
  loading,
}: {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  value: React.ReactNode;
  description: React.ReactNode;
  loading?: boolean;
}) {
  return (
    <CardGlow className="h-full">
      <Card className="h-full border-0 bg-transparent shadow-none">
        <CardHeader className="relative z-10 flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium text-foreground/80 transition-colors duration-300 group-hover:text-[var(--accent)]">
            {title}
          </CardTitle>
          <span
            className={cn(
              'flex h-9 w-9 items-center justify-center rounded-md',
              'bg-muted/60 text-muted-foreground',
              'transition-all duration-300',
              'group-hover:bg-[var(--accent)]/15 group-hover:text-[var(--accent)] group-hover:scale-110',
              'group-hover:shadow-[0_0_12px_-2px_var(--accent)]',
            )}
          >
            <Icon className="h-4 w-4" />
          </span>
        </CardHeader>
        <CardContent className="relative z-10">
          <div className="text-2xl font-bold tracking-tight transition-colors duration-300 group-hover:text-[var(--accent)]">
            {loading ? <span className="text-muted-foreground">--</span> : value}
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        </CardContent>
      </Card>
    </CardGlow>
  );
}

/* ========== Rate Limit Modal ========== */

function RateLimitModal({
  client,
  upload: initialUpload,
  uploadUnit: initialUploadUnit,
  download: initialDownload,
  downloadUnit: initialDownloadUnit,
  notify,
  onClose,
}: {
  client: OnlineClient;
  upload: string;
  uploadUnit: string;
  download: string;
  downloadUnit: string;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  onClose: () => void;
}) {
  const [upload, setUpload] = useState(initialUpload);
  const [uploadUnit, setUploadUnit] = useState(initialUploadUnit);
  const [download, setDownload] = useState(initialDownload);
  const [downloadUnit, setDownloadUnit] = useState(initialDownloadUnit);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});

  const unitOptions = [
    { value: 'bytes/second', label: 'B/s' },
    { value: 'kbytes/second', label: 'KB/s' },
    { value: 'mbytes/second', label: 'MB/s' },
  ];

  function clearError(key: string) {
    setErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function validate(): FieldErrors {
    const e: FieldErrors = {};
    if (!upload.trim() && !download.trim()) {
      e.upload = '请至少填写上传或下载速率';
      e.download = '请至少填写上传或下载速率';
    }
    return e;
  }

  async function handleSave(ev: React.FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    // 前端校验失败：仅在字段下方提示，不弹 toast
    if (Object.keys(e).length) return;
    setSaving(true);
    try {
      await api.postForm('/ovpn/firewall?a=set_rateLimit', {
        vip: client.vip || client.vip6 || '',
        upload: upload.trim(),
        uploadUnit,
        download: download.trim(),
        downloadUnit,
      });
      notify('success', '限速规则已更新');
      // 无需手动刷新，后端采集器会在下一周期（≤5s）通过 WebSocket 推送最新数据
      onClose();
    } catch (error) {
      // 后端错误：使用 toast 提醒
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>限速：{getClientName(client)}</DialogTitle>
          <DialogDescription>设置客户端上传和下载速率限制</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSave} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">上传速率</Label>
            <div className="min-w-0 space-y-2">
              <div className="flex flex-col gap-2 sm:flex-row">
                <div className="min-w-0 flex-1">
                  <Input
                    value={upload}
                    onChange={(ev) => { setUpload(ev.target.value); clearError('upload'); }}
                    placeholder="速率值"
                    aria-invalid={errors.upload ? 'true' : undefined}
                    className={cn(errors.upload && 'border-destructive focus-visible:ring-destructive/40')}
                  />
                </div>
                <Select value={uploadUnit} onValueChange={setUploadUnit}>
                  <SelectTrigger className="w-full sm:w-[100px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {unitOptions.map((u) => <SelectItem key={u.value} value={u.value}>{u.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              {errors.upload && <p className="text-xs text-destructive">{errors.upload}</p>}
            </div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">下载速率</Label>
            <div className="min-w-0 space-y-2">
              <div className="flex flex-col gap-2 sm:flex-row">
                <div className="min-w-0 flex-1">
                  <Input
                    value={download}
                    onChange={(ev) => { setDownload(ev.target.value); clearError('download'); }}
                    placeholder="速率值"
                    aria-invalid={errors.download ? 'true' : undefined}
                    className={cn(errors.download && 'border-destructive focus-visible:ring-destructive/40')}
                  />
                </div>
                <Select value={downloadUnit} onValueChange={setDownloadUnit}>
                  <SelectTrigger className="w-full sm:w-[100px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {unitOptions.map((u) => <SelectItem key={u.value} value={u.value}>{u.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              {errors.download && <p className="text-xs text-destructive">{errors.download}</p>}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={saving}>取消</Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存限速'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
