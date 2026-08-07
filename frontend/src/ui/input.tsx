import * as React from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

type InputProps = React.ComponentProps<"input"> & {
  /** Set false when an adjacent control (such as a password visibility toggle) owns the trailing area. */
  clearable?: boolean
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, value, onChange, clearable = type !== 'password' && type !== 'number', ...props }, ref) => {
    const hasValue = typeof value === 'string' && value.length > 0;
    const showClear = clearable && hasValue;

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
            "flex h-11 w-full rounded-md sm:h-9 border border-[color-mix(in_srgb,var(--accent)_22%,transparent)] bg-[var(--surface-strong)] backdrop-blur-md px-3 py-1 text-base shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-xs placeholder:text-[color-mix(in_srgb,var(--text)_32%,transparent)] hover:border-[color-mix(in_srgb,var(--accent)_42%,transparent)] focus-visible:outline-none focus-visible:border-[color-mix(in_srgb,var(--accent)_70%,transparent)] focus-visible:ring-1 focus-visible:ring-[color-mix(in_srgb,var(--accent)_50%,transparent)] disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
            showClear && "pr-12 sm:pr-10",
            className
          )}
          ref={ref}
          {...props}
        />
        {showClear && (
          <button
            type="button"
            onClick={handleClear}
            className="absolute right-1 inset-y-0 my-auto inline-flex aspect-square h-[calc(100%-0.5rem)] items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent/20 hover:text-foreground"
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
