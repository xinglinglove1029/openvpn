import { Globe2, MapPinned, Router, ShieldCheck, UsersRound } from 'lucide-react';
import type { DashboardGeoPoint, DashboardGeoResponse, DashboardGeoSource } from '@/types';
import { cn } from '@/lib/utils';

type MapView = 'world' | 'china';
type MapPosition = readonly [string, number, number];

const SOURCE_META: Record<
  DashboardGeoSource,
  { label: string; short: string; icon: typeof UsersRound; description: string; tone: string }
> = {
  online: {
    label: '在线客户端来源',
    short: '客户端来源',
    icon: UsersRound,
    description: '来源 IP 归属地：仅使用 OpenVPN management 返回的客户端公网 Rip。',
    tone: 'text-emerald-500 bg-emerald-500/10 border-emerald-500/25',
  },
  audit: {
    label: '管理操作来源',
    short: '管理操作',
    icon: ShieldCheck,
    description: '来源 IP 归属地：仅使用管理后台操作审计日志记录的访问来源。',
    tone: 'text-sky-500 bg-sky-500/10 border-sky-500/25',
  },
  website: {
    label: '网站目标服务',
    short: '目标服务',
    icon: Router,
    description: '网站目标服务所在地：仅使用 Suricata 观察到的目标 IP，不代表 VPN 用户所在地。',
    tone: 'text-violet-500 bg-violet-500/10 border-violet-500/25',
  },
};

// These are only approximate, curated regional anchor points for the lightweight
// SVG illustration. Regions without an anchor remain in the ranking instead of
// being rendered at a fabricated random coordinate.
const WORLD_POSITIONS: MapPosition[] = [
  ['中国香港', 75, 51],
  ['中国台湾', 78, 52],
  ['中国', 73, 42],
  ['美国', 22, 39],
  ['加拿大', 19, 24],
  ['英国', 46, 30],
  ['德国', 50, 33],
  ['法国', 47, 36],
  ['俄罗斯', 63, 25],
  ['日本', 81, 42],
  ['韩国', 79, 42],
  ['新加坡', 74, 68],
  ['印度', 67, 55],
  ['澳大利亚', 84, 76],
  ['巴西', 34, 68],
  ['阿联酋', 59, 51],
];
const CHINA_POSITIONS: MapPosition[] = [
  ['内蒙古', 55, 25],
  ['黑龙江', 80, 17],
  ['北京市', 69, 29],
  ['北京', 69, 29],
  ['天津', 71, 33],
  ['河北', 64, 36],
  ['山西', 59, 37],
  ['辽宁', 76, 28],
  ['吉林', 79, 23],
  ['上海市', 76, 54],
  ['上海', 76, 54],
  ['江苏', 73, 50],
  ['浙江', 75, 59],
  ['安徽', 68, 52],
  ['福建', 70, 65],
  ['江西', 64, 62],
  ['山东', 70, 42],
  ['河南', 62, 47],
  ['湖北', 60, 55],
  ['湖南', 57, 62],
  ['广东', 59, 71],
  ['广西', 50, 70],
  ['海南', 51, 84],
  ['重庆', 51, 57],
  ['四川', 43, 55],
  ['贵州', 47, 65],
  ['云南', 39, 69],
  ['西藏', 23, 55],
  ['陕西', 53, 46],
  ['甘肃', 43, 39],
  ['青海', 34, 43],
  ['宁夏', 49, 38],
  ['新疆', 23, 28],
  ['香港', 61, 75],
  ['澳门', 59, 76],
  ['台湾', 79, 68],
  ['中国', 60, 50],
];

function coordinateForCandidates(positions: MapPosition[], candidates: string[]): [number, number] | undefined {
  for (const candidate of candidates.filter(Boolean)) {
    const exact = positions.find(([name]) => name === candidate);
    if (exact) return [exact[1], exact[2]];
    const partial = positions
      .filter(([name]) => candidate.includes(name) || name.includes(candidate))
      .sort(([a], [b]) => b.length - a.length)[0];
    if (partial) return [partial[1], partial[2]];
  }
  return undefined;
}

function approximatePoint(point: DashboardGeoPoint, view: MapView): [number, number] | undefined {
  if (view === 'china')
    return coordinateForCandidates(CHINA_POSITIONS, [
      point.city || '',
      point.province || '',
      point.country,
      point.label,
    ]);
  return coordinateForCandidates(WORLD_POSITIONS, [point.country, point.province || '', point.label]);
}

function sourceTitle(source: DashboardGeoSource) {
  return SOURCE_META[source]?.label || source;
}

export function OperationsMap({
  data,
  source,
  view,
  onSourceChange,
  onViewChange,
  loading,
  error,
}: {
  data?: DashboardGeoResponse;
  source: DashboardGeoSource;
  view: MapView;
  onSourceChange: (source: DashboardGeoSource) => void;
  onViewChange: (view: MapView) => void;
  loading: boolean;
  error: boolean;
}) {
  const available = data?.availableSources ?? [];
  const allPoints = (data?.points ?? []).filter((point) => point.source === source);
  const chinaPoints = allPoints.filter(
    (point) => point.country.includes('中国') || point.country.toLowerCase() === 'china'
  );
  const pointsForView = view === 'china' ? chinaPoints : allPoints;
  const displayPoints = pointsForView.slice(0, 12);
  const mappedPoints = displayPoints.flatMap((point) => {
    const coordinates = approximatePoint(point, view);
    return coordinates ? [{ point, coordinates }] : [];
  });
  const unmappedCount = displayPoints.length - mappedPoints.length;
  const total = pointsForView.reduce((sum, point) => sum + point.count, 0);
  const max = Math.max(...displayPoints.map((point) => point.count), 1);
  const meta = SOURCE_META[source];
  const Icon = meta.icon;
  const unknown = data?.unknown?.[source] || 0;
  const hasChinaPoints = chinaPoints.length > 0;
  const hasChinaDataForSource = (candidate: DashboardGeoSource) =>
    (data?.points ?? []).some(
      (point) =>
        point.source === candidate && (point.country.includes('中国') || point.country.toLowerCase() === 'china')
    );

  return (
    <div className="rounded-xl border border-border/60 bg-card/75 p-4 shadow-sm backdrop-blur">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="flex items-center gap-2">
            <MapPinned className="h-4 w-4 text-primary" />
            <h2 className="font-semibold">地理态势</h2>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">按区域聚合的公网 IP 分布，不展示原始 IP 或精确坐标。</p>
        </div>
        <div className="flex rounded-lg border border-border bg-muted/50 p-0.5 text-xs">
          {(Object.keys(SOURCE_META) as DashboardGeoSource[]).map((candidate) => (
            <button
              key={candidate}
              type="button"
              disabled={!available.includes(candidate)}
              onClick={() => {
                onSourceChange(candidate);
                if (view === 'china' && !hasChinaDataForSource(candidate)) onViewChange('world');
              }}
              className={cn(
                'rounded-md px-2.5 py-1 transition-colors disabled:cursor-not-allowed disabled:opacity-45',
                source === candidate
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {SOURCE_META[candidate].short}
            </button>
          ))}
        </div>
      </div>
      <div className="mb-3 flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">区域视图</span>
        <div className="flex rounded-lg border border-border bg-muted/50 p-0.5 text-xs">
          <button
            type="button"
            onClick={() => onViewChange('world')}
            className={cn(
              'rounded-md px-2.5 py-1 transition-colors',
              view === 'world'
                ? 'bg-primary text-primary-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            世界
          </button>
          <button
            type="button"
            disabled={!hasChinaPoints}
            onClick={() => onViewChange('china')}
            title={hasChinaPoints ? '查看中国区域聚合' : '当前来源暂无中国区域数据'}
            className={cn(
              'rounded-md px-2.5 py-1 transition-colors disabled:cursor-not-allowed disabled:opacity-45',
              view === 'china'
                ? 'bg-primary text-primary-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            )}
          >
            中国
          </button>
        </div>
      </div>
      {loading ? (
        <div className="flex h-[245px] items-center justify-center text-sm text-muted-foreground">
          <Globe2 className="mr-2 h-5 w-5 animate-spin" />
          正在汇总地理分布…
        </div>
      ) : error ? (
        <div className="flex h-[245px] items-center justify-center text-center text-sm text-muted-foreground">
          <div>
            <MapPinned className="mx-auto mb-2 h-7 w-7 text-amber-500" />
            地理分布暂不可用，其他大屏数据不受影响。
          </div>
        </div>
      ) : !available.includes(source) ? (
        <div className="flex h-[245px] items-center justify-center text-center text-sm text-muted-foreground">
          <div>
            <ShieldCheck className="mx-auto mb-2 h-7 w-7" />
            当前账号没有查看{sourceTitle(source)}明细的权限。
          </div>
        </div>
      ) : (
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1.45fr)_minmax(190px,.75fr)]">
          <div className="relative h-[245px] overflow-hidden rounded-xl border border-border/60 bg-[radial-gradient(circle_at_50%_38%,hsl(var(--primary)/.17),transparent_42%),linear-gradient(145deg,hsl(var(--muted)/.8),hsl(var(--background)/.35))]">
            <div className="absolute inset-0 opacity-40 [background-image:linear-gradient(hsl(var(--border))_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border))_1px,transparent_1px)] [background-size:28px_28px]" />
            <svg
              className="absolute inset-4 h-[calc(100%-2rem)] w-[calc(100%-2rem)] opacity-60"
              viewBox="0 0 100 100"
              preserveAspectRatio="none"
              aria-hidden="true"
            >
              {view === 'world' ? (
                <>
                  <path
                    d="M5 30l8-11 12 1 8 10-3 12-13 2-9-5zM34 19l14-5 11 4 6 10-5 8-14-1-8-7zM66 16l17 4 11 11-5 13-16 4-12-10zM36 49l10 4 4 13-8 22-9-8-4-18zM60 48l13 3 9 12-4 20-15 4-7-15zM79 69l10 2 4 9-7 6-9-6z"
                    fill="hsl(var(--primary) / .20)"
                    stroke="hsl(var(--primary) / .55)"
                    strokeWidth="0.45"
                  />
                  <path
                    d="M50 4v92M25 8v84M75 8v84M4 50h92M8 25h84M8 75h84"
                    stroke="hsl(var(--border))"
                    strokeWidth=".25"
                    strokeDasharray="1.5 1.5"
                  />
                </>
              ) : (
                <>
                  <path
                    d="M15 29l13-11 16 5 10-8 14 7 14-2 6 10-8 9 5 11-9 8 1 14-15 7-14-5-14 4-10-10 2-14-10-8zM67 77l6 3-2 4-6-2zM76 66l4 1-1 4-4-1z"
                    fill="hsl(var(--primary) / .20)"
                    stroke="hsl(var(--primary) / .55)"
                    strokeWidth=".6"
                  />
                  <path
                    d="M22 38l56 29M30 23l32 56M18 53l68-19"
                    stroke="hsl(var(--border))"
                    strokeWidth=".3"
                    strokeDasharray="1.2 2.1"
                  />
                </>
              )}
            </svg>
            <div className="absolute left-1/2 top-1/2 flex -translate-x-1/2 -translate-y-1/2 items-center gap-1.5 rounded-full border border-primary/35 bg-background/80 px-2.5 py-1 text-[11px] font-semibold shadow-lg backdrop-blur">
              <span className="h-2 w-2 animate-pulse rounded-full bg-primary" />
              {view === 'china' ? '中国区域' : 'OpenVPN'}
            </div>
            {mappedPoints.map(({ point, coordinates }, index) => {
              const size = 8 + Math.min(14, Math.round((point.count / max) * 14));
              return (
                <div
                  key={`${point.country}-${point.province}-${point.city}-${index}`}
                  className="group absolute"
                  style={{ left: `${coordinates[0]}%`, top: `${coordinates[1]}%`, transform: 'translate(-50%, -50%)' }}
                >
                  <span
                    className="absolute inset-0 animate-ping rounded-full bg-primary/35"
                    style={{ width: size, height: size }}
                  />
                  <span
                    className="relative block rounded-full border-2 border-background bg-primary shadow-[0_0_16px_hsl(var(--primary)/.8)]"
                    style={{ width: size, height: size }}
                  />
                  <span className="pointer-events-none absolute left-1/2 top-full z-10 mt-1 hidden min-w-max -translate-x-1/2 rounded-md border border-border bg-popover px-2 py-1 text-[10px] text-popover-foreground shadow-lg group-hover:block">
                    {point.label} · {point.count} 个公网 IP
                  </span>
                </div>
              );
            })}
            {!displayPoints.length && (
              <div className="absolute inset-0 flex items-center justify-center px-6 text-center text-sm text-muted-foreground">
                <div>
                  <Globe2 className="mx-auto mb-2 h-7 w-7 opacity-60" />
                  {view === 'china' ? '当前来源暂无中国区域数据。' : '当前时间范围没有可定位的公网 IP。'}
                </div>
              </div>
            )}
            <div className="absolute bottom-3 left-3 rounded-lg border border-border/50 bg-background/75 px-2.5 py-1.5 text-[11px] text-muted-foreground backdrop-blur">
              已定位 <strong className="ml-1 text-foreground">{total}</strong> 个
              {displayPoints.length < pointsForView.length && <span> · 展示前 {displayPoints.length} 个区域</span>} ·
              无法定位 <strong className="ml-1 text-foreground">{unknown}</strong> 个
            </div>
            {unmappedCount > 0 && (
              <div className="absolute bottom-3 right-3 rounded-lg border border-amber-500/30 bg-background/80 px-2 py-1 text-[10px] text-amber-700 dark:text-amber-300">
                {unmappedCount} 个区域未配置示意坐标
              </div>
            )}
          </div>
          <div className="space-y-2 overflow-hidden rounded-xl border border-border/50 bg-background/25 p-3">
            <div className="flex items-center gap-2">
              <span className={cn('rounded-lg p-1.5', meta.tone)}>
                <Icon className="h-4 w-4" />
              </span>
              <div>
                <p className="text-sm font-medium">区域排行</p>
                <p className="text-[11px] text-muted-foreground">
                  去重后的公网 IP{displayPoints.length < pointsForView.length ? '（前 12 个区域）' : ''}
                </p>
              </div>
            </div>
            <div className="max-h-[163px] space-y-2 overflow-auto pr-1">
              {displayPoints.map((point, index) => (
                <div key={`${point.label}-rank-${index}`}>
                  <div className="mb-1 flex items-center justify-between gap-2 text-xs">
                    <span className="truncate">
                      {String(index + 1).padStart(2, '0')} · {point.label}
                    </span>
                    <strong className="tabular-nums">{point.count}</strong>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                      className="h-full rounded-full bg-gradient-to-r from-primary to-violet-500"
                      style={{ width: `${Math.max(5, (point.count / max) * 100)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
            {source === 'website' && (
              <p className="border-t border-border/50 pt-2 text-[11px] leading-5 text-muted-foreground">
                普通 DNS 无目标 IP：{data?.websiteDomainOnly ?? 0}
              </p>
            )}
          </div>
        </div>
      )}
      <p className="rounded-lg border border-border/50 bg-background/25 px-3 py-2 text-[11px] leading-5 text-muted-foreground">
        {meta.description}{' '}
        {source === 'website'
          ? '普通 DNS 域名无法确定用户位置。'
          : '内网、VPN 虚拟 IP、回环地址、IPv6 和解析失败地址不会生成地图点位。'}{' '}
        {source === 'online' && data?.onlineAsOf
          ? `在线快照时间：${new Date(data.onlineAsOf * 1000).toLocaleString()}。`
          : ''}{' '}
        {data?.notes?.join(' ')}
      </p>
    </div>
  );
}
