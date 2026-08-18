import { useMemo, useState } from 'react';
import { BarChart3, TrendingUp } from 'lucide-react';
import { Button } from '@/ui/button';
import { cn } from '@/lib/utils';
import { formatBytes } from '@/lib/format';
import type { DashboardSummary } from '@/types';

type TrendMode = 'bar' | 'line';
type TrendPoint = DashboardSummary['trends'][number];

interface DashboardTrendChartProps {
  points: TrendPoint[];
}

const SVG_WIDTH = 720;
const SVG_HEIGHT = 236;
const PADDING = { top: 16, right: 14, bottom: 31, left: 35 };

function chartAxisMax(maximum: number) {
  if (maximum <= 1) return 1;
  const roughStep = maximum / 4;
  const magnitude = 10 ** Math.floor(Math.log10(roughStep));
  const normalized = roughStep / magnitude;
  const step = (normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10) * magnitude;
  return Math.ceil(maximum / step) * step;
}

function svgLinePath(points: Array<{ x: number; y: number }>) {
  if (points.length === 0) return '';
  return points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
}

export function DashboardTrendChart({ points }: DashboardTrendChartProps) {
  const [mode, setMode] = useState<TrendMode>('bar');
  const [activeIndex, setActiveIndex] = useState<number | null>(null);

  const chart = useMemo(() => {
    const safePoints = points ?? [];
    const innerWidth = SVG_WIDTH - PADDING.left - PADDING.right;
    const innerHeight = SVG_HEIGHT - PADDING.top - PADDING.bottom;
    const maximumConnections = Math.max(0, ...safePoints.map((point) => Number(point.connections) || 0));
    const axisMax = chartAxisMax(maximumConnections);
    const step = safePoints.length > 1 ? innerWidth / (safePoints.length - 1) : innerWidth;
    const coordinates = safePoints.map((point, index) => {
      const connections = Math.max(0, Number(point.connections) || 0);
      return {
        x: PADDING.left + step * index,
        y: PADDING.top + innerHeight - (connections / axisMax) * innerHeight,
      };
    });
    const totalConnections = safePoints.reduce((sum, point) => sum + (Number(point.connections) || 0), 0);
    const totalReceived = safePoints.reduce((sum, point) => sum + (Number(point.received) || 0), 0);
    const totalSent = safePoints.reduce((sum, point) => sum + (Number(point.sent) || 0), 0);

    return {
      innerHeight,
      innerWidth,
      axisMax,
      step,
      coordinates,
      totalConnections,
      totalReceived,
      totalSent,
      hasData: totalConnections > 0 || totalReceived > 0 || totalSent > 0,
    };
  }, [points]);

  const activePoint = activeIndex == null ? null : points[activeIndex];
  const yTicks = [0, 0.25, 0.5, 0.75, 1].map((factor) => ({
    value: Math.round(chart.axisMax * factor),
    y: PADDING.top + chart.innerHeight - chart.innerHeight * factor,
  }));
  const xLabelIndexes = new Set<number>();
  points.forEach((_, index) => {
    if (index === 0 || index === points.length - 1 || index % 4 === 0) xLabelIndexes.add(index);
  });
  const linePath = svgLinePath(chart.coordinates);
  const areaPath = linePath
    ? `${linePath} L ${(PADDING.left + chart.innerWidth).toFixed(2)} ${(PADDING.top + chart.innerHeight).toFixed(2)} L ${PADDING.left} ${(PADDING.top + chart.innerHeight).toFixed(2)} Z`
    : '';
  const barWidth = Math.max(4, Math.min(24, chart.step * 0.58));

  return (
    <div className="rounded-xl border border-border/60 bg-muted/10 p-3 sm:p-4">
      <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="flex items-center gap-1.5 text-sm font-medium">
            <BarChart3 className="h-4 w-4 text-primary" />
            每小时连接记录
          </p>
          <p className="mt-1 text-xs text-muted-foreground">按客户端断开时写入的连接历史聚合</p>
        </div>
        <div className="flex w-full rounded-lg border border-border/60 bg-background/40 p-0.5 sm:w-auto" role="group" aria-label="趋势图展示方式">
          <Button
            type="button"
            size="sm"
            variant={mode === 'bar' ? 'secondary' : 'ghost'}
            className="h-7 min-h-0 flex-1 px-2 sm:flex-none"
            aria-pressed={mode === 'bar'}
            onClick={() => setMode('bar')}
          >
            <BarChart3 className="h-3.5 w-3.5" />
            柱状图
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === 'line' ? 'secondary' : 'ghost'}
            className="h-7 min-h-0 flex-1 px-2 sm:flex-none"
            aria-pressed={mode === 'line'}
            onClick={() => setMode('line')}
          >
            <TrendingUp className="h-3.5 w-3.5" />
            折线图
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2 border-y border-border/40 py-2.5 text-center">
        <div>
          <p className="text-[11px] text-muted-foreground">连接记录</p>
          <p className="mt-0.5 text-sm font-semibold tabular-nums">{chart.totalConnections}</p>
        </div>
        <div>
          <p className="text-[11px] text-muted-foreground">下载</p>
          <p className="mt-0.5 text-sm font-semibold tabular-nums">{formatBytes(chart.totalReceived)}</p>
        </div>
        <div>
          <p className="text-[11px] text-muted-foreground">上传</p>
          <p className="mt-0.5 text-sm font-semibold tabular-nums">{formatBytes(chart.totalSent)}</p>
        </div>
      </div>

      {chart.hasData ? (
        <div className="mt-3 overflow-hidden">
          <svg
            viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
            className="block h-[220px] w-full select-none"
            role="img"
            aria-label="最近 24 小时每小时连接记录趋势图"
            onPointerLeave={() => setActiveIndex(null)}
          >
            <defs>
              <linearGradient id="dashboard-trend-area" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.34" />
                <stop offset="100%" stopColor="var(--primary)" stopOpacity="0.02" />
              </linearGradient>
              <linearGradient id="dashboard-trend-bars" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.95" />
                <stop offset="100%" stopColor="var(--primary)" stopOpacity="0.38" />
              </linearGradient>
            </defs>

            {yTicks.map((tick) => (
              <g key={tick.y}>
                <line
                  x1={PADDING.left}
                  x2={PADDING.left + chart.innerWidth}
                  y1={tick.y}
                  y2={tick.y}
                  className="stroke-border/60"
                  strokeWidth="1"
                  strokeDasharray={tick.value === 0 ? undefined : '3 4'}
                  vectorEffect="non-scaling-stroke"
                />
                <text x={PADDING.left - 7} y={tick.y + 3.5} textAnchor="end" className="fill-muted-foreground text-[10px]">
                  {tick.value}
                </text>
              </g>
            ))}

            {mode === 'bar' ? (
              points.map((point, index) => {
                const coord = chart.coordinates[index];
                const baseY = PADDING.top + chart.innerHeight;
                const height = Math.max(0, baseY - coord.y);
                return (
                  <rect
                    key={point.hour}
                    x={coord.x - barWidth / 2}
                    y={coord.y}
                    width={barWidth}
                    height={height}
                    rx="2"
                    fill="url(#dashboard-trend-bars)"
                    opacity={activeIndex == null || activeIndex === index ? 1 : 0.42}
                  />
                );
              })
            ) : (
              <>
                <path d={areaPath} fill="url(#dashboard-trend-area)" />
                <path
                  d={linePath}
                  fill="none"
                  stroke="var(--primary)"
                  strokeWidth="2.4"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
                {chart.coordinates.map((coord, index) => (
                  <circle
                    key={points[index].hour}
                    cx={coord.x}
                    cy={coord.y}
                    r={activeIndex === index ? 4.5 : 2.6}
                    className={cn('fill-primary stroke-background transition-all', activeIndex == null || activeIndex === index ? 'opacity-100' : 'opacity-40')}
                    strokeWidth="1.5"
                    vectorEffect="non-scaling-stroke"
                  />
                ))}
              </>
            )}

            {points.map((point, index) => {
              const coord = chart.coordinates[index];
              const hitWidth = Math.max(12, chart.step);
              return (
                <rect
                  key={`${point.hour}-hit`}
                  x={coord.x - hitWidth / 2}
                  y={PADDING.top}
                  width={hitWidth}
                  height={chart.innerHeight}
                  fill="transparent"
                  className="cursor-pointer"
                  onPointerEnter={() => setActiveIndex(index)}
                  onPointerDown={() => setActiveIndex(index)}
                />
              );
            })}

            {activeIndex != null && chart.coordinates[activeIndex] && (
              <line
                x1={chart.coordinates[activeIndex].x}
                x2={chart.coordinates[activeIndex].x}
                y1={PADDING.top}
                y2={PADDING.top + chart.innerHeight}
                className="stroke-primary/70"
                strokeWidth="1"
                strokeDasharray="3 3"
                vectorEffect="non-scaling-stroke"
              />
            )}

            {points.map((point, index) => xLabelIndexes.has(index) && (
              <text
                key={`${point.hour}-label`}
                x={chart.coordinates[index].x}
                y={SVG_HEIGHT - 9}
                textAnchor={index === 0 ? 'start' : index === points.length - 1 ? 'end' : 'middle'}
                className="fill-muted-foreground text-[10px]"
              >
                {point.hour}
              </text>
            ))}
          </svg>

          <div className="mt-1 min-h-11 rounded-lg border border-border/50 bg-background/30 px-3 py-2 text-xs sm:flex sm:items-center sm:justify-between">
            {activePoint ? (
              <>
                <span className="font-medium tabular-nums">{activePoint.hour}</span>
                <span className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground sm:mt-0">
                  <span>连接 <b className="font-medium text-foreground">{activePoint.connections}</b></span>
                  <span>下载 <b className="font-medium text-foreground">{formatBytes(activePoint.received)}</b></span>
                  <span>上传 <b className="font-medium text-foreground">{formatBytes(activePoint.sent)}</b></span>
                </span>
              </>
            ) : (
              <span className="text-muted-foreground">移动或点击图表中的时段，查看该小时的连接和流量明细。</span>
            )}
          </div>
        </div>
      ) : (
        <div className="mt-3 flex h-[220px] flex-col items-center justify-center rounded-lg border border-dashed border-border/70 bg-background/20 text-center">
          <BarChart3 className="h-7 w-7 text-muted-foreground/60" />
          <p className="mt-2 text-sm font-medium">最近 24 小时暂无连接历史</p>
          <p className="mt-1 text-xs text-muted-foreground">客户端断开后会自动写入趋势统计。</p>
        </div>
      )}
    </div>
  );
}
