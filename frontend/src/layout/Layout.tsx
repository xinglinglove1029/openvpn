import { useEffect } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { BackgroundScene } from '@/components/BackgroundScene';
import { useAuth } from '@/store/auth';

export function Layout() {
  const { user, isLoading } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    if (!isLoading && !user) {
      const next = encodeURIComponent(location.pathname + location.search);
      navigate(`/login?next=${next}`, { replace: true });
    }
  }, [isLoading, user, navigate, location.pathname, location.search]);

  // 加载中或未登录时显示骨架占位，避免页面闪烁
  if (isLoading || !user) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--accent)]" />
      </div>
    );
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
