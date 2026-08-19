import { useEffect, useState, type CSSProperties } from 'react';
import { CalendarClock, ChevronDown } from 'lucide-react';
import { format } from 'date-fns';
import { cn } from '@/lib/utils';
import { Button } from '@/ui/button';
import { Calendar } from '@/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/ui/popover';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/ui/select';

interface DateTimePickerProps {
  value: string; // local yyyy-MM-ddTHH:mm
  onChange: (value: string) => void;
  placeholder?: string;
  allowClear?: boolean;
  className?: string;
}

const hours = Array.from({ length: 24 }, (_, hour) => String(hour).padStart(2, '0'));
const minutes = Array.from({ length: 60 }, (_, minute) => String(minute).padStart(2, '0'));

function parseDateTimeLocal(value?: string): Date | undefined {
  if (!value || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return undefined;
  const [datePart, timePart] = value.split('T');
  const [year, month, day] = datePart.split('-').map(Number);
  const [hour, minute] = timePart.split(':').map(Number);
  if (!year || month < 1 || month > 12 || day < 1 || day > 31 || hour < 0 || hour > 23 || minute < 0 || minute > 59) return undefined;
  const date = new Date(year, month - 1, day, hour, minute, 0, 0);
  return date.getFullYear() === year && date.getMonth() === month - 1 && date.getDate() === day && date.getHours() === hour && date.getMinutes() === minute
    ? date
    : undefined;
}

function toDateTimeLocalValue(date: Date, hour: string, minute: string) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}T${hour}:${minute}`;
}

/**
 * Theme-aware replacement for the browser-owned datetime-local popup.
 * It intentionally commits only after confirmation so changing the date and time is atomic.
 */
export function DateTimePicker({ value, onChange, placeholder = '选择日期和时间', allowClear = true, className }: DateTimePickerProps) {
  const [open, setOpen] = useState(false);
  const [draftDate, setDraftDate] = useState<Date | undefined>();
  const [hour, setHour] = useState('00');
  const [minute, setMinute] = useState('00');
  const [draftChanged, setDraftChanged] = useState(false);

  const selected = parseDateTimeLocal(value);
  const hasInvalidValue = Boolean(value && !selected);

  useEffect(() => {
    if (!open) return;
    const initial = selected ?? new Date();
    setDraftDate(initial);
    setHour(String(initial.getHours()).padStart(2, '0'));
    setMinute(String(initial.getMinutes()).padStart(2, '0'));
    setDraftChanged(false);
  }, [open, value]);

  const displayText = selected ? format(selected, 'yyyy/MM/dd HH:mm') : (hasInvalidValue ? '日期时间无效' : placeholder);
  const ariaLabel = selected
    ? `${placeholder}：${displayText}`
    : (hasInvalidValue ? `${placeholder}：当前值无效，请重新选择` : placeholder);

  const commit = () => {
    if (!draftDate) return;
    onChange(toDateTimeLocalValue(draftDate, hour, minute));
    setOpen(false);
  };

  const setNow = () => {
    const now = new Date();
    setDraftDate(now);
    setHour(String(now.getHours()).padStart(2, '0'));
    setMinute(String(now.getMinutes()).padStart(2, '0'));
    setDraftChanged(true);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn('h-9 w-full justify-between gap-2 px-3 text-left font-normal', !value && 'text-muted-foreground', className)}
          aria-label={ariaLabel}
        >
          <span className="flex min-w-0 items-center gap-2 truncate">
            <CalendarClock className="h-4 w-4 shrink-0" />
            <span className="truncate">{displayText}</span>
          </span>
          <ChevronDown className="h-4 w-4 shrink-0 opacity-65" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-auto max-h-[calc(100vh-1.5rem)] max-h-[var(--radix-popover-content-available-height)] max-w-[calc(100vw-1.5rem)] overflow-x-hidden overflow-y-auto border-[var(--panel-border)] bg-[var(--dropdown-bg)] p-0 text-[var(--text)] shadow-[var(--panel-shadow)]"
        align="start"
      >
        <Calendar
          mode="single"
          selected={draftDate}
          onSelect={(date) => {
            setDraftDate(date);
            setDraftChanged(true);
          }}
          style={{ '--cell-size': 'min(2.25rem, calc((100vw - 3rem) / 7))' } as CSSProperties}
        />
        {hasInvalidValue && (
          <div className="border-t border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
            当前日期时间无效，请重新选择后再确定。
          </div>
        )}
        <div className="grid grid-cols-[auto_1fr_1fr] items-center gap-2 border-t border-border/70 px-3 py-2.5">
          <span className="text-xs font-medium text-muted-foreground">时间</span>
          <Select value={hour} onValueChange={(nextHour) => {
            setHour(nextHour);
            setDraftChanged(true);
          }}>
            <SelectTrigger aria-label="小时" className="h-8 min-h-0 min-w-0 px-2 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {hours.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
            </SelectContent>
          </Select>
          <Select value={minute} onValueChange={(nextMinute) => {
            setMinute(nextMinute);
            setDraftChanged(true);
          }}>
            <SelectTrigger aria-label="分钟" className="h-8 min-h-0 min-w-0 px-2 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {minutes.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center justify-between border-t border-border/70 p-3">
          {allowClear ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => {
                onChange('');
                setOpen(false);
              }}
            >
              清空
            </Button>
          ) : <span />}
          <div className="flex items-center gap-1">
            <Button type="button" variant="ghost" size="sm" onClick={setNow}>现在</Button>
            <Button type="button" size="sm" onClick={commit} disabled={!draftDate || (hasInvalidValue && !draftChanged)}>确定</Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
