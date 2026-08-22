import { ChevronLeft, Globe2, MapPinned, MousePointer2, RotateCcw, ZoomIn, ZoomOut } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent, type WheelEvent as ReactWheelEvent } from 'react';
import { cn } from '@/lib/utils';
import { feature as topojsonFeature } from 'topojson-client';
import type { DashboardGeoIPDetail } from '@/types';
import type { GeoGlobeMarker } from './InteractiveGeoGlobe';

export type CountryDrillContext = {
  country: string;
  iso3: string;
  label: string;
  target?: GeoGlobeMarker;
};

type Position = [number, number];
type CountryMapLevel = 'adm1' | 'adm2';
type GeoFeature = {
  properties?: Record<string, unknown> & {
    shapeName?: string;
    shapeISO?: string;
    shapeGroup?: string;
    name?: string;
    adcode?: number | string;
    center?: Position;
    centroid?: Position;
    'hc-key'?: string;
  };
  geometry?: { type?: 'Polygon' | 'MultiPolygon'; coordinates?: unknown };
};
type GeoFeatureCollection = { features?: GeoFeature[] };
type Bounds = { minLng: number; maxLng: number; minY: number; maxY: number };
type Viewport = { scale: number; translateX: number; translateY: number };
type Breadcrumb = { name: string; feature?: GeoFeature };
type Usage = { name: string; markers: GeoGlobeMarker[]; locationCount: number; ipCount: number };
type Hovered = Usage & { x: number; y: number; key: string; canDrill: boolean; loading: boolean; error: boolean; details: DashboardGeoIPDetail[] };
type DragState = { pointerId: number; clientX: number; clientY: number; viewport: Viewport };

type CountryUsageMapProps = {
  country: CountryDrillContext;
  markers: GeoGlobeMarker[];
  emptyMessage: string;
  onBackToWorld: () => void;
  onMarkerSelect?: (markers: GeoGlobeMarker[]) => void;
  loadIPDetails?: (markers: GeoGlobeMarker[]) => Promise<DashboardGeoIPDetail[]>;
};

const WIDTH = 1000;
const HEIGHT = 680;
const PADDING = 36;
const MIN_SCALE = 1;
const MAX_SCALE = 5;
const INITIAL_VIEWPORT: Viewport = { scale: 1, translateX: 0, translateY: 0 };
const boundaryCache = new Map<string, Promise<GeoFeature[]>>();

function clean(value: string | undefined) {
  return (value || '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLocaleLowerCase()
    .replace(/[\s\-_'’.,()\[\]{}]/g, '')
    .replace(/province|state|prefecture|region|governorate|district|county|city|municipality|oblast|krai|republic|territory|department|division|province|省|市|州|邦|县|区|自治区|特别行政区/g, '');
}

function collectPositions(value: unknown, output: Position[] = []): Position[] {
  if (!Array.isArray(value)) return output;
  if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') { output.push([value[0], value[1]]); return output; }
  value.forEach((entry) => collectPositions(entry, output));
  return output;
}

function mercatorY(latitude: number) {
  const safe = Math.max(-85, Math.min(85, latitude));
  const radians = (safe * Math.PI) / 180;
  return Math.log(Math.tan(Math.PI / 4 + radians / 2));
}

function boundsFor(features: GeoFeature[]): Bounds | null {
  const positions = features.flatMap((feature) => collectPositions(feature.geometry?.coordinates));
  if (!positions.length) return null;
  const longitudes = positions.map(([longitude]) => longitude);
  const ys = positions.map(([, latitude]) => mercatorY(latitude));
  return { minLng: Math.min(...longitudes), maxLng: Math.max(...longitudes), minY: Math.min(...ys), maxY: Math.max(...ys) };
}

function projectionFor(features: GeoFeature[]) {
  const bounds = boundsFor(features);
  if (!bounds) return () => [WIDTH / 2, HEIGHT / 2] as Position;
  const lngSpan = Math.max(0.01, bounds.maxLng - bounds.minLng);
  const ySpan = Math.max(0.01, bounds.maxY - bounds.minY);
  const scale = Math.min((WIDTH - PADDING * 2) / lngSpan, (HEIGHT - PADDING * 2) / ySpan);
  const usedWidth = lngSpan * scale;
  const usedHeight = ySpan * scale;
  const offsetX = (WIDTH - usedWidth) / 2 - bounds.minLng * scale;
  const offsetY = (HEIGHT - usedHeight) / 2 + bounds.maxY * scale;
  return ([longitude, latitude]: Position): Position => [longitude * scale + offsetX, offsetY - mercatorY(latitude) * scale];
}

function pathFor(feature: GeoFeature, project: (position: Position) => Position) {
  const geometry = feature.geometry;
  if (!geometry?.coordinates) return '';
  const polygons = geometry.type === 'Polygon' ? [geometry.coordinates] : geometry.type === 'MultiPolygon' ? geometry.coordinates : [];
  if (!Array.isArray(polygons)) return '';
  return polygons.flatMap((polygon) => Array.isArray(polygon) ? polygon.map((ring) => {
    if (!Array.isArray(ring)) return '';
    const points = ring.map((value, index) => {
      if (!Array.isArray(value) || typeof value[0] !== 'number' || typeof value[1] !== 'number') return '';
      const [x, y] = project([value[0], value[1]]);
      return `${index ? 'L' : 'M'}${x.toFixed(2)},${y.toFixed(2)}`;
    }).filter(Boolean);
    return points.length ? `${points.join(' ')} Z` : '';
  }) : []).join(' ');
}

function featureName(feature: GeoFeature, index = 0) {
  return String(feature.properties?.shapeName || feature.properties?.name || `行政区 ${index + 1}`);
}

function featureCenter(feature: GeoFeature): Position | undefined {
  const configured = feature.properties?.centroid || feature.properties?.center;
  if (Array.isArray(configured) && typeof configured[0] === 'number' && typeof configured[1] === 'number') return [configured[0], configured[1]];
  const positions = collectPositions(feature.geometry?.coordinates);
  if (!positions.length) return undefined;
  return [(Math.min(...positions.map(([x]) => x)) + Math.max(...positions.map(([x]) => x))) / 2, (Math.min(...positions.map(([, y]) => y)) + Math.max(...positions.map(([, y]) => y))) / 2];
}

const HIGHCHARTS_MAP_CODES: Record<string, string> = {
  USA: 'us', CAN: 'ca', GBR: 'gb', DEU: 'de', FRA: 'fr', RUS: 'ru', JPN: 'jp', KOR: 'kr', PRK: 'kp', SGP: 'sg',
  IND: 'in', AUS: 'au', BRA: 'br', ARE: 'ae', NLD: 'nl', ITA: 'it', ESP: 'es', SWE: 'se', TUR: 'tr', IDN: 'id',
  VNM: 'vn', THA: 'th', MYS: 'my', PHL: 'ph', SAU: 'sa', ZAF: 'za', EGY: 'eg', ARG: 'ar', NZL: 'nz', CHE: 'ch',
  POL: 'pl', UKR: 'ua', HKG: 'hk', TWN: 'tw', MAC: 'mo',
};

type TopologyLike = { objects?: Record<string, unknown> };
type GeoJSONResult = { type?: string; features?: GeoFeature[]; geometry?: GeoFeature['geometry']; properties?: GeoFeature['properties'] };

async function fetchHighchartsBoundary(iso3: string, level: CountryMapLevel, parent: GeoFeature | undefined, signal: AbortSignal): Promise<GeoFeature[] | undefined> {
  const countryCode = HIGHCHARTS_MAP_CODES[iso3];
  if (!countryCode) return undefined;
  const parentKey = String(parent?.properties?.['hc-key'] || '');
  const mapKey = level === 'adm1' ? `${countryCode}-all` : parentKey ? `${parentKey}-all` : '';
  if (!mapKey) return undefined;
  try {
    // Highcharts' country maps are compact TopoJSON files (typically tens of KB
    // for ADM1), which makes country entry responsive even on constrained links.
    const response = await fetch(`https://code.highcharts.com/mapdata/countries/${countryCode}/${mapKey}.topo.json`, { signal });
    if (!response.ok) return undefined;
    const topology = await response.json() as TopologyLike;
    const object = Object.values(topology.objects || {})[0];
    if (!object) return undefined;
    const converted = topojsonFeature(topology as never, object as never) as GeoJSONResult;
    const features = converted.type === 'FeatureCollection' ? converted.features || [] : converted.geometry ? [{ geometry: converted.geometry, properties: converted.properties }] : [];
    return features.filter((item) => Boolean(item.geometry?.coordinates));
  } catch (error) {
    if ((error as { name?: string })?.name === 'AbortError') throw error;
    return undefined;
  }
}

async function fetchBoundary(iso3: string, level: CountryMapLevel, parent: GeoFeature | undefined, signal: AbortSignal): Promise<GeoFeature[]> {
  const parentKey = level === 'adm2' ? String(parent?.properties?.['hc-key'] || parent?.properties?.shapeISO || featureName(parent || {})) : '';
  const key = `${iso3}:${level}:${parentKey}`;
  if (!boundaryCache.has(key)) {
    boundaryCache.set(key, (async () => {
      const compactFeatures = await fetchHighchartsBoundary(iso3, level, parent, signal);
      if (compactFeatures?.length) return compactFeatures;
      // Fallback for countries or subdivisions not provided by the compact map
      // collection. This same-origin route resolves the geoBoundaries Git-LFS
      // resource server-side, avoiding browser CORS failures.
      const response = await fetch(`/ovpn/dashboard/geo-boundary/${encodeURIComponent(iso3)}/${level.toUpperCase()}`, { signal });
      if (!response.ok) throw new Error(`行政区边界加载失败（${response.status}）`);
      const collection = await response.json() as GeoFeatureCollection;
      const features = collection.features || [];
      if (!features.length) throw new Error('行政区边界为空');
      return features;
    })());
  }
  // Race the shared cache with the component cancellation signal, without poisoning cache on a view switch.
  return await Promise.race([
    boundaryCache.get(key)!,
    new Promise<GeoFeature[]>((_, reject) => signal.addEventListener('abort', () => reject(new DOMException('请求已取消', 'AbortError')), { once: true })),
  ]);
}

function matchingFeature(marker: GeoGlobeMarker, features: GeoFeature[], level: CountryMapLevel) {
  const wanted = clean(level === 'adm1' ? marker.point.province || marker.point.city || marker.point.label : marker.point.city || marker.point.label);
  if (!wanted) return undefined;
  return features.find((feature) => {
    const actual = clean(featureName(feature));
    return actual === wanted || actual.includes(wanted) || wanted.includes(actual);
  });
}

function usageForFeature(feature: GeoFeature, markers: GeoGlobeMarker[], level: CountryMapLevel): Usage {
  const name = featureName(feature);
  const key = clean(name);
  const matched = markers.filter((marker) => {
    const candidate = clean(level === 'adm1' ? marker.point.province || marker.point.city || marker.point.label : marker.point.city || marker.point.label);
    return candidate && (candidate === key || candidate.includes(key) || key.includes(candidate));
  });
  return { name, markers: matched, locationCount: matched.length, ipCount: matched.reduce((total, marker) => total + marker.count, 0) };
}

function hasChildren(feature: GeoFeature, selectedParent?: GeoFeature) {
  if (!selectedParent) return true;
  const parentISO = String(selectedParent.properties?.shapeISO || '');
  const group = String(feature.properties?.shapeGroup || '');
  return !parentISO || !group || group === parentISO || group.includes(parentISO) || parentISO.includes(group);
}

export function CountryUsageMap({ country, markers, emptyMessage, onBackToWorld, onMarkerSelect, loadIPDetails }: CountryUsageMapProps) {
  const [level, setLevel] = useState<CountryMapLevel>('adm1');
  const [features, setFeatures] = useState<GeoFeature[]>([]);
  const [parent, setParent] = useState<GeoFeature>();
  const [breadcrumbs, setBreadcrumbs] = useState<Breadcrumb[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [viewport, setViewport] = useState<Viewport>(INITIAL_VIEWPORT);
  const [hovered, setHovered] = useState<Hovered>();
  const [selected, setSelected] = useState<Usage>();
  const mapRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const dragRef = useRef<DragState | undefined>(undefined);
  const hoverTimer = useRef<number | undefined>(undefined);
  const hoverRequestId = useRef(0);
  const hoverCache = useRef(new Map<string, DashboardGeoIPDetail[]>());

  const load = useCallback((nextLevel: CountryMapLevel, nextParent?: GeoFeature) => {
    const controller = new AbortController();
    setLoading(true); setLoadError(''); setSelected(undefined); setHovered(undefined); setViewport(INITIAL_VIEWPORT);
    void fetchBoundary(country.iso3, nextLevel, nextParent, controller.signal)
      .then((all) => {
        if (controller.signal.aborted) return;
        const scoped = nextLevel === 'adm2' && nextParent ? all.filter((feature) => hasChildren(feature, nextParent)) : all;
        setFeatures(scoped.length ? scoped : all);
        setLevel(nextLevel); setParent(nextParent);
      })
      .catch((error: unknown) => {
        if ((error as { name?: string })?.name === 'AbortError') return;
        setFeatures([]);
        setLoadError(error instanceof Error ? error.message : '行政区边界加载失败');
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [country.iso3]);

  useEffect(() => load('adm1'), [load]);
  useEffect(() => () => { if (hoverTimer.current !== undefined) window.clearTimeout(hoverTimer.current); }, []);

  const project = useMemo(() => projectionFor(features), [features]);
  const renderedFeatures = useMemo(() => features.slice(0, 6000), [features]);
  const points = useMemo(() => markers.map((marker) => {
    const feature = matchingFeature(marker, renderedFeatures, level);
    const center = featureCenter(feature || {}) || [marker.longitude, marker.latitude] as Position;
    const [x, y] = project(center);
    return { marker, x, y };
  }), [level, markers, project, renderedFeatures]);
  const activeCount = useMemo(() => renderedFeatures.reduce((count, feature) => count + (usageForFeature(feature, markers, level).ipCount ? 1 : 0), 0), [level, markers, renderedFeatures]);

  const showUsage = useCallback((usage: Usage, event: { clientX: number; clientY: number }, canDrill: boolean) => {
    const rect = mapRef.current?.getBoundingClientRect();
    if (!rect) return;
    const x = Math.max(145, Math.min(rect.width - 145, event.clientX - rect.left));
    const y = Math.max(96, Math.min(rect.height - 14, event.clientY - rect.top));
    const key = `${usage.name}:${usage.markers.map((item) => item.id).sort().join('|')}`;
    const cached = hoverCache.current.get(key);
    hoverRequestId.current += 1;
    const id = hoverRequestId.current;
    if (hoverTimer.current !== undefined) window.clearTimeout(hoverTimer.current);
    setHovered({ ...usage, x, y, key, canDrill, loading: Boolean(loadIPDetails && usage.markers.length && !cached), error: false, details: cached || [] });
    if (!loadIPDetails || !usage.markers.length || cached) return;
    hoverTimer.current = window.setTimeout(() => {
      void loadIPDetails(usage.markers).then((items) => {
        if (hoverRequestId.current !== id) return;
        const details = [...new Map(items.map((item) => [item.ip, item])).values()].slice(0, 5);
        hoverCache.current.set(key, details);
        setHovered((current) => current?.key === key ? { ...current, loading: false, details } : current);
      }).catch(() => setHovered((current) => current?.key === key ? { ...current, loading: false, error: true } : current));
    }, 280);
  }, [loadIPDetails]);

  const drill = useCallback((feature: GeoFeature, usage: Usage) => {
    if (usage.markers.length) { setSelected(usage); onMarkerSelect?.(usage.markers); }
    if (level !== 'adm1' || loading) return;
    setBreadcrumbs([{ name: country.label }, { name: featureName(feature), feature }]);
    load('adm2', feature);
  }, [country.label, level, load, loading, onMarkerSelect]);

  const showMarker = useCallback((marker: GeoGlobeMarker) => { setSelected({ name: marker.label, markers: [marker], locationCount: 1, ipCount: marker.count }); onMarkerSelect?.([marker]); }, [onMarkerSelect]);
  const svgPoint = useCallback((clientX: number, clientY: number): Position => { const rect = svgRef.current?.getBoundingClientRect(); return rect ? [((clientX - rect.left) / rect.width) * WIDTH, ((clientY - rect.top) / rect.height) * HEIGHT] : [WIDTH / 2, HEIGHT / 2]; }, []);
  const constrain = useCallback((next: Viewport): Viewport => ({ scale: Math.max(MIN_SCALE, Math.min(MAX_SCALE, next.scale)), translateX: Math.max(-WIDTH * 2, Math.min(WIDTH * 2, next.translateX)), translateY: Math.max(-HEIGHT * 2, Math.min(HEIGHT * 2, next.translateY)) }), []);
  const zoom = useCallback((factor: number, clientX?: number, clientY?: number) => { const [x, y] = clientX === undefined || clientY === undefined ? [WIDTH / 2, HEIGHT / 2] : svgPoint(clientX, clientY); setViewport((current) => { const scale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, current.scale * factor)); const ratio = scale / current.scale; return constrain({ scale, translateX: x - (x - current.translateX) * ratio, translateY: y - (y - current.translateY) * ratio }); }); }, [constrain, svgPoint]);
  const onPointerDown = (event: ReactPointerEvent<SVGSVGElement>) => { if (event.button !== 0 || (event.target as Element).closest('.country-map-region,.country-map-marker')) return; event.currentTarget.setPointerCapture(event.pointerId); dragRef.current = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, viewport }; };
  const onPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => { const drag = dragRef.current; const rect = svgRef.current?.getBoundingClientRect(); if (!drag || !rect || drag.pointerId !== event.pointerId) return; setViewport(constrain({ scale: drag.viewport.scale, translateX: drag.viewport.translateX + ((event.clientX - drag.clientX) / rect.width) * WIDTH, translateY: drag.viewport.translateY + ((event.clientY - drag.clientY) / rect.height) * HEIGHT })); };
  const finishDrag = () => { dragRef.current = undefined; };
  const back = () => { if (level === 'adm2') { setBreadcrumbs([]); load('adm1'); return; } onBackToWorld(); };
  const tip = hovered;

  return <div ref={mapRef} className="country-usage-map relative isolate h-[320px] overflow-hidden rounded-[1.35rem] border" aria-label={`${country.label}行政区使用分布图`}>
    <div className="china-map-grid pointer-events-none absolute inset-0 opacity-55" />
    <svg ref={svgRef} className="relative z-[1] h-full w-full touch-none select-none cursor-grab active:cursor-grabbing" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" tabIndex={0} aria-label={`${country.label}行政区地图，支持下钻、拖动平移和缩放`} onWheel={(event: ReactWheelEvent<SVGSVGElement>) => { event.preventDefault(); zoom(event.deltaY < 0 ? 1.2 : 1 / 1.2, event.clientX, event.clientY); }} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={finishDrag} onPointerCancel={finishDrag}>
      <defs><filter id="country-map-marker-glow" x="-200%" y="-200%" width="400%" height="400%"><feGaussianBlur stdDeviation="9" /></filter></defs>
      <g transform={`translate(${viewport.translateX} ${viewport.translateY}) scale(${viewport.scale})`}>
        {renderedFeatures.map((feature, index) => { const usage = usageForFeature(feature, markers, level); const canDrill = level === 'adm1'; const label = `${usage.name}，${usage.ipCount} 个去重公网 IP${canDrill ? '，点击下钻至二级行政区' : '，点击查看该区域明细'}`; return <path key={`${featureName(feature, index)}-${index}`} className={cn('country-map-region cursor-pointer transition-colors duration-200', usage.ipCount ? 'fill-primary/25 stroke-primary/75 hover:fill-primary/45' : 'fill-muted/60 stroke-border hover:fill-muted')} strokeWidth="1.25" d={pathFor(feature, project)} role="button" tabIndex={0} aria-label={label} onPointerMove={(event) => showUsage(usage, event, canDrill)} onPointerLeave={() => { hoverRequestId.current += 1; setHovered(undefined); }} onClick={() => drill(feature, usage)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); drill(feature, usage); } }}><title>{`${usage.name} · ${usage.ipCount} 个去重公网 IP`}</title></path>; })}
        {points.map(({ marker, x, y }) => <g key={marker.id} className="country-map-marker cursor-pointer" transform={`translate(${x} ${y})`} role="button" tabIndex={0} aria-label={`${marker.label}，${marker.count} 个去重公网 IP，点击查看明细`} onPointerMove={(event) => showUsage({ name: marker.label, markers: [marker], locationCount: 1, ipCount: marker.count }, event, level === 'adm1')} onPointerLeave={() => { hoverRequestId.current += 1; setHovered(undefined); }} onClick={(event) => { event.stopPropagation(); showMarker(marker); }} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); showMarker(marker); } }}><circle r="14" className="fill-primary/25" filter="url(#country-map-marker-glow)" /><circle r="7" className="fill-primary stroke-background" strokeWidth="2" /><text y="4" textAnchor="middle" className="pointer-events-none fill-primary-foreground text-[9px] font-bold">{marker.count}</text></g>)}
      </g>
    </svg>
    <div className="china-map-control pointer-events-none absolute left-3 top-3 flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur"><MapPinned className="h-3.5 w-3.5 text-primary" />{country.label} · {level === 'adm1' ? '一级行政区' : '二级行政区'}</div>
    <div className="china-map-control pointer-events-none absolute right-3 top-3 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">{loading ? '正在加载边界…' : `${points.length} 个使用位置 · ${activeCount} 个活跃区域`}</div>
    <div className="absolute left-3 top-[3.35rem] z-20 flex max-w-[calc(100%-9.2rem)] items-center gap-1 overflow-x-auto rounded-lg border border-border bg-card/86 p-1 shadow-sm backdrop-blur"><button type="button" onClick={back} className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground" title={level === 'adm2' ? '返回国家一级行政区' : '返回全球'}><ChevronLeft className="h-3.5 w-3.5" /></button><button type="button" onClick={onBackToWorld} className="shrink-0 rounded-md px-2 py-1 text-[11px] font-medium text-primary hover:bg-muted">全球</button>{breadcrumbs.map((node, index) => <span key={`${node.name}-${index}`} className="inline-flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground"><span>/</span><button type="button" className="rounded-md px-1.5 py-1 hover:bg-muted hover:text-foreground" onClick={() => index === 0 ? load('adm1') : node.feature && load('adm2', node.feature)}>{node.name}</button></span>)}</div>
    <div className="absolute right-3 top-[3.35rem] z-20 flex overflow-hidden rounded-lg border border-border bg-card/86 shadow-sm backdrop-blur"><button type="button" className="inline-flex h-8 w-8 items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground" onClick={() => zoom(1.25)} title="放大地图"><ZoomIn className="h-3.5 w-3.5" /></button><button type="button" className="inline-flex h-8 w-8 items-center justify-center border-x border-border text-muted-foreground hover:bg-muted hover:text-foreground" onClick={() => zoom(1 / 1.25)} title="缩小地图"><ZoomOut className="h-3.5 w-3.5" /></button><button type="button" className="inline-flex h-8 w-8 items-center justify-center text-muted-foreground hover:bg-muted hover:text-foreground" onClick={() => setViewport(INITIAL_VIEWPORT)} title="复位地图"><RotateCcw className="h-3.5 w-3.5" /></button></div>
    {tip && <div className="china-map-control pointer-events-none absolute z-30 w-[276px] -translate-x-1/2 -translate-y-[calc(100%+12px)] rounded-xl border px-3 py-2.5 text-xs text-foreground shadow-xl backdrop-blur" style={{ left: tip.x, top: tip.y }} role="tooltip"><p className="font-semibold tracking-tight">{tip.name}</p>{tip.ipCount ? <><p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{tip.locationCount} 个使用位置 · {tip.ipCount} 个去重公网 IP</p>{tip.loading ? <p className="mt-1 text-[10px] text-muted-foreground">正在读取去重后的公网 IP 明细…</p> : tip.details.length ? <div className="mt-1.5 rounded-lg border border-border/80 bg-background/45 px-2 py-1.5"><p className="text-[10px] font-medium text-muted-foreground">去重公网 IP（展示前 {tip.details.length} 条）</p><ul className="mt-1 space-y-0.5 font-mono text-[10px] leading-4 text-primary">{tip.details.map((item) => <li key={item.ip} className="truncate">{item.ip}{item.city ? ` · ${item.city}` : ''}</li>)}</ul></div> : tip.error ? <p className="mt-1 text-[10px] text-muted-foreground">IP 明细暂时无法读取，点击节点可重试。</p> : null}<p className="mt-1 text-[10px] text-primary">{tip.canDrill ? '点击区域可下钻至二级行政区，并查看完整 IP 明细' : '点击节点可查看完整去重 IP 明细'}</p></> : <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">当前暂无已定位的使用记录{tip.canDrill ? '，仍可点击下钻查看' : ''}</p>}</div>}
    {selected ? <div className="china-map-control pointer-events-none absolute bottom-3 left-3 max-w-[min(72%,310px)] rounded-xl border px-3 py-2 text-xs text-foreground shadow-lg backdrop-blur"><p className="font-semibold">{selected.name}</p><p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{selected.ipCount} 个去重公网 IP</p></div> : loadError ? <div className="china-map-control pointer-events-none absolute inset-x-8 bottom-3 rounded-xl border px-4 py-2.5 text-center text-sm text-muted-foreground shadow-sm backdrop-blur">{loadError}。仍可返回全球查看此国家的 IP 明细。</div> : !loading && !points.length ? <div className="china-map-control pointer-events-none absolute inset-x-8 bottom-3 rounded-xl border px-4 py-2.5 text-center text-sm text-muted-foreground shadow-sm backdrop-blur">{emptyMessage}</div> : <div className="china-map-control pointer-events-none absolute bottom-3 left-3 inline-flex max-w-[calc(100%-1.5rem)] items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur"><MousePointer2 className="h-3.5 w-3.5 shrink-0 text-primary" />{level === 'adm1' ? '点击一级行政区继续下钻；点击位置节点查看 IP 明细' : '二级行政区边界辅助查看；点击位置节点查看 IP 明细'}</div>}
  </div>;
}
