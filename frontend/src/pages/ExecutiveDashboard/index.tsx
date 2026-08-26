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
import { Tooltip } from '@/ui/tooltip';
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
  tone?: 'primary' | 'accent' | 'emerald';
}) {
  const tones = {
    primary: 'from-primary/18 to-primary/5 text-primary',
    accent: 'from-accent/18 to-accent/5 text-accent',
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
  contentClassName,
}: {
  title: string;
  subtitle?: string;
  icon: ComponentType<{ className?: string }>;
  children: ReactNode;
  className?: string;
  contentClassName?: string;
}) {
  return (
    <section
      className={cn(
        'overflow-hidden rounded-2xl border border-border/60 bg-card/70 shadow-sm backdrop-blur-xl',
        className
      )}
    >
      <div className="shrink-0 flex items-start justify-between gap-4 border-b border-border/50 px-4 py-3.5 sm:px-5">
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
      <div className={cn('p-4 sm:p-5', contentClassName)}>{children}</div>
    </section>
  );
}

function TrendChart({ points, compact = false }: { points: TrendPoint[]; compact?: boolean }) {
  const chartHeight = compact ? 154 : 190;
  const emptyHeight = compact ? 174 : 210;
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
      <div style={{ height: emptyHeight }} className="flex items-center justify-center rounded-xl border border-dashed border-border/60 bg-background/20 text-sm text-muted-foreground">
        等待实时连接数据…
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border/50 bg-background/25 p-2">
      <svg viewBox={`0 0 ${width} ${height}`} style={{ height: chartHeight }} className="w-full" role="img" aria-label="在线连接趋势">
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
    <div className="relative min-h-[330px] h-full overflow-x-auto overflow-y-hidden rounded-xl border border-border/50 bg-[radial-gradient(circle_at_center,color-mix(in_srgb,var(--accent)_12%,transparent),transparent_58%)] p-4 xl:min-h-0">
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
                  <Tooltip
                    side="top"
                    delayMs={120}
                    content={
                      <span className="inline-flex items-center gap-1.5">
                        <span className="text-muted-foreground">公网 IP</span>
                        <code className="font-mono text-[11px] text-foreground">{client.rip?.trim() || client.rip6?.trim() || '暂不可用'}</code>
                        {client.vip && <span className="text-muted-foreground">· VPN {client.vip}</span>}
                      </span>
                    }
                  >
                    <div className="relative flex w-max max-w-[min(120px,28vw)] cursor-help items-center gap-1.5 rounded-xl border border-border/60 bg-card/90 px-2 py-1.5 shadow-lg backdrop-blur transition-colors hover:border-emerald-500/55 hover:bg-card">
                      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-500">
                        <Wifi className="h-3.5 w-3.5" />
                      </span>
                      <span className="truncate text-[10px] font-medium">{getClientName(client)}</span>
                    </div>
                  </Tooltip>
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
  const [geoView, setGeoView] = useState<'world' | 'china' | 'country'>('world');
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
    <div className="relative min-h-full overflow-hidden pb-6">
      <div className="screen-page-ambient pointer-events-none absolute inset-x-0 top-0 -z-10 h-[680px]" />

      <header className="relative mb-4 overflow-hidden rounded-[1.7rem] border border-primary/15 bg-card/80 px-4 py-4 shadow-lg shadow-primary/10 backdrop-blur-xl sm:px-6 sm:py-5">
        <div className="screen-header-veil pointer-events-none absolute inset-y-0 right-0 w-[38%]" />
        <div className="relative flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div className="flex min-w-0 items-center gap-3 sm:gap-4">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl border border-primary/20 bg-gradient-to-br from-primary/15 via-primary/10 to-accent/20 text-primary shadow-inner">
              <Monitor className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-primary">
                <span>Secure Operations Command</span>
                <span className="hidden h-1 w-1 rounded-full bg-primary/50 sm:inline-block" />
                <span className="normal-case tracking-normal text-muted-foreground">区域级态势 · 实时数据</span>
              </div>
              <h1 className="mt-1 text-xl font-semibold tracking-tight text-foreground sm:text-2xl">OpenVPN 安全运营指挥中心</h1>
              <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
                以连接、流量、服务与区域态势为主线组织信息；地图可旋转、缩放并聚焦区域节点。
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={wsConnected ? 'default' : 'secondary'} className="gap-1.5 border border-emerald-500/15 bg-emerald-500/10 px-2.5 py-1.5 text-emerald-700 dark:text-emerald-300">
              <span className={cn('relative flex h-1.5 w-1.5 rounded-full', wsConnected ? 'bg-emerald-500' : 'bg-muted-foreground')}>
                {wsConnected && <span className="absolute inset-0 animate-ping rounded-full bg-emerald-400 motion-reduce:hidden" />}
              </span>
              {wsConnected ? '实时通道已连接' : '实时通道连接中'}
            </Badge>
            {lastUpdated && (
              <span className="rounded-lg border border-border/50 bg-background/45 px-2.5 py-1.5 text-[11px] text-muted-foreground">
                最近同步 {new Date(lastUpdated).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
              </span>
            )}
            <Button variant="outline" size="sm" className="bg-background/60" onClick={() => navigate('/overview')}>
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              返回概览
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="bg-background/60"
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

      <div className="screen-dashboard-grid">
        <aside className="screen-dashboard-metrics min-w-0">
          <section className="relative h-full overflow-hidden rounded-[1.45rem] border border-primary/15 bg-card/80 p-4 shadow-lg shadow-primary/10 backdrop-blur-xl">
            <div className="absolute -right-9 -top-10 h-32 w-32 rounded-full bg-primary/10 blur-2xl" />
            <div className="relative">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-[11px] font-medium text-muted-foreground">服务运行态势</p>
                  <p className="mt-1 text-sm font-semibold text-foreground">核心指标</p>
                </div>
                <span className={cn('inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[10px] font-medium', serviceOk ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'border-destructive/25 bg-destructive/10 text-destructive')}>
                  <span className={cn('h-1.5 w-1.5 rounded-full', serviceOk ? 'bg-emerald-500' : 'bg-destructive')} />
                  {serviceOk ? '运行正常' : '需要关注'}
                </span>
              </div>

              <div className="mt-5 rounded-2xl border border-primary/15 bg-[linear-gradient(135deg,color-mix(in_srgb,var(--primary)_11%,transparent),transparent)] p-4">
                <p className="text-[11px] text-muted-foreground">当前在线连接</p>
                <div className="mt-1 flex items-end justify-between gap-3">
                  <strong className="text-4xl font-semibold tracking-tight tabular-nums text-foreground">{stats?.onlineClients ?? online.length}</strong>
                  <span className="mb-1 text-[11px] text-emerald-700 dark:text-emerald-300">management 实时状态</span>
                </div>
                <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-primary/10">
                  <div className="h-full rounded-full bg-gradient-to-r from-primary via-primary to-accent" style={{ width: `${Math.min(100, Math.max(8, ((stats?.onlineClients ?? online.length) / Math.max(1, stats?.enabledUsers ?? 1)) * 100))}%` }} />
                </div>
              </div>

              <div className="mt-3 grid grid-cols-2 gap-3">
                <StatCard label="启用账号" value={stats?.enabledUsers ?? 0} caption={`共 ${stats?.totalUsers ?? 0} 个账号`} icon={Users} tone="primary" />
                <StatCard label="24 小时流量" value={formatBytes(totalTraffic)} caption={`下载 ${receivedPercent}%`} icon={Gauge} tone="accent" />
              </div>
              <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-xl border border-emerald-500/15 bg-emerald-500/5 px-3 py-2.5">
                  <p className="text-[10px] text-muted-foreground">今日上线</p>
                  <p className="mt-1 font-semibold tabular-nums text-foreground">{stats?.todayConnections ?? 0}</p>
                </div>
                <div className="rounded-xl border border-primary/15 bg-primary/5 px-3 py-2.5">
                  <p className="text-[10px] text-muted-foreground">客户端配置</p>
                  <p className="mt-1 font-semibold tabular-nums text-foreground">{stats?.clientConfigs ?? 0}</p>
                </div>
              </div>
            </div>
          </section>
        </aside>

        <aside className="screen-dashboard-traffic min-w-0">
          <Panel
            title="用户流量排行"
            subtitle="最近 24 小时 · 按用户汇总"
            icon={ArrowDownToLine}
            className="flex h-full flex-col"
            contentClassName="min-h-0 flex-1"
          >
            {trafficState === 'loading' ? (
              <div className="flex h-44 items-center justify-center text-sm text-muted-foreground"><RefreshCw className="mr-2 h-4 w-4 animate-spin" />正在加载流量统计…</div>
            ) : trafficState === 'error' ? (
              <EmptyState title="流量数据暂不可用" description="时间段流量接口返回异常。" compact />
            ) : topUsers.length ? (
              <div className="space-y-2.5">
                {topUsers.slice(0, 6).map((user, index) => {
                  const percent = user.total > 0 ? Math.round((user.total / Math.max(1, topUsers[0].total)) * 100) : 0;
                  return (
                    <div key={`${user.username}-${index}`} className="grid grid-cols-[20px_minmax(60px,1fr)_minmax(58px,.68fr)] items-center gap-2 text-[11px]">
                      <span className="font-semibold tabular-nums text-muted-foreground">{String(index + 1).padStart(2, '0')}</span>
                      <div className="min-w-0"><p className="truncate font-medium text-foreground">{user.username}</p><div className="mt-1 h-1 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-gradient-to-r from-primary to-accent" style={{ width: `${Math.max(5, percent)}%` }} /></div></div>
                      <span className="text-right font-semibold tabular-nums text-foreground">{formatBytes(user.total)}</span>
                    </div>
                  );
                })}
                <div className="grid grid-cols-2 gap-2 border-t border-border/50 pt-3">
                  <div className="rounded-lg bg-primary/8 p-2"><p className="flex items-center gap-1 text-[10px] text-primary"><ArrowDownToLine className="h-3 w-3" />下载</p><p className="mt-1 text-xs font-semibold tabular-nums">{formatBytes(received)}</p></div>
                  <div className="rounded-lg bg-accent/12 p-2"><p className="flex items-center gap-1 text-[10px] text-accent"><ArrowUpFromLine className="h-3 w-3" />上传</p><p className="mt-1 text-xs font-semibold tabular-nums">{formatBytes(sent)}</p></div>
                </div>
              </div>
            ) : <EmptyState title="暂无时间段流量" description="等待连接历史采样数据产生" compact />}
          </Panel>
        </aside>

        <main className="screen-dashboard-map min-w-0">
          <OperationsMap
            className="h-full"
            data={geo}
            source={geoSource}
            view={geoView}
            onSourceChange={setGeoSource}
            onViewChange={setGeoView}
            loading={geoState === 'loading'}
            error={geoState === 'error'}
          />
        </main>

        <main className="screen-dashboard-topology min-w-0">
          <Panel
            title="实时连接拓扑"
            subtitle="OpenVPN Server 与在线客户端关系 · 不代表地理位置"
            icon={Share2Icon}
            className="flex h-full flex-col"
            contentClassName="min-h-0 flex-1"
          >
            <Topology online={online} state={onlineState} />
          </Panel>
        </main>

        <aside className="screen-dashboard-health min-w-0">
          <Panel
            title="服务健康"
            subtitle="运行状态与管理通道"
            icon={Server}
            className="flex h-full flex-col"
            contentClassName="min-h-0 flex-1 p-3 sm:p-3"
          >
            <div className="space-y-2">
              <HealthRow icon={serviceOk ? CheckCircle2 : XCircle} label="OpenVPN management" value={status} ok={serviceOk} />
              <HealthRow icon={server?.RunDate ? CheckCircle2 : WifiOff} label="运行信息" value={server?.RunDate || '暂无信息'} ok={Boolean(server?.RunDate)} />
              <HealthRow icon={server?.Address ? CheckCircle2 : WifiOff} label="监听地址" value={server?.Address || '暂无信息'} ok={Boolean(server?.Address)} />
              <HealthRow icon={Wifi} label="WebSocket 实时通道" value={wsConnected ? '已连接' : '连接中'} ok={wsConnected} />
            </div>
          </Panel>
        </aside>

        <aside className="screen-dashboard-trend min-w-0">
          <Panel title="在线连接趋势" subtitle="持续采样 · 最近 18 个节点" icon={Activity} className="flex h-full flex-col" contentClassName="min-h-0 flex-1">
            <TrendChart points={trend} compact />
          </Panel>
        </aside>

        <aside className="screen-dashboard-risks min-w-0">
          <Panel
            title="风险与运维提示"
            subtitle="仅展示，不提供危险操作"
            icon={ShieldAlert}
            className="flex h-full flex-col"
            contentClassName="flex min-h-0 flex-1 flex-col justify-center"
          >
            {risks.length ? (
              <div className="space-y-2">{risks.slice(0, 3).map((risk, index) => <RiskCard key={`${risk.title}-${index}`} risk={risk} />)}</div>
            ) : <EmptyState title="当前没有待处理风险" description="未发现账号、证书、防火墙或服务异常。" compact />}
          </Panel>
        </aside>
      </div>

      <section className="mt-4 grid gap-3 rounded-[1.45rem] border border-border/60 bg-card/70 p-3 shadow-sm backdrop-blur-xl sm:grid-cols-3 sm:p-4">
        <div className="flex items-center gap-3 rounded-xl border border-primary/10 bg-primary/5 px-3 py-3"><span className="grid h-9 w-9 place-items-center rounded-xl bg-primary/10 text-primary"><Activity className="h-4 w-4" /></span><div><p className="text-[11px] text-muted-foreground">连接数据</p><p className="mt-0.5 text-sm font-semibold">{onlineState === 'forbidden' ? '已按权限隐藏明细' : `${online.length} 个实时会话`}</p></div></div>
        <div className="flex items-center gap-3 rounded-xl border border-primary/10 bg-primary/5 px-3 py-3"><span className="grid h-9 w-9 place-items-center rounded-xl bg-primary/10 text-primary"><ArrowDownToLine className="h-4 w-4" /></span><div><p className="text-[11px] text-muted-foreground">时间段流量</p><p className="mt-0.5 text-sm font-semibold tabular-nums">{trafficTotals.activeUsers} 位活跃用户 · {formatBytes(totalTraffic)}</p></div></div>
        <div className="flex items-center gap-3 rounded-xl border border-emerald-500/10 bg-emerald-500/5 px-3 py-3"><span className="grid h-9 w-9 place-items-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-300"><ShieldAlert className="h-4 w-4" /></span><div><p className="text-[11px] text-muted-foreground">安全态势</p><p className="mt-0.5 text-sm font-semibold">{risks.length ? `${risks.length} 项待处理提醒` : '当前未发现待处理风险'}</p></div></div>
      </section>

      {(summaryState === 'error' || (summaryState === 'ready' && !summary)) && (
        <div className="mt-4 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-300">
          概览统计暂时不可用；大屏仍保留当前可用的实时连接、流量与地理态势数据。
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
    <div className="flex items-center gap-2.5 rounded-xl border border-border/50 bg-background/25 px-3 py-2">
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
            : 'border-primary/30 bg-primary/10'
      )}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle
          className={cn(
            'mt-0.5 h-4 w-4 shrink-0',
            danger ? 'text-destructive' : warning ? 'text-amber-500' : 'text-primary'
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
