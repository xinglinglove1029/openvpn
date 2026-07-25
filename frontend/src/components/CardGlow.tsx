import { forwardRef, useCallback, useRef, type HTMLAttributes, type ReactNode } from 'react';
import { cn } from '@/lib/utils';

/**
 * 通用卡片容器 —— 主题色光晕（默认展示 + 持续动效，hover 更炫）
 *
 * 设计要点：
 * - 默认状态始终展示主题色边框 + 柔和外发光 + 持续动画：
 *   1) 边框 / 阴影颜色呼吸（card-glow-breathe 4.8s）
 *   2) 左上 / 右下两个光晕缓慢漂移（card-glow-orb-1/2 6.5s / 7.5s）
 *   3) 内部斜向扫光带循环播放（card-glow-sheen 6s）
 *   4) 顶部细光带常驻（默认状态：半透 + 缩放）
 * - 鼠标移入时切换到一个更炫酷的复合效果：
 *   1) 上浮 4px
 *   2) 边框加粗 + 多层光晕
 *   3) 顶部高亮线变满
 *   4) 鼠标位置径向高光（通过 CSS 变量 --mx / --my 控制）
 *   5) 右上 / 左下双光晕叠加
 *   6) 呼吸 / 漂移 / 扫光动画全部停止，避免和 hover 效果打架
 *
 * 使用方式：直接替换 <Card>，并保留 shadcn Card 的所有 props。
 */
export interface CardGlowProps extends HTMLAttributes<HTMLDivElement> {
  /** 是否启用顶部高亮线；默认 true */
  withTopLine?: boolean;
  /** 鼠标位置径向高光是否启用；默认 true */
  withMouseGlow?: boolean;
  /** 是否启用默认持续动画（呼吸 / 漂移 / 扫光）；默认 true */
  withAmbient?: boolean;
  /** 透传给外层容器的额外 className */
  className?: string;
  children?: ReactNode;
}

export const CardGlow = forwardRef<HTMLDivElement, CardGlowProps>(
  (
    { className, children, withTopLine = true, withMouseGlow = true, withAmbient = true, onMouseMove, ...rest },
    ref,
  ) => {
    const innerRef = useRef<HTMLDivElement | null>(null);

    const handleMouseMove = useCallback(
      (e: React.MouseEvent<HTMLDivElement>) => {
        if (withMouseGlow && innerRef.current) {
          const rect = innerRef.current.getBoundingClientRect();
          const x = ((e.clientX - rect.left) / rect.width) * 100;
          const y = ((e.clientY - rect.top) / rect.height) * 100;
          innerRef.current.style.setProperty('--mx', `${x}%`);
          innerRef.current.style.setProperty('--my', `${y}%`);
        }
        onMouseMove?.(e);
      },
      [onMouseMove, withMouseGlow],
    );

    return (
      <div
        ref={(node) => {
          innerRef.current = node;
          if (typeof ref === 'function') ref(node);
          else if (ref) (ref as React.MutableRefObject<HTMLDivElement | null>).current = node;
        }}
        className={cn('card-glow group', withAmbient ? 'is-ambient' : 'is-static', className)}
        onMouseMove={handleMouseMove}
        {...rest}
      >
        {/* 默认动效：角落光晕与内部扫光 */}
        {withAmbient ? (
          <>
            <span aria-hidden className="glow-orb-a" />
            <span aria-hidden className="glow-orb-b" />
            <span aria-hidden className="sheen" />
          </>
        ) : null}
        {withTopLine ? <span aria-hidden className="top-accent-line" /> : null}
        <div className="relative z-10 h-full w-full">{children}</div>
      </div>
    );
  },
);
CardGlow.displayName = 'CardGlow';
