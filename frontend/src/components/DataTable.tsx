import { useMemo, useState } from 'react';
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
import { cn } from '@/lib/utils';

export interface Column<T> {
  key: string;
  header: React.ReactNode;
  render: (item: T, index: number) => React.ReactNode;
  className?: string;
  /** 是否允许点击表头排序 */
  sortable?: boolean;
  /** 自定义排序取值；不提供时尝试按 render 输出文本排序 */
  sortAccessor?: (item: T) => string | number | Date | null | undefined;
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
}: DataTableProps<T>) {
  if (!total) return null;

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

  return (
    <div className="space-y-3">
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
                    <span className="inline-flex items-center gap-1.5">
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

      {/* 分页栏：左侧统计、右侧分页器+每页条数 */}
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
            <Button variant="outline" size="icon" className="h-8 w-8" disabled={page <= 1} onClick={() => onPageChange(1)}>
              «
            </Button>
            <Button variant="outline" size="icon" className="h-8 w-8" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="px-2 font-medium">
              {page} / {pageCount}
            </span>
            <Button variant="outline" size="icon" className="h-8 w-8" disabled={page >= pageCount} onClick={() => onPageChange(page + 1)}>
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button variant="outline" size="icon" className="h-8 w-8" disabled={page >= pageCount} onClick={() => onPageChange(pageCount)}>
              »
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
