import { useEffect, useMemo, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { BackgroundScene } from '@/components/BackgroundScene';
import { useAuth } from '@/store/auth';
import { useIsMobile } from '@/hooks/useIsMobile';
import AIWidget from '@/components/AIWidget';

export function Layout() {
  const { user, isLoading, hasPermission, permissionTree } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  // 移动端 Sidebar 抽屉开关
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const isMobile = useIsMobile();
  const [browserFullscreen, setBrowserFullscreen] = useState(() =>
    typeof document !== 'undefined' && Boolean(document.fullscreenElement)
  );
  const isScreenRoute = location.pathname === '/screen' || location.pathname.startsWith('/screen/');
  const screenOnlyMode = browserFullscreen && isScreenRoute;
  // The operations screen contains its own interactive Three.js globe. Do not
  // run a second full-window WebGL animation behind it on normal (non-fullscreen)
  // visits, especially on small 1-core/2-GB deployments.
  const showAmbientEffects = !isScreenRoute;
  // 桌面端 Sidebar 折叠状态（仅图标模式），持久化到 localStorage
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() => {
    try {
      return localStorage.getItem('openvpn-sidebar-collapsed') === 'true';
    } catch {
      return false;
    }
  });

  // 原生全屏下，大屏页面只保留运营看板，避免侧栏、顶部栏及 AI 浮标干扰展示。
  useEffect(() => {
    const syncFullscreen = () => setBrowserFullscreen(Boolean(document.fullscreenElement));
    document.addEventListener('fullscreenchange', syncFullscreen);
    return () => document.removeEventListener('fullscreenchange', syncFullscreen);
  }, []);

  // 折叠状态变更时持久化
  useEffect(() => {
    try {
      localStorage.setItem('openvpn-sidebar-collapsed', String(sidebarCollapsed));
    } catch {
      // 忽略 localStorage 不可用的情况
    }
  }, [sidebarCollapsed]);

  // 路由切换时自动关闭抽屉
  useEffect(() => {
    setSidebarOpen(false);
  }, [location.pathname]);

  // 动态构建 路径 → 菜单权限 code 映射
  // 遍历 permissionTree 中 type=menu 的节点，构建 { path: node.path, permission: node.code } 映射
  // 未在此映射中的路径（如 /overview、/profile）不做权限校验
  const pathPermissionMap = useMemo(() => {
    const map: { path: string; permission: string }[] = [];
    function walk(nodes: typeof permissionTree | undefined) {
      if (!nodes) return;
      for (const node of nodes) {
        if (node.type === 'menu' && node.path && node.code !== 'menu:profile') {
          map.push({ path: node.path, permission: node.code });
        }
        if (node.children && node.children.length) {
          walk(node.children);
        }
      }
    }
    walk(permissionTree);
    return map;
  }, [permissionTree]);

  function getRequiredPermission(pathname: string): string | undefined {
    // 运营大屏不是独立菜单节点，但必须继承概览权限，避免普通用户绕过 RBAC。
    if (pathname === '/screen' || pathname.startsWith('/screen/')) return 'menu:overview';
    // 精确匹配优先，前缀匹配兜底
    const exact = pathPermissionMap.find((item) => pathname === item.path);
    if (exact) return exact.permission;
    const prefix = pathPermissionMap.find((item) => pathname.startsWith(item.path + '/'));
    return prefix?.permission;
  }

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
  }, [user, isLoading, location.pathname, navigate, hasPermission, pathPermissionMap]);

  // 加载中或未登录时显示骨架占位，避免页面闪烁
  if (isLoading || !user) {
    return (
      <div className="flex h-screen h-dvh min-h-screen min-h-dvh items-center justify-center bg-background">
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
        <div className="flex h-screen h-dvh min-h-screen min-h-dvh items-center justify-center bg-background">
          <div className="text-center space-y-3">
            <div className="text-xl font-semibold text-foreground">无访问权限</div>
            <div className="text-sm text-muted-foreground">您当前的角色未分配任何可访问的菜单，请联系管理员</div>
          </div>
        </div>
      );
    }
  }

  return (
    <div className="relative flex h-screen h-dvh min-h-screen min-h-dvh min-w-0 overflow-hidden bg-background">
      {/* 全屏 three.js 背景层（z-0） */}
      {showAmbientEffects && <BackgroundScene />}
      {/* CSS 动态光晕层 - 呼吸+漂浮效果，与全站风格统一
          移动端：减小尺寸/降低透明度，提升性能并避免视觉过载 */}
      {showAmbientEffects && <div aria-hidden className="pointer-events-none absolute inset-0 z-[1]">
        <div
          className={`absolute -top-32 -right-32 w-[28rem] h-[28rem] sm:w-[40rem] sm:h-[40rem] lg:w-[50rem] lg:h-[50rem] rounded-full blur-3xl ${isMobile ? 'opacity-10 dark:opacity-8' : 'opacity-20 dark:opacity-15 sm:opacity-25 sm:dark:opacity-20 lg:opacity-30 lg:dark:opacity-20'} animate-pulse`}
          style={{
            background:
              'radial-gradient(circle, color-mix(in srgb, var(--accent) 35%, transparent) 0%, transparent 70%)',
          }}
        />
        <div
          className={`absolute bottom-0 -left-32 w-[24rem] h-[24rem] sm:w-[32rem] sm:h-[32rem] lg:w-[40rem] lg:h-[40rem] rounded-full blur-3xl ${isMobile ? 'opacity-8 dark:opacity-5' : 'opacity-15 dark:opacity-10 sm:opacity-18 sm:dark:opacity-13 lg:opacity-20 lg:dark:opacity-15'}`}
          style={{
            background:
              'radial-gradient(circle, color-mix(in srgb, var(--accent-2, var(--accent)) 30%, transparent) 0%, transparent 70%)',
            animation: 'float 14s ease-in-out infinite',
          }}
        />
        {!isMobile && (
          <div
            className="hidden sm:block absolute top-1/4 right-1/4 w-72 h-72 rounded-full blur-3xl opacity-15"
            style={{
              background:
                'radial-gradient(circle, color-mix(in srgb, var(--accent-3, var(--accent)) 35%, transparent) 0%, transparent 70%)',
              animation: 'float 18s ease-in-out infinite reverse',
            }}
          />
        )}
      </div>}
      {/* 遮罩让背景不抢戏（z-5） */}
      {showAmbientEffects && <div className="app-bg-mask pointer-events-none absolute inset-0 z-[5]" aria-hidden="true" />}

      {/* 移动端 Sidebar 抽屉遮罩 */}
      {!screenOnlyMode && sidebarOpen && (
        <div
          aria-hidden
          onClick={() => setSidebarOpen(false)}
          className="fixed inset-0 z-30 bg-black/50 backdrop-blur-sm lg:hidden animate-in fade-in duration-200"
          style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
        />
      )}

      {/* Sidebar 容器：
          - lg+ 桌面端常驻显示
          - 移动端为抽屉，由 sidebarOpen 控制滑入/滑出 */}
      {!screenOnlyMode && <div
        data-testid="mobile-sidebar-drawer"
        className={`fixed z-40 lg:z-20 lg:!translate-x-0 lg:!block ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'
        } transition-transform duration-300 ease-out lg:transition-none lg:static inset-y-0 left-0 lg:!flex lg:flex`}
      >
        <Sidebar collapsed={isMobile ? false : sidebarCollapsed} />
      </div>}

      <div className="relative z-10 flex min-w-0 flex-1 basis-0 flex-col overflow-hidden">
        {!screenOnlyMode && <TopBar
          onMenuClick={() => setSidebarOpen((v) => !v)}
          sidebarCollapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed((v) => !v)}
        />}
        <main data-testid="app-main" className={screenOnlyMode ? 'flex-1 min-w-0 overflow-y-auto overflow-x-clip p-0' : 'flex-1 min-w-0 overflow-y-auto overflow-x-clip p-4 sm:p-6'} style={{ paddingBottom: screenOnlyMode ? 0 : 'calc(1rem + env(safe-area-inset-bottom))' }}>
          {/* key 绑定路由路径，切换页面时重新触发 panel-enter 入场动画 */}
          <div key={location.pathname} className="panel-enter">
            <Outlet />
          </div>
        </main>
      </div>

      {/* 全局 AI 助手入口 */}
      {!screenOnlyMode && <AIWidget />}
    </div>
  );
}
