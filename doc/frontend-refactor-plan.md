# 前端架构文档

## 一、技术选型

### 1.1 核心框架
- **React 19**: 函数组件 + Hooks
- **React Router v7**: 路由拆分与懒加载（`react-router-dom`）
- **TypeScript**: 全量类型安全
- **Vite**: 构建工具，开发代理 + 生产嵌入

### 1.2 UI 框架
- **shadcn/ui**: 基于 Radix UI 的高质量组件库
- **Tailwind CSS v4**: 原子化 CSS，通过 `@theme inline` 桥接 CSS 变量
- **lucide-react**: 图标库
- **sonner**: Toast 通知
- **react-hook-form + zod**: 表单与校验

### 1.3 状态管理
- **React Context**: 轻量级全局状态（`auth` / `theme` / `systemStatus`）
- **原生 Hooks**: `useAsync` / `usePagination` / `useNotificationHub`

### 1.4 实时通信
- **WebSocket**: 通过 `notificationHub` 单例管理连接，按 topic 派发消息
- **自动重连**: 断线指数退避重连

## 二、目录结构

```
frontend/admin/src/
├── main.tsx                      # 入口文件，挂载 App.new
├── App.new.tsx                   # 应用根组件（Provider 嵌套 + Router）
├── App.tsx                       # 旧版单文件应用（保留过渡）
├── api.ts                        # API 层（fetch 封装，统一错误处理）
├── types.ts                      # 全局类型定义
│
├── store/                        # 全局状态（Context + Provider）
│   ├── auth.tsx                  # 认证状态：登录/登出/用户信息/跨标签同步
│   ├── theme.tsx                 # 主题状态：4 主题切换 + CSS 变量注入
│   └── systemStatus.tsx          # 系统状态：Management 接口可用性 + 风险列表
│
├── layout/                       # 布局组件
│   ├── Layout.tsx                # 主布局（背景层 + Sidebar + TopBar + Outlet）
│   ├── Sidebar.tsx               # 左侧导航栏（NavLink + 光晕动效）
│   └── TopBar.tsx                # 顶部工具栏（主题切换 + 通知 + 用户菜单）
│
├── components/                   # 业务组件
│   ├── AvatarPicker.tsx          # 头像选择器（预设 + dicebear 兜底）
│   ├── BackgroundScene.tsx       # three.js 全屏背景层
│   ├── CardGlow.tsx              # 卡片光晕悬停效果
│   ├── ConfirmDialog.tsx         # 确认对话框
│   ├── DataTable.tsx             # 通用数据表格
│   ├── DatePickerField.tsx       # 日期选择字段
│   ├── FormField.tsx             # 表单字段（label + input + error）
│   ├── GlowButton.tsx            # 发光按钮
│   ├── HeroOrbitScene.tsx        # 首页 3D 轨道场景
│   ├── ManagementStatus.tsx      # Management 接口呼吸灯
│   ├── NotificationBell.tsx      # 通知铃铛（未读数 + WebSocket）
│   ├── PageHeader.tsx            # 页面标题区
│   ├── PasswordStrength.tsx      # 密码强度指示器
│   ├── StatusBadge.tsx           # 状态徽章
│   ├── SystemMonitor.tsx         # 系统监控面板
│   └── TimeRangePicker.tsx       # 时间范围选择器
│
├── pages/                        # 页面组件（每个目录一个 index.tsx）
│   ├── Login/                    # 登录页（账号密码 / MFA / 首次改密）
│   ├── Overview/                 # 态势总览（统计卡片 + 3D 拓扑 + 在线连接）
│   ├── Users/                    # 账号管理（CRUD + 分组树 + 到期状态）
│   ├── Clients/                  # 客户端管理（在线列表 + 配置编辑）
│   ├── Firewall/                 # 防火墙规则
│   ├── History/                  # 连接历史
│   ├── Certs/                    # 证书管理
│   ├── Audit/                    # 操作审计（筛选 + 分页 + 导出）
│   ├── Settings/                 # 系统设置（基础 / LDAP / OpenVPN 参数）
│   ├── ChannelProviders/         # 通知渠道（钉钉 / 企微 / 飞书 / 邮件 / Webhook）
│   ├── Notifications/            # 站内信（列表 + 已读 / 未读）
│   └── Profile/                  # 个人设置（头像 + 密码）
│
├── hooks/                        # 自定义 Hooks
│   ├── useAsync.ts               # 异步加载（loading / error / data）
│   ├── usePagination.ts          # 分页逻辑（page / pageSize / slice）
│   └── useNotificationHub.ts     # 通知 Hub（WebSocket 状态 + 未读快照）
│
├── lib/                          # 工具函数
│   ├── utils.ts                  # cn() 类名合并
│   ├── format.ts                 # 格式化（字节 / 日期 / 过期状态 / 树构建）
│   ├── validators.ts             # 验证函数（邮箱 / IP / CIDR / 密码强度）
│   └── notificationHub.ts        # WebSocket 单例（连接管理 + topic 订阅）
│
├── ui/                           # shadcn/ui 组件库（Radix UI 封装）
│   ├── alert-dialog.tsx
│   ├── avatar.tsx
│   ├── badge.tsx
│   ├── button.tsx
│   ├── calendar.tsx
│   ├── card.tsx
│   ├── checkbox.tsx
│   ├── dialog.tsx
│   ├── dropdown-menu.tsx
│   ├── form.tsx
│   ├── input.tsx
│   ├── label.tsx
│   ├── popover.tsx
│   ├── progress.tsx
│   ├── radio-group.tsx
│   ├── scroll-area.tsx
│   ├── select.tsx
│   ├── separator.tsx
│   ├── sonner.tsx                # Toast 容器
│   ├── switch.tsx
│   ├── table.tsx
│   ├── tabs.tsx
│   ├── textarea.tsx
│   └── tooltip.tsx
│
└── styles/
    └── index.css                 # 全局样式（Tailwind 主题 + FX 变量 + 组件样式）
```

## 三、应用入口与 Provider 嵌套

```tsx
// main.tsx
import App from './App.new';
import './styles/index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

```tsx
// App.new.tsx —— Provider 嵌套顺序：Theme > Auth > SystemStatus > Router
function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <SystemStatusProvider>
          <BrowserRouter>
            <Routes>...</Routes>
          </BrowserRouter>
          <ThemedToaster />
        </SystemStatusProvider>
      </AuthProvider>
    </ThemeProvider>
  );
}
```

## 四、路由设计

### 4.1 路由配置

所有页面组件通过 `lazy()` 懒加载，`Suspense` 包裹显示加载指示器：

```tsx
const LoginPage = lazy(() => import('./pages/Login'));
const OverviewPage = lazy(() => import('./pages/Overview'));
// ...其余页面同理

<Route path="/login" element={<SuspenseWrap><LoginPage /></SuspenseWrap>} />
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
</Route>
<Route path="*" element={<Navigate to="/overview" replace />} />
```

### 4.2 路由守卫

`Layout` 组件内置认证守卫，未登录时重定向到 `/login?next=...`：

```tsx
// layout/Layout.tsx
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

  if (isLoading || !user) {
    return <div className="flex h-screen items-center justify-center">...</div>;
  }

  return (
    <div className="flex h-screen bg-background overflow-hidden relative">
      <BackgroundScene />
      <div className="app-bg-mask pointer-events-none absolute inset-0 z-[5]" />
      <div className="relative z-20"><Sidebar /></div>
      <div className="flex-1 flex flex-col overflow-hidden relative z-10">
        <TopBar />
        <main className="flex-1 overflow-auto p-6"><Outlet /></main>
      </div>
    </div>
  );
}
```

## 五、状态管理

### 5.1 认证状态（auth.tsx）

```tsx
interface AuthContextType {
  user: ClientUserInfo | null;
  isLoading: boolean;
  login: (user: ClientUserInfo, remember?: boolean) => void;
  loginWithCredentials: (username: string, password: string, remember?: boolean) => Promise<LoginResponse>;
  logout: () => Promise<void>;
  updateUser: (user: Partial<ClientUserInfo>) => void;
}
```

- 登录态持久化到 `localStorage`，支持跨标签页 `storage` 事件同步
- `loginWithCredentials` 走 `/login` POST 表单，后端返回 `user` 对象才算成功
- `logout` 调用后端 `/logout` 后清理本地状态并整页刷新

### 5.2 主题状态（theme.tsx）

```tsx
export type ThemeKey = 'midnight' | 'aurora' | 'emerald' | 'daylight';
```

- 4 套主题：曜石蓝（默认）/ 极光紫 / 青峦绿 / 晨雾白
- 通过 `data-theme` 属性 + `.dark` / `.light` 类名切换
- 主题颜色注入 CSS 变量（`--background` / `--foreground` / `--primary` ...）
- 持久化到 `localStorage`，键名 `openvpn-admin-theme`

### 5.3 系统状态（systemStatus.tsx）

```tsx
export interface SystemStatus {
  managementOk: boolean;   // OpenVPN management 接口可用性
  serverStatus: string;    // 服务器状态文本
  risks: RiskItem[];       // 风险列表
  pushedAt: number;        // 最近推送时间
}
```

- 通过 WebSocket 订阅 `dashboard:stats` 主题
- TopBar 的 `ManagementStatus` 组件消费此状态，异常时呼吸灯提醒

## 六、实时通信（notificationHub）

```typescript
// lib/notificationHub.ts —— WebSocket 单例
class RealtimeHub {
  connect(): void;
  disconnect(): void;
  subscribe<T>(topic: string, handler: (data: T) => void): () => void;
  onState(handler: (state: ConnectionState) => void): () => void;
  onUnread(handler: (snapshot: UnreadSnapshot) => void): () => void;
  onNotification(handler: (item: IncomingNotification) => void): () => void;
  refreshUnread(): Promise<void>;
  markRead(lastReadId?: number): void;
}
export const realtimeHub = new RealtimeHub();
```

- 连接地址：`/ovpn/ws/notifications`
- 断线自动重连，指数退避
- 主题命名规范：`<业务域>:<事件>`，例如 `notify:new` / `dashboard:stats`

## 七、Vite 构建配置

```typescript
// vite.config.ts
export default defineConfig(({ command }) => ({
  resolve: {
    alias: { '@': path.resolve(configDir, './src') },
    dedupe: ['react', 'react-dom'],  // 强制去重 React 副本
  },
  base: command === 'build' ? '/static/admin/' : '/',
  server: {
    proxy: {
      '/login': { target: 'http://127.0.0.1:8888', bypass: ... },
      '/logout': { target: 'http://127.0.0.1:8888' },
      '/mfa': { target: 'http://127.0.0.1:8888' },
      '/ovpn': { target: 'http://127.0.0.1:8888', ws: true },
      '/client': { target: 'http://127.0.0.1:8888', ws: true, bypass: ... },
      '/user/template': 'http://127.0.0.1:8888',
      '/settings': { target: 'http://127.0.0.1:8888', bypass: ... },
      '/email': 'http://127.0.0.1:8888',
      '/static/server': 'http://127.0.0.1:8888',
    },
  },
  build: {
    outDir: '../../internal/openvpnweb/templates/static/admin',  // 直接输出到 Go embed 目录
    rollupOptions: {
      output: {
        entryFileNames: 'assets/app.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: (assetInfo) => assetInfo.name?.endsWith('.css') ? 'assets/app.css' : 'assets/[name][extname]',
      },
    },
  },
}));
```

**关键点：**
- 开发环境 `base: '/'`，生产构建 `base: '/static/admin/'`
- 构建产物直接输出到 `internal/openvpnweb/templates/static/admin`，由 Go `embed` 嵌入
- `bypass` 规则区分 SPA 路由（GET HTML）和 API 请求（GET JSON / POST）
- `dedupe` 避免依赖内联 CJS 版 React 导致 Hooks 报错

## 八、主题系统设计

### 8.1 CSS 变量分层

`styles/index.css` 分三层定义主题变量：

1. **shadcn/ui 层**（`@layer base`）：HSL 格式变量，供 Tailwind 工具类使用
   ```css
   :root { --background: 0 0% 100%; --foreground: 222.2 84% 4.9%; ... }
   .dark { --background: 222.2 84% 4.9%; --foreground: 210 40% 98%; ... }
   ```

2. **FX 装饰层**（`:root`）：Hex / 渐变值，覆盖同名 shadcn 变量
   ```css
   :root {
     --page-bg: #050713;
     --accent: #6df3ff;
     --scroll-thumb: linear-gradient(135deg, ...);
     --panel-bg: linear-gradient(145deg, ...);
   }
   ```

3. **主题变体**（`data-theme="xxx"`）：每个主题独立覆盖 FX 变量
   ```css
   :root[data-theme="aurora"] { --page-bg: #080315; --accent: #ff7adf; ... }
   :root[data-theme="emerald"] { --page-bg: #03120f; --accent: #69f0ae; ... }
   :root[data-theme="daylight"] { --page-bg: #edf4ff; --accent: #1d4ed8; ... }
   ```

### 8.2 主题切换机制

`theme.tsx` 的 `ThemeProvider` 在 `useEffect` 中：
1. 设置 `document.documentElement.setAttribute('data-theme', theme)`
2. 切换 `.dark` / `.light` 类名（让 shadcn/ui 变量正确切换）
3. 注入主题颜色到 CSS 变量
4. 持久化到 `localStorage`

## 九、关键组件

### 9.1 Layout 布局

```
┌─────────────────────────────────────────────┐
│  BackgroundScene (z-0)  ← three.js 全屏背景  │
│  app-bg-mask (z-5)      ← 遮罩层             │
│ ┌─────────┬───────────────────────────────┐ │
│ │ Sidebar │  TopBar (z-10)                │ │
│ │ (z-20)  ├───────────────────────────────┤ │
│ │         │  main (Outlet)                │ │
│ │         │                               │ │
│ └─────────┴───────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 9.2 Sidebar 导航

10 个导航项，每项包含图标 + 文字，支持：
- hover：渐变描边 + 主题色光晕 + 顶部光带 + 左侧高亮条
- active：更明显的边框 + 主题色背景 + 字体加粗

```tsx
const navItems = [
  { path: '/overview', label: '概览', icon: LayoutDashboard },
  { path: '/users', label: '账号管理', icon: Users },
  { path: '/clients', label: '客户端', icon: Smartphone },
  { path: '/firewall', label: '防火墙', icon: Shield },
  { path: '/history', label: '连接历史', icon: History },
  { path: '/certs', label: '证书', icon: FileKey },
  { path: '/audit', label: '操作审计', icon: FileText },
  { path: '/settings', label: '系统设置', icon: Settings },
  { path: '/channels', label: '通知渠道', icon: BellRing },
  { path: '/notifications', label: '站内信', icon: Bell },
];
```

### 9.3 TopBar 工具栏

- 左侧：占位区（未来扩展）
- 居中：`ManagementStatus` 呼吸灯（OpenVPN Management 异常时提醒）
- 右侧：主题切换 + `NotificationBell`（未读数）+ 用户菜单（头像 + 下拉）

## 十、API 层

```typescript
// api.ts
export const api = {
  get: <T>(url: string) => request<T>(url),
  post: <T>(url: string, json?: unknown) => request<T>(url, { method: 'POST', json }),
  postForm: <T>(url: string, form: Record<string, unknown>) => request<T>(url, { method: 'POST', form }),
  put: <T>(url: string, json?: unknown) => request<T>(url, { method: 'PUT', json }),
  del: <T>(url: string) => request<T>(url, { method: 'DELETE' }),
};
```

- 统一 `credentials: 'same-origin'`
- 检测 `response.redirected` 到 `/login` 时自动跳转登录页
- 非 2xx 响应抛出 `Error`，消息取自 `payload.message`

## 十一、自定义 Hooks

### useAsync

```typescript
function useAsync<T>(loader: () => Promise<T>, deps: unknown[] = []): AsyncState<T>
// 返回 { loading, error?, data? }，deps 变化时重新加载
```

### usePagination

```typescript
function usePagination<T>(items: T[], resetKey = '', initialPageSize = 10)
// 返回 { page, pageSize, setPageSize, total, pageCount, start, end, pagedItems, setPage }
```

### useNotificationHub

```typescript
function useNotificationHub()
// 返回 { state, snapshot, latest, refresh, markRead, subscribe }
// 自动跟随 user.username 变化连接/断开 WebSocket
```

## 十二、工具函数

### lib/format.ts

- `formatBytes(value)` — 字节数格式化（B / KB / MB / GB / TB）
- `getClientName(client)` — 客户端名称提取
- `expiryStatus(user)` — 用户到期状态（长期 / 正常 / 即将过期 / 已过期）
- `normalizeList<T>(value, candidates)` — 后端列表数据归一化
- `buildTree(groups, parentId, depth)` — 用户组树构建
- `toDateInputValue(date)` — 日期转 `YYYY-MM-DD`

### lib/validators.ts

- `isValidEmail(value)` — 邮箱格式
- `isValidPort(value)` — 端口（1-65535）
- `isValidIp(value)` / `isValidIpv4` / `isValidIpv6` — IP 地址
- `isValidCidr(value, requiredVersion?)` — CIDR 格式
- `isValidIpOrCidrList(value)` — 逗号分隔 IP/CIDR 列表
- `isValidHost(value)` / `isValidHostPort(value)` — 主机名 / 主机:端口
- `isStrongPassword(value)` — 强密码（≥12 位 + 大小写 + 数字 + 特殊字符）
- `isValidSafeName(value)` — 安全名称（`[A-Za-z0-9._-]{2,64}`）

## 十三、样式规范

### 13.1 全局样式（styles/index.css）

- `@import "tailwindcss"` 引入 Tailwind v4
- `@theme inline` 桥接 CSS 变量到 Tailwind 颜色系统
- `@layer base` 定义 shadcn/ui HSL 变量（`:root` 浅色 / `.dark` 深色）
- FX 装饰变量在 `:root` 和 `data-theme="xxx"` 中定义
- 滚动条样式：`::-webkit-scrollbar` 全局定制，`.tree-list` 局部覆盖

### 13.2 滚动条

```css
::-webkit-scrollbar { width: 10px; height: 10px; }
::-webkit-scrollbar-track { border-radius: 999px; background: var(--scroll-track); }
::-webkit-scrollbar-thumb { border-radius: 999px; background: var(--scroll-thumb); box-shadow: 0 0 16px var(--scroll-glow); }
```

> 注意：为避免横向滚动条悬停闪烁，卡片 hover 不使用 `transform` 位移，滚动条 thumb 不使用 `:hover` 渐变切换。

## 十四、开发与调试

### 14.1 启动开发服务器

```bash
cd frontend/admin
npm install
npm run dev
# 访问 http://127.0.0.1:5173
```

开发服务器自动代理后端接口到 `http://127.0.0.1:8888`，需确保后端服务已启动。

### 14.2 构建生产产物

```bash
cd frontend/admin
npm run build
```

产物直接输出到 `internal/openvpnweb/templates/static/admin/`，由 Go `embed` 嵌入二进制。

### 14.3 路径别名

- `@` → `frontend/admin/src/`
- 导入示例：`import { useAuth } from '@/store/auth'`

## 十五、后续优化建议

1. **性能优化**: 引入 React Query 管理服务端状态，减少重复请求
2. **可访问性**: 补充 ARIA 标签，支持键盘导航
3. **国际化**: 抽取文案到 i18n 资源文件
4. **单元测试**: 使用 Vitest + React Testing Library 编写测试
5. **移动端适配**: 响应式断点优化
6. **旧代码清理**: 移除 `App.tsx` 和 `styles.css`（已被 `App.new.tsx` 和 `styles/index.css` 替代）

## 十六、参考资料

- [React Router v7 文档](https://reactrouter.com/)
- [shadcn/ui 组件库](https://ui.shadcn.com/)
- [Tailwind CSS v4 文档](https://tailwindcss.com/)
- [lucide-react 图标库](https://lucide.dev/)
- [Radix UI Primitives](https://www.radix-ui.com/primitives)
- [Vite 配置](https://vite.dev/config/)
