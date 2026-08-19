import { useState, type CSSProperties } from 'react';
import { CalendarIcon } from 'lucide-react';
import { format } from 'date-fns';
import { cn } from '@/lib/utils';
import { Button } from '@/ui/button';
import { Calendar } from '@/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/ui/popover';
import { useIsMobile } from '@/hooks/useIsMobile';

interface DatePickerFieldProps {
  value: string; // yyyy-MM-dd
  onChange: (value: string) => void;
  placeholder?: string;
  allowClear?: boolean;
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

export function DatePickerField({ value, onChange, placeholder = '选择日期', allowClear = true }: DatePickerFieldProps) {
  const [open, setOpen] = useState(false);
  const selected = parseDateInputValue(value);
  const hasInvalidValue = Boolean(value && !selected);
  const displayText = selected ? format(selected, 'yyyy/MM/dd') : (hasInvalidValue ? '日期无效' : placeholder);
  const isMobile = useIsMobile();

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          aria-label={selected ? `${placeholder}：${displayText}` : (hasInvalidValue ? `${placeholder}：当前值无效，请重新选择` : placeholder)}
          className={cn(
            'justify-start text-left font-normal',
            isMobile ? 'h-11 min-h-[44px]' : 'h-9',
            !value && 'text-muted-foreground',
          )}
        >
          <CalendarIcon className={cn('mr-2', isMobile ? 'h-5 w-5' : 'h-4 w-4')} />
          {displayText}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className={cn(
          'max-h-[calc(100vh-1.5rem)] max-h-[var(--radix-popover-content-available-height)] max-w-[calc(100vw-1.5rem)] overflow-x-hidden overflow-y-auto border-[var(--panel-border)] bg-[var(--dropdown-bg)] p-0 text-[var(--text)] shadow-[var(--panel-shadow)]',
          isMobile ? 'w-[95vw]' : 'w-auto',
        )}
        align="start"
      >
        <Calendar
          mode="single"
          selected={selected}
          onSelect={(date) => {
            if (date) {
              onChange(toDateInputValue(date));
              setOpen(false);
            }
          }}
          style={isMobile ? ({ '--cell-size': 'min(2.25rem, calc((100vw - 3rem) / 7))' } as CSSProperties) : undefined}
        />
        <div className="flex items-center justify-between p-3 border-t">
          {allowClear && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                onChange('');
                setOpen(false);
              }}
            >
              清空
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onChange(toDateInputValue(new Date()));
              setOpen(false);
            }}
          >
            今天
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
