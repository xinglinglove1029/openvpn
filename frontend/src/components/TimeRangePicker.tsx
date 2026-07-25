import { useState } from 'react';
import { CalendarIcon } from 'lucide-react';
import { format } from 'date-fns';
import { cn } from '@/lib/utils';
import { Button } from '@/ui/button';
import { Calendar } from '@/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/ui/popover';

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
  if (!value) return undefined;
  const [year, month, day] = value.split('-').map(Number);
  if (!year || !month || !day) return undefined;
  const date = new Date(year, month - 1, day);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

export function TimeRangePicker({ value, onChange, placeholder = '选择时间范围' }: TimeRangePickerProps) {
  const [open, setOpen] = useState(false);
  const [selectingStart, setSelectingStart] = useState(true);
  const [currentFrom, setCurrentFrom] = useState<Date | undefined>();
  const [currentTo, setCurrentTo] = useState<Date | undefined>();

  const fromDate = parseDateInputValue(value.from);
  const toDate = parseDateInputValue(value.to);

  const selected = { from: currentFrom || fromDate, to: currentTo || toDate };

  const displayText = value.from && value.to
    ? `${format(fromDate || new Date(), 'yyyy/MM/dd')} - ${format(toDate || new Date(), 'yyyy/MM/dd')}`
    : placeholder;

  const handleOpenChange = (isOpen: boolean) => {
    if (isOpen) {
      setSelectingStart(true);
      setCurrentFrom(undefined);
      setCurrentTo(undefined);
    }
    setOpen(isOpen);
  };

  const handleSelect = (dates: unknown) => {
    const dateRange = dates as { from?: Date; to?: Date };
    
    if (!dateRange.from) return;

    if (selectingStart) {
      setCurrentFrom(dateRange.from);
      setSelectingStart(false);
    } else {
      const from = currentFrom || dateRange.from;
      const to = dateRange.to || dateRange.from;
      
      if (from > to) {
        onChange({
          from: toDateInputValue(to),
          to: toDateInputValue(from),
        });
      } else {
        onChange({
          from: toDateInputValue(from),
          to: toDateInputValue(to),
        });
      }
      setSelectingStart(true);
      setCurrentFrom(undefined);
      setCurrentTo(undefined);
      setOpen(false);
    }
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          className={cn(
            'h-9 justify-start text-left font-normal min-w-[220px]',
            !value.from && !value.to && 'text-muted-foreground'
          )}
        >
          <CalendarIcon className="mr-2 h-4 w-4" />
          {displayText}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="range"
          numberOfMonths={2}
          selected={selected}
          onSelect={handleSelect}
        />
        <div className="flex items-center justify-between p-3 border-t">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              onChange({ from: '', to: '' });
              setSelectingStart(true);
              setCurrentFrom(undefined);
              setCurrentTo(undefined);
              setOpen(false);
            }}
          >
            清空
          </Button>
          <div className="flex items-center gap-2">
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
                setCurrentTo(undefined);
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
                setCurrentTo(undefined);
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