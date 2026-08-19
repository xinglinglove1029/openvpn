import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  CalendarRange,
  Clock3,
  Download,
  RefreshCw,
  Upload,
  Users,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { Button } from '@/ui/button';
import { DateTimePicker } from '@/components/DateTimePicker';
import { Badge } from '@/ui/badge';
import { api } from '@/api';
import { formatBytes } from '@/lib/format';
import { formatDuration } from '@/lib/utils';
import { cn } from '@/lib/utils';
import type { DashboardTrafficUser, DashboardTrafficUsersResponse } from '@/types';

type RangePreset = '1h' | '6h' | '24h' | '7d' | '30d' | 'custom';

type ActiveRange = { start: number; end: number };

const PRESETS: Array<{ value: Exclude<RangePreset, 'custom'>; label: string; seconds: number }> = [
  { value: '1h', label: '最近 1 小时', seconds: 60 * 60 },
  { value: '6h', label: '最近 6 小时', seconds: 6 * 60 * 60 },
  { value: '24h', label: '最近 24 小时', seconds: 24 * 60 * 60 },
  { value: '7d', label: '最近 7 天', seconds: 7 * 24 * 60 * 60 },
  { value: '30d', label: '最近 30 天', seconds: 30 * 24 * 60 * 60 },
];

function rangeForPreset(value: Exclude<RangePreset, 'custom'>): ActiveRange {
  const preset = PRESETS.find((item) => item.value === value) ?? PRESETS[2];
  const end = Math.floor(Date.now() / 1000);
  return { start: end - preset.seconds, end };
}

function toDateTimeLocal(timestamp: number) {
  const date = new Date(timestamp * 1000);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function timestampFromDateTimeLocal(value: string) {
  const timestamp = new Date(value).getTime();
  return Number.isFinite(timestamp) ? Math.floor(timestamp / 1000) : 0;
}

function formatLastSeen(timestamp: number) {
  if (!timestamp) return '-';
  return new Date(timestamp * 1000).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

function StatItem({ label, value, icon: Icon, className }: { label: string; value: string | number; icon: typeof Download; className: string }) {
  return (
    <div className="rounded-xl border border-border/50 bg-background/35 px-3 py-3 shadow-sm">
      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <Icon className={cn('h-3.5 w-3.5', className)} />
        {label}
      </div>
      <p className="mt-1.5 text-lg font-semibold tracking-tight tabular-nums">{value}</p>
    </div>
  );
}

export function DashboardTrafficUsers() {
  const [preset, setPreset] = useState<RangePreset>('24h');
  const [customStart, setCustomStart] = useState(() => toDateTimeLocal(rangeForPreset('24h').start));
  const [customEnd, setCustomEnd] = useState(() => toDateTimeLocal(Math.floor(Date.now() / 1000)));
  const [activeRange, setActiveRange] = useState<ActiveRange>(() => rangeForPreset('24h'));
  const [data, setData] = useState<DashboardTrafficUsersResponse>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async (range: ActiveRange, showLoading = true) => {
    if (showLoading) setLoading(true);
    setError('');
    try {
      const response = await api.get<DashboardTrafficUsersResponse>(
        `/ovpn/dashboard/traffic-users?start=${range.start}&end=${range.end}`,
      );
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载流量统计失败');
    } finally {
      if (showLoading) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(activeRange);
  }, [activeRange, load]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      const range = preset === 'custom' ? activeRange : rangeForPreset(preset);
      if (preset !== 'custom') setActiveRange(range);
      void load(range, false);
    }, 60_000);
    return () => window.clearInterval(timer);
  }, [activeRange, load, preset]);

  const selectPreset = (value: Exclude<RangePreset, 'custom'>) => {
    setPreset(value);
    setActiveRange(rangeForPreset(value));
  };

  const applyCustomRange = () => {
    const start = timestampFromDateTimeLocal(customStart);
    const end = timestampFromDateTimeLocal(customEnd);
    if (!start || !end || start >= end) {
      setError('请选择有效的开始和结束时间');
      return;
    }
    setPreset('custom');
    setActiveRange({ start, end });
  };

  const refresh = () => {
    const range = preset === 'custom' ? activeRange : rangeForPreset(preset as Exclude<RangePreset, 'custom'>);
    if (preset !== 'custom') setActiveRange(range);
    void load(range);
  };

  const users = data?.users ?? [];
  const topUsers = useMemo(() => users.slice(0, 8), [users]);
  const maxTraffic = Math.max(1, ...topUsers.map((item) => item.total));
  const totals = data?.totals;

  return (
    <div className="rounded-2xl border border-border/60 bg-gradient-to-br from-primary/[0.07] via-background/35 to-violet-500/[0.05] p-3 shadow-sm sm:p-4">
      <div className="flex flex-col gap-3 border-b border-border/50 pb-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <p className="flex items-center gap-1.5 text-base font-semibold">
              <Activity className="h-4.5 w-4.5 text-primary" />
              时间段用户流量统计
            </p>
            <Badge variant="secondary" className="gap-1 text-[11px] font-normal">
              <Wifi className="h-3 w-3 text-emerald-500" />
              在线流量每分钟采样
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">按用户汇总选定时间内的下载与上传流量；在线客户端会自动刷新。</p>
        </div>
        <Button type="button" variant="outline" size="sm" className="h-8 self-start" onClick={refresh} disabled={loading}>
          <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
          刷新
        </Button>
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        {PRESETS.map((item) => (
          <Button
            key={item.value}
            type="button"
            size="sm"
            variant={preset === item.value ? 'secondary' : 'ghost'}
            className="h-7 rounded-full px-3 text-xs"
            onClick={() => selectPreset(item.value)}
          >
            {item.label}
          </Button>
        ))}
        <Button
          type="button"
          size="sm"
          variant={preset === 'custom' ? 'secondary' : 'ghost'}
          className="h-7 rounded-full px-3 text-xs"
          onClick={() => setPreset('custom')}
        >
          <CalendarRange className="h-3.5 w-3.5" />
          自定义
        </Button>
      </div>

      {preset === 'custom' && (
        <div className="mt-3 grid grid-cols-1 gap-2 rounded-xl border border-border/50 bg-background/35 p-2.5 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <label className="space-y-1">
            <span className="text-[11px] text-muted-foreground">开始时间</span>
            <DateTimePicker value={customStart} onChange={setCustomStart} placeholder="选择开始时间" className="h-8 text-xs" />
          </label>
          <label className="space-y-1">
            <span className="text-[11px] text-muted-foreground">结束时间</span>
            <DateTimePicker value={customEnd} onChange={setCustomEnd} placeholder="选择结束时间" className="h-8 text-xs" />
          </label>
          <Button type="button" size="sm" className="h-8" onClick={applyCustomRange}>应用</Button>
        </div>
      )}

      <div className="mt-3 grid grid-cols-2 gap-2 lg:grid-cols-4">
        <StatItem label="活跃用户" value={totals?.activeUsers ?? 0} icon={Users} className="text-primary" />
        <StatItem label="下载总量" value={totals?.receivedText ?? '0 B'} icon={Download} className="text-sky-500" />
        <StatItem label="上传总量" value={totals?.sentText ?? '0 B'} icon={Upload} className="text-violet-500" />
        <StatItem label="总流量" value={totals?.totalText ?? '0 B'} icon={Activity} className="text-emerald-500" />
      </div>

      {error ? (
        <div className="mt-3 rounded-xl border border-destructive/35 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>
      ) : loading && !data ? (
        <div className="mt-3 h-72 animate-pulse rounded-xl border border-border/50 bg-muted/25" />
      ) : users.length ? (
        <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(430px,1.35fr)]">
          <div className="rounded-xl border border-border/50 bg-background/25 p-3">
            <div className="flex items-center justify-between gap-2">
              <p className="text-sm font-medium">流量最高用户</p>
              <span className="text-[11px] text-muted-foreground">蓝色下载 · 紫色上传</span>
            </div>
            <div className="mt-4 space-y-3">
              {topUsers.map((user) => {
                const receivedPercent = user.total > 0 ? (user.received / maxTraffic) * 100 : 0;
                const sentPercent = user.total > 0 ? (user.sent / maxTraffic) * 100 : 0;
                return (
                  <div key={`${user.username}-${user.commonName ?? ''}`}>
                    <div className="mb-1 flex items-center justify-between gap-2 text-xs">
                      <span className="flex min-w-0 items-center gap-1.5 font-medium">
                        {user.online ? <Wifi className="h-3.5 w-3.5 shrink-0 text-emerald-500" /> : <WifiOff className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                        <span className="truncate">{user.username}</span>
                      </span>
                      <span className="shrink-0 tabular-nums text-muted-foreground">{formatBytes(user.total)}</span>
                    </div>
                    <div className="flex h-2.5 overflow-hidden rounded-full bg-muted/70" title={`${user.username}：下载 ${formatBytes(user.received)}，上传 ${formatBytes(user.sent)}`}>
                      <div className="bg-sky-500 transition-all" style={{ width: `${receivedPercent}%` }} />
                      <div className="bg-violet-500 transition-all" style={{ width: `${sentPercent}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          <div className="overflow-hidden rounded-xl border border-border/50 bg-background/25">
            <div className="flex items-center justify-between border-b border-border/50 px-3 py-2.5">
              <p className="text-sm font-medium">用户流量明细</p>
              <span className="text-[11px] text-muted-foreground">{users.length} 位用户</span>
            </div>
            <div className="max-h-[352px] overflow-auto">
              <table className="w-full min-w-[650px] text-left text-xs">
                <thead className="sticky top-0 z-10 bg-muted/90 text-muted-foreground backdrop-blur">
                  <tr>
                    <th className="px-3 py-2 font-medium">用户</th>
                    <th className="px-2 py-2 text-right font-medium">下载</th>
                    <th className="px-2 py-2 text-right font-medium">上传</th>
                    <th className="px-2 py-2 text-right font-medium">总流量</th>
                    <th className="px-2 py-2 text-right font-medium">在线时长</th>
                    <th className="px-3 py-2 text-right font-medium">状态</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/40">
                  {users.map((user) => (
                    <tr key={`${user.username}-${user.commonName ?? ''}`} className="transition-colors hover:bg-primary/[0.045]">
                      <td className="max-w-[165px] px-3 py-2.5">
                        <div className="truncate font-medium">{user.username}</div>
                        <div className="mt-0.5 flex items-center gap-1 text-[10px] text-muted-foreground">
                          <Clock3 className="h-3 w-3" />
                          最近活动 {formatLastSeen(user.lastSeen)}
                        </div>
                      </td>
                      <td className="px-2 py-2.5 text-right font-medium tabular-nums text-sky-500">{formatBytes(user.received)}</td>
                      <td className="px-2 py-2.5 text-right font-medium tabular-nums text-violet-500">{formatBytes(user.sent)}</td>
                      <td className="px-2 py-2.5 text-right font-semibold tabular-nums">{formatBytes(user.total)}</td>
                      <td className="px-2 py-2.5 text-right tabular-nums text-muted-foreground">{formatDuration(user.onlineSeconds)}</td>
                      <td className="px-3 py-2.5 text-right">
                        <Badge variant={user.online ? 'default' : 'secondary'} className="gap-1 text-[10px]">
                          {user.online ? <Wifi className="h-3 w-3" /> : <WifiOff className="h-3 w-3" />}
                          {user.online ? '在线' : '离线'}
                        </Badge>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      ) : (
        <div className="mt-3 flex h-60 flex-col items-center justify-center rounded-xl border border-dashed border-border/70 bg-background/20 text-center">
          <Activity className="h-7 w-7 text-muted-foreground/60" />
          <p className="mt-2 text-sm font-medium">该时间段暂无用户流量</p>
          <p className="mt-1 max-w-md text-xs text-muted-foreground">服务会从在线客户端开始按分钟采样；已断开的历史会话也会在此汇总展示。</p>
        </div>
      )}

      <p className="mt-3 text-[11px] leading-5 text-muted-foreground">
        说明：新产生的在线流量按 60 秒采样后精确归属到所选时间段；正在进行中的连接会额外补充尚未采样的实时流量。
      </p>
    </div>
  );
}
