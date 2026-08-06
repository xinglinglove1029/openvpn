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
  KeyRound,
  User,
  type LucideIcon,
} from 'lucide-react';
import { cn } from '../lib/utils';
import { useAuth } from '../store/auth';
import { Tooltip } from '../ui/tooltip';

// 图标名称 → lucide-react 组件映射表
// 后端 permission.icon 字段（字符串）通过此映射渲染为图标组件
// 未识别的图标名称 fallback 到 LayoutDashboard
const iconMap: Record<string, LucideIcon> = {
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
  KeyRound,
  User,
};

// settings 菜单特殊处理：需要检查至少有一个 Tab 权限
const settingsTabPermissions = [
  'settings:base',
  'settings:ldap',
  'settings:openvpn',
  'settings:service',
  'settings:packages',
];

interface NavItem {
  path: string;
  label: string;
  icon: LucideIcon;
  permission: string;
  /** 是否为 settings 菜单（需额外检查 Tab 权限） */
  isSettings?: boolean;
}

interface SidebarProps {
  /** 桌面端折叠模式：仅显示图标 */
  collapsed?: boolean;
}

export function Sidebar({ collapsed = false }: SidebarProps) {
  const location = useLocation();
  const { hasPermission, permissionTree } = useAuth();

  // 从 permissionTree 动态构建菜单项：
  // - 仅展示 type=menu 的根节点（parentId 为空或 0）
  // - 按 sort 排序
  // - 仅展示用户拥有对应 menu 权限的节点
  // - settings 菜单特殊处理：需检查至少有一个 Tab 权限（子节点中 type=button 的权限）
  const navItems: NavItem[] = (permissionTree || [])
    .filter((node) => node.type === 'menu' && node.code !== 'menu:profile')
    .sort((a, b) => a.sort - b.sort)
    .map((node) => ({
      path: node.path || `/${node.code.split(':')[1] || node.code}`,
      label: node.name,
      icon: iconMap[node.icon || ''] || LayoutDashboard,
      permission: node.code,
      isSettings: node.code === 'menu:settings',
    }))
    .filter((item) => {
      if (!hasPermission(item.permission)) return false;
      // settings 菜单特殊处理：需检查至少有一个 Tab 权限
      if (item.isSettings) {
        return settingsTabPermissions.some((p) => hasPermission(p));
      }
      return true;
    });

  return (
    <nav
      className={cn(
        'min-h-screen border-r border-border/40 bg-card/50 backdrop-blur-xl flex flex-col transition-[width] duration-300 ease-out',
        collapsed ? 'w-16' : 'w-64',
      )}
    >
      {/* Logo 区域：折叠时只显示盾牌图标并居中 */}
      <div
        className={cn(
          'border-b border-border/40',
          collapsed ? 'p-4 flex justify-center' : 'p-6',
        )}
      >
        <div
          className={cn(
            'flex items-center gap-2.5',
            collapsed && 'gap-0',
          )}
        >
          <div
            className="flex h-8 w-8 items-center justify-center rounded-lg shadow-lg shrink-0"
            style={{
              background:
                'linear-gradient(135deg, color-mix(in srgb, var(--accent) 85%, white) 0%, var(--accent) 100%)',
              boxShadow:
                '0 4px 14px color-mix(in srgb, var(--accent) 40%, transparent), inset 0 1px 0 rgba(255,255,255,0.2)',
            }}
          >
            <ShieldCheck className="h-4 w-4 text-white" strokeWidth={2.5} />
          </div>
          {!collapsed && (
            <h1 className="text-xl font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-[var(--text)] to-[color-mix(in_srgb,var(--text)_60%,var(--accent))]">
              OpenVPN
            </h1>
          )}
        </div>
      </div>

      {/* 菜单列表：折叠时仅显示图标，hover 显示原生 title tooltip */}
      <div
        className={cn(
          'flex-1 py-4 space-y-1 overflow-y-auto overflow-x-hidden',
          collapsed ? 'px-2' : 'px-3',
        )}
      >
        {navItems.map((item) => {
          const isActive = location.pathname === item.path;
          const Icon = item.icon;

          const navLink = (
            <NavLink
              key={item.path}
              to={item.path}
              className={cn(
                // 默认：完全透明，无任何装饰；只有文字和图标作为基本可见元素。
                'group/nav relative flex items-center overflow-hidden rounded-lg text-sm font-medium',
                'border border-transparent bg-transparent text-foreground/70',
                'transition-all duration-300 ease-out',
                collapsed ? 'justify-center px-2 py-2.5' : 'gap-3 px-3 py-2.5',
                // hover 状态：渐变描边 + 主题色光晕 + 文字加亮
                'hover:border-[var(--accent)]/45 hover:text-foreground',
                'hover:bg-[var(--accent)]/8',
                'hover:shadow-[0_0_22px_-8px_var(--accent),0_8px_24px_-14px_color-mix(in_srgb,var(--accent)_60%,transparent),inset_0_1px_0_color-mix(in_srgb,var(--accent)_22%,transparent)]',
                // active 状态：更明显的边框 + 主题色背景 + 字体加粗
                isActive &&
                  'border-[var(--accent)]/65 bg-[var(--accent)]/14 text-[var(--accent)] font-semibold shadow-[0_0_24px_-6px_var(--accent),inset_0_1px_0_color-mix(in_srgb,var(--accent)_28%,transparent)]',
              )}
            >
              {/* 顶部细光带：仅展开状态显示（折叠时宽度太窄不显示） */}
              {!collapsed && (
                <span
                  aria-hidden
                  className="pointer-events-none absolute inset-x-3 top-0 h-px bg-gradient-to-r from-transparent via-[var(--accent)] to-transparent opacity-0 transition-all duration-300 group-hover/nav:opacity-90 group-hover/nav:scale-x-100 scale-x-50"
                />
              )}
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
              {/* 文字：折叠时隐藏 */}
              {!collapsed && (
                <span className="relative z-[1] flex-1">{item.label}</span>
              )}
            </NavLink>
          );

          // 折叠状态下：用 Tooltip 包裹，hover 显示菜单名称（即时显示，无原生 title 延迟）
          if (collapsed) {
            return (
              <Tooltip key={item.path} content={item.label} side="right" delayMs={150}>
                {navLink}
              </Tooltip>
            );
          }

          return navLink;
        })}
      </div>

      {/* 底部版权：折叠时隐藏文字 */}
      <div
        className={cn(
          'border-t border-border/40',
          collapsed ? 'p-2' : 'p-4',
        )}
      >
        {!collapsed && (
          <p className="text-xs text-muted-foreground text-center">
            © 2024 OpenVPN Admin
          </p>
        )}
      </div>
    </nav>
  );
}
