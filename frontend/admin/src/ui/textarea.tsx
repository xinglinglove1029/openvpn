import * as React from "react"

import { cn } from "@/lib/utils"

const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.ComponentProps<"textarea">
>(({ className, ...props }, ref) => {
  return (
    <textarea
      className={cn(
        "flex min-h-[60px] w-full rounded-md border border-[color-mix(in_srgb,var(--accent)_22%,transparent)] bg-[var(--surface-strong)] backdrop-blur-md px-3 py-2 text-base shadow-sm placeholder:text-xs placeholder:text-[color-mix(in_srgb,var(--text)_32%,transparent)] hover:border-[color-mix(in_srgb,var(--accent)_42%,transparent)] focus-visible:outline-none focus-visible:border-[color-mix(in_srgb,var(--accent)_70%,transparent)] focus-visible:ring-1 focus-visible:ring-[color-mix(in_srgb,var(--accent)_50%,transparent)] disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
        className
      )}
      ref={ref}
      {...props}
    />
  )
})
Textarea.displayName = "Textarea"

export { Textarea }
