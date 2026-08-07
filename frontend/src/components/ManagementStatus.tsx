import { useMemo } from 'react';
import { AlertTriangle, PlugZap } from 'lucide-react';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/ui/popover';
import { Separator } from '@/ui/separator';
import { useSystemStatus } from '@/store/systemStatus';
import { cn } from '@/lib/utils';

/**
 * 顶部导航上的 OpenVPN Management 状态指示器：
 * - 正常时不渲染（保持导航栏简洁）
 * - 异常时渲染一个红色呼吸灯 + 文字标签，提醒管理员"管理面不可用"
 * - 点击 / hover 弹出 Popover，列出 risks 详情
 *
 * 设计动机：management 不可用时最关键的信息是"是否在线"，原本放在概览页
 * 的 risk 卡片很容易被忽略；放到顶部任何页面都一眼可见，体验更好。
 */
export function ManagementStatus({ compact = false }: { compact?: boolean }) {
  const { status } = useSystemStatus();
  const managementRisk = useMemo(
    () => status.risks.find((r) => r.level === 'danger' && /Management/i.test(r.title)) || null,
    [status.risks],
  );
  const otherRisks = useMemo(
    () => status.risks.filter((r) => r !== managementRisk),
    [status.risks, managementRisk],
  );

  // 正常态不渲染任何内容，避免占用导航空间
  if (status.managementOk && !managementRisk) return null;

  const title = managementRisk?.title || 'OpenVPN Management 不可用';
  const message = managementRisk?.message || '无法连接 OpenVPN management 端口，请检查服务状态和 management 配置。';

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={title}
          className={cn(
            'management-pill group inline-flex items-center gap-2.5 h-10 rounded-full',
            compact ? 'h-11 w-11 justify-center p-0' : 'pl-3 pr-4',
            'border border-red-500/50 bg-red-500/12 text-red-500',
            'hover:bg-red-500/20 hover:border-red-500/70 transition-colors',
            'focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500/40',
          )}
        >
          {/* 呼吸灯：双层 ping/pulse 扩散 + 红光内核（多层 keyframe 强光晕呼吸） */}
          <span className="relative inline-flex h-3 w-3 shrink-0">
            <span aria-hidden className="management-dot-pulse" />
            <span aria-hidden className="management-dot-ping" />
            <span
              aria-hidden
              className="relative inline-flex h-3 w-3 rounded-full bg-red-500 shadow-[0_0_10px_3px_rgba(239,68,68,0.7)]"
            />
          </span>
          <PlugZap className="h-4 w-4 drop-shadow-[0_0_6px_rgba(239,68,68,0.55)]" />
          <span className={cn("management-label text-sm font-semibold tracking-wide", compact && "sr-only")}>
            Management 离线
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 p-0">
        <div className="px-4 py-3 space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="relative inline-flex h-2 w-2">
              <span aria-hidden className="absolute inset-0 rounded-full bg-red-500 animate-ping opacity-60" />
              <span aria-hidden className="relative inline-flex h-2 w-2 rounded-full bg-red-500" />
            </span>
            <span className="text-xs font-semibold text-red-500">高危</span>
          </div>
          <p className="text-sm font-medium leading-tight">{title}</p>
          <p className="text-xs text-muted-foreground leading-relaxed">{message}</p>
        </div>
        {otherRisks.length > 0 && (
          <>
            <Separator />
            <div className="px-4 py-2.5 space-y-2">
              <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
                其他提醒
              </p>
              {otherRisks.map((risk, idx) => (
                <div key={`${risk.title}-${idx}`} className="flex items-start gap-2">
                  <AlertTriangle
                    className={cn(
                      'h-3.5 w-3.5 mt-0.5 shrink-0',
                      risk.level === 'warning' ? 'text-amber-500' : 'text-blue-500',
                    )}
                  />
                  <div className="min-w-0">
                    <p className="text-xs font-medium leading-tight">{risk.title}</p>
                    <p className="text-[11px] text-muted-foreground leading-snug">
                      {risk.message}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
