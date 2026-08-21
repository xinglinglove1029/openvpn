import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ComponentProps, ComponentType, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Activity,
  AlertTriangle,
  ArrowDownToLine,
  ArrowLeft,
  ArrowUpFromLine,
  CheckCircle2,
  Expand,
  Gauge,
  Monitor,
  RefreshCw,
  Server,
  ShieldAlert,
  Users,
  Wifi,
  WifiOff,
  XCircle,
} from 'lucide-react';
import { Button } from '@/ui/button';
import { Badge } from '@/ui/badge';
import { api } from '@/api';
import { realtimeHub } from '@/lib/notificationHub';
import { formatBytes, getClientName } from '@/lib/format';
import type {
  DashboardGeoResponse,
  DashboardGeoSource,
  DashboardStatsPayload,
  DashboardSummary,
  DashboardTrafficUsersResponse,
  OnlineClient,
  OnlineResponse,
} from '@/types';
import { cn } from '@/lib/utils';
import { useAuth } from '@/store/auth';
import { OperationsMap } from '@/components/ExecutiveDashboard/OperationsMap';

const MAX_TREND_POINTS = 18;

type TrendPoint = { at: number; online: number };
type LoadState = 'loading' | 'ready' | 'error';
type OnlineLoadState = LoadState | 'forbidden';

const ZERO_TRAFFIC_TOTALS: DashboardTrafficUsersResponse['totals'] = {
  activeUsers: 0,
  received: 0,
  sent: 0,
  total: 0,
  receivedText: '0 B',
  sentText: '0 B',
  totalText: '0 B',
};

function asFiniteNumber(value: unknown, fallback = 0) {
  const number = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(number) ? number : fallback;
}

function normalizeTraffic(value: unknown): DashboardTrafficUsersResponse | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const source = value as Record<string, unknown>;
  const rawTotals =
    source.totals && typeof source.totals === 'object' ? (source.totals as Record<string, unknown>) : {};
  const users = Array.isArray(source.users)
    ? source.users
        .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object')
        .map((user) => ({
          username: String(user.username ?? '未知用户'),
          commonName: user.commonName ? String(user.commonName) : undefined,
          online: Boolean(user.online),
          connections: asFiniteNumber(user.connections),
          onlineSeconds: asFiniteNumber(user.onlineSeconds),
          received: asFiniteNumber(user.received),
          sent: asFiniteNumber(user.sent),
          total: asFiniteNumber(user.total),
          receivedText: String(user.receivedText ?? formatBytes(asFiniteNumber(user.received))),
          sentText: String(user.sentText ?? formatBytes(asFiniteNumber(user.sent))),
          totalText: String(user.totalText ?? formatBytes(asFiniteNumber(user.total))),
          lastSeen: asFiniteNumber(user.lastSeen),
        }))
    : [];
  return {
    start: asFiniteNumber(source.start),
    end: asFiniteNumber(source.end),
    sampleSeconds: asFiniteNumber(source.sampleSeconds),
    users,
    totals: {
      ...ZERO_TRAFFIC_TOTALS,
      activeUsers: asFiniteNumber(rawTotals.activeUsers),
      received: asFiniteNumber(rawTotals.received),
      sent: asFiniteNumber(rawTotals.sent),
      total: asFiniteNumber(rawTotals.total),
      receivedText: String(rawTotals.receivedText ?? formatBytes(asFiniteNumber(rawTotals.received))),
      sentText: String(rawTotals.sentText ?? formatBytes(asFiniteNumber(rawTotals.sent))),
      totalText: String(rawTotals.totalText ?? formatBytes(asFiniteNumber(rawTotals.total))),
    },
  };
}

function StatCard({
  label,
  value,
  caption,
  icon: Icon,
  tone = 'primary',
}: {
  label: string;
  value: string | number;
  caption: string;
  icon: ComponentType<{ className?: string }>;
  tone?: 'primary' | 'sky' | 'violet' | 'emerald';
}) {
  const tones = {
    primary: 'from-primary/18 to-primary/5 text-primary',
    sky: 'from-sky-500/18 to-sky-500/5 text-sky-500',
    violet: 'from-violet-500/18 to-violet-500/5 text-violet-500',
    emerald: 'from-emerald-500/18 to-emerald-500/5 text-emerald-500',
  };
  return (
    <div className="relative overflow-hidden rounded-2xl border border-border/60 bg-card/75 p-4 shadow-sm backdrop-blur-xl transition-colors hover:border-primary/40">
      <div className={cn('absolute -right-6 -top-8 h-24 w-24 rounded-full bg-gradient-to-br blur-2xl', tones[tone])} />
      <div className="relative flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium text-muted-foreground">{label}</p>
          <p className="mt-2 text-2xl font-semibold tracking-tight tabular-nums text-foreground">{value}</p>
          <p className="mt-1 text-[11px] text-muted-foreground">{caption}</p>
        </div>
        <div className={cn('rounded-xl bg-muted/60 p-2.5', tones[tone].split(' ').at(-1))}>
          <Icon className="h-5 w-5" />
        </div>
      </div>
    </div>
  );
}

function Panel({
  title,
  subtitle,
  icon: Icon,
  children,
  className,
}: {
  title: string;
  subtitle?: string;
  icon: ComponentType<{ className?: string }>;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn(
        'overflow-hidden rounded-2xl border border-border/60 bg-card/70 shadow-sm backdrop-blur-xl',
        className
      )}
    >
      <div className="flex items-start justify-between gap-4 border-b border-border/50 px-4 py-3.5 sm:px-5">
        <div className="flex min-w-0 items-center gap-2.5">
          <div className="rounded-lg bg-primary/10 p-2 text-primary">
            <Icon className="h-4 w-4" />
          </div>
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold text-foreground">{title}</h2>
            {subtitle && <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{subtitle}</p>}
          </div>
        </div>
      </div>
      <div className="p-4 sm:p-5">{children}</div>
    </section>
  );
}

function TrendChart({ points }: { points: TrendPoint[] }) {
  const values = points.map((point) => point.online);
  const max = Math.max(1, ...values);
  const min = Math.min(0, ...values);
  const range = Math.max(1, max - min);
  const width = 640;
  const height = 190;
  const coords = points
    .map((point, index) => {
      const x = points.length <= 1 ? width / 2 : (index / (points.length - 1)) * width;
      const y = height - ((point.online - min) / range) * (height - 20) - 10;
      return `${x},${y}`;
    })
    .join(' ');

  if (!points.length) {
    return (
      <div className="flex h-[210px] items-center justify-center rounded-xl border border-dashed border-border/60 bg-background/20 text-sm text-muted-foreground">
        等待实时连接数据…
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border/50 bg-background/25 p-2">
      <svg viewBox={`0 0 ${width} ${height}`} className="h-[190px] w-full" role="img" aria-label="在线连接趋势">
        <defs>
          <linearGradient id="screenTrendFill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="var(--accent)" stopOpacity=".28" />
            <stop offset="100%" stopColor="var(--accent)" stopOpacity="0" />
          </linearGradient>
        </defs>
        {[0, 1, 2, 3].map((line) => {
          const y = 10 + (line / 3) * (height - 20);
          return (
            <line
              key={line}
              x1="0"
              x2={width}
              y1={y}
              y2={y}
              stroke="currentColor"
              className="text-border/50"
              strokeDasharray="4 8"
            />
          );
        })}
        {points.length > 1 && (
          <polygon points={`0,${height} ${coords} ${width},${height}`} fill="url(#screenTrendFill)" />
        )}
        <polyline
          points={coords}
          fill="none"
          stroke="var(--accent)"
          strokeWidth="4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {points.slice(-1).map((point, index) => {
          const x = points.length <= 1 ? width / 2 : ((points.length - 1) / (points.length - 1)) * width;
          const y = height - ((point.online - min) / range) * (height - 20) - 10;
          return <circle key={index} cx={x} cy={y} r="6" fill="var(--card)" stroke="var(--accent)" strokeWidth="4" />;
        })}
      </svg>
      <div className="flex items-center justify-between px-2 text-[10px] text-muted-foreground">
        <span>{points.length > 1 ? `${points.length} 个实时采样点` : '刚刚连接'}</span>
        <span className="font-medium text-foreground">当前 {points.at(-1)?.online ?? 0} 人</span>
      </div>
    </div>
  );
}

function Topology({ online, state }: { online: OnlineClient[]; state: OnlineLoadState }) {
  const nodes = online.slice(0, 8);
  return (
    <div className="relative min-h-[330px] overflow-x-auto overflow-y-hidden rounded-xl border border-border/50 bg-[radial-gradient(circle_at_center,color-mix(in_srgb,var(--accent)_12%,transparent),transparent_58%)] p-4">
      <div className="pointer-events-none absolute inset-0 opacity-30 [background-image:linear-gradient(color-mix(in_srgb,var(--accent)_12%,transparent)_1px,transparent_1px),linear-gradient(90deg,color-mix(in_srgb,var(--accent)_12%,transparent)_1px,transparent_1px)] [background-size:32px_32px]" />
      {state === 'forbidden' ? (
        <div className="absolute inset-0 flex items-center justify-center px-6 text-center text-sm text-muted-foreground">
          当前账号没有在线客户端明细权限，已隐藏连接节点。
        </div>
      ) : state === 'error' ? (
        <div className="absolute inset-0 flex items-center justify-center px-6 text-center text-sm text-destructive">
          在线客户端数据暂时不可用，请稍后重试。
        </div>
      ) : (
        <>
          <div className="absolute left-1/2 top-1/2 z-10 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center">
            <div className="flex h-20 w-20 items-center justify-center rounded-full border-4 border-primary/30 bg-primary/15 shadow-[0_0_45px_color-mix(in_srgb,var(--accent)_32%,transparent)]">
              <Server className="h-8 w-8 text-primary" />
            </div>
            <span className="mt-2 whitespace-nowrap rounded-full bg-card/90 px-3 py-1 text-xs font-semibold shadow-sm">
              OpenVPN Server
            </span>
          </div>
          {nodes.length > 0 ? (
            nodes.map((client, index) => {
              const angle = (index / Math.max(nodes.length, 1)) * Math.PI * 2 - Math.PI / 2;
              const left = 50 + Math.cos(angle) * 33;
              const top = 50 + Math.sin(angle) * 33;
              return (
                <div
                  key={`${getClientName(client)}-${index}`}
                  className="absolute z-10 -translate-x-1/2 -translate-y-1/2"
                  style={{ left: `${left}%`, top: `${top}%` }}
                >
                  <div
                    className="absolute left-1/2 top-1/2 h-px w-[clamp(70px,15vw,150px)] origin-left -translate-y-1/2 bg-gradient-to-r from-primary/65 to-transparent"
                    style={{ transform: `rotate(${Math.atan2(50 - top, 50 - left)}rad)` }}
                  />
                  <div className="relative flex w-max max-w-[min(120px,28vw)] items-center gap-1.5 rounded-xl border border-border/60 bg-card/90 px-2 py-1.5 shadow-lg backdrop-blur">
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-500">
                      <Wifi className="h-3.5 w-3.5" />
                    </span>
                    <span className="truncate text-[10px] font-medium">{getClientName(client)}</span>
                  </div>
                </div>
              );
            })
          ) : (
            <div className="absolute inset-x-0 bottom-6 text-center text-xs text-muted-foreground">
              暂无在线客户端，服务节点仍保持可见
            </div>
          )}
          {online.length > nodes.length && (
            <div className="absolute bottom-3 right-3 rounded-full bg-muted/80 px-2 py-1 text-[10px] text-muted-foreground">
              另有 {online.length - nodes.length} 个客户端
            </div>
          )}
        </>
      )}
    </div>
  );
}

function statusText(ok: boolean, status?: string) {
  if (!ok) return '异常';
  if (!status) return '正常';
  return status.toLowerCase().includes('run') || status.toLowerCase() === 'up' ? '运行中' : status;
}

export default function ExecutiveDashboardPage() {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();
  const canViewOnline = hasPermission('client:view_online');
  const [summary, setSummary] = useState<DashboardSummary>();
  const [online, setOnline] = useState<OnlineClient[]>([]);
  const [server, setServer] = useState<OnlineResponse['server']>();
  const [traffic, setTraffic] = useState<DashboardTrafficUsersResponse>();
  const [geo, setGeo] = useState<DashboardGeoResponse>();
  const [geoSource, setGeoSource] = useState<DashboardGeoSource>('online');
  const [geoView, setGeoView] = useState<'world' | 'china'>('world');
  const [summaryState, setSummaryState] = useState<LoadState>('loading');
  const [onlineState, setOnlineState] = useState<OnlineLoadState>(canViewOnline ? 'loading' : 'forbidden');
  const [trafficState, setTrafficState] = useState<LoadState>('loading');
  const [geoState, setGeoState] = useState<LoadState>('loading');
  const [wsConnected, setWsConnected] = useState(false);
  const trendSampleRef = useRef<{ at: number; online: number } | undefined>(undefined);
  const snapshotRequestRef = useRef(0);
  const [trend, setTrend] = useState<TrendPoint[]>([]);
  const [lastUpdated, setLastUpdated] = useState<number>();
  const [fullscreen, setFullscreen] = useState(false);

  const appendTrend = useCallback((count: number, at = Date.now()) => {
    setTrend((current) => {
      const previous = trendSampleRef.current;
      if (previous && at - previous.at < 3000 && previous.online === count) return current;
      trendSampleRef.current = { at, online: count };
      return [...current, { at, online: count }].slice(-MAX_TREND_POINTS);
    });
  }, []);

  const loadSnapshot = useCallback(async () => {
    const requestId = ++snapshotRequestRef.current;
    const end = Math.floor(Date.now() / 1000);
    const start = end - 24 * 60 * 60;
    const [summaryResult, trafficResult, geoResult] = await Promise.allSettled([
      api.get<DashboardSummary>('/ovpn/dashboard/summary'),
      api.get<DashboardTrafficUsersResponse>(`/ovpn/dashboard/traffic-users?start=${start}&end=${end}`),
      api.get<DashboardGeoResponse>(`/ovpn/dashboard/geo-map?start=${start}&end=${end}`),
    ]);
    if (requestId !== snapshotRequestRef.current) return;
    if (summaryResult.status === 'fulfilled' && summaryResult.value?.stats) {
      setSummary(summaryResult.value);
      setSummaryState('ready');
    } else setSummaryState('error');
    if (trafficResult.status === 'fulfilled') {
      const normalized = normalizeTraffic(trafficResult.value);
      if (normalized) {
        setTraffic(normalized);
        setTrafficState('ready');
      } else setTrafficState('error');
    } else setTrafficState('error');
    if (geoResult.status === 'fulfilled') {
      setGeo(geoResult.value);
      setGeoState('ready');
      setGeoSource((current) =>
        geoResult.value.availableSources.includes(current) ? current : geoResult.value.availableSources[0] || 'online'
      );
    } else setGeoState('error');

    if (!canViewOnline) {
      setOnlineState('forbidden');
      setLastUpdated(Date.now());
      return;
    }
    const onlineResult = await Promise.allSettled([api.get<OnlineResponse>('/ovpn/online-client')]);
    const result = onlineResult[0];
    if (requestId !== snapshotRequestRef.current) return;
    if (result.status === 'fulfilled') {
      setOnline(result.value.clients ?? []);
      setServer(result.value.server);
      setOnlineState('ready');
      appendTrend(result.value.clients?.length ?? 0);
    } else setOnlineState('error');
    setLastUpdated(Date.now());
  }, [appendTrend, canViewOnline]);

  useEffect(() => {
    void loadSnapshot();
    const timer = window.setInterval(() => {
      void loadSnapshot();
    }, 60_000);
    return () => window.clearInterval(timer);
  }, [loadSnapshot]);

  useEffect(() => {
    const offState = realtimeHub.onState((state) => setWsConnected(state === 'open'));
    setWsConnected(realtimeHub.getState() === 'open');
    const offReconnect = realtimeHub.subscribe<null>('ws:reconnected', () => {
      void loadSnapshot();
    });
    const offSub = realtimeHub.subscribe<DashboardStatsPayload>('dashboard:stats', (payload) => {
      if (!payload?.summary?.stats) return;
      setSummary(payload.summary);
      setSummaryState('ready');
      if (canViewOnline) {
        setOnline(payload.online ?? []);
        setOnlineState('ready');
        setServer(payload.server);
        appendTrend(payload.online?.length ?? 0, payload.pushedAt ? payload.pushedAt * 1000 : Date.now());
      }
      setLastUpdated(payload.pushedAt ? payload.pushedAt * 1000 : Date.now());
    });
    return () => {
      offState();
      offReconnect();
      offSub();
    };
  }, [appendTrend, canViewOnline, loadSnapshot]);

  useEffect(() => {
    const selectedPoints = (geo?.points ?? []).filter((point) => point.source === geoSource);
    const hasChinaPoint = selectedPoints.some(
      (point) => point.country.includes('中国') || point.country.toLowerCase() === 'china'
    );
    if (geoView === 'china' && !hasChinaPoint) setGeoView('world');
  }, [geo, geoSource, geoView]);

  useEffect(() => {
    const onFullscreen = () => setFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', onFullscreen);
    return () => document.removeEventListener('fullscreenchange', onFullscreen);
  }, []);

  const fullscreenSupported = typeof document !== 'undefined' && Boolean(document.fullscreenEnabled);
  const toggleFullscreen = async () => {
    if (!fullscreenSupported) return;
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await document.documentElement.requestFullscreen();
    } catch {
      // 浏览器不支持或用户拒绝时，保留普通页面模式即可
    }
  };

  const stats = summary?.stats;
  const topUsers = useMemo(() => [...(traffic?.users ?? [])].sort((a, b) => b.total - a.total).slice(0, 8), [traffic]);
  const trafficTotals = traffic?.totals ?? ZERO_TRAFFIC_TOTALS;
  const received = trafficState === 'ready' ? trafficTotals.received : 0;
  const sent = trafficState === 'ready' ? trafficTotals.sent : 0;
  const totalTraffic = trafficState === 'ready' ? trafficTotals.total : 0;
  const receivedPercent = totalTraffic > 0 ? Math.round((received / totalTraffic) * 100) : 0;
  const serviceOk = stats?.managementOk ?? false;
  const risks = summary?.risks ?? [];
  const status = statusText(serviceOk, stats?.serverStatus);

  return (
    <div className="min-h-full space-y-4 pb-3 sm:space-y-5">
      <header className="relative overflow-hidden rounded-3xl border border-primary/20 bg-gradient-to-br from-primary/10 via-card/80 to-violet-500/10 p-5 shadow-sm backdrop-blur-xl sm:p-7">
        <div className="absolute -right-16 -top-24 h-64 w-64 rounded-full bg-primary/15 blur-3xl" />
        <div className="absolute -bottom-28 left-1/3 h-64 w-64 rounded-full bg-violet-500/10 blur-3xl" />
        <div className="relative flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="mb-3 flex items-center gap-2 text-xs font-medium uppercase tracking-[0.24em] text-primary">
              <Monitor className="h-4 w-4" /> Operations Center
            </div>
            <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">OpenVPN 运营大屏</h1>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
              实时连接、用户流量、服务健康与风险态势一屏掌握。数据范围遵循当前账号权限。
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={wsConnected ? 'default' : 'secondary'} className="gap-1.5 px-3 py-1.5">
              <span
                className={cn('h-1.5 w-1.5 rounded-full', wsConnected ? 'bg-emerald-400' : 'bg-muted-foreground')}
              />
              {wsConnected ? '实时连接' : '实时连接中'}
            </Badge>
            {lastUpdated && (
              <span className="text-xs text-muted-foreground">
                更新于{' '}
                {new Date(lastUpdated).toLocaleTimeString('zh-CN', {
                  hour: '2-digit',
                  minute: '2-digit',
                  second: '2-digit',
                })}
              </span>
            )}
            <Button variant="outline" size="sm" onClick={() => navigate('/overview')}>
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              返回概览
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={!fullscreenSupported}
              title={fullscreenSupported ? '切换浏览器全屏' : '当前浏览器环境不支持全屏'}
              onClick={() => void toggleFullscreen()}
            >
              <Expand className="mr-1.5 h-4 w-4" />
              {fullscreen ? '退出全屏' : '全屏展示'}
            </Button>
          </div>
        </div>
      </header>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="在线连接"
          value={stats?.onlineClients ?? online.length}
          caption={serviceOk ? 'management 实时状态' : 'management 暂不可用'}
          icon={Activity}
          tone="primary"
        />
        <StatCard
          label="账号总数"
          value={stats?.totalUsers ?? 0}
          caption={`${stats?.enabledUsers ?? 0} 个启用账号`}
          icon={Users}
          tone="sky"
        />
        <StatCard
          label="24 小时总流量"
          value={formatBytes(totalTraffic)}
          caption={`下载占比 ${receivedPercent}%`}
          icon={Gauge}
          tone="violet"
        />
        <StatCard
          label="今日上线"
          value={stats?.todayConnections ?? 0}
          caption={`${stats?.clientConfigs ?? 0} 个客户端配置`}
          icon={Activity}
          tone="emerald"
        />
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Panel
          title="连接拓扑"
          subtitle="VPN 服务与在线客户端关系（非地理位置）"
          icon={Share2Icon}
          className="xl:col-span-7"
        >
          <Topology online={online} state={onlineState} />
        </Panel>
        <Panel title="在线连接趋势" subtitle="dashboard:stats 实时采样" icon={Activity} className="xl:col-span-5">
          <TrendChart points={trend} />
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Panel
          title="地理态势"
          subtitle="客户端来源、管理操作与网站目标服务分层展示"
          icon={Monitor}
          className="xl:col-span-7"
        >
          <OperationsMap
            data={geo}
            source={geoSource}
            view={geoView}
            onSourceChange={setGeoSource}
            onViewChange={setGeoView}
            loading={geoState === 'loading'}
            error={geoState === 'error'}
          />
        </Panel>
        <Panel title="服务健康" subtitle="运行状态与管理通道" icon={Server} className="xl:col-span-5">
          <div className="space-y-3">
            <HealthRow
              icon={serviceOk ? CheckCircle2 : XCircle}
              label="OpenVPN management"
              value={status}
              ok={serviceOk}
            />
            <HealthRow
              icon={server?.RunDate ? CheckCircle2 : WifiOff}
              label="运行信息"
              value={server?.RunDate || '暂无信息'}
              ok={Boolean(server?.RunDate)}
            />
            <HealthRow
              icon={server?.Address ? CheckCircle2 : WifiOff}
              label="监听地址"
              value={server?.Address || '暂无信息'}
              ok={Boolean(server?.Address)}
            />
            <HealthRow
              icon={Wifi}
              label="WebSocket 实时通道"
              value={wsConnected ? '已连接' : '连接中'}
              ok={wsConnected}
            />
            {!serviceOk && (
              <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-xs leading-5 text-destructive">
                <strong>管理通道异常：</strong>在线客户端可能暂时无法读取，账号统计和页面布局仍可正常展示。
              </div>
            )}
          </div>
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <Panel
          title="用户流量排行"
          subtitle="最近 24 小时，按用户汇总"
          icon={ArrowDownToLine}
          className="xl:col-span-7"
        >
          {trafficState === 'loading' ? (
            <div className="flex h-60 items-center justify-center text-sm text-muted-foreground">
              <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
              正在加载流量统计…
            </div>
          ) : trafficState === 'error' ? (
            <EmptyState
              title="流量数据暂不可用"
              description="时间段流量接口返回异常，已避免使用在线会话累计值冒充统计结果。"
            />
          ) : topUsers.length ? (
            <div className="space-y-3">
              {topUsers.map((user, index) => {
                const percent = user.total > 0 ? Math.round((user.total / Math.max(1, topUsers[0].total)) * 100) : 0;
                return (
                  <div
                    key={`${user.username}-${index}`}
                    className="grid grid-cols-[24px_minmax(90px,1fr)_minmax(100px,1.8fr)_auto] items-center gap-2 text-xs sm:gap-3"
                  >
                    <span className="text-center font-semibold text-muted-foreground">
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span className="truncate font-medium">{user.username}</span>
                    <div className="h-2 overflow-hidden rounded-full bg-muted/70">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-primary to-violet-500 transition-all"
                        style={{ width: `${percent}%` }}
                      />
                    </div>
                    <span className="text-right font-semibold tabular-nums">{formatBytes(user.total)}</span>
                  </div>
                );
              })}
              <div className="grid grid-cols-2 gap-3 pt-2">
                <div className="rounded-xl bg-sky-500/10 p-3">
                  <div className="flex items-center gap-1.5 text-xs text-sky-500">
                    <ArrowDownToLine className="h-3.5 w-3.5" />
                    下载
                  </div>
                  <p className="mt-1 text-lg font-semibold tabular-nums">{formatBytes(received)}</p>
                </div>
                <div className="rounded-xl bg-violet-500/10 p-3">
                  <div className="flex items-center gap-1.5 text-xs text-violet-500">
                    <ArrowUpFromLine className="h-3.5 w-3.5" />
                    上传
                  </div>
                  <p className="mt-1 text-lg font-semibold tabular-nums">{formatBytes(sent)}</p>
                </div>
              </div>
            </div>
          ) : (
            <EmptyState title="暂无时间段流量" description="等待连接历史采样数据产生" />
          )}
        </Panel>
      </div>

      <Panel title="风险与运维提示" subtitle="沿用概览页风险计算，不提供危险操作" icon={ShieldAlert}>
        {risks.length ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {risks.map((risk, index) => (
              <RiskCard key={`${risk.title}-${index}`} risk={risk} />
            ))}
          </div>
        ) : (
          <EmptyState title="当前没有待处理风险" description="系统未发现账号、证书、防火墙或服务状态异常。" compact />
        )}
      </Panel>

      {(summaryState === 'error' || (summaryState === 'ready' && !summary)) && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-300">
          概览统计暂时不可用，大屏仍保留可用的实时连接和流量数据。
        </div>
      )}
    </div>
  );
}

function HealthRow({
  icon: Icon,
  label,
  value,
  ok,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
  ok: boolean;
}) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-border/50 bg-background/25 px-3 py-3">
      <Icon className={cn('h-4 w-4 shrink-0', ok ? 'text-emerald-500' : 'text-destructive')} />
      <span className="min-w-0 flex-1 text-sm">{label}</span>
      <span className={cn('max-w-[50%] truncate text-right text-xs', ok ? 'text-foreground' : 'text-destructive')}>
        {value}
      </span>
    </div>
  );
}

function RiskCard({ risk }: { risk: DashboardSummary['risks'][number] }) {
  const danger = risk.level === 'danger';
  const warning = risk.level === 'warning';
  return (
    <div
      className={cn(
        'rounded-xl border p-3',
        danger
          ? 'border-destructive/30 bg-destructive/10'
          : warning
            ? 'border-amber-500/30 bg-amber-500/10'
            : 'border-sky-500/30 bg-sky-500/10'
      )}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle
          className={cn(
            'mt-0.5 h-4 w-4 shrink-0',
            danger ? 'text-destructive' : warning ? 'text-amber-500' : 'text-sky-500'
          )}
        />
        <div className="min-w-0">
          <p className="text-sm font-medium">{risk.title}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{risk.message}</p>
        </div>
      </div>
    </div>
  );
}

function EmptyState({
  title,
  description,
  compact = false,
}: {
  title: string;
  description: string;
  compact?: boolean;
}) {
  return (
    <div className={cn('flex flex-col items-center justify-center text-center', compact ? 'py-3' : 'h-60')}>
      <Activity className="h-7 w-7 text-muted-foreground/50" />
      <p className="mt-2 text-sm font-medium">{title}</p>
      <p className="mt-1 max-w-md text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

function Share2Icon(props: ComponentProps<typeof Activity>) {
  return (
    <svg
      {...props}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="18" cy="5" r="3" />
      <circle cx="6" cy="12" r="3" />
      <circle cx="18" cy="19" r="3" />
      <line x1="8.59" y1="13.51" x2="15.42" y2="17.49" />
      <line x1="15.41" y1="6.51" x2="8.59" y2="10.49" />
    </svg>
  );
}
