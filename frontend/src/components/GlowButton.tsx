import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { cn } from '@/lib/utils';

export interface GlowButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** 按钮内图标，会自动加间距 */
  icon?: ReactNode;
  /** 加载态文案，启用时按钮禁用 */
  loading?: boolean;
  /** 加载中文案 */
  loadingText?: ReactNode;
  /** 默认内容（在 loading=false 时显示） */
  children?: ReactNode;
}

/**
 * 渐变光晕主题按钮
 *
 * - 背景使用 var(--primary-bg)、外发光使用 var(--accent)，全程跟随当前主题。
 * - 默认浅色主题下显示深蓝/青渐变，深色主题下显示浅色渐变（青/紫/绿）。
 * - hover 时扩散一圈主题色光晕。
 *
 * 之所以单独做一个组件：
 * 1. styles/index.css 里的 .primary-action 必须挂在原生 <button> 上才能保留 :hover/:active 行为。
 * 2. 把 loading 态和图标布局封在这里，避免每个页面重复写。
 */
export const GlowButton = forwardRef<HTMLButtonElement, GlowButtonProps>(
  ({ className, icon, loading, loadingText, disabled, children, type = 'button', ...rest }, ref) => {
    const isDisabled = disabled || loading;
    return (
      <button
        ref={ref}
        type={type}
        disabled={isDisabled}
        className={cn('primary-action', className)}
        {...rest}
      >
        {icon ? <span className="inline-flex items-center">{icon}</span> : null}
        <span>{loading ? loadingText ?? '保存中…' : children}</span>
      </button>
    );
  },
);
GlowButton.displayName = 'GlowButton';
