import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense, ReactNode } from 'react';
import { ThemeProvider, useTheme } from './store/theme';
import { AuthProvider } from './store/auth';
import { SystemStatusProvider } from './store/systemStatus';
import { Layout } from './layout/Layout';
import { Toaster } from './ui/sonner';

// 懒加载页面组件
const LoginPage = lazy(() => import('./pages/Login'));
const OverviewPage = lazy(() => import('./pages/Overview'));
const UsersPage = lazy(() => import('./pages/Users'));
const ClientsPage = lazy(() => import('./pages/Clients'));
const FirewallPage = lazy(() => import('./pages/Firewall'));
const HistoryPage = lazy(() => import('./pages/History'));
const CertsPage = lazy(() => import('./pages/Certs'));
const AuditPage = lazy(() => import('./pages/Audit'));
const SettingsPage = lazy(() => import('./pages/Settings'));
const NotificationsPage = lazy(() => import('./pages/Notifications'));
const ChannelProvidersPage = lazy(() => import('./pages/ChannelProviders'));
const ProfilePage = lazy(() => import('./pages/Profile'));
const RolesPage = lazy(() => import('./pages/Roles'));
const PermissionsPage = lazy(() => import('./pages/Permissions'));

// 加载指示器
function LoadingSpinner() {
  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--accent)]"></div>
    </div>
  );
}

function SuspenseWrap({ children }: { children: ReactNode }) {
  return <Suspense fallback={<LoadingSpinner />}>{children}</Suspense>;
}

function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <SystemStatusProvider>
          <BrowserRouter>
            <Routes>
              {/* 登录页 - 无需认证 */}
              <Route
                path="/login"
                element={<SuspenseWrap><LoginPage /></SuspenseWrap>}
              />

              {/* 主应用 - 需要布局 */}
              <Route path="/" element={<Layout />}>
                <Route index element={<Navigate to="/overview" replace />} />
                <Route path="overview" element={<SuspenseWrap><OverviewPage /></SuspenseWrap>} />
                <Route path="users" element={<SuspenseWrap><UsersPage /></SuspenseWrap>} />
                <Route path="clients" element={<SuspenseWrap><ClientsPage /></SuspenseWrap>} />
                <Route path="firewall" element={<SuspenseWrap><FirewallPage /></SuspenseWrap>} />
                <Route path="history" element={<SuspenseWrap><HistoryPage /></SuspenseWrap>} />
                <Route path="certs" element={<SuspenseWrap><CertsPage /></SuspenseWrap>} />
                <Route path="audit" element={<SuspenseWrap><AuditPage /></SuspenseWrap>} />
                <Route path="settings" element={<SuspenseWrap><SettingsPage /></SuspenseWrap>} />
                <Route path="notifications" element={<SuspenseWrap><NotificationsPage /></SuspenseWrap>} />
                <Route path="channels" element={<SuspenseWrap><ChannelProvidersPage /></SuspenseWrap>} />
                <Route path="profile" element={<SuspenseWrap><ProfilePage /></SuspenseWrap>} />
                <Route path="roles" element={<SuspenseWrap><RolesPage /></SuspenseWrap>} />
                <Route path="permissions" element={<SuspenseWrap><PermissionsPage /></SuspenseWrap>} />
              </Route>

              {/* 404 */}
              <Route path="*" element={<Navigate to="/overview" replace />} />
            </Routes>
          </BrowserRouter>
          {/* 顶部居中 toast：所有保存/操作反馈统一走这里，不在右侧抽屉弹 */}
          <ThemedToaster />
        </SystemStatusProvider>
      </AuthProvider>
    </ThemeProvider>
  );
}

/**
 * 根据当前主题动态决定 sonner Toaster 的 theme。
 * 浅色主题（daylight）走 light，其余（midnight / aurora / emerald）走 dark。
 */
function ThemedToaster() {
  const { theme } = useTheme();
  return <Toaster theme={theme === 'daylight' ? 'light' : 'dark'} richColors />;
}

export default App;
