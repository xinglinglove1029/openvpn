import { Badge } from '@/ui/badge';

type StatusType = 'success' | 'warning' | 'danger' | 'neutral' | 'info';

const statusVariants: Record<StatusType, string> = {
  success: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/25',
  warning: 'bg-amber-500/15 text-amber-600 border-amber-500/25',
  danger: 'bg-red-500/15 text-red-600 border-red-500/25',
  neutral: 'bg-slate-500/15 text-slate-600 border-slate-500/25',
  info: 'bg-blue-500/15 text-blue-600 border-blue-500/25',
};

interface StatusBadgeProps {
  status: StatusType;
  className?: string;
  children: React.ReactNode;
}

export function StatusBadge({ status, className, children }: StatusBadgeProps) {
  return (
    <Badge variant="outline" className={`${statusVariants[status]} ${className || ''}`}>
      {children}
    </Badge>
  );
}
