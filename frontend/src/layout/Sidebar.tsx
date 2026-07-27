import { NavLink, useLocation } from 'react-router-dom';
import {
  LayoutDashboard,
  Users,
  Smartphone,
  Shield,
  History,
  FileKey,
  FileText,
  Settings,
  Bell,
  BellRing,
  ShieldCheck,
} from 'lucide-react';
import { cn } from '../lib/utils';
import { useAuth } from '../store/auth';

// 菜单项：使用 menu:* 权限 code 控制可见性
// adminOnly 字段保留向后兼容，但已不再用于过滤（统一通过 permission 字段）
const allNavItems: { path: string; label: string; icon: typeof LayoutDashboard; permission: string }[] = [
  { path: '/overview', label: '概览', icon: LayoutDashboard, permission: 'menu:overview' },
  { path: '/users', label: '账号管理', icon: Users, permission: 'menu:users' },
  { path: '/clients', label: '客户端', icon: Smartphone, permission: 'menu:clients' },
  { path: '/firewall', label: '防火墙', icon: Shield, permission: 'menu:firewall' },
  { path: '/history', label: '连接历史', icon: History, permission: 'menu:history' },
  { path: '/certs', label: '证书', icon: FileKey, permission: 'menu:certs' },
  { path: '/audit', label: '操作审计', icon: FileText, permission: 'menu:audit' },
  { path: '/settings', label: '系统设置', icon: Settings, permission: 'menu:settings' },
  { path: '/channels', label: '通知渠道', icon: BellRing, permission: 'menu:channels' },
  { path: '/notifications', label: '站内信', icon: Bell, permission: 'menu:notifications' },
  { path: '/roles', label: '角色管理', icon: ShieldCheck, permission: 'menu:roles' },
];

export function Sidebar() {
  const location = useLocation();
  const { hasPermission } = useAuth();

  // RBAC：仅展示当前用户拥有对应 menu 权限的菜单项
  const navItems = allNavItems.filter((item) => hasPermission(item.permission));

  return (
    <nav className="w-64 min-h-screen border-r bg-card/80 backdrop-blur flex flex-col">
      <div className="p-6 border-b border-border">
        <h1 className="text-xl font-bold text-foreground">OpenVPN</h1>
      </div>

      <div className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {navItems.map((item) => {
          const isActive = location.pathname === item.path;
          const Icon = item.icon;

          return (
            <NavLink
              key={item.path}
              to={item.path}
              className={cn(
                // 默认：完全透明，无任何装饰；只有文字和图标作为基本可见元素。
                'group/nav relative flex items-center gap-3 overflow-hidden rounded-lg px-3 py-2.5 text-sm font-medium',
                'border border-transparent bg-transparent text-foreground/70',
                'transition-all duration-300 ease-out',
                // hover 状态：渐变描边 + 主题色光晕 + 顶部光带 + 左侧高亮条 + 文字加亮
                'hover:border-[var(--accent)]/45 hover:text-foreground',
                'hover:bg-[var(--accent)]/8',
                'hover:shadow-[0_0_22px_-8px_var(--accent),0_8px_24px_-14px_color-mix(in_srgb,var(--accent)_60%,transparent),inset_0_1px_0_color-mix(in_srgb,var(--accent)_22%,transparent)]',
                // active 状态：更明显的边框 + 主题色背景 + 字体加粗
                isActive &&
                  'border-[var(--accent)]/65 bg-[var(--accent)]/14 text-[var(--accent)] font-semibold shadow-[0_0_24px_-6px_var(--accent),inset_0_1px_0_color-mix(in_srgb,var(--accent)_28%,transparent)]',
              )}
            >
              {/* 顶部细光带：hover/active 时浮现 */}
              <span
                aria-hidden
                className="pointer-events-none absolute inset-x-3 top-0 h-px bg-gradient-to-r from-transparent via-[var(--accent)] to-transparent opacity-0 transition-all duration-300 group-hover/nav:opacity-90 group-hover/nav:scale-x-100 scale-x-50"
              />
              {/* 渐变描边层（hover 时的二次光） */}
              <span
                aria-hidden
                className="pointer-events-none absolute inset-0 rounded-lg opacity-0 transition-opacity duration-300 group-hover/nav:opacity-100"
                style={{
                  background:
                    'linear-gradient(135deg, color-mix(in srgb, var(--accent) 28%, transparent), color-mix(in srgb, var(--accent-2) 22%, transparent) 50%, color-mix(in srgb, var(--accent-3) 28%, transparent))',
                  WebkitMask:
                    'linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0)',
                  WebkitMaskComposite: 'xor',
                  maskComposite: 'exclude',
                  padding: '1px',
                }}
              />
              {/* 左侧高亮条：仅 active 状态显示 */}
              {isActive && (
                <span className="absolute left-0 top-1/2 h-6 w-[3px] -translate-y-1/2 rounded-r-full bg-[var(--accent)] shadow-[0_0_8px_var(--accent)]" />
              )}
              {/* 图标容器 */}
              <span
                className={cn(
                  'relative z-[1] flex h-5 w-5 shrink-0 items-center justify-center rounded transition-all duration-300',
                  isActive
                    ? 'bg-[var(--accent)]/22 text-[var(--accent)]'
                    : 'bg-muted/60 text-muted-foreground group-hover/nav:bg-[var(--accent)]/20 group-hover/nav:text-[var(--accent)] group-hover/nav:scale-110',
                )}
              >
                <Icon className="h-3.5 w-3.5" />
              </span>
              <span className="relative z-[1] flex-1">{item.label}</span>
            </NavLink>
          );
        })}
      </div>

      <div className="p-4 border-t border-border">
        <p className="text-xs text-muted-foreground text-center">
          © 2024 OpenVPN Admin
        </p>
      </div>
    </nav>
  );
}
