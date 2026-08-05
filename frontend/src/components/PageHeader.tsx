interface PageHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
  children?: React.ReactNode;
}

export function PageHeader({ eyebrow, title, description, children }: PageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        {eyebrow && (
          <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
            {eyebrow}
          </span>
        )}
        <h2 className="text-2xl font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-[var(--text)] to-[color-mix(in_srgb,var(--text)_60%,var(--accent))]">{title}</h2>
        {description && (
          <p className="text-sm text-muted-foreground mt-1">{description}</p>
        )}
      </div>
      {children && <div className="flex items-center gap-2">{children}</div>}
    </div>
  );
}
