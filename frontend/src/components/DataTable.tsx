import React, { useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight, ChevronsUpDown, ChevronUp, ChevronDown } from 'lucide-react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/ui/table';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import { Button } from '@/ui/button';
import { Card, CardContent } from '@/ui/card';
import { cn } from '@/lib/utils';
import { useIsMobile } from '@/hooks/useIsMobile';

export interface Column<T> {
  key: string;
  header: React.ReactNode;
  /** 移动端卡片中显示的 label；不传时回退为 header。如果 header 是非字符串 ReactNode（如全选 Checkbox），必须设置此项才不会在每张卡片里渲染一个表头组件。 */
  mobileHeader?: React.ReactNode;
  render: (item: T, index: number) => React.ReactNode;
  className?: string;
  /** Optional class applied only to the mobile key/value row. Desktop width classes never leak into cards. */
  mobileClassName?: string;
  /** Optional mobile-specific value renderer. */
  mobileRender?: (item: T, index: number) => React.ReactNode;
  /** Excludes an implementation-only desktop column from mobile cards. */
  hideOnMobile?: boolean;
  /** 是否允许点击表头排序 */
  sortable?: boolean;
  /** 自定义排序取值；不提供时尝试按 render 输出文本排序 */
  sortAccessor?: (item: T) => string | number | Date | null | undefined;
  /**
   * 移动端卡片布局位置：
   * - body（默认）：正文 key-value 行
   * - header-left：卡片头部左侧（主标题区），不会出现在正文
   * - header-right：卡片头部右侧（状态徽章/次要信息），不会出现在正文
   * - header-action：卡片头部最左侧（操作区，如小 Checkbox），不会出现在正文
   */
  cardPlacement?: 'body' | 'header-left' | 'header-right' | 'header-action';
}

interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  page: number;
  pageSize: number;
  pageCount: number;
  total: number;
  start: number;
  end: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  emptyTitle?: string;
  emptyDescription?: string;
  keyFn?: (item: T, index: number) => string | number;
  /**
   * 完整未分页数据。如果提供，排序作用于全集而不是当前页。
   * 推荐：在用 usePagination 时，把 source items 一并传入。
   */
  fullData?: T[];
  /**
   * 移动端卡片列表顶部的工具栏区域（全选 Checkbox 等可放此处）。
   * 桌面端不渲染。
   */
  mobileToolbar?: React.ReactNode;
  /** 计算每条数据行的卡片是否处于「选中」态，移动端会加强调边框/背景。 */
  isCardSelected?: (item: T) => boolean;
}

const pageSizeOptions = [
  { value: '10', label: '10 条/页' },
  { value: '20', label: '20 条/页' },
  { value: '50', label: '50 条/页' },
  { value: '100', label: '100 条/页' },
];

type SortDir = 'asc' | 'desc' | null;

function compareValues(
  a: string | number | Date | null | undefined,
  b: string | number | Date | null | undefined,
): number {
  if (a == null && b == null) return 0;
  if (a == null) return 1;
  if (b == null) return -1;
  if (a instanceof Date || b instanceof Date) {
    const ad = a instanceof Date ? a.getTime() : Number(new Date(a));
    const bd = b instanceof Date ? b.getTime() : Number(new Date(b));
    if (Number.isNaN(ad) && Number.isNaN(bd)) return 0;
    if (Number.isNaN(ad)) return 1;
    if (Number.isNaN(bd)) return -1;
    return ad - bd;
  }
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  // 数字字符串按数字比较
  const an = Number(a);
  const bn = Number(b);
  if (!Number.isNaN(an) && !Number.isNaN(bn) && a !== '' && b !== '') {
    return an - bn;
  }
  return String(a).localeCompare(String(b), 'zh-CN', { numeric: true, sensitivity: 'base' });
}

export function DataTable<T>({
  columns,
  data,
  page,
  pageSize,
  pageCount,
  total,
  start,
  end,
  onPageChange,
  onPageSizeChange,
  keyFn,
  fullData,
  mobileToolbar,
  isCardSelected,
}: DataTableProps<T>) {
  const isMobile = useIsMobile();

  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>(null);

  function handleSort(col: Column<T>) {
    if (!col.sortable) return;
    if (sortKey !== col.key) {
      setSortKey(col.key);
      setSortDir('asc');
      return;
    }
    // 同列：asc -> desc -> null -> asc
    if (sortDir === 'asc') setSortDir('desc');
    else if (sortDir === 'desc') {
      setSortDir(null);
      setSortKey(null);
    } else setSortDir('asc');
  }

  // 排序：作用于 fullData（如提供）或当前页 data
  const sortedFullData = useMemo(() => {
    const source = fullData ?? data;
    if (!sortKey || !sortDir) return source;
    const col = columns.find((c) => c.key === sortKey);
    if (!col) return source;
    const accessor = col.sortAccessor;
    return [...source].sort((a, b) => {
      const av = accessor ? accessor(a) : undefined;
      const bv = accessor ? accessor(b) : undefined;
      const cmp = compareValues(av as never, bv as never);
      return sortDir === 'asc' ? cmp : -cmp;
    });
  }, [fullData, data, columns, sortKey, sortDir]);

  // 如果有 fullData，外部的 page/start/end 不再可信，需根据 sortedFullData 重新分页
  const { finalData, finalStart, finalEnd } = useMemo(() => {
    if (!fullData) {
      return { finalData: data, finalStart: start, finalEnd: end };
    }
    const s = (page - 1) * pageSize;
    const e = Math.min(s + pageSize, sortedFullData.length);
    return { finalData: sortedFullData.slice(s, e), finalStart: s, finalEnd: e };
  }, [fullData, data, sortedFullData, page, pageSize, start, end]);

  if (!total) return null;

  return (
    <div className="space-y-3">
      {isMobile ? (
        <div className="space-y-2">
          {mobileToolbar && (
            <div className="flex items-center gap-3 rounded-md border bg-card px-3 h-11">
              {mobileToolbar}
            </div>
          )}
          {finalData.map((item, index) => {
            const globalIndex = finalStart + index;
            const selected = isCardSelected ? isCardSelected(item) : false;
            const actionCols = columns.filter((c) => !c.hideOnMobile && c.cardPlacement === 'header-action');
            const headerLeftCols = columns.filter((c) => !c.hideOnMobile && c.cardPlacement === 'header-left');
            const headerRightCols = columns.filter((c) => !c.hideOnMobile && c.cardPlacement === 'header-right');
            const bodyCols = columns.filter(
              (c) => !c.hideOnMobile && (c.cardPlacement === 'body' || !c.cardPlacement),
            );
            const hasHeader =
              actionCols.length > 0 || headerLeftCols.length > 0 || headerRightCols.length > 0;
            return (
              <Card
                key={keyFn ? keyFn(item, globalIndex) : globalIndex}
                data-testid="data-table-mobile-card"
                className={cn(
                  'transition-colors duration-150',
                  selected && 'border-[var(--accent)] ring-1 ring-[var(--accent)]/30 bg-[var(--accent)]/[0.06]',
                )}
              >
                <CardContent className="p-3 sm:p-4 space-y-3">
                  {hasHeader && (
                    <div className="flex items-center gap-2 min-w-0">
                      {actionCols.length > 0 && (
                        <div className="flex items-center gap-1 shrink-0 [&_button]:h-8 [&_button]:w-8 [&_button]:min-h-0 [&_button]:min-w-0 [&_button]:p-0 [&_span[data-slot]]:h-4 [&_span[data-slot]]:w-4 [&_svg]:h-4 [&_svg]:w-4">
                          {actionCols.map((col) => (
                            <React.Fragment key={col.key}>
                              {col.mobileRender
                                ? col.mobileRender(item, globalIndex)
                                : col.render(item, globalIndex)}
                            </React.Fragment>
                          ))}
                        </div>
                      )}
                      {headerLeftCols.length > 0 && (
                        <div className="flex flex-col items-start min-w-0 flex-1 gap-0.5">
                          {headerLeftCols.map((col) => (
                            <div
                              key={col.key}
                              className="min-w-0 truncate text-left"
                            >
                              {col.mobileRender
                                ? col.mobileRender(item, globalIndex)
                                : col.render(item, globalIndex)}
                            </div>
                          ))}
                        </div>
                      )}
                      {headerRightCols.length > 0 && (
                        <div className="flex flex-col items-end gap-1 shrink-0 ml-auto">
                          {headerRightCols.map((col) => (
                            <div key={col.key} className="max-w-[45%]">
                              {col.mobileRender
                                ? col.mobileRender(item, globalIndex)
                                : col.render(item, globalIndex)}
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                  {hasHeader && bodyCols.length > 0 && (
                    <div className="h-px bg-border/60 -mx-1" />
                  )}
                  {bodyCols.length > 0 && (
                    <div className="space-y-2">
                      {bodyCols.map((col) => (
                        <div
                          key={col.key}
                          className={cn(
                            'grid grid-cols-[minmax(5rem,38%)_minmax(0,1fr)] items-start gap-x-3 gap-y-1',
                            col.mobileClassName,
                          )}
                        >
                          <span className="min-w-0 text-[11px] sm:text-xs font-medium leading-5 text-muted-foreground/90">
                            {typeof col.mobileHeader !== 'undefined'
                              ? col.mobileHeader
                              : col.header}
                          </span>
                          <span className="min-w-0 break-words text-right text-[13px] sm:text-sm leading-5 [&_.row-actions]:justify-end [&_.row-actions]:whitespace-normal [&_.row-actions]:flex-wrap [&_button]:min-h-9 [&_button]:min-w-9">
                            {col.mobileRender
                              ? col.mobileRender(item, globalIndex)
                              : col.render(item, globalIndex)}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                {columns.map((col) => {
                  const isActive = sortKey === col.key;
                  const SortIcon = !col.sortable
                    ? null
                    : isActive && sortDir === 'asc'
                      ? ChevronUp
                      : isActive && sortDir === 'desc'
                        ? ChevronDown
                        : ChevronsUpDown;
                  return (
                    <TableHead
                      key={col.key}
                      className={cn(
                        col.className,
                        col.sortable && 'cursor-pointer select-none hover:text-[var(--accent)] transition-colors',
                        isActive && 'text-[var(--accent)]',
                      )}
                      onClick={() => handleSort(col)}
                    >
                      <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                        {col.header}
                        {SortIcon && (
                          <SortIcon
                            className={cn(
                              'h-3.5 w-3.5 shrink-0 transition-opacity',
                              isActive ? 'opacity-100' : 'opacity-40',
                            )}
                          />
                        )}
                      </span>
                    </TableHead>
                  );
                })}
              </TableRow>
            </TableHeader>
            <TableBody>
              {finalData.map((item, index) => (
                <TableRow key={keyFn ? keyFn(item, finalStart + index) : finalStart + index}>
                  {columns.map((col) => (
                    <TableCell key={col.key} className={col.className}>
                      {col.render(item, finalStart + index)}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {isMobile ? (
        <div className="flex flex-col gap-2 text-sm">
          <div className="text-muted-foreground text-center">
            显示 <strong>{finalStart + 1}-{finalEnd}</strong> / 共 {total} 条
          </div>
          <div className="flex items-center justify-center gap-1">
            <Button variant="outline" size="icon" className="h-11 w-11" aria-label="First page" disabled={page <= 1} onClick={() => onPageChange(1)}>
              «
            </Button>
            <Button variant="outline" size="icon" className="h-11 w-11" aria-label="Previous page" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="px-2 font-medium tabular-nums">
              {page} / {pageCount}
            </span>
            <Button variant="outline" size="icon" className="h-11 w-11" aria-label="Next page" disabled={page >= pageCount} onClick={() => onPageChange(page + 1)}>
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button variant="outline" size="icon" className="h-11 w-11" aria-label="Last page" disabled={page >= pageCount} onClick={() => onPageChange(pageCount)}>
              »
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-4 text-sm">
          <div className="text-muted-foreground">
            显示 <strong>{finalStart + 1}-{finalEnd}</strong> / 共 {total} 条
          </div>
          <div className="flex items-center gap-3">
            <Select
              value={String(pageSize)}
              onValueChange={(value) => onPageSizeChange(Number(value))}
            >
              <SelectTrigger className="w-[130px] h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="flex items-center gap-1">
              <Button variant="outline" size="icon" className="h-8 w-8" aria-label="First page" disabled={page <= 1} onClick={() => onPageChange(1)}>
                «
              </Button>
              <Button variant="outline" size="icon" className="h-8 w-8" aria-label="Previous page" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="px-2 font-medium">
                {page} / {pageCount}
              </span>
              <Button variant="outline" size="icon" className="h-8 w-8" aria-label="Next page" disabled={page >= pageCount} onClick={() => onPageChange(page + 1)}>
                <ChevronRight className="h-4 w-4" />
              </Button>
              <Button variant="outline" size="icon" className="h-8 w-8" aria-label="Last page" disabled={page >= pageCount} onClick={() => onPageChange(pageCount)}>
                »
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
