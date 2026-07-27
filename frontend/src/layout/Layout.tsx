import { useEffect } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { BackgroundScene } from '@/components/BackgroundScene';
import { useAuth } from '@/store/auth';

// 路由 → 必备菜单权限 code 映射
// 未在此映射中的路径（如 /overview、/profile）不做权限校验
const pathPermissionMap: { path: string; permission: string }[] = [
  { path: '/users', permission: 'menu:users' },
  { path: '/clients', permission: 'menu:clients' },
  { path: '/firewall', permission: 'menu:firewall' },
  { path: '/history', permission: 'menu:history' },
  { path: '/certs', permission: 'menu:certs' },
  { path: '/audit', permission: 'menu:audit' },
  { path: '/settings', permission: 'menu:settings' },
  { path: '/channels', permission: 'menu:channels' },
  { path: '/notifications', permission: 'menu:notifications' },
  { path: '/roles', permission: 'menu:roles' },
];

function getRequiredPermission(pathname: string): string | undefined {
  // 精确匹配优先，前缀匹配兜底
  const exact = pathPermissionMap.find((item) => pathname === item.path);
  if (exact) return exact.permission;
  const prefix = pathPermissionMap.find((item) => pathname.startsWith(item.path + '/'));
  return prefix?.permission;
}

export function Layout() {
  const { user, isLoading, hasPermission } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  // 未登录跳转登录页
  useEffect(() => {
    if (!isLoading && !user) {
      const next = encodeURIComponent(location.pathname + location.search);
      navigate(`/login?next=${next}`, { replace: true });
    }
  }, [isLoading, user, navigate, location.pathname, location.search]);

  // RBAC：路由守卫，无对应 menu 权限则重定向到 /overview
  // - 跳转前检查 menu:overview 权限，避免用户连概览也无权限时陷入空白页卡死
  // - 连 menu:overview 都没权限时尝试 /profile（所有登录用户必备）
  // - 两者都无权限时不再跳转，由下方无权限占位页渲染
  useEffect(() => {
    if (!user || isLoading) return;
    const required = getRequiredPermission(location.pathname);
    if (!required || hasPermission(required)) return;
    // 当前路径无权限，尝试跳转到有权限的默认页
    if (hasPermission('menu:overview')) {
      navigate('/overview', { replace: true });
    } else if (hasPermission('menu:profile')) {
      navigate('/profile', { replace: true });
    }
    // 两者都无权限时不跳转，由下方占位页处理
  }, [user, isLoading, location.pathname, navigate, hasPermission]);

  // 加载中或未登录时显示骨架占位，避免页面闪烁
  if (isLoading || !user) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--accent)]" />
      </div>
    );
  }

  // RBAC：用户对当前路径无权限且连默认跳转目标也无权限时，渲染"无访问权限"占位页
  // 触发场景：admin 移除了角色的全部菜单权限
  const currentRequired = getRequiredPermission(location.pathname);
  if (currentRequired && !hasPermission(currentRequired)) {
    // 仅当 overview 与 profile 都无权限时才显示占位页（否则守卫会跳转过去）
    if (!hasPermission('menu:overview') && !hasPermission('menu:profile')) {
      return (
        <div className="flex h-screen items-center justify-center bg-background">
          <div className="text-center space-y-3">
            <div className="text-xl font-semibold text-foreground">无访问权限</div>
            <div className="text-sm text-muted-foreground">您当前的角色未分配任何可访问的菜单，请联系管理员</div>
          </div>
        </div>
      );
    }
  }

  return (
    <div className="flex h-screen bg-background overflow-hidden relative">
      {/* 全屏 three.js 背景层（z-0） */}
      <BackgroundScene />
      {/* 遮罩让背景不抢戏（z-5） */}
      <div className="app-bg-mask pointer-events-none absolute inset-0 z-[5]" aria-hidden="true" />
      {/* Sidebar 必须置于顶层（z-20），不被背景/遮罩盖住 */}
      <div className="relative z-20">
        <Sidebar />
      </div>
      <div className="flex-1 flex flex-col overflow-hidden relative z-10">
        <TopBar />
        <main className="flex-1 overflow-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
