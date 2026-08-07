import { useIsMobile } from '@/hooks/useIsMobile';

interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  children?: React.ReactNode;
}

export function PageHeader({ eyebrow, title, description, children }: PageHeaderProps) {
  const isMobile = useIsMobile();

  return (
    <div className={`flex ${isMobile ? 'flex-col items-start gap-3' : 'items-center justify-between'}`}>
      <div className={isMobile ? 'space-y-1' : ''}>
        {eyebrow && (
          <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
            {eyebrow}
          </span>
        )}
        <h2 className={`${isMobile ? 'text-xl' : 'text-2xl'} font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-[var(--text)] to-[color-mix(in_srgb,var(--text)_60%,var(--accent))]`}>{title}</h2>
        {description && (
          <p className="text-sm text-muted-foreground mt-1">{description}</p>
        )}
      </div>
      {children && (
        <div className={`flex ${isMobile ? 'w-full flex-col gap-2 sm:flex-row sm:items-center' : 'items-center gap-2'}`}>
          {children}
        </div>
      )}
    </div>
  );
}
