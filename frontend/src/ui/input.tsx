import * as React from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

const Input = React.forwardRef<HTMLInputElement, React.ComponentProps<"input">>(
  ({ className, type, value, onChange, ...props }, ref) => {
    const hasValue = typeof value === 'string' && value.length > 0;

    const handleClear = (e: React.MouseEvent) => {
      e.preventDefault();
      if (onChange) {
        onChange({
          target: { value: '', type: 'text' },
        } as React.ChangeEvent<HTMLInputElement>);
      }
    };

    return (
      <div className="relative">
        <input
          type={type}
          value={value}
          onChange={onChange}
          className={cn(
            "flex h-9 w-full rounded-md border border-[color-mix(in_srgb,var(--accent)_22%,transparent)] bg-[var(--surface-strong)] backdrop-blur-md px-3 py-1 text-base shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-xs placeholder:text-[color-mix(in_srgb,var(--text)_32%,transparent)] hover:border-[color-mix(in_srgb,var(--accent)_42%,transparent)] focus-visible:outline-none focus-visible:border-[color-mix(in_srgb,var(--accent)_70%,transparent)] focus-visible:ring-1 focus-visible:ring-[color-mix(in_srgb,var(--accent)_50%,transparent)] disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
            hasValue && "pr-8",
            className
          )}
          ref={ref}
          {...props}
        />
        {hasValue && (
          <button
            type="button"
            onClick={handleClear}
            className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-1 text-muted-foreground hover:text-foreground hover:bg-accent/20 transition-colors"
            aria-label="清除输入"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    )
  }
)
Input.displayName = "Input"

export { Input }
