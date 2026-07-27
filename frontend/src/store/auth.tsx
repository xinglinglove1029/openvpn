import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import type { ClientUserInfo } from '../types';

interface LoginResponse {
  message: string;
  redirect?: string;
  user?: ClientUserInfo;
  mfaRequired?: boolean;
}

interface AuthContextType {
  user: ClientUserInfo | null;
  isLoading: boolean;
  login: (user: ClientUserInfo, remember?: boolean) => void;
  loginWithCredentials: (username: string, password: string, remember?: boolean) => Promise<LoginResponse>;
  logout: () => Promise<void>;
  updateUser: (user: Partial<ClientUserInfo>) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const STORAGE_KEY = 'openvpn-admin-user';

function readStoredUser(): ClientUserInfo | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as ClientUserInfo;
    // 允许 id=0 的管理员账号（数据库无对应记录）
    if (parsed && typeof parsed === 'object' && parsed.username && typeof parsed.id === 'number') {
      return parsed;
    }
  } catch {
    window.localStorage.removeItem(STORAGE_KEY);
  }
  return null;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<ClientUserInfo | null>(() => readStoredUser());
  const [isLoading, setIsLoading] = useState(false);

  // 监听跨标签页的 localStorage 变化，保持登录态一致
  useEffect(() => {
    function handleStorage(event: StorageEvent) {
      if (event.key !== STORAGE_KEY) return;
      if (event.newValue) {
        try {
          setUser(JSON.parse(event.newValue) as ClientUserInfo);
        } catch {
          setUser(null);
        }
      } else {
        setUser(null);
      }
    }
    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, []);

  // 简单的凭据登录：仅当后端返回 user 对象时才算登录成功
  const loginWithCredentials = async (
    username: string,
    password: string,
    remember = false,
  ): Promise<LoginResponse> => {
    const response = await fetch('/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8' },
      credentials: 'same-origin',
      body: new URLSearchParams({ username, password, remember: remember ? 'on' : '' }).toString(),
    });

    const contentType = response.headers.get('content-type') || '';
    const payload: LoginResponse = contentType.includes('application/json')
      ? await response.json()
      : { message: response.statusText };

    if (!response.ok) {
      throw new Error(payload?.message || `登录失败（${response.status}）`);
    }

    if (payload.user) {
      login(payload.user, remember);
    }

    return payload;
  };

  // 直接注入已登录用户信息（后端已通过其他方式完成认证）
  // 始终写入 localStorage，确保新 tab / 刷新页面后登录态不丢失
  const login = (nextUser: ClientUserInfo, _remember = true) => {
    setUser(nextUser);
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(nextUser));
  };

  const logout = async () => {
    try {
      await fetch('/logout', { method: 'GET', credentials: 'same-origin' });
    } catch {
      // 忽略网络错误，前端仍继续清理本地状态
    }
    setUser(null);
    window.localStorage.removeItem(STORAGE_KEY);
    // 强制整页刷新，跳回登录页（避免 SPA 内存中残留 user 状态）
    window.location.href = '/login';
  };

  const updateUser = (updates: Partial<ClientUserInfo>) => {
    setUser((current) => {
      if (!current) return current;
      const next = { ...current, ...updates };
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
      return next;
    });
  };

  return (
    <AuthContext.Provider
      value={{ user, isLoading, login, loginWithCredentials, logout, updateUser }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
