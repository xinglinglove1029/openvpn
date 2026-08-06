"use client"

import * as React from "react"
import { createPortal } from "react-dom"
import { cn } from "@/lib/utils"

/**
 * 轻量级 Tooltip 组件
 * - 不依赖额外库，基于 CSS hover 实现
 * - 使用 React Portal 渲染到 document.body，避免父容器 overflow 裁剪
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
  const [coords, setCoords] = React.useState({ top: 0, left: 0 })
  const triggerRef = React.useRef<HTMLSpanElement | null>(null)
  const timerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null)

  // 基于触发器位置计算 tooltip 定位（相对 viewport，配合 position:fixed）
  const computePosition = React.useCallback(() => {
    const el = triggerRef.current
    if (!el) return
    const rect = el.getBoundingClientRect()
    const gap = 8
    let top = 0
    let left = 0
    switch (side) {
      case "right":
        top = rect.top + rect.height / 2
        left = rect.right + gap
        break
      case "left":
        top = rect.top + rect.height / 2
        left = rect.left - gap
        break
      case "top":
        top = rect.top - gap
        left = rect.left + rect.width / 2
        break
      case "bottom":
        top = rect.bottom + gap
        left = rect.left + rect.width / 2
        break
    }
    setCoords({ top, left })
  }, [side])

  const show = React.useCallback(() => {
    if (disabled || !content) return
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      computePosition()
      setVisible(true)
    }, delayMs)
  }, [disabled, content, delayMs, computePosition])

  const hide = React.useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    setVisible(false)
  }, [])

  React.useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  // 显示期间监听滚动（捕获阶段，任意祖先容器滚动都触发）和 resize
  // - 滚动时直接隐藏，避免 tooltip 跟随不自然
  // - resize 时重新计算位置
  React.useEffect(() => {
    if (!visible) return
    const handleScroll = () => hide()
    const handleResize = () => computePosition()
    window.addEventListener("scroll", handleScroll, true)
    window.addEventListener("resize", handleResize)
    return () => {
      window.removeEventListener("scroll", handleScroll, true)
      window.removeEventListener("resize", handleResize)
    }
  }, [visible, hide, computePosition])

  if (disabled) return <>{children}</>

  // 各方向的 transform 偏移（配合 fixed 定位 + 计算 coords）
  const sideTransform: Record<string, string> = {
    top: "translate(-50%, -100%)",
    right: "translate(0, -50%)",
    bottom: "translate(-50%, 0)",
    left: "translate(-100%, -50%)",
  }

  return (
    <>
      <span
        ref={triggerRef}
        className="relative inline-flex w-full"
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
      >
        {children}
      </span>
      {visible &&
        typeof document !== "undefined" &&
        createPortal(
          <span
            role="tooltip"
            style={{
              position: "fixed",
              top: coords.top,
              left: coords.left,
              transform: sideTransform[side],
              zIndex: 9999,
              // 简洁深色背景：与项目 dropdown/popover 浮层一致
              backgroundColor: "var(--dropdown-bg)",
              color: "var(--text-strong)",
              // 细边框：弱化存在感，仅作边界区分
              border: "1px solid var(--panel-border)",
              // 单层柔和阴影：深度感但不喧宾夺主
              boxShadow: "0 4px 14px rgba(0, 0, 0, 0.4)",
            }}
            className={cn(
              "pointer-events-none whitespace-nowrap rounded-md px-2.5 py-1",
              "text-xs font-medium",
              // 进入动画：轻微缩放 + 淡入，自然不突兀
              "animate-in fade-in-0 zoom-in-95 duration-150",
            )}
          >
            {content}
          </span>,
          document.body,
        )}
    </>
  )
}
