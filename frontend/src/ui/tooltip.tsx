"use client"

import * as React from "react"
import { cn } from "@/lib/utils"

/**
 * 轻量级 Tooltip 组件
 * - 不依赖额外库，基于 CSS hover 实现
 * - 即时显示（无原生 title 的 1-2 秒延迟）
 * - 样式与主题一致，支持暗色模式
 *
 * 用法：
 * <Tooltip content="菜单名称" side="right">
 *   <button>...</button>
 * </Tooltip>
 */

interface TooltipProps {
  /** tooltip 内容 */
  content: React.ReactNode
  /** 显示位置，默认 right */
  side?: "top" | "right" | "bottom" | "left"
  /** 是否禁用 tooltip（例如展开状态下不需要显示） */
  disabled?: boolean
  /** 子元素（触发器） */
  children: React.ReactNode
  /** 延迟显示时间（ms），默认 200 */
  delayMs?: number
}

export function Tooltip({
  content,
  side = "right",
  disabled = false,
  children,
  delayMs = 200,
}: TooltipProps) {
  const [visible, setVisible] = React.useState(false)
  const timerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null)

  const showTooltip = React.useCallback(() => {
    if (disabled || !content) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => setVisible(true), delayMs)
  }, [disabled, content, delayMs])

  const hideTooltip = React.useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    setVisible(false)
  }, [])

  React.useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  // 定位样式
  const sideClasses: Record<string, string> = {
    top: "bottom-full left-1/2 -translate-x-1/2 mb-2",
    right: "left-full top-1/2 -translate-y-1/2 ml-2",
    bottom: "top-full left-1/2 -translate-x-1/2 mt-2",
    left: "right-full top-1/2 -translate-y-1/2 mr-2",
  }

  // 箭头定位样式
  const arrowClasses: Record<string, string> = {
    top: "top-full left-1/2 -translate-x-1/2 -mt-1 border-t-foreground border-x-transparent border-b-transparent",
    right: "right-full top-1/2 -translate-y-1/2 -ml-1 border-r-foreground border-y-transparent border-l-transparent",
    bottom: "bottom-full left-1/2 -translate-x-1/2 -mb-1 border-b-foreground border-x-transparent border-t-transparent",
    left: "left-full top-1/2 -translate-y-1/2 -mr-1 border-l-foreground border-y-transparent border-r-transparent",
  }

  if (disabled) return <>{children}</>

  return (
    <span
      className="relative inline-flex w-full"
      onMouseEnter={showTooltip}
      onMouseLeave={hideTooltip}
      onFocus={showTooltip}
      onBlur={hideTooltip}
    >
      {children}
      {visible && (
        <span
          role="tooltip"
          className={cn(
            "pointer-events-none absolute z-[100] whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs font-medium",
            "bg-foreground text-background shadow-md",
            "animate-in fade-in-0 zoom-in-95 duration-150",
            sideClasses[side],
          )}
        >
          {content}
          <span
            className={cn(
              "absolute h-0 w-0 border-4",
              arrowClasses[side],
            )}
          />
        </span>
      )}
    </span>
  )
}
