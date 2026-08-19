import { useState, type CSSProperties } from 'react';
import { CalendarIcon } from 'lucide-react';
import { format } from 'date-fns';
import { cn } from '@/lib/utils';
import { Button } from '@/ui/button';
import { Calendar } from '@/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/ui/popover';
import { useIsMobile } from '@/hooks/useIsMobile';

interface TimeRange {
  from: string;
  to: string;
}

interface TimeRangePickerProps {
  value: TimeRange;
  onChange: (value: TimeRange) => void;
  placeholder?: string;
}

function toDateInputValue(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function parseDateInputValue(value?: string) {
  if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
  const [year, month, day] = value.split('-').map(Number);
  if (!year || month < 1 || month > 12 || day < 1 || day > 31) return undefined;
  const date = new Date(year, month - 1, day);
  return date.getFullYear() === year && date.getMonth() === month - 1 && date.getDate() === day ? date : undefined;
}

export function TimeRangePicker({ value, onChange, placeholder = '选择时间范围' }: TimeRangePickerProps) {
  const [open, setOpen] = useState(false);
  const [selectingStart, setSelectingStart] = useState(true);
  const [currentFrom, setCurrentFrom] = useState<Date | undefined>();
  const isCompact = useIsMobile();

  const fromDate = parseDateInputValue(value.from);
  const toDate = parseDateInputValue(value.to);

  const selected = currentFrom
    ? { from: currentFrom, to: undefined }
    : (fromDate ? { from: fromDate, to: toDate } : undefined);
  const hasInvalidValue = Boolean((value.from && !fromDate) || (value.to && !toDate));

  const displayText = hasInvalidValue
    ? '日期范围无效'
    : (value.from && value.to && fromDate && toDate
      ? `${format(fromDate, 'yyyy/MM/dd')} - ${format(toDate, 'yyyy/MM/dd')}`
      : placeholder);
  const ariaLabel = value.from && value.to && !hasInvalidValue
    ? `${placeholder}：${displayText}`
    : (hasInvalidValue ? `${placeholder}：当前值无效，请重新选择` : placeholder);

  const handleOpenChange = (isOpen: boolean) => {
    if (isOpen) {
      setSelectingStart(true);
      setCurrentFrom(undefined);
    }
    setOpen(isOpen);
  };

  // DayPicker's range `onSelect` is based on the currently controlled range.
  // For a replacement range it can therefore keep the old start date. Use the
  // clicked day directly so the two-step picker always starts a fresh range.
  const handleDayClick = (day: Date) => {
    if (selectingStart) {
      setCurrentFrom(day);
      setSelectingStart(false);
      return;
    }

    const from = currentFrom || day;
    const [start, end] = from > day ? [day, from] : [from, day];
    onChange({
      from: toDateInputValue(start),
      to: toDateInputValue(end),
    });
    setSelectingStart(true);
    setCurrentFrom(undefined);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          aria-label={ariaLabel}
          className={cn(
            'h-9 w-full min-w-0 justify-start text-left font-normal sm:w-auto sm:min-w-[220px]',
            !value.from && !value.to && 'text-muted-foreground'
          )}
        >
          <CalendarIcon className="mr-2 h-4 w-4" />
          {displayText}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-auto max-h-[calc(100vh-1.5rem)] max-h-[var(--radix-popover-content-available-height)] max-w-[calc(100vw-1.5rem)] overflow-x-hidden overflow-y-auto border-[var(--panel-border)] bg-[var(--dropdown-bg)] p-0 text-[var(--text)] shadow-[var(--panel-shadow)]"
        align="start"
      >
        <Calendar
          mode="range"
          numberOfMonths={isCompact ? 1 : 2}
          style={isCompact ? ({ '--cell-size': 'min(2.25rem, calc((100vw - 3rem) / 7))' } as CSSProperties) : undefined}
          selected={selected}
          onDayClick={handleDayClick}
        />
        <div className="border-t border-border/70 px-3 py-2 text-xs text-muted-foreground">
          {selectingStart ? '请选择开始日期' : '请选择结束日期'}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border/70 p-3">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onChange({ from: '', to: '' });
              setSelectingStart(true);
              setCurrentFrom(undefined);
              setOpen(false);
            }}
          >
            清空
          </Button>
          <div className="flex flex-wrap items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                const today = new Date();
                const sevenDaysAgo = new Date();
                sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
                onChange({
                  from: toDateInputValue(sevenDaysAgo),
                  to: toDateInputValue(today),
                });
                setSelectingStart(true);
                setCurrentFrom(undefined);
                setOpen(false);
              }}
            >
              近7天
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                const today = new Date();
                const thirtyDaysAgo = new Date();
                thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);
                onChange({
                  from: toDateInputValue(thirtyDaysAgo),
                  to: toDateInputValue(today),
                });
                setSelectingStart(true);
                setCurrentFrom(undefined);
                setOpen(false);
              }}
            >
              近30天
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
