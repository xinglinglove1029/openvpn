/**
 * 首页系统监控大屏
 *
 * 数据来源：WebSocket 实时推送（topic: system:stats）
 * - 不会发起任何 HTTP 轮询；
 * - 组件挂载时订阅 realtimeHub；卸载时取消订阅；
 * - 首屏若没有数据，显示"等待实时数据"占位；
 * - 5 秒一次的采集频率，环形进度条 + 实时数字 + 网卡速率榜。
 *
 * 设计要点：
 * - 四个核心环形指标（CPU、内存、磁盘、网络）用 SVG 圆环 + 主题色渐变；
 * - 数值随 WebSocket 推送平滑过渡（CSS transition on dashoffset）；
 * - 主题色统一跟随 --accent，自动适配 light/dark；
 * - 网络接口 + 物理分区用紧凑列表展示，迷你柱状条显示使用率。
 */
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  Cpu,
  HardDrive,
  MemoryStick,
  Network,
  Server,
  ArrowDownToLine,
  ArrowUpFromLine,
  Gauge,
  CircleDot,
  Wifi,
  Clock,
  Layers,
  ListTree,
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/ui/card';
import { Badge } from '@/ui/badge';
import { CardGlow } from '@/components/CardGlow';
import { StatusBadge } from '@/components/StatusBadge';
import { cn } from '@/lib/utils';
import { realtimeHub, type ConnectionState } from '@/lib/notificationHub';
import { formatBytes } from '@/lib/format';
import type { SystemStatsPayload } from '@/types';

/* ---------- 工具函数 ---------- */

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  if (value < 0) return 0;
  if (value > 100) return 100;
  return value;
}

function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '-';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d} 天 ${h} 时 ${m} 分`;
  if (h > 0) return `${h} 时 ${m} 分`;
  return `${m} 分`;
}

function bpsText(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return '0 B/s';
  return `${formatBytes(bps)}/s`;
}

function timeAgo(ts?: number): string {
  if (!ts) return '尚未推送';
  const diff = Math.max(0, Date.now() - ts);
  if (diff < 1500) return '刚刚';
  if (diff < 60_000) return `${Math.floor(diff / 1000)} 秒前`;
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  return `${Math.floor(diff / 3_600_000)} 小时前`;
}

/** 把新值追加到历史窗口，超过上限则丢弃最旧的（环形队列） */
function appendBounded(prev: number[], next: number, limit: number): number[] {
  const out = prev.length >= limit ? prev.slice(prev.length - limit + 1) : prev.slice();
  out.push(next);
  return out;
}

/* ---------- SVG 趋势迷你图（sparkline） ---------- */

interface SparklineProps {
  /** 数值序列（0-100 百分比，或任意数值；由 normalize 决定） */
  data: number[];
  /** 像素宽度，默认 100% */
  width?: number | string;
  /** 像素高度，默认 28 */
  height?: number;
  /** 主题色 */
  color?: string;
  /** 是否填充曲线下方区域（带透明度），默认 true */
  filled?: boolean;
  /** 后缀：tooltip 数值后面追加的单位（如 "%"），留空则不显示 */
  unit?: string;
  /** tooltip 标题前缀（如 "CPU"），不传则仅显示数值 */
  label?: string;
}

/**
 * 紧凑趋势线：把任意序列归一化到 0-1 渲染为 SVG polyline。
 * - 网络速率等非百分比数据：内部按当前窗口最大值归一化，保证曲线始终清晰；
 * - 平滑度：直接折线，不做样条插值（5s 间隔足够稀疏，直线更准）；
 * - 性能：最多 60 个点，纯 SVG，开销可忽略。
 * - 交互：hover 时显示最近点的值 + 窗口 min/max/avg，悬浮线 + 高亮点。
 */
function Sparkline({
  data,
  width = '100%',
  height = 28,
  color = 'var(--accent)',
  filled = true,
  unit,
  label,
}: SparklineProps) {
  // 1) 数据为空时画一条 0 基线
  const points = data.length === 0 ? [0] : data;
  // 2) 计算统计信息
  const stats = useMemo(() => {
    if (points.length === 0) return { min: 0, max: 0, avg: 0 };
    let mn = Infinity, mx = -Infinity, sum = 0;
    for (const v of points) {
      if (!Number.isFinite(v)) continue;
      if (v < mn) mn = v;
      if (v > mx) mx = v;
      sum += v;
    }
    if (!Number.isFinite(mn)) mn = 0;
    if (!Number.isFinite(mx)) mx = 0;
    return { min: mn, max: mx, avg: sum / points.length };
  }, [points]);

  // 3) 归一化
  const range = stats.max - stats.min || 1;
  const w = 100;
  const h = 100;
  const stepX = points.length > 1 ? w / (points.length - 1) : 0;
  const coords = points.map((v, i) => {
    const x = i * stepX;
    const y = h - ((v - stats.min) / range) * h;
    return [x, y] as const;
  });

  // 4) 路径
  const linePath = coords
    .map(([x, y], i) => (i === 0 ? `M ${x.toFixed(2)} ${y.toFixed(2)}` : `L ${x.toFixed(2)} ${y.toFixed(2)}`))
    .join(' ');
  const areaPath = `${linePath} L ${w.toFixed(2)} ${h.toFixed(2)} L 0 ${h.toFixed(2)} Z`;

  // 5) 交互态：追踪鼠标位置 / 最近的点
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const [hover, setHover] = useState<{ idx: number; left: number } | null>(null);

  const onMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const el = wrapRef.current;
    if (!el || points.length === 0) return;
    const rect = el.getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width) * w; // 0-100
    const idx = Math.max(0, Math.min(points.length - 1, Math.round((x / w) * (points.length - 1))));
    setHover({ idx, left: (coords[idx][0] / w) * 100 });
  };
  const onLeave = () => setHover(null);

  const id = `spark-grad-${color.replace(/[^a-z0-9]/gi, '')}`;
  const fmt = (v: number) => (Number.isFinite(v) ? v.toFixed(1) : '-');
  const suffix = unit ?? '';

  return (
    <div
      ref={wrapRef}
      className="relative -mx-1 w-full"
      onMouseMove={onMove}
      onMouseLeave={onLeave}
    >
      <svg
        viewBox={`0 0 ${w} ${h}`}
        preserveAspectRatio="none"
        width={width}
        height={height}
        className="block"
        aria-hidden
      >
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.35" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        {filled && <path d={areaPath} fill={`url(#${id})`} />}
        <path
          d={linePath}
          fill="none"
          stroke={color}
          strokeWidth={1.6}
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
        />
        {/* hover 状态：竖线 + 高亮点 */}
        {hover && coords[hover.idx] && (
          <>
            <line
              x1={coords[hover.idx][0]}
              y1={0}
              x2={coords[hover.idx][0]}
              y2={h}
              stroke="currentColor"
              strokeWidth={0.6}
              strokeDasharray="2 2"
              className="text-muted-foreground"
              vectorEffect="non-scaling-stroke"
            />
            <circle
              cx={coords[hover.idx][0]}
              cy={coords[hover.idx][1]}
              r={1.6}
              fill={color}
              stroke="white"
              strokeWidth={0.6}
              vectorEffect="non-scaling-stroke"
            />
          </>
        )}
      </svg>
      {/* 透明 hit area：保证 100% 宽度内 hover 都生效（viewBox 会按 width 100% 撑开但 height 不会，留一个绝对定位的热区） */}
      <div className="absolute inset-0" />
      {/* Tooltip：浮在 sparkline 上方 */}
      {hover && points[hover.idx] !== undefined && (
        <div
          className="pointer-events-none absolute z-20 -translate-x-1/2 -top-1 rounded-md border border-border/60 bg-popover px-2 py-1 text-[10px] text-popover-foreground shadow-md"
          style={{ left: `${hover.left}%` }}
          role="tooltip"
        >
          <div className="flex items-center gap-1.5 font-medium tabular-nums">
            {label && <span className="text-muted-foreground">{label}</span>}
            <span>{fmt(points[hover.idx])}{suffix}</span>
          </div>
          <div className="mt-0.5 flex items-center gap-2 text-[9px] text-muted-foreground tabular-nums">
            <span>min {fmt(stats.min)}</span>
            <span>avg {fmt(stats.avg)}</span>
            <span>max {fmt(stats.max)}</span>
          </div>
        </div>
      )}
    </div>
  );
}



interface RingChartProps {
  /** 0-100 */
  value: number;
  /** 圆环像素直径（外径），默认 120 */
  size?: number;
  /** 圆环描边宽度，默认 10 */
  strokeWidth?: number;
  /** 中心内容（数字、标签） */
  children?: React.ReactNode;
  /** 主题色 CSS 变量名，默认 var(--accent) */
  color?: string;
  /** 数值描述，用于 aria-label */
  label?: string;
}

function RingChart({
  value,
  size = 132,
  strokeWidth = 10,
  children,
  color = 'var(--accent)',
  label,
}: RingChartProps) {
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const dash = (clampPercent(value) / 100) * circumference;
  const center = size / 2;
  return (
    <div
      className="relative inline-flex items-center justify-center"
      style={{ width: size, height: size }}
      role="img"
      aria-label={label ?? `${value.toFixed(0)}%`}
    >
      {/* 外层柔光 */}
      <div
        className="absolute inset-0 rounded-full opacity-40 blur-xl transition-opacity duration-500"
        style={{ background: `radial-gradient(circle, ${color}55 0%, transparent 70%)` }}
      />
      <svg width={size} height={size} className="relative -rotate-90">
        <defs>
          <linearGradient id={`ring-grad-${value.toFixed(0)}-${size}`} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.95" />
            <stop offset="100%" stopColor={color} stopOpacity="0.55" />
          </linearGradient>
        </defs>
        {/* 背景轨道 */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke="currentColor"
          strokeWidth={strokeWidth}
          className="text-muted/30"
        />
        {/* 进度弧 */}
        <circle
          cx={center}
          cy={center}
          r={radius}
          fill="none"
          stroke={`url(#ring-grad-${value.toFixed(0)}-${size})`}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={circumference - dash}
          style={{ transition: 'stroke-dashoffset 700ms cubic-bezier(0.4, 0, 0.2, 1)' }}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">{children}</div>
    </div>
  );
}

/* ---------- 核心指标卡（环形） ---------- */

interface MetricCardProps {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  percent: number;
  primary: string;
  secondary: string;
  /** 用于控制环形颜色 */
  tone?: 'default' | 'warning' | 'danger';
  /** 自定义中心内容；不传则用 percent */
  ringLabel?: React.ReactNode;
  /** 最近 N 个采样点（用于 sparkline 趋势），单位由 caller 决定（0-100 或任意数值） */
  history?: number[];
  /** sparkline 主题色；缺省跟随 tone */
}

function MetricCard({ title, icon: Icon, percent, primary, secondary, tone = 'default', ringLabel, history }: MetricCardProps) {
  const color =
    tone === 'danger'
      ? 'var(--destructive, #ef4444)'
      : tone === 'warning'
        ? '#f59e0b'
        : 'var(--accent)';
  const sparkData = history && history.length > 0 ? history : [percent];
  return (
    <CardGlow className="h-full">
      <Card className="h-full border-0 bg-transparent shadow-none">
        <CardContent className="relative z-10 flex h-full flex-col items-center justify-center gap-3 py-5">
          <div className="flex w-full items-center justify-between">
            <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Icon className="h-3.5 w-3.5" />
              {title}
            </div>
            <span
              className={cn(
                'flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-all duration-300',
                'group-hover:bg-[var(--accent)]/15 group-hover:text-[var(--accent)] group-hover:scale-110',
              )}
            >
              <Icon className="h-3.5 w-3.5" />
            </span>
          </div>
          <RingChart value={percent} color={color} label={ringLabel ? undefined : `${title} ${percent.toFixed(0)}%`}>
            {ringLabel ?? (
              <>
                <span className="text-2xl font-bold tabular-nums tracking-tight">{percent.toFixed(0)}</span>
                <span className="text-[10px] text-muted-foreground -mt-1">%</span>
              </>
            )}
          </RingChart>
          <div className="w-full text-center">
            <p className="text-sm font-semibold tabular-nums">{primary}</p>
            <p className="mt-0.5 text-[11px] text-muted-foreground">{secondary}</p>
          </div>
          {/* 趋势迷你图：固定 28px 高度，宽度撑满 */}
          <Sparkline
            data={sparkData}
            color={color}
            height={28}
            unit={title.includes('网络') ? ' B/s' : '%'}
            label={title}
          />
        </CardContent>
      </Card>
    </CardGlow>
  );
}

/* ---------- 磁盘行 ---------- */

function DiskRow({ disk }: { disk: SystemStatsPayload['disks'][number] }) {
  const used = clampPercent(disk.usedPercent);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2 text-xs">
        <div className="flex min-w-0 items-center gap-1.5">
          <HardDrive className="h-3 w-3 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium" title={disk.mountpoint}>
            {disk.mountpoint}
          </span>
          <span className="shrink-0 text-[10px] text-muted-foreground">{disk.fsType}</span>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <span className="tabular-nums font-medium">{used.toFixed(0)}%</span>
          <span className="text-[10px] text-muted-foreground tabular-nums">
            {formatBytes(disk.usedBytes)} / {formatBytes(disk.totalBytes)}
          </span>
        </div>
      </div>
      <div className="relative h-1.5 w-full overflow-hidden rounded-full bg-muted/40">
        <div
          className={cn(
            'h-full rounded-full transition-all duration-700',
            used >= 90
              ? 'bg-gradient-to-r from-amber-500 to-red-500'
              : used >= 70
                ? 'bg-gradient-to-r from-[var(--accent)] to-amber-500'
                : 'bg-gradient-to-r from-[var(--accent)]/80 to-[var(--accent)]',
          )}
          style={{ width: `${used}%` }}
        />
      </div>
    </div>
  );
}

/* ---------- 网络接口行 ---------- */

function NetRow({ net, totalBps }: { net: SystemStatsPayload['networks'][number]; totalBps: number }) {
  // 速率占总速率比例，用于右侧迷你条
  const total = totalBps || 1;
  const rxPct = clampPercent((net.rxBps / total) * 100);
  const txPct = clampPercent((net.txBps / total) * 100);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2 text-xs">
        <div className="flex min-w-0 items-center gap-1.5">
          <Wifi className="h-3 w-3 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium" title={net.name}>
            {net.name}
          </span>
          {net.isPhysical && (
            <Badge variant="outline" className="h-4 px-1 text-[9px] text-muted-foreground">
              物理
            </Badge>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className="flex items-center gap-0.5 text-emerald-500 tabular-nums">
            <ArrowDownToLine className="h-3 w-3" />
            {bpsText(net.rxBps)}
          </span>
          <span className="flex items-center gap-0.5 text-sky-500 tabular-nums">
            <ArrowUpFromLine className="h-3 w-3" />
            {bpsText(net.txBps)}
          </span>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-1.5">
        <div className="relative h-1 overflow-hidden rounded-full bg-muted/40">
          <div
            className="h-full rounded-full bg-emerald-500/80 transition-all duration-500"
            style={{ width: `${rxPct}%` }}
          />
        </div>
        <div className="relative h-1 overflow-hidden rounded-full bg-muted/40">
          <div
            className="h-full rounded-full bg-sky-500/80 transition-all duration-500"
            style={{ width: `${txPct}%` }}
          />
        </div>
      </div>
    </div>
  );
}

/* ---------- 主组件 ---------- */

/** 趋势历史窗口大小：5 秒一次 × 60 = 5 分钟 */
const HISTORY_LIMIT = 60;

export default function SystemMonitor() {
  const [stats, setStats] = useState<SystemStatsPayload | null>(null);
  const [connection, setConnection] = useState<ConnectionState>(realtimeHub.getState());
  const [now, setNow] = useState(Date.now());

  // 趋势历史：4 个核心指标的最近 N 个采样点
  const [cpuHist, setCpuHist] = useState<number[]>([]);
  const [memHist, setMemHist] = useState<number[]>([]);
  const [diskHist, setDiskHist] = useState<number[]>([]);
  const [netHist, setNetHist] = useState<number[]>([]);

  useEffect(() => {
    const off = realtimeHub.subscribe<SystemStatsPayload>('system:stats', (payload) => {
      if (payload) {
        setStats(payload);
        // 把核心指标追加到历史窗口
        const nextCpu = clampPercent(payload.cpuPercent);
        const nextMem = clampPercent(payload.memory.usedPercent);
        const nextDisk =
          payload.disks.length > 0 ? Math.max(...payload.disks.map((d) => d.usedPercent)) : 0;
        // 网络总速率（bytes/s），不归一化为百分比，原值入栈，Sparkline 内部按窗口最大归一化
        const nextNet = (payload.netTotalRxBps || 0) + (payload.netTotalTxBps || 0);
        setCpuHist((prev) => appendBounded(prev, nextCpu, HISTORY_LIMIT));
        setMemHist((prev) => appendBounded(prev, nextMem, HISTORY_LIMIT));
        setDiskHist((prev) => appendBounded(prev, nextDisk, HISTORY_LIMIT));
        setNetHist((prev) => appendBounded(prev, nextNet, HISTORY_LIMIT));
      }
    });
    const offState = realtimeHub.onState(setConnection);
    return () => {
      off();
      offState();
    };
  }, []);

  // 每秒更新"几秒前"，让时间标签保持新鲜
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  // 派生指标
  const cpuPercent = stats?.cpuPercent ?? 0;
  const memPercent = stats?.memory.usedPercent ?? 0;
  const totalBps = (stats?.netTotalRxBps ?? 0) + (stats?.netTotalTxBps ?? 0);
  // 磁盘使用率：取最大物理分区的使用率作为"磁盘整体"指标
  const diskPercent = useMemo(() => {
    if (!stats || stats.disks.length === 0) return 0;
    return Math.max(...stats.disks.map((d) => d.usedPercent));
  }, [stats]);

  // CPU 警戒色
  const cpuTone: 'default' | 'warning' | 'danger' = cpuPercent >= 90 ? 'danger' : cpuPercent >= 70 ? 'warning' : 'default';
  const memTone: 'default' | 'warning' | 'danger' = memPercent >= 90 ? 'danger' : memPercent >= 80 ? 'warning' : 'default';
  const diskTone: 'default' | 'warning' | 'danger' = diskPercent >= 90 ? 'danger' : diskPercent >= 80 ? 'warning' : 'default';
  // 网络颜色：按总速率分级
  const netTone: 'default' | 'warning' | 'danger' =
    totalBps > 100 * 1024 * 1024 ? 'danger' : totalBps > 20 * 1024 * 1024 ? 'warning' : 'default';

  // 连接状态
  const isOpen = connection === 'open';
  const statusKind: 'success' | 'warning' | 'danger' | 'neutral' = isOpen ? 'success' : connection === 'connecting' ? 'warning' : 'danger';
  const statusLabel = isOpen ? '实时' : connection === 'connecting' ? '连接中' : '已断开';

  // 断线提示：WS 状态为 closed/connecting，或者超过 15s 没收到数据
  const staleMs = stats?.timestamp ? Math.max(0, now - stats.timestamp) : Number.POSITIVE_INFINITY;
  const showDisconnectAlert = !isOpen || staleMs > 15_000;

  return (
    <CardGlow>
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1">
              <CardTitle className="flex items-center gap-2">
                <Gauge className="h-5 w-5 text-[var(--accent)]" />
                系统实时监控
              </CardTitle>
              <CardDescription>
                CPU / 内存 / 磁盘 / 网络通过 WebSocket 实时推送
                {stats ? ` · 采集间隔 ${(stats.intervalMs / 1000).toFixed(0)} 秒` : ''}
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <StatusBadge status={statusKind}>
                <CircleDot className="mr-1 inline h-3 w-3" />
                {statusLabel}
              </StatusBadge>
              <Badge variant="outline" className="text-[10px]">
                {timeAgo(stats?.timestamp)}
              </Badge>
              {stats?.host.hostname && (
                <Badge variant="outline" className="hidden max-w-[200px] truncate text-[10px] sm:inline-block" title={stats.host.hostname}>
                  <Server className="mr-1 inline h-3 w-3" />
                  {stats.host.hostname}
                </Badge>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-5">
          {/* 断线提示：仅在 WS 异常或长时间无数据时显示 */}
          {showDisconnectAlert && (
            <div
              className="flex items-center gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300"
              role="alert"
            >
              <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
              <span>
                实时推送中断
                {stats?.timestamp
                  ? `，最近数据 ${timeAgo(stats.timestamp)}`
                  : '，尚未收到任何数据'}
                ，系统将自动尝试重连。
              </span>
            </div>
          )}

          {/* 四个核心指标卡 */}
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard
              title="CPU 使用率"
              icon={Cpu}
              percent={cpuPercent}
              primary={`${cpuPercent.toFixed(1)} %`}
              secondary={stats ? `${stats.host.cpuCores || '-'} 核 · ${stats.host.cpuModel || '-'}` : '等待数据'}
              tone={cpuTone}
              history={cpuHist}
            />
            <MetricCard
              title="内存使用率"
              icon={MemoryStick}
              percent={memPercent}
              primary={`${formatBytes(stats?.memory.usedBytes ?? 0)}`}
              secondary={stats ? `共 ${formatBytes(stats.memory.totalBytes)} · 可用 ${formatBytes(stats.memory.availableBytes)}` : '等待数据'}
              tone={memTone}
              history={memHist}
            />
            <MetricCard
              title="磁盘使用率"
              icon={HardDrive}
              percent={diskPercent}
              primary={
                stats && stats.disks.length > 0
                  ? `${stats.disks.length} 个分区`
                  : '无分区'
              }
              secondary={stats && stats.disks.length > 0 ? `最满 ${stats.disks[0].mountpoint}` : '等待数据'}
              tone={diskTone}
              history={diskHist}
            />
            <MetricCard
              title="网络总速率"
              icon={Network}
              percent={clampPercent(Math.min(100, (totalBps / (200 * 1024 * 1024)) * 100))}
              primary={bpsText(totalBps)}
              secondary={
                stats
                  ? `↓ ${bpsText(stats.netTotalRxBps)} · ↑ ${bpsText(stats.netTotalTxBps)}`
                  : '等待数据'
              }
              tone={netTone}
              history={netHist}
            />
          </div>

          {/* 主机信息行 */}
          <div className="grid gap-2 rounded-lg border border-border/40 bg-muted/20 p-3 text-xs sm:grid-cols-2 lg:grid-cols-4">
            <HostInfoItem
              icon={Server}
              label="主机"
              value={stats ? `${stats.host.hostname || '-'} · ${stats.host.platform || '-'} ${stats.host.platformVersion || ''}`.trim() : '等待数据'}
            />
            <HostInfoItem
              icon={Layers}
              label="内核"
              value={stats ? `${stats.host.kernelArch || '-'} · ${stats.host.kernelVersion || '-'}` : '等待数据'}
            />
            <HostInfoItem
              icon={Clock}
              label="运行时长"
              value={stats ? formatUptime(stats.host.uptimeSeconds) : '等待数据'}
            />
            <HostInfoItem
              icon={Activity}
              label="负载"
              value={
                stats && (stats.host.loadAvg1 || stats.host.loadAvg5 || stats.host.loadAvg15)
                  ? `${stats.host.loadAvg1.toFixed(2)} / ${stats.host.loadAvg5.toFixed(2)} / ${stats.host.loadAvg15.toFixed(2)}`
                  : '仅 Linux/Darwin'
              }
            />
          </div>

          {/* 每核 CPU 占用 + 磁盘 + 网络接口 */}
          <div className="grid gap-4 lg:grid-cols-3">
            {/* 每核 CPU */}
            <div className="space-y-2 rounded-lg border border-border/40 bg-muted/10 p-3">
              <div className="flex items-center justify-between">
                <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <Cpu className="h-3.5 w-3.5" />
                  每核 CPU
                </p>
                <span className="text-[10px] text-muted-foreground">{stats?.cpuPerCore.length ?? 0} 核</span>
              </div>
              {stats && stats.cpuPerCore.length > 0 ? (
                <div className="grid grid-cols-4 gap-1.5 sm:grid-cols-6 lg:grid-cols-4 xl:grid-cols-6">
                  {stats.cpuPerCore.map((v, idx) => {
                    const p = clampPercent(v);
                    const tone = p >= 90 ? 'bg-red-500' : p >= 70 ? 'bg-amber-500' : 'bg-[var(--accent)]';
                    return (
                      <div key={idx} className="space-y-1" title={`Core ${idx + 1}: ${p.toFixed(0)}%`}>
                        <div className="relative h-10 overflow-hidden rounded bg-muted/40">
                          <div
                            className={cn('absolute bottom-0 left-0 right-0 transition-all duration-500', tone)}
                            style={{ height: `${p}%`, opacity: 0.85 }}
                          />
                          <div className="absolute inset-0 flex items-center justify-center text-[9px] font-medium tabular-nums text-foreground/80 mix-blend-difference">
                            {p.toFixed(0)}
                          </div>
                        </div>
                        <p className="text-center text-[9px] text-muted-foreground">C{idx + 1}</p>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <p className="py-4 text-center text-xs text-muted-foreground">等待数据</p>
              )}
            </div>

            {/* 物理分区 */}
            <div className="space-y-2 rounded-lg border border-border/40 bg-muted/10 p-3">
              <div className="flex items-center justify-between">
                <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <HardDrive className="h-3.5 w-3.5" />
                  物理分区
                </p>
                <span className="text-[10px] text-muted-foreground">按使用率 Top 3</span>
              </div>
              {stats && stats.disks.length > 0 ? (
                <div className="space-y-2.5">
                  {stats.disks.map((d) => (
                    <DiskRow key={d.mountpoint} disk={d} />
                  ))}
                </div>
              ) : (
                <p className="py-4 text-center text-xs text-muted-foreground">暂无物理分区</p>
              )}
            </div>

            {/* 网络接口 */}
            <div className="space-y-2 rounded-lg border border-border/40 bg-muted/10 p-3">
              <div className="flex items-center justify-between">
                <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <Network className="h-3.5 w-3.5" />
                  网络接口
                </p>
                <span className="text-[10px] text-muted-foreground">按瞬时速率 Top 5</span>
              </div>
              {stats && stats.networks.length > 0 ? (
                <div className="space-y-2.5">
                  {stats.networks.map((n) => (
                    <NetRow key={n.name} net={n} totalBps={totalBps} />
                  ))}
                </div>
              ) : (
                <p className="py-4 text-center text-xs text-muted-foreground">暂无网络接口</p>
              )}
            </div>
          </div>

          {/* 进程 Top N（CPU / 内存 切换） */}
          <ProcessTopSection
            cpuList={stats?.topCpuProcesses ?? []}
            memList={stats?.topMemProcesses ?? []}
          />

          {/* 进程信息（轻量） */}
          {stats && (
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-border/40 bg-muted/10 px-3 py-2 text-[11px] text-muted-foreground">
              <span>
                进程 RSS <span className="font-medium text-foreground/80 tabular-nums">{formatBytes(stats.process.memoryRss)}</span>
              </span>
              <span>
                堆分配 <span className="font-medium text-foreground/80 tabular-nums">{formatBytes(stats.process.memoryVsz)}</span>
              </span>
              <span>
                Goroutine <span className="font-medium text-foreground/80 tabular-nums">{stats.process.numThreads}</span>
              </span>
              {(stats.memory.swapTotalBytes ?? 0) > 0 && (
                <span>
                  Swap <span className="font-medium text-foreground/80 tabular-nums">
                    {formatBytes(stats.memory.swapUsedBytes)} / {formatBytes(stats.memory.swapTotalBytes)}
                  </span>
                </span>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </CardGlow>
  );
}

function HostInfoItem({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
}) {
  return (
    <div className="flex min-w-0 items-start gap-2">
      <Icon className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <p className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</p>
        <p className="truncate text-xs font-medium text-foreground/90" title={value}>
          {value}
        </p>
      </div>
    </div>
  );
}

/* ---------- 进程 Top N 区块（CPU / 内存 双 Tab） ---------- */

type ProcessTab = 'cpu' | 'mem';

interface ProcessTopSectionProps {
  cpuList: SystemStatsPayload['topCpuProcesses'];
  memList: SystemStatsPayload['topMemProcesses'];
}

/** 把 RSS 字节数转为 MB（保留 1 位小数） */
function memMb(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 MB';
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function ProcessRow({
  p,
  mode,
  maxValue,
}: {
  p: SystemStatsPayload['topCpuProcesses'][number];
  mode: ProcessTab;
  maxValue: number;
}) {
  const isCpu = mode === 'cpu';
  const value = isCpu ? p.cpuPercent : p.memoryRss;
  const pct = maxValue > 0 ? clampPercent((value / maxValue) * 100) : 0;
  const tone = isCpu
    ? pct >= 90 ? 'bg-red-500' : pct >= 70 ? 'bg-amber-500' : 'bg-[var(--accent)]'
    : pct >= 90 ? 'bg-red-500' : pct >= 70 ? 'bg-amber-500' : 'bg-sky-500';
  const display = isCpu ? `${p.cpuPercent.toFixed(1)}%` : memMb(p.memoryRss);
  const user = p.username ? ` · ${p.username}` : '';
  const title = `${p.name} (PID ${p.pid})${user} · ${isCpu ? 'CPU' : 'RSS'}: ${display} · 状态: ${p.status || '-'}`;
  return (
    <div className="space-y-1.5" title={title}>
      <div className="flex items-center justify-between gap-2 text-xs">
        <div className="flex min-w-0 items-center gap-1.5">
          <ListTree className="h-3 w-3 shrink-0 text-muted-foreground" />
          <span className="truncate font-medium" title={p.name}>
            {p.name}
          </span>
          <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
            PID {p.pid}
          </span>
        </div>
        <span className="shrink-0 tabular-nums font-medium">{display}</span>
      </div>
      <div className="relative h-1.5 w-full overflow-hidden rounded-full bg-muted/40">
        <div
          className={cn('h-full rounded-full transition-all duration-500', tone)}
          style={{ width: `${pct}%`, opacity: 0.85 }}
        />
      </div>
    </div>
  );
}

function ProcessTopSection({ cpuList, memList }: ProcessTopSectionProps) {
  const [tab, setTab] = useState<ProcessTab>('cpu');
  const list = tab === 'cpu' ? cpuList : memList;
  const maxValue = useMemo(() => {
    if (list.length === 0) return 0;
    if (tab === 'cpu') {
      return Math.max(...list.map((p) => p.cpuPercent), 0.0001);
    }
    return Math.max(...list.map((p) => p.memoryRss), 0.0001);
  }, [list, tab]);

  const tabCls = (active: boolean) =>
    cn(
      'inline-flex h-6 items-center gap-1 rounded-md px-2.5 text-[11px] font-medium transition-colors',
      active
        ? 'bg-[var(--accent)]/15 text-[var(--accent)]'
        : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground',
    );

  return (
    <div className="space-y-2 rounded-lg border border-border/40 bg-muted/10 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <ListTree className="h-3.5 w-3.5" />
          进程占用 Top 5
        </p>
        <div className="inline-flex items-center gap-1 rounded-md border border-border/40 bg-background/40 p-0.5">
          <button
            type="button"
            className={tabCls(tab === 'cpu')}
            onClick={() => setTab('cpu')}
            aria-pressed={tab === 'cpu'}
          >
            <Cpu className="h-3 w-3" />
            CPU
          </button>
          <button
            type="button"
            className={tabCls(tab === 'mem')}
            onClick={() => setTab('mem')}
            aria-pressed={tab === 'mem'}
          >
            <MemoryStick className="h-3 w-3" />
            内存
          </button>
        </div>
      </div>
      {list.length > 0 ? (
        <div className="grid gap-2.5 md:grid-cols-2">
          {list.map((p) => (
            <ProcessRow
              key={`${tab}-${p.pid}`}
              p={p}
              mode={tab}
              maxValue={maxValue}
            />
          ))}
        </div>
      ) : (
        <p className="py-4 text-center text-xs text-muted-foreground">等待数据</p>
      )}
    </div>
  );
}
