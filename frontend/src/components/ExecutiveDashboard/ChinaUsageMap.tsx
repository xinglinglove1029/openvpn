import { ChevronLeft, MapPinned, MousePointer2, RotateCcw, ZoomIn, ZoomOut } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent, type WheelEvent as ReactWheelEvent } from 'react';
import { cn } from '@/lib/utils';
import type { DashboardGeoIPDetail } from '@/types';
import type { GeoGlobeMarker } from './InteractiveGeoGlobe';

type Position = [number, number];
type ChinaMapLevel = 'province' | 'city' | 'district';
type GeoFeature = {
  properties?: {
    name?: string;
    adcode?: number | string;
    level?: string;
    center?: Position;
    centroid?: Position;
    childrenNum?: number;
  };
  geometry?: { type?: 'Polygon' | 'MultiPolygon'; coordinates?: unknown };
};
type GeoFeatureCollection = { features?: GeoFeature[] };
type MapPoint = {
  id: string;
  label: string;
  count: number;
  longitude: number;
  latitude: number;
  labels: string[];
  markers: GeoGlobeMarker[];
  drillFeature?: GeoFeature;
};
type RegionUsage = {
  name: string;
  locationCount: number;
  ipCount: number;
  labels: string[];
  markers: GeoGlobeMarker[];
};
type HoveredRegion = RegionUsage & {
  x: number;
  y: number;
  key: string;
  canDrillDown: boolean;
  ipDetails: DashboardGeoIPDetail[];
  detailsLoading: boolean;
  detailsError: boolean;
};
type ChinaUsageMapProps = {
  markers: GeoGlobeMarker[];
  emptyMessage: string;
  onMarkerSelect?: (markers: GeoGlobeMarker[]) => void;
  loadIPDetails?: (markers: GeoGlobeMarker[]) => Promise<DashboardGeoIPDetail[]>;
};
type DrilldownNode = {
  name: string;
  adcode: string;
  level: ChinaMapLevel;
  center?: Position;
};
type MapViewport = {
  scale: number;
  translateX: number;
  translateY: number;
};
type DragState = {
  pointerId: number;
  clientX: number;
  clientY: number;
  viewport: MapViewport;
  moved: boolean;
};

const MAP_WIDTH = 1000;
const MAP_HEIGHT = 680;
const MAP_PADDING = 32;
const MIN_SCALE = 1;
const MAX_SCALE = 4;
const CHINA_MAP_URL = `${import.meta.env.BASE_URL}maps/china-provinces.geo.json`;
const ADMIN_BOUNDARY_URL = 'https://geo.datav.aliyun.com/areas_v3/bound';
const INITIAL_VIEWPORT: MapViewport = { scale: 1, translateX: 0, translateY: 0 };

function mercatorY(latitude: number) {
  const clamped = Math.max(-85, Math.min(85, latitude));
  const radians = (clamped * Math.PI) / 180;
  return Math.log(Math.tan(Math.PI / 4 + radians / 2));
}

function collectPositions(value: unknown, output: Position[] = []): Position[] {
  if (!Array.isArray(value)) return output;
  if (value.length >= 2 && typeof value[0] === 'number' && typeof value[1] === 'number') {
    output.push([value[0], value[1]]);
    return output;
  }
  value.forEach((item) => collectPositions(item, output));
  return output;
}

function pathForGeometry(geometry: GeoFeature['geometry'], project: (position: Position) => Position) {
  if (!geometry?.coordinates) return '';
  const polygons = geometry.type === 'Polygon' ? [geometry.coordinates] : geometry.type === 'MultiPolygon' ? geometry.coordinates : [];
  if (!Array.isArray(polygons)) return '';

  return polygons
    .flatMap((polygon) => {
      if (!Array.isArray(polygon)) return [];
      return polygon.map((ring) => {
        if (!Array.isArray(ring) || !ring.length) return '';
        return ring
          .map((coordinate, index) => {
            if (!Array.isArray(coordinate) || typeof coordinate[0] !== 'number' || typeof coordinate[1] !== 'number') return '';
            const [x, y] = project([coordinate[0], coordinate[1]]);
            return `${index === 0 ? 'M' : 'L'}${x.toFixed(2)},${y.toFixed(2)}`;
          })
          .join(' ') + ' Z';
      });
    })
    .join(' ');
}

function normaliseRegionName(value: string) {
  return value
    .replace(/特别行政区|维吾尔自治区|壮族自治区|回族自治区|自治区|省|市|地区|盟|自治州|区|县/g, '')
    .trim();
}

function featureCenter(feature: GeoFeature): Position | undefined {
  const configured = feature.properties?.centroid || feature.properties?.center;
  if (configured && configured.length >= 2 && Number.isFinite(configured[0]) && Number.isFinite(configured[1])) return configured;

  const positions = collectPositions(feature.geometry?.coordinates);
  if (!positions.length) return undefined;
  const longitudes = positions.map(([longitude]) => longitude);
  const latitudes = positions.map(([, latitude]) => latitude);
  return [
    (Math.min(...longitudes) + Math.max(...longitudes)) / 2,
    (Math.min(...latitudes) + Math.max(...latitudes)) / 2,
  ];
}

function mapLevelForFeatures(features: GeoFeature[], fallback: ChinaMapLevel): ChinaMapLevel {
  const level = features.find((feature) => feature.properties?.level)?.properties?.level;
  return level === 'province' || level === 'city' || level === 'district' ? level : fallback;
}

function mapLevelLabel(level: ChinaMapLevel) {
  if (level === 'city') return '地级市';
  if (level === 'district') return '区县';
  return '省级行政区';
}

function mapLevelChildLabel(level: ChinaMapLevel) {
  if (level === 'province') return '地级市';
  if (level === 'city') return '区县';
  return '';
}

function constrainViewport(scale: number, translateX: number, translateY: number): MapViewport {
  const nextScale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, scale));
  return {
    scale: nextScale,
    translateX: Math.min(0, Math.max(MAP_WIDTH * (1 - nextScale), translateX)),
    translateY: Math.min(0, Math.max(MAP_HEIGHT * (1 - nextScale), translateY)),
  };
}

function matchesNode(marker: GeoGlobeMarker, node: DrilldownNode) {
  const expected = normaliseRegionName(node.name);
  if (!expected) return true;
  const value = node.level === 'province' ? marker.point.province : marker.point.city;
  return normaliseRegionName(String(value || '')) === expected;
}

export function ChinaUsageMap({ markers, emptyMessage, onMarkerSelect, loadIPDetails }: ChinaUsageMapProps) {
  const [features, setFeatures] = useState<GeoFeature[]>([]);
  const [mapLevel, setMapLevel] = useState<ChinaMapLevel>('province');
  const [breadcrumbs, setBreadcrumbs] = useState<DrilldownNode[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<MapPoint | null>(null);
  const [hoveredRegion, setHoveredRegion] = useState<HoveredRegion | null>(null);
  const [viewport, setViewport] = useState<MapViewport>(INITIAL_VIEWPORT);
  const [dragging, setDragging] = useState(false);
  const mapRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const requestIdRef = useRef(0);
  const dragRef = useRef<DragState | null>(null);
  const suppressMarkerClickRef = useRef(false);
  const hoverRequestIdRef = useRef(0);
  const hoverTimerRef = useRef<number | null>(null);
  const hoverKeyRef = useRef('');
  const hoverDetailsCacheRef = useRef(new Map<string, DashboardGeoIPDetail[]>());

  const loadMap = useCallback(async (url: string, nextBreadcrumbs: DrilldownNode[], fallbackLevel: ChinaMapLevel) => {
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setLoadError(null);
    hoverRequestIdRef.current += 1;
    hoverKeyRef.current = '';
    if (hoverTimerRef.current !== null) {
      window.clearTimeout(hoverTimerRef.current);
      hoverTimerRef.current = null;
    }
    setHoveredRegion(null);
    setSelected(null);
    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error(`Unable to load China map (${response.status})`);
      const payload = await response.json() as GeoFeatureCollection;
      const nextFeatures = Array.isArray(payload.features) ? payload.features : [];
      if (!nextFeatures.length) throw new Error('China map has no administrative boundaries');
      if (requestId !== requestIdRef.current) return;
      setFeatures(nextFeatures);
      setMapLevel(mapLevelForFeatures(nextFeatures, fallbackLevel));
      setBreadcrumbs(nextBreadcrumbs);
      setViewport(INITIAL_VIEWPORT);
    } catch (error) {
      console.error('Unable to load China administrative map.', error);
      if (requestId === requestIdRef.current) {
        setLoadError(nextBreadcrumbs.length ? '下级行政区边界加载失败，请稍后重试。' : '中国地图资源加载失败，刷新页面后重试。');
      }
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadMap(CHINA_MAP_URL, [], 'province');
  }, [loadMap]);

  const scopedMarkers = useMemo(
    () => markers.filter((marker) => breadcrumbs.every((node) => matchesNode(marker, node))),
    [breadcrumbs, markers]
  );

  const bounds = useMemo(() => {
    const positions = features.flatMap((feature) => collectPositions(feature.geometry?.coordinates));
    if (!positions.length) return { minLongitude: 73, maxLongitude: 135, minLatitude: 18, maxLatitude: 55 };
    const longitudes = positions.map(([longitude]) => longitude);
    const latitudes = positions.map(([, latitude]) => latitude);
    return {
      minLongitude: Math.min(...longitudes),
      maxLongitude: Math.max(...longitudes),
      minLatitude: Math.min(...latitudes),
      maxLatitude: Math.max(...latitudes),
    };
  }, [features]);

  const project = useCallback((position: Position): Position => {
    const [longitude, latitude] = position;
    const minY = mercatorY(bounds.minLatitude);
    const maxY = mercatorY(bounds.maxLatitude);
    const usableWidth = MAP_WIDTH - MAP_PADDING * 2;
    const usableHeight = MAP_HEIGHT - MAP_PADDING * 2;
    const x = MAP_PADDING + ((longitude - bounds.minLongitude) / (bounds.maxLongitude - bounds.minLongitude)) * usableWidth;
    const y = MAP_PADDING + ((maxY - mercatorY(latitude)) / (maxY - minY)) * usableHeight;
    return [x, y];
  }, [bounds]);

  const regionFeatureForMarker = useCallback((marker: GeoGlobeMarker) => {
    if (mapLevel === 'district') return undefined;
    const desired = normaliseRegionName(String(mapLevel === 'province' ? marker.point.province || marker.point.label : marker.point.city || marker.point.label));
    if (!desired) return undefined;
    return features.find((feature) => normaliseRegionName(String(feature.properties?.name || '')) === desired);
  }, [features, mapLevel]);

  const points = useMemo<MapPoint[]>(() => {
    const grouped = new Map<string, MapPoint>();
    scopedMarkers.forEach((marker) => {
      const feature = regionFeatureForMarker(marker);
      const parentCenter = breadcrumbs.at(-1)?.center;
      const center = mapLevel === 'district' ? parentCenter : featureCenter(feature || {});
      const [longitude, latitude] = center || [marker.longitude, marker.latitude];
      const label = mapLevel === 'district'
        ? `${breadcrumbs.at(-1)?.name || marker.label}（市级定位）`
        : feature?.properties?.name || marker.point.city || marker.point.province || marker.label;
      const key = mapLevel === 'district' ? 'city-aggregate' : `${longitude.toFixed(3)}:${latitude.toFixed(3)}`;
      const current = grouped.get(key);
      if (current) {
        current.count += marker.count;
        if (!current.labels.includes(marker.label)) current.labels.push(marker.label);
        current.markers.push(marker);
        return;
      }
      grouped.set(key, {
        id: key,
        label,
        count: marker.count,
        longitude,
        latitude,
        labels: [marker.label],
        markers: [marker],
        drillFeature: feature,
      });
    });
    return [...grouped.values()].sort((a, b) => b.count - a.count);
  }, [breadcrumbs, mapLevel, regionFeatureForMarker, scopedMarkers]);

  const regionUsage = useMemo(() => {
    const usage = new Map<string, RegionUsage>();
    features.forEach((feature, index) => {
      const name = feature.properties?.name || `区域 ${index + 1}`;
      usage.set(normaliseRegionName(name), { name, locationCount: 0, ipCount: 0, labels: [], markers: [] });
    });
    if (mapLevel === 'district') return usage;

    scopedMarkers.forEach((marker) => {
      const candidate = mapLevel === 'province' ? marker.point.province : marker.point.city;
      const regionName = normaliseRegionName(String(candidate || ''));
      const current = usage.get(regionName);
      if (!current) return;
      current.locationCount += 1;
      current.ipCount += marker.count;
      if (!current.labels.includes(marker.label)) current.labels.push(marker.label);
      current.markers.push(marker);
    });
    return usage;
  }, [features, mapLevel, scopedMarkers]);

  const activeRegions = useMemo(
    () => new Set([...regionUsage.entries()].filter(([, usage]) => usage.ipCount > 0).map(([regionName]) => regionName)),
    [regionUsage]
  );
  const maxCount = Math.max(...points.map((point) => point.count), 1);
  const activeRegionCount = activeRegions.size;

  const clearHover = useCallback(() => {
    hoverRequestIdRef.current += 1;
    hoverKeyRef.current = '';
    if (hoverTimerRef.current !== null) {
      window.clearTimeout(hoverTimerRef.current);
      hoverTimerRef.current = null;
    }
    setHoveredRegion(null);
  }, []);

  useEffect(() => {
    setSelected(null);
    hoverDetailsCacheRef.current.clear();
    clearHover();
  }, [clearHover, markers]);

  const showRegionTooltip = useCallback((event: ReactPointerEvent<SVGElement>, usage: RegionUsage, canDrillDown = false) => {
    const container = mapRef.current;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const tooltipWidth = 276;
    const x = Math.min(Math.max(event.clientX - rect.left, tooltipWidth / 2 + 12), rect.width - tooltipWidth / 2 - 12);
    const y = Math.min(Math.max(event.clientY - rect.top, 98), rect.height - 14);
    const markerKey = usage.markers.map((marker) => marker.id).sort().join('|');
    const key = `${usage.name}:${markerKey}`;
    const cached = hoverDetailsCacheRef.current.get(key);

    if (hoverKeyRef.current === key) {
      setHoveredRegion((current) => current ? { ...current, x, y } : current);
      return;
    }

    hoverRequestIdRef.current += 1;
    const requestId = hoverRequestIdRef.current;
    hoverKeyRef.current = key;
    if (hoverTimerRef.current !== null) window.clearTimeout(hoverTimerRef.current);
    setHoveredRegion({
      ...usage,
      x,
      y,
      key,
      canDrillDown,
      ipDetails: cached || [],
      detailsLoading: Boolean(loadIPDetails && usage.markers.length && !cached),
      detailsError: false,
    });
    if (!loadIPDetails || !usage.markers.length || cached) return;

    hoverTimerRef.current = window.setTimeout(() => {
      hoverTimerRef.current = null;
      void loadIPDetails(usage.markers)
        .then((items) => {
          if (requestId !== hoverRequestIdRef.current || hoverKeyRef.current !== key) return;
          const unique = new Map(items.map((item) => [item.ip, item]));
          const details = [...unique.values()].sort((a, b) => a.ip.localeCompare(b.ip, undefined, { numeric: true })).slice(0, 5);
          hoverDetailsCacheRef.current.set(key, details);
          setHoveredRegion((current) => current?.key === key ? { ...current, ipDetails: details, detailsLoading: false } : current);
        })
        .catch(() => {
          if (requestId !== hoverRequestIdRef.current || hoverKeyRef.current !== key) return;
          setHoveredRegion((current) => current?.key === key ? { ...current, detailsLoading: false, detailsError: true } : current);
        });
    }, 350);
  }, [loadIPDetails]);

  useEffect(() => () => {
    if (hoverTimerRef.current !== null) window.clearTimeout(hoverTimerRef.current);
  }, []);

  const svgPoint = useCallback((clientX: number, clientY: number): Position => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return [MAP_WIDTH / 2, MAP_HEIGHT / 2];
    return [
      ((clientX - rect.left) / rect.width) * MAP_WIDTH,
      ((clientY - rect.top) / rect.height) * MAP_HEIGHT,
    ];
  }, []);

  const zoomAt = useCallback((factor: number, clientX?: number, clientY?: number) => {
    const [anchorX, anchorY] = clientX === undefined || clientY === undefined ? [MAP_WIDTH / 2, MAP_HEIGHT / 2] : svgPoint(clientX, clientY);
    setViewport((current) => {
      const scale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, current.scale * factor));
      if (scale === current.scale) return current;
      const ratio = scale / current.scale;
      return constrainViewport(
        scale,
        anchorX - (anchorX - current.translateX) * ratio,
        anchorY - (anchorY - current.translateY) * ratio
      );
    });
  }, [svgPoint]);

  const resetViewport = useCallback(() => setViewport(INITIAL_VIEWPORT), []);

  const onWheel = (event: ReactWheelEvent<SVGSVGElement>) => {
    event.preventDefault();
    zoomAt(event.deltaY < 0 ? 1.2 : 1 / 1.2, event.clientX, event.clientY);
  };

  const onPointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { pointerId: event.pointerId, clientX: event.clientX, clientY: event.clientY, viewport, moved: false };
    setDragging(true);
  };

  const onPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    const deltaX = ((event.clientX - drag.clientX) / rect.width) * MAP_WIDTH;
    const deltaY = ((event.clientY - drag.clientY) / rect.height) * MAP_HEIGHT;
    if (Math.abs(deltaX) > 3 || Math.abs(deltaY) > 3) drag.moved = true;
    setViewport(constrainViewport(drag.viewport.scale, drag.viewport.translateX + deltaX, drag.viewport.translateY + deltaY));
  };

  const finishDrag = (event: ReactPointerEvent<SVGSVGElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    dragRef.current = null;
    setDragging(false);
    if (drag.moved) {
      suppressMarkerClickRef.current = true;
      window.setTimeout(() => { suppressMarkerClickRef.current = false; }, 0);
    }
  };

  const drillDown = (feature: GeoFeature) => {
    const name = feature.properties?.name || '当前区域';
    const adcode = String(feature.properties?.adcode || '').trim();
    const featureLevel = mapLevelForFeatures([feature], mapLevel);
    // Some direct-administered municipalities omit or under-report childrenNum in the boundary data.
    // As long as an adcode exists, attempt to load its child boundary and report a real loading error if absent.
    if (!adcode || featureLevel === 'district' || loading) return;
    const center = featureCenter(feature);
    const nextBreadcrumbs = [...breadcrumbs, { name, adcode, level: featureLevel, center }];
    void loadMap(`${ADMIN_BOUNDARY_URL}/${adcode}_full.json`, nextBreadcrumbs, featureLevel === 'province' ? 'city' : 'district');
  };

  const navigateToBreadcrumb = (index: number) => {
    if (index < 0) {
      void loadMap(CHINA_MAP_URL, [], 'province');
      return;
    }
    const node = breadcrumbs[index];
    if (!node) return;
    void loadMap(`${ADMIN_BOUNDARY_URL}/${node.adcode}_full.json`, breadcrumbs.slice(0, index + 1), node.level === 'province' ? 'city' : 'district');
  };

  const selectedText = selected
    ? `${selected.labels.slice(0, 3).join('、')}${selected.labels.length > 3 ? ` 等 ${selected.labels.length} 个区域` : ''} · ${selected.count} 个去重公网 IP`
    : '';
  const canGoBack = breadcrumbs.length > 0;
  const districtPrecisionNotice = mapLevel === 'district';

  return (
    <div ref={mapRef} className="china-usage-map relative isolate h-[320px] overflow-hidden rounded-[1.35rem] border" aria-label="中国行政区使用分布图">
      <div className="china-map-grid pointer-events-none absolute inset-0 opacity-55" />
      <svg
        ref={svgRef}
        className={cn('relative z-[1] h-full w-full touch-none select-none', dragging ? 'cursor-grabbing' : 'cursor-grab')}
        viewBox={`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`}
        role="img"
        tabIndex={0}
        aria-label="中国地图，支持省、市、区县下钻、拖动平移和滚轮缩放"
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={finishDrag}
        onPointerCancel={finishDrag}
        onKeyDown={(event) => {
          if (event.key === '+' || event.key === '=') { event.preventDefault(); zoomAt(1.2); }
          if (event.key === '-') { event.preventDefault(); zoomAt(1 / 1.2); }
          if (event.key === '0') { event.preventDefault(); resetViewport(); }
        }}
      >
        <defs>
          <filter id="china-map-marker-glow" x="-200%" y="-200%" width="400%" height="400%">
            <feGaussianBlur stdDeviation="9" />
          </filter>
        </defs>
        <g transform={`translate(${viewport.translateX} ${viewport.translateY}) scale(${viewport.scale})`}>
          <g className="china-map-provinces">
            {features.map((feature, index) => {
              const name = feature.properties?.name || `区域 ${index + 1}`;
              const usage = regionUsage.get(normaliseRegionName(name)) || { name, locationCount: 0, ipCount: 0, labels: [], markers: [] };
              const active = activeRegions.has(normaliseRegionName(name));
              const canDrillDown = mapLevel !== 'district' && Boolean(feature.properties?.adcode);
              const tooltipText = districtPrecisionNotice
                ? `${name}：当前数据仅精确到市级，区县边界用于辅助查看`
                : usage.ipCount
                  ? `${name}：${usage.locationCount} 个使用位置，${usage.ipCount} 个去重公网 IP${canDrillDown ? `，点击查看${mapLevelChildLabel(mapLevel)}` : ''}`
                  : `${name}：当前暂无已定位的使用记录${canDrillDown ? `，点击查看${mapLevelChildLabel(mapLevel)}` : ''}`;
              return (
                <path
                  key={`${name}-${index}`}
                  d={pathForGeometry(feature.geometry, project)}
                  className={cn('china-map-region', active && 'china-map-region-active', canDrillDown ? 'cursor-zoom-in' : 'cursor-help')}
                  fillRule="evenodd"
                  role={canDrillDown ? 'button' : undefined}
                  tabIndex={canDrillDown ? 0 : undefined}
                  onClick={() => { if (canDrillDown) drillDown(feature); }}
                  onKeyDown={(event) => {
                    if (!canDrillDown || (event.key !== 'Enter' && event.key !== ' ')) return;
                    event.preventDefault();
                    drillDown(feature);
                  }}
                  onPointerEnter={(event) => showRegionTooltip(event, usage, canDrillDown)}
                  onPointerMove={(event) => showRegionTooltip(event, usage, canDrillDown)}
                  onPointerLeave={clearHover}
                >
                  <title>{tooltipText}</title>
                </path>
              );
            })}
          </g>
          <g className="pointer-events-none select-none" aria-label={`${mapLevelLabel(mapLevel)}名称`}>
            {features.map((feature, index) => {
              const name = feature.properties?.name || `区域 ${index + 1}`;
              const center = featureCenter(feature);
              if (!center) return null;
              const [x, y] = project(center);
              const active = activeRegions.has(normaliseRegionName(name));
              return (
                <text
                  key={`region-label-${name}-${index}`}
                  x={x}
                  y={y}
                  textAnchor="middle"
                  className={cn('fill-muted-foreground font-semibold', mapLevel === 'district' ? 'text-[10px]' : 'text-[12px]', active && 'fill-primary')}
                  stroke="color-mix(in srgb, var(--background) 86%, transparent)"
                  strokeWidth="4"
                  paintOrder="stroke"
                >
                  {name}
                </text>
              );
            })}
          </g>
          <g aria-label="使用位置">
            {points.map((point) => {
              const [x, y] = project([point.longitude, point.latitude]);
              const radius = 8 + Math.max(0, (point.count / maxCount) * 10);
              const style = { '--map-marker-radius': `${radius}px` } as CSSProperties;
              return (
                <g
                  key={point.id}
                  className="china-map-marker cursor-pointer"
                  role="button"
                  tabIndex={0}
                  aria-label={`${point.label}，${point.count} 个去重公网 IP`}
                  onClick={() => {
                    if (suppressMarkerClickRef.current) return;
                    setSelected(point);
                    onMarkerSelect?.(point.markers);
                    if (point.drillFeature) drillDown(point.drillFeature);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      setSelected(point);
                      onMarkerSelect?.(point.markers);
                      if (point.drillFeature) drillDown(point.drillFeature);
                    }
                  }}
                  onPointerEnter={(event) => showRegionTooltip(event, { name: point.label, locationCount: point.markers.length, ipCount: point.count, labels: point.labels, markers: point.markers }, Boolean(point.drillFeature && mapLevel !== 'district'))}
                  onPointerMove={(event) => showRegionTooltip(event, { name: point.label, locationCount: point.markers.length, ipCount: point.count, labels: point.labels, markers: point.markers }, Boolean(point.drillFeature && mapLevel !== 'district'))}
                  onPointerLeave={clearHover}
                >
                  <title>{`${point.label}：${point.count} 个去重公网 IP`}</title>
                  <circle cx={x} cy={y} r={radius * 1.85} className="china-map-marker-glow" style={style} />
                  <circle cx={x} cy={y} r={radius} className="china-map-marker-ring" />
                  <circle cx={x} cy={y} r={Math.max(4, radius * 0.36)} className="china-map-marker-core" />
                  <text
                    x={x}
                    y={y - radius - 10}
                    textAnchor="middle"
                    className="pointer-events-none select-none fill-foreground text-[11px] font-semibold"
                    stroke="color-mix(in srgb, var(--background) 88%, transparent)"
                    strokeWidth="4"
                    paintOrder="stroke"
                  >
                    {point.label}
                  </text>
                  <text
                    x={x}
                    y={y + 4}
                    textAnchor="middle"
                    className="pointer-events-none select-none fill-primary-foreground text-[12px] font-bold"
                    stroke="rgba(2, 6, 23, 0.68)"
                    strokeWidth="3"
                    paintOrder="stroke"
                  >
                    {point.count}
                  </text>
                </g>
              );
            })}
          </g>
        </g>
      </svg>

      <div className="china-map-control pointer-events-none absolute left-3 top-3 flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur">
        <MapPinned className="h-3.5 w-3.5 text-primary" />
        {mapLevelLabel(mapLevel)} · 使用位置
      </div>
      <div className="china-map-control pointer-events-none absolute right-3 top-3 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
        {loading ? '正在加载边界…' : points.length ? `${points.length} 个使用位置 · ${activeRegionCount} 个活跃区域` : '等待位置数据'}
      </div>

      <div className="absolute left-3 top-[3.35rem] z-20 flex max-w-[calc(100%-9.2rem)] items-center gap-1 overflow-x-auto rounded-lg border border-border bg-card/86 p-1 shadow-sm backdrop-blur">
        {canGoBack ? (
          <button type="button" className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={() => navigateToBreadcrumb(breadcrumbs.length - 2)} title="返回上级" aria-label="返回上级">
            <ChevronLeft className="h-3.5 w-3.5" />
          </button>
        ) : null}
        <button type="button" className="shrink-0 rounded-md px-2 py-1 text-[11px] font-medium text-primary transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={() => navigateToBreadcrumb(-1)}>中国</button>
        {breadcrumbs.map((node, index) => (
          <span key={`${node.adcode}-${index}`} className="inline-flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground">
            <span>/</span>
            <button type="button" className={cn('rounded-md px-1.5 py-1 transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary', index === breadcrumbs.length - 1 && 'font-medium text-foreground')} onClick={() => navigateToBreadcrumb(index)}>{node.name}</button>
          </span>
        ))}
      </div>

      <div className="absolute right-3 top-[3.35rem] z-20 flex overflow-hidden rounded-lg border border-border bg-card/86 shadow-sm backdrop-blur">
        <button type="button" className="inline-flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={() => zoomAt(1.25)} title="放大地图" aria-label="放大地图">
          <ZoomIn className="h-3.5 w-3.5" />
        </button>
        <button type="button" className="inline-flex h-8 w-8 items-center justify-center border-x border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={() => zoomAt(1 / 1.25)} title="缩小地图" aria-label="缩小地图">
          <ZoomOut className="h-3.5 w-3.5" />
        </button>
        <button type="button" className="inline-flex h-8 w-8 items-center justify-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" onClick={resetViewport} title="复位地图" aria-label="复位地图">
          <RotateCcw className="h-3.5 w-3.5" />
        </button>
      </div>

      {hoveredRegion && (
        <div
          className="china-map-control pointer-events-none absolute z-30 w-[276px] -translate-x-1/2 -translate-y-[calc(100%+12px)] rounded-xl border px-3 py-2.5 text-xs text-foreground shadow-xl backdrop-blur"
          style={{ left: hoveredRegion.x, top: hoveredRegion.y }}
          role="tooltip"
        >
          <p className="font-semibold tracking-tight">{hoveredRegion.name}</p>
          {districtPrecisionNotice ? (
            <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">当前 IP 归属地数据精确到市级；区县边界仅用于辅助查看，不会将市级数据错误归入具体区县。</p>
          ) : hoveredRegion.ipCount ? (
            <>
              <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{hoveredRegion.locationCount} 个使用位置 · {hoveredRegion.ipCount} 个去重公网 IP</p>
              {hoveredRegion.detailsLoading ? (
                <p className="mt-1 text-[10px] text-muted-foreground">正在读取去重后的公网 IP 明细…</p>
              ) : hoveredRegion.ipDetails.length ? (
                <div className="mt-1.5 rounded-lg border border-border/80 bg-background/45 px-2 py-1.5">
                  <p className="text-[10px] font-medium text-muted-foreground">去重公网 IP（展示前 {hoveredRegion.ipDetails.length} 条）</p>
                  <ul className="mt-1 space-y-0.5 font-mono text-[10px] leading-4 text-primary">
                    {hoveredRegion.ipDetails.map((item) => <li key={item.ip} className="truncate">{item.ip}{item.city ? ` · ${item.city}` : ''}</li>)}
                  </ul>
                </div>
              ) : hoveredRegion.detailsError ? (
                <p className="mt-1 text-[10px] text-muted-foreground">IP 明细暂时无法读取，点击节点可重试查看完整列表。</p>
              ) : null}
              <p className="mt-1 text-[10px] text-primary">{hoveredRegion.canDrillDown ? `点击节点可下钻至${mapLevelChildLabel(mapLevel)}，并打开完整 IP 明细` : '点击节点可打开完整去重 IP 明细'}</p>
            </>
          ) : (
            <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">当前暂无已定位的使用记录</p>
          )}
        </div>
      )}

      {selected ? (
        <div className="china-map-control pointer-events-none absolute bottom-3 left-3 max-w-[min(72%,310px)] rounded-xl border px-3 py-2 text-xs text-foreground shadow-lg backdrop-blur">
          <p className="font-semibold">{selected.label}</p>
          <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{selectedText}</p>
        </div>
      ) : loadError ? (
        <div className="china-map-control pointer-events-none absolute inset-x-8 bottom-3 rounded-xl border px-4 py-2.5 text-center text-sm text-muted-foreground shadow-sm backdrop-blur">
          {loadError}
        </div>
      ) : points.length ? (
        <div className="china-map-control pointer-events-none absolute bottom-3 left-3 inline-flex max-w-[calc(100%-1.5rem)] items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
          <MousePointer2 className="h-3.5 w-3.5 shrink-0 text-primary" />
          {mapLevel === 'district'
            ? '区县边界辅助查看 · 拖动平移、滚轮或右上角按钮缩放 · 点击圆点查看 IP 明细'
            : `点击节点或${mapLevel === 'province' ? '省份' : '城市'}下钻至${mapLevelChildLabel(mapLevel)} · 节点悬浮可查看 IP · 拖动平移、滚轮或右上角按钮缩放`}
        </div>
      ) : (
        <div className="china-map-control pointer-events-none absolute inset-x-8 bottom-3 rounded-xl border px-4 py-2.5 text-center text-sm text-muted-foreground shadow-sm backdrop-blur">
          {emptyMessage}
        </div>
      )}
    </div>
  );
}
