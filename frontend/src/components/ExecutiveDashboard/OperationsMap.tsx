import { ChevronLeft, ChevronRight, Copy, Globe2, MapPinned, Router, ShieldCheck, UsersRound } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '@/api';
import type { DashboardGeoIPDetail, DashboardGeoIPDetailsResponse, DashboardGeoPoint, DashboardGeoResponse, DashboardGeoSource } from '@/types';
import { cn } from '@/lib/utils';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/ui/dialog';
import { ChinaUsageMap } from './ChinaUsageMap';
import { InteractiveGeoGlobe, type GeoGlobeMarker, type GeoGlobeView } from './InteractiveGeoGlobe';

type MapPosition = readonly [string, number, number];

const SOURCE_META: Record<
  DashboardGeoSource,
  {
    label: string;
    short: string;
    icon: typeof UsersRound;
    description: string;
    tone: string;
    globeTone: 'emerald' | 'sky' | 'violet';
  }
> = {
  online: {
    label: '在线客户端来源',
    short: '客户端来源',
    icon: UsersRound,
    description: '实时快照：仅使用 OpenVPN management 返回的客户端公网 Rip。',
    tone: 'text-emerald-700 bg-emerald-500/10 border-emerald-500/25 dark:text-emerald-300',
    globeTone: 'emerald',
  },
  audit: {
    label: '管理操作来源',
    short: '管理操作',
    icon: ShieldCheck,
    description: '历史审计：仅使用管理后台操作审计日志记录的访问来源。',
    tone: 'text-sky-700 bg-sky-500/10 border-sky-500/25 dark:text-sky-300',
    globeTone: 'sky',
  },
  website: {
    label: '网站目标服务',
    short: '目标服务',
    icon: Router,
    description: '网站目标服务所在地：仅使用 Suricata 观察到的目标 IP，不代表 VPN 用户所在地。',
    tone: 'text-violet-700 bg-violet-500/10 border-violet-500/25 dark:text-violet-300',
    globeTone: 'violet',
  },
};

// Curated country/province centroids are used only for visualising an already
// aggregated region. The backend never returns raw IPs or exact coordinates.
const WORLD_POSITIONS: MapPosition[] = [
  ['中国香港', 114.17, 22.32],
  ['中国台湾', 120.96, 23.7],
  ['中国', 104.2, 35.86],
  ['美国', -98.58, 39.83],
  ['加拿大', -106.35, 56.13],
  ['英国', -3.44, 55.38],
  ['德国', 10.45, 51.17],
  ['法国', 2.21, 46.23],
  ['俄罗斯', 105.32, 61.52],
  ['日本', 138.25, 36.2],
  ['韩国', 127.77, 35.91],
  ['新加坡', 103.82, 1.35],
  ['印度', 78.96, 20.59],
  ['澳大利亚', 133.78, -25.27],
  ['巴西', -51.93, -14.24],
  ['阿联酋', 53.85, 23.42],
  ['荷兰', 5.29, 52.13],
  ['意大利', 12.57, 41.87],
  ['西班牙', -3.75, 40.46],
  ['瑞典', 18.64, 60.13],
  ['土耳其', 35.24, 38.96],
  ['印度尼西亚', 113.92, -0.79],
];

const CHINA_POSITIONS: MapPosition[] = [
  ['北京市', 116.41, 39.9],
  ['北京', 116.41, 39.9],
  ['天津', 117.2, 39.12],
  ['河北', 114.5, 38.04],
  ['山西', 112.55, 37.87],
  ['内蒙古', 111.67, 40.82],
  ['辽宁', 123.43, 41.81],
  ['吉林', 125.32, 43.9],
  ['黑龙江', 126.64, 45.76],
  ['上海市', 121.47, 31.23],
  ['上海', 121.47, 31.23],
  ['江苏', 118.8, 32.06],
  ['浙江', 120.16, 30.25],
  ['安徽', 117.28, 31.86],
  ['福建', 119.3, 26.08],
  ['江西', 115.86, 28.68],
  ['山东', 117.0, 36.67],
  ['河南', 113.62, 34.75],
  ['湖北', 114.3, 30.6],
  ['湖南', 112.94, 28.23],
  ['广东', 113.27, 23.13],
  ['广西', 108.32, 22.82],
  ['海南', 110.35, 20.02],
  ['重庆', 106.55, 29.56],
  ['四川', 104.07, 30.67],
  ['贵州', 106.63, 26.65],
  ['云南', 102.71, 25.04],
  ['西藏', 91.13, 29.65],
  ['陕西', 108.94, 34.34],
  ['甘肃', 103.83, 36.06],
  ['青海', 101.78, 36.62],
  ['宁夏', 106.28, 38.47],
  ['新疆', 87.62, 43.82],
  ['香港', 114.17, 22.32],
  ['澳门', 113.54, 22.2],
  ['台湾', 121.51, 25.04],
  ['中国', 104.2, 35.86],
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

function coordinateForPoint(point: DashboardGeoPoint, view: GeoGlobeView): [number, number] | undefined {
  const positions = view === 'china' ? CHINA_POSITIONS : WORLD_POSITIONS;
  const candidates =
    view === 'china'
      ? [point.city || '', point.province || '', point.country, point.label]
      : [point.country, point.province || '', point.city || '', point.label];
  return coordinateForCandidates(positions, candidates);
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
  className,
}: {
  data?: DashboardGeoResponse;
  source: DashboardGeoSource;
  view: GeoGlobeView;
  onSourceChange: (source: DashboardGeoSource) => void;
  onViewChange: (view: GeoGlobeView) => void;
  loading: boolean;
  error: boolean;
  className?: string;
}) {
  const available = data?.availableSources ?? [];
  const allPoints = (data?.points ?? []).filter((point) => point.source === source);
  const chinaPoints = allPoints.filter(
    (point) => point.country.includes('中国') || point.country.toLowerCase() === 'china'
  );
  const pointsForView = view === 'china' ? chinaPoints : allPoints;
  const displayPoints = pointsForView.slice(0, 12);
  const total = pointsForView.reduce((sum, point) => sum + point.count, 0);
  const max = Math.max(...displayPoints.map((point) => point.count), 1);
  const meta = SOURCE_META[source];
  const Icon = meta.icon;
  const unknown = data?.unknown?.[source] || 0;
  // Camera presets must remain usable even when the selected data source has
  // no points yet. This lets operators inspect the China view before data
  // arrives, and prevents a refresh from unexpectedly changing their view.
  const mappedMarkers = useMemo<GeoGlobeMarker[]>(
    () =>
      pointsForView.flatMap((point, index) => {
        const coordinate = coordinateForPoint(point, view);
        if (!coordinate) return [];
        return [
          {
            id: `${source}:${point.country}:${point.province ?? ''}:${point.city ?? ''}:${index}`,
            label: point.label,
            count: point.count,
            longitude: coordinate[0],
            latitude: coordinate[1],
            point,
          },
        ];
      }),
    [pointsForView, source, view]
  );
  const visibleMarkers = mappedMarkers.slice(0, 48);
  const unmappedRegions = pointsForView.length - mappedMarkers.length;
  const sourceAvailable = available.includes(source);
  // The 3D globe is a core visual anchor for the command center, not a data
  // response. Keep it mounted while the API is loading, unavailable, or the
  // account lacks the selected source permission so transient backend issues
  // never turn the central map area into an empty placeholder.
  const hasGeoData = !loading && !error && sourceAvailable;
  const globeMarkers = hasGeoData ? visibleMarkers : [];
  const globeEmptyMessage = loading
    ? '正在同步区域态势数据…'
    : error
      ? '区域数据暂不可用；仍可拖动、缩放并查看地球。'
      : !sourceAvailable
        ? `当前账号没有查看${sourceTitle(source)}明细的权限。`
        : view === 'china'
          ? '当前来源暂无可定位的中国区域数据。'
          : '当前时间范围没有可定位的公网 IP。';
  const [detailMarkers, setDetailMarkers] = useState<GeoGlobeMarker[]>([]);
  const [detailPage, setDetailPage] = useState(1);
  const [detailItems, setDetailItems] = useState<DashboardGeoIPDetail[]>([]);
  const [detailTotal, setDetailTotal] = useState(0);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState('');
  const detailOpen = detailMarkers.length > 0;
  const detailLabel = detailMarkers.length === 1 ? detailMarkers[0].label : detailMarkers.map((marker) => marker.label).join('、');
  const detailPageSize = 50;
  const detailPages = Math.max(1, Math.ceil(detailTotal / detailPageSize));

  const openDetail = useCallback((markers: GeoGlobeMarker[]) => {
    if (!markers.length) return;
    setDetailMarkers(markers);
    setDetailPage(1);
    setDetailError('');
  }, []);
  const selectGlobeMarker = useCallback((marker: GeoGlobeMarker) => openDetail([marker]), [openDetail]);
  const selectChinaMarkers = useCallback((markers: GeoGlobeMarker[]) => openDetail(markers), [openDetail]);
  const loadChinaIPDetailsPreview = useCallback(async (markers: GeoGlobeMarker[]) => {
    const responses = await Promise.all(markers.map((marker) => {
      const params = new URLSearchParams({
        source,
        country: marker.point.country,
        page: '1',
        pageSize: '5',
      });
      if (marker.point.province) params.set('province', marker.point.province);
      if (marker.point.city) params.set('city', marker.point.city);
      if (data?.start) params.set('start', String(data.start));
      if (data?.end) params.set('end', String(data.end));
      return api.get<DashboardGeoIPDetailsResponse>(`/ovpn/dashboard/geo-map/ips?${params.toString()}`);
    }));
    const unique = new Map<string, DashboardGeoIPDetail>();
    responses.forEach((response) => response.items.forEach((item) => unique.set(item.ip, item)));
    return [...unique.values()].sort((a, b) => a.ip.localeCompare(b.ip, undefined, { numeric: true })).slice(0, 5);
  }, [data?.end, data?.start, source]);

  useEffect(() => {
    setDetailMarkers([]);
  }, [source, view]);

  useEffect(() => {
    if (!detailOpen || !detailMarkers.length) return;
    let cancelled = false;
    setDetailLoading(true);
    setDetailError('');
    const requestRegion = (marker: GeoGlobeMarker, page: number) => {
      const params = new URLSearchParams({
        source,
        country: marker.point.country,
        page: String(page),
        pageSize: '100',
      });
      if (marker.point.province) params.set('province', marker.point.province);
      if (marker.point.city) params.set('city', marker.point.city);
      if (data?.start) params.set('start', String(data.start));
      if (data?.end) params.set('end', String(data.end));
      return api.get<DashboardGeoIPDetailsResponse>(`/ovpn/dashboard/geo-map/ips?${params.toString()}`);
    };
    Promise.all(detailMarkers.map((marker) => requestRegion(marker, 1)))
      .then(async (firstPages) => {
        const remaining = firstPages.flatMap((response, markerIndex) =>
          Array.from({ length: Math.max(0, Math.ceil(response.total / 100) - 1) }, (_, index) => requestRegion(detailMarkers[markerIndex], index + 2))
        );
        return [...firstPages, ...(remaining.length ? await Promise.all(remaining) : [])];
      })
      .then((responses) => {
        if (cancelled) return;
        const unique = new Map<string, DashboardGeoIPDetail>();
        responses.forEach((response) => response.items.forEach((item) => unique.set(item.ip, item)));
        const allItems = [...unique.values()].sort((a, b) => a.ip.localeCompare(b.ip, undefined, { numeric: true }));
        setDetailItems(allItems);
        setDetailTotal(allItems.length);
      })
      .catch(() => {
        if (!cancelled) {
          setDetailItems([]);
          setDetailTotal(0);
          setDetailError('公网 IP 明细暂时无法读取，请稍后重试。');
        }
      })
      .finally(() => {
        if (!cancelled) setDetailLoading(false);
      });
    return () => { cancelled = true; };
  }, [data?.end, data?.start, detailMarkers, detailOpen, source]);

  const pageItems = detailItems.slice((detailPage - 1) * detailPageSize, detailPage * detailPageSize);

  const copyIP = useCallback((ip: string) => {
    navigator.clipboard?.writeText(ip).catch(() => undefined);
  }, []);

  return (
    <section className={cn('flex min-h-[560px] flex-col rounded-[1.4rem] border border-primary/20 bg-card/85 p-4 shadow-lg shadow-primary/10 backdrop-blur xl:min-h-0', className)}>
      <div className="mb-3 shrink-0 flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-primary/20 to-accent/20 text-primary shadow-inner">
            <Globe2 className="h-5 w-5" />
          </span>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="font-semibold tracking-tight">地理态势中心</h2>
              <span className="rounded-full border border-primary/20 bg-primary/5 px-2 py-0.5 text-[10px] font-medium text-primary">交互式 3D</span>
            </div>
            <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
              区域级地理聚合。拖动地球可旋转，滚轮或右上角按钮可缩放，点击发光节点可自动聚焦查看。
            </p>
          </div>
        </div>
        <div className="flex w-full flex-nowrap gap-1 overflow-x-auto rounded-xl border border-border bg-muted/65 p-1 xl:w-auto xl:overflow-visible">
          {(Object.keys(SOURCE_META) as DashboardGeoSource[]).map((candidate) => (
            <button
              key={candidate}
              type="button"
              disabled={!available.includes(candidate)}
              onClick={() => onSourceChange(candidate)}
              className={cn(
                'min-w-max flex-1 cursor-pointer whitespace-nowrap rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-40 xl:flex-none',
                source === candidate
                  ? 'bg-card text-foreground shadow-sm ring-1 ring-border'
                  : 'text-muted-foreground hover:bg-card/85 hover:text-foreground'
              )}
            >
              {SOURCE_META[candidate].short}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-3 shrink-0 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 px-3 py-2">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <MapPinned className="h-3.5 w-3.5 text-primary" />
          <span>视图范围</span>
          <span className="font-medium text-foreground">{view === 'china' ? '中国区域' : '全球区域'}</span>
        </div>
        <div className="flex rounded-lg border border-border bg-card/75 p-0.5 text-xs">
          <button
            type="button"
            onClick={() => onViewChange('world')}
            className={cn(
              'cursor-pointer rounded-md px-2.5 py-1 transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
              view === 'world' ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
            )}
          >
            全球
          </button>
          <button
            type="button"
            onClick={() => onViewChange('china')}
            title="切换到中国区域视角（暂无数据时仍可查看）"
            className={cn(
              'cursor-pointer rounded-md px-2.5 py-1 transition-colors duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
              view === 'china' ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
            )}
          >
            中国聚焦
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(0,1.55fr)_minmax(220px,.72fr)]">
        <div className="relative min-h-[260px] xl:min-h-0 [&_.china-usage-map]:h-full [&_.geo-globe-shell]:h-full">
          {view === 'china' ? (
            <ChinaUsageMap
              markers={globeMarkers}
              emptyMessage={globeEmptyMessage}
              onMarkerSelect={selectChinaMarkers}
              loadIPDetails={loadChinaIPDetailsPreview}
            />
          ) : (
            <InteractiveGeoGlobe
              markers={globeMarkers}
              view={view}
              tone={meta.globeTone}
              emptyMessage={globeEmptyMessage}
              onMarkerSelect={selectGlobeMarker}
            />
          )}
          {!hasGeoData && (
            <div
              className={cn(
                'pointer-events-none absolute inset-x-4 bottom-4 rounded-xl border px-3 py-2.5 text-center text-xs leading-5 shadow-sm backdrop-blur',
                error
                  ? 'border-amber-500/30 bg-amber-500/10 text-foreground'
                  : 'border-border bg-card/90 text-muted-foreground'
              )}
              role={error ? 'alert' : undefined}
            >
              {loading ? (
                <span className="inline-flex items-center gap-2"><Globe2 className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" />正在同步区域态势…</span>
              ) : error ? (
                <span className="inline-flex items-center gap-2"><MapPinned className="h-3.5 w-3.5" />区域数据暂不可用；地球仍可交互查看。</span>
              ) : (
                <span className="inline-flex items-center gap-2"><ShieldCheck className="h-3.5 w-3.5" />当前账号没有查看{sourceTitle(source)}明细的权限。</span>
              )}
            </div>
          )}
        </div>

        <aside className="geo-insight-panel flex min-h-[260px] flex-col rounded-[1.35rem] border p-3 xl:min-h-0">
          <div className="grid grid-cols-2 gap-2">
            <div className="rounded-xl border border-emerald-500/20 bg-card/75 p-3 shadow-sm">
              <p className="text-[11px] text-muted-foreground">已定位公网 IP</p>
              <p className="mt-1 text-2xl font-semibold tracking-tight tabular-nums text-foreground">{hasGeoData ? total : '—'}</p>
              <p className="mt-1 text-[10px] text-emerald-700 dark:text-emerald-300">{hasGeoData ? `${pointsForView.length} 个区域聚合` : '等待区域数据'}</p>
            </div>
            <div className="rounded-xl border border-amber-500/20 bg-card/75 p-3 shadow-sm">
              <p className="text-[11px] text-muted-foreground">无法定位</p>
              <p className="mt-1 text-2xl font-semibold tracking-tight tabular-nums text-foreground">{hasGeoData ? unknown : '—'}</p>
              <p className="mt-1 text-[10px] text-amber-700 dark:text-amber-300">内网 / IPv6 / 解析失败</p>
            </div>
          </div>

          <div className="mt-3 flex min-h-0 flex-1 flex-col rounded-xl border border-border bg-card/75 p-3">
            <div className="flex items-center gap-2">
              <span className={cn('rounded-lg border p-1.5', meta.tone)}>
                <Icon className="h-3.5 w-3.5" />
              </span>
              <div className="min-w-0">
                <p className="text-sm font-semibold">区域活跃排行</p>
                <p className="text-[11px] text-muted-foreground">去重公网 IP · 前 {Math.min(displayPoints.length, 12)} 个区域</p>
              </div>
            </div>
            {hasGeoData ? (
              <div className="mt-3 max-h-[188px] space-y-2 overflow-auto pr-1">
                {displayPoints.map((point, index) => (
                  <div key={`${point.label}-${index}`} className="group">
                    <div className="mb-1 flex items-center justify-between gap-2 text-xs">
                      <span className="min-w-0 truncate font-medium text-foreground">
                        <span className="mr-1.5 text-[10px] tabular-nums text-muted-foreground/70">{String(index + 1).padStart(2, '0')}</span>
                        {point.label}
                      </span>
                      <strong className="shrink-0 tabular-nums text-foreground">{point.count}</strong>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full bg-gradient-to-r from-primary to-accent transition-[width] duration-500 ease-out"
                        style={{ width: `${Math.max(5, (point.count / max) * 100)}%` }}
                      />
                    </div>
                  </div>
                ))}
                {!displayPoints.length && <p className="py-6 text-center text-xs text-muted-foreground">暂无可展示的区域排行</p>}
              </div>
            ) : (
              <div className="flex flex-1 items-center justify-center px-3 text-center text-xs leading-5 text-muted-foreground">
                {loading ? '正在获取可定位公网 IP。' : error ? '数据接口暂时异常，恢复后会自动刷新区域排行。' : '当前来源尚未授权，无法展示区域排行。'}
              </div>
            )}
            {hasGeoData && (source === 'website' || unmappedRegions > 0) && (
              <div className="mt-3 space-y-1 border-t border-border pt-2 text-[11px] text-muted-foreground">
                {source === 'website' && <p>普通 DNS 无目标 IP：{data?.websiteDomainOnly ?? 0}</p>}
                {unmappedRegions > 0 && <p>{unmappedRegions} 个区域未配置示意坐标，仍保留在总量与排行中。</p>}
              </div>
            )}
          </div>
        </aside>
      </div>

      <p className="mt-3 shrink-0 rounded-xl border border-border bg-muted/50 px-3 py-2 text-[11px] leading-5 text-muted-foreground">
        {meta.description}{' '}
        {source === 'website'
          ? '普通 DNS 域名无法确定用户位置。'
          : '内网、VPN 虚拟 IP、回环地址、IPv6 和解析失败地址不会生成地图点位。'}{' '}
        {source === 'online' && data?.onlineAsOf
          ? `在线快照时间：${new Date(data.onlineAsOf * 1000).toLocaleString()}。`
          : ''}{' '}
        {data?.notes?.join(' ')}
      </p>

      <Dialog open={detailOpen} onOpenChange={(open) => { if (!open) setDetailMarkers([]); }}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>公网 IP 明细 · {detailLabel}</DialogTitle>
            <DialogDescription>
              {sourceTitle(source)} · 已按公网 IPv4 去重，仅展示当前账号有权限查看的数据。
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-xl border border-border bg-muted/45 px-3 py-2 text-sm text-muted-foreground">
            去重公网 IP：<strong className="text-foreground">{detailLoading ? '读取中…' : detailTotal}</strong> 个
            {source === 'online' && data?.onlineAsOf ? ` · 在线快照 ${new Date(data.onlineAsOf * 1000).toLocaleString()}` : ''}
          </div>
          {detailError ? <p className="rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{detailError}</p> : null}
          <div className="max-h-[48vh] overflow-auto rounded-xl border border-border">
            {detailLoading ? (
              <p className="px-4 py-10 text-center text-sm text-muted-foreground">正在读取公网 IP 明细…</p>
            ) : detailItems.length ? (
              <ul className="divide-y divide-border">
                {pageItems.map((item) => (
                  <li key={item.ip} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div className="min-w-0">
                      <p className="font-mono text-sm font-medium text-foreground">{item.ip}</p>
                      <p className="truncate text-xs text-muted-foreground">{[item.country, item.province, item.city].filter(Boolean).join(' · ')}</p>
                    </div>
                    <button type="button" onClick={() => copyIP(item.ip)} className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" title={`复制 ${item.ip}`} aria-label={`复制 ${item.ip}`}>
                      <Copy className="h-3.5 w-3.5" />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="px-4 py-10 text-center text-sm text-muted-foreground">当前区域没有可展示的公网 IP。</p>
            )}
          </div>
          {detailTotal > detailPageSize ? (
            <div className="flex items-center justify-between text-sm text-muted-foreground">
              <span>第 {detailPage} / {detailPages} 页</span>
              <div className="flex gap-1">
                <button type="button" disabled={detailPage <= 1 || detailLoading} onClick={() => setDetailPage((page) => Math.max(1, page - 1))} className="inline-flex h-9 items-center gap-1 rounded-lg border border-border px-2.5 disabled:cursor-not-allowed disabled:opacity-45"><ChevronLeft className="h-4 w-4" />上一页</button>
                <button type="button" disabled={detailPage >= detailPages || detailLoading} onClick={() => setDetailPage((page) => Math.min(detailPages, page + 1))} className="inline-flex h-9 items-center gap-1 rounded-lg border border-border px-2.5 disabled:cursor-not-allowed disabled:opacity-45">下一页<ChevronRight className="h-4 w-4" /></button>
              </div>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </section>
  );
}