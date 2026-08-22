import { MapPinned, MousePointer2 } from 'lucide-react';
import { useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from 'react';
import { cn } from '@/lib/utils';
import type { GeoGlobeMarker } from './InteractiveGeoGlobe';

type Position = [number, number];
type GeoFeature = {
  properties?: { name?: string };
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
};
type ProvinceUsage = {
  name: string;
  locationCount: number;
  ipCount: number;
  labels: string[];
};
type HoveredProvince = ProvinceUsage & {
  x: number;
  y: number;
};

const MAP_WIDTH = 1000;
const MAP_HEIGHT = 680;
const MAP_PADDING = 32;
const CHINA_MAP_URL = `${import.meta.env.BASE_URL}maps/china-provinces.geo.json`;

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
    .replace(/特别行政区|维吾尔自治区|壮族自治区|回族自治区|自治区|省|市/g, '')
    .trim();
}

function geometryLabelPosition(geometry: GeoFeature['geometry']): Position | undefined {
  const positions = collectPositions(geometry?.coordinates);
  if (!positions.length) return undefined;
  const longitudes = positions.map(([longitude]) => longitude);
  const latitudes = positions.map(([, latitude]) => latitude);
  return [
    (Math.min(...longitudes) + Math.max(...longitudes)) / 2,
    (Math.min(...latitudes) + Math.max(...latitudes)) / 2,
  ];
}

function pointLocationLabel(point: MapPoint) {
  const names = point.markers
    .flatMap((marker) => [marker.point.province, marker.point.city, marker.point.label])
    .map((value) => String(value || '').trim())
    .filter((value, index, values) => value && values.indexOf(value) === index);
  return names.slice(0, 2).join(' · ') || point.labels.slice(0, 2).join(' · ');
}

export function ChinaUsageMap({ markers, emptyMessage, onMarkerSelect }: { markers: GeoGlobeMarker[]; emptyMessage: string; onMarkerSelect?: (markers: GeoGlobeMarker[]) => void }) {
  const [features, setFeatures] = useState<GeoFeature[]>([]);
  const [loadError, setLoadError] = useState(false);
  const [selected, setSelected] = useState<MapPoint | null>(null);
  const [hoveredProvince, setHoveredProvince] = useState<HoveredProvince | null>(null);
  const mapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    setLoadError(false);
    fetch(CHINA_MAP_URL)
      .then((response) => {
        if (!response.ok) throw new Error(`Unable to load China map (${response.status})`);
        return response.json() as Promise<GeoFeatureCollection>;
      })
      .then((payload) => {
        if (!cancelled) setFeatures(Array.isArray(payload.features) ? payload.features : []);
      })
      .catch((error) => {
        console.error('Unable to load China province map.', error);
        if (!cancelled) setLoadError(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const points = useMemo<MapPoint[]>(() => {
    const grouped = new Map<string, MapPoint>();
    markers.forEach((marker) => {
      const key = `${marker.longitude.toFixed(2)}:${marker.latitude.toFixed(2)}`;
      const current = grouped.get(key);
      if (current) {
        current.count += marker.count;
        if (!current.labels.includes(marker.label)) current.labels.push(marker.label);
        current.markers.push(marker);
        return;
      }
      grouped.set(key, {
        id: key,
        label: marker.label,
        count: marker.count,
        longitude: marker.longitude,
        latitude: marker.latitude,
        labels: [marker.label],
        markers: [marker],
      });
    });
    return [...grouped.values()].sort((a, b) => b.count - a.count);
  }, [markers]);

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

  const project = (position: Position): Position => {
    const [longitude, latitude] = position;
    const minY = mercatorY(bounds.minLatitude);
    const maxY = mercatorY(bounds.maxLatitude);
    const usableWidth = MAP_WIDTH - MAP_PADDING * 2;
    const usableHeight = MAP_HEIGHT - MAP_PADDING * 2;
    const x = MAP_PADDING + ((longitude - bounds.minLongitude) / (bounds.maxLongitude - bounds.minLongitude)) * usableWidth;
    const y = MAP_PADDING + ((maxY - mercatorY(latitude)) / (maxY - minY)) * usableHeight;
    return [x, y];
  };

  const provinceUsage = useMemo(() => {
    const usage = new Map<string, ProvinceUsage>();
    features.forEach((feature, index) => {
      const name = feature.properties?.name || `区域 ${index + 1}`;
      usage.set(normaliseRegionName(name), { name, locationCount: 0, ipCount: 0, labels: [] });
    });

    markers.forEach((marker) => {
      const candidates = [marker.point.province, marker.point.city, marker.point.label]
        .filter(Boolean)
        .map((value) => normaliseRegionName(String(value)));
      const matchedKey = [...usage.keys()].find((regionName) => candidates.includes(regionName));
      if (!matchedKey) return;

      const current = usage.get(matchedKey);
      if (!current) return;
      current.locationCount += 1;
      current.ipCount += marker.count;
      if (!current.labels.includes(marker.label)) current.labels.push(marker.label);
    });

    return usage;
  }, [features, markers]);
  const activeRegions = useMemo(
    () => new Set([...provinceUsage.entries()].filter(([, usage]) => usage.ipCount > 0).map(([regionName]) => regionName)),
    [provinceUsage]
  );
  const maxCount = Math.max(...points.map((point) => point.count), 1);
  const activeRegionCount = activeRegions.size;

  useEffect(() => {
    setSelected(null);
    setHoveredProvince(null);
  }, [markers]);

  const showProvinceTooltip = (event: ReactPointerEvent<SVGPathElement>, usage: ProvinceUsage) => {
    const container = mapRef.current;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const tooltipWidth = 230;
    const x = Math.min(Math.max(event.clientX - rect.left, tooltipWidth / 2 + 12), rect.width - tooltipWidth / 2 - 12);
    const y = Math.min(Math.max(event.clientY - rect.top, 98), rect.height - 14);
    setHoveredProvince({ ...usage, x, y });
  };

  const selectedText = selected
    ? `${selected.labels.slice(0, 3).join('、')}${selected.labels.length > 3 ? ` 等 ${selected.labels.length} 个区域` : ''} · ${selected.count} 个去重公网 IP`
    : '';

  return (
    <div ref={mapRef} className="china-usage-map relative isolate h-[320px] overflow-hidden rounded-[1.35rem] border" aria-label="中国省级使用分布图">
      <div className="china-map-grid pointer-events-none absolute inset-0 opacity-55" />
      <svg className="relative z-[1] h-full w-full" viewBox={`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`} role="img" aria-label="中国地图，展示各地区的 VPN 使用来源">
        <defs>
          <filter id="china-map-marker-glow" x="-200%" y="-200%" width="400%" height="400%">
            <feGaussianBlur stdDeviation="9" />
          </filter>
        </defs>
        <g className="china-map-provinces">
          {features.map((feature, index) => {
            const name = feature.properties?.name || `区域 ${index + 1}`;
            const usage = provinceUsage.get(normaliseRegionName(name)) || { name, locationCount: 0, ipCount: 0, labels: [] };
            const active = activeRegions.has(normaliseRegionName(name));
            const tooltipText = usage.ipCount
              ? `${name}：${usage.locationCount} 个使用位置，${usage.ipCount} 个去重公网 IP`
              : `${name}：当前暂无已定位的使用记录`;
            return (
              <path
                key={`${name}-${index}`}
                d={pathForGeometry(feature.geometry, project)}
                className={cn('china-map-region cursor-help', active && 'china-map-region-active')}
                fillRule="evenodd"
                onPointerEnter={(event) => showProvinceTooltip(event, usage)}
                onPointerMove={(event) => showProvinceTooltip(event, usage)}
                onPointerLeave={() => setHoveredProvince(null)}
              >
                <title>{tooltipText}</title>
              </path>
            );
          })}
        </g>
        <g className="pointer-events-none select-none" aria-label="省级行政区名称">
          {features.map((feature, index) => {
            const name = feature.properties?.name || `区域 ${index + 1}`;
            const center = geometryLabelPosition(feature.geometry);
            if (!center) return null;
            const [x, y] = project(center);
            const active = activeRegions.has(normaliseRegionName(name));
            return (
              <text
                key={`province-label-${name}-${index}`}
                x={x}
                y={y}
                textAnchor="middle"
                className={cn('fill-muted-foreground text-[12px] font-semibold', active && 'fill-primary')}
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
                onClick={() => { setSelected(point); onMarkerSelect?.(point.markers); }}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    setSelected(point);
                    onMarkerSelect?.(point.markers);
                  }
                }}
              >
                <title>{`${point.labels.join('、')}：${point.count} 个去重公网 IP`}</title>
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
                  {pointLocationLabel(point)}
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
      </svg>

      <div className="china-map-control pointer-events-none absolute left-3 top-3 flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-[11px] font-medium text-foreground shadow-sm backdrop-blur">
        <MapPinned className="h-3.5 w-3.5 text-primary" />
        省级行政区 · 使用位置
      </div>
      <div className="china-map-control pointer-events-none absolute right-3 top-3 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
        {points.length ? `${points.length} 个使用位置 · ${activeRegionCount} 个省级区域` : '等待位置数据'}
      </div>

      {hoveredProvince && (
        <div
          className="china-map-control pointer-events-none absolute z-20 w-[230px] -translate-x-1/2 -translate-y-[calc(100%+12px)] rounded-xl border px-3 py-2.5 text-xs text-foreground shadow-xl backdrop-blur"
          style={{ left: hoveredProvince.x, top: hoveredProvince.y }}
          role="tooltip"
        >
          <p className="font-semibold tracking-tight">{hoveredProvince.name}</p>
          {hoveredProvince.ipCount ? (
            <>
              <p className="mt-0.5 text-[11px] leading-5 text-muted-foreground">{hoveredProvince.locationCount} 个使用位置 · {hoveredProvince.ipCount} 个去重公网 IP</p>
              <p className="mt-1 truncate text-[10px] text-primary">{hoveredProvince.labels.slice(0, 3).join('、')}{hoveredProvince.labels.length > 3 ? ` 等 ${hoveredProvince.labels.length} 个区域` : ''}</p>
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
      ) : points.length ? (
        <div className="china-map-control pointer-events-none absolute bottom-3 left-3 inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[11px] text-muted-foreground shadow-sm backdrop-blur">
          <MousePointer2 className="h-3.5 w-3.5 text-primary" />
          省份、位置默认显示 · 悬停省份查看统计 · 点击发光圆点查看详情
        </div>
      ) : (
        <div className="china-map-control pointer-events-none absolute inset-x-8 bottom-3 rounded-xl border px-4 py-2.5 text-center text-sm text-muted-foreground shadow-sm backdrop-blur">
          {loadError ? '中国地图资源加载失败，刷新页面后重试。' : emptyMessage}
        </div>
      )}
    </div>
  );
}
