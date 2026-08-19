import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';

export type ThemeKey = 'midnight' | 'aurora' | 'emerald' | 'daylight' | 'amber-glass' | 'deep-blue';

interface ThemeContextType {
  theme: ThemeKey;
  setTheme: (theme: ThemeKey) => void;
  toggleTheme: () => void;
}

const themes: ThemeKey[] = ['midnight', 'aurora', 'emerald', 'daylight', 'amber-glass', 'deep-blue'];

// 主题中文显示名（供下拉选择使用）
export const themeLabels: Record<ThemeKey, string> = {
  midnight: '午夜蓝',
  aurora: '极光紫',
  emerald: '翡翠绿',
  daylight: '日光白',
  'amber-glass': '琥珀玻璃',
  'deep-blue': '深蓝玻璃',
};

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeKey>(() => {
    const stored = localStorage.getItem('openvpn-admin-theme') as ThemeKey;
    return themes.includes(stored) ? stored : 'midnight';
  });

  useEffect(() => {
    const root = document.documentElement;
    root.setAttribute('data-theme', theme);

    // 浅色主题使用 .light，深色主题（midnight / aurora / emerald / amber-glass / deep-blue）使用 .dark，
    // 让 shadcn/ui 主题变量（--popover、--card、--background 等）正确切换。
    // 否则未在下方覆盖的 CSS 变量会停留在 :root 默认值（白色），下拉菜单会全白。
    if (theme === 'daylight') {
      root.classList.add('light');
      root.classList.remove('dark');
    } else {
      root.classList.add('dark');
      root.classList.remove('light');
    }

    // 主题令牌统一使用可直接渲染的 CSS 颜色值。Tailwind v4 的主题映射直接引用
    // 这些变量，避免把 #hex 再包进 hsl(...) 后导致 Popover / Calendar 等组件失效。
    const themeColors: Record<ThemeKey, Record<string, string>> = {
      midnight: {
        background: '#0a0e1a', foreground: '#e4e7f0', card: '#111827', 'card-foreground': '#e4e7f0',
        popover: '#111827', 'popover-foreground': '#e4e7f0', primary: '#3b82f6', 'primary-foreground': '#ffffff',
        secondary: '#3730a3', 'secondary-foreground': '#ffffff', muted: '#1e293b', 'muted-foreground': '#94a3b8',
        accent: '#06b6d4', 'accent-foreground': '#082f49', destructive: '#ef4444', 'destructive-foreground': '#ffffff',
        border: '#1e293b', input: '#1e293b', ring: '#3b82f6',
        'sidebar-background': '#0f172a', 'sidebar-foreground': '#e4e7f0', 'sidebar-primary': '#3b82f6',
        'sidebar-primary-foreground': '#ffffff', 'sidebar-accent': '#1e293b', 'sidebar-accent-foreground': '#e4e7f0',
        'sidebar-border': '#1e293b', 'sidebar-ring': '#3b82f6',
      },
      aurora: {
        background: '#0f0a1a', foreground: '#f0e7f4', card: '#1a1125', 'card-foreground': '#f0e7f4',
        popover: '#1a1125', 'popover-foreground': '#f0e7f4', primary: '#a855f7', 'primary-foreground': '#ffffff',
        secondary: '#db2777', 'secondary-foreground': '#ffffff', muted: '#2d1f3d', 'muted-foreground': '#c4b5d0',
        accent: '#06b6d4', 'accent-foreground': '#083344', destructive: '#ef4444', 'destructive-foreground': '#ffffff',
        border: '#2d1f3d', input: '#2d1f3d', ring: '#a855f7',
        'sidebar-background': '#160d22', 'sidebar-foreground': '#f0e7f4', 'sidebar-primary': '#a855f7',
        'sidebar-primary-foreground': '#ffffff', 'sidebar-accent': '#2d1f3d', 'sidebar-accent-foreground': '#f0e7f4',
        'sidebar-border': '#2d1f3d', 'sidebar-ring': '#a855f7',
      },
      emerald: {
        background: '#0a1a14', foreground: '#e7f0e9', card: '#112a1e', 'card-foreground': '#e7f0e9',
        popover: '#112a1e', 'popover-foreground': '#e7f0e9', primary: '#10b981', 'primary-foreground': '#052e22',
        secondary: '#0891b2', 'secondary-foreground': '#061128', muted: '#1e3d2f', 'muted-foreground': '#a7c8b3',
        accent: '#22c55e', 'accent-foreground': '#052e16', destructive: '#ef4444', 'destructive-foreground': '#ffffff',
        border: '#1e3d2f', input: '#1e3d2f', ring: '#10b981',
        'sidebar-background': '#0d241a', 'sidebar-foreground': '#e7f0e9', 'sidebar-primary': '#10b981',
        'sidebar-primary-foreground': '#052e22', 'sidebar-accent': '#1e3d2f', 'sidebar-accent-foreground': '#e7f0e9',
        'sidebar-border': '#1e3d2f', 'sidebar-ring': '#10b981',
      },
      daylight: {
        background: '#f8fafc', foreground: '#0f172a', card: '#ffffff', 'card-foreground': '#0f172a',
        popover: '#ffffff', 'popover-foreground': '#0f172a', primary: '#2563eb', 'primary-foreground': '#ffffff',
        secondary: '#4f46e5', 'secondary-foreground': '#ffffff', muted: '#e2e8f0', 'muted-foreground': '#64748b',
        accent: '#06b6d4', 'accent-foreground': '#083344', destructive: '#dc2626', 'destructive-foreground': '#ffffff',
        border: '#e2e8f0', input: '#e2e8f0', ring: '#2563eb',
        'sidebar-background': '#ffffff', 'sidebar-foreground': '#0f172a', 'sidebar-primary': '#2563eb',
        'sidebar-primary-foreground': '#ffffff', 'sidebar-accent': '#e2e8f0', 'sidebar-accent-foreground': '#0f172a',
        'sidebar-border': '#e2e8f0', 'sidebar-ring': '#2563eb',
      },
      'amber-glass': {
        background: '#08070a', foreground: '#f5e9d4', card: '#0f0c08', 'card-foreground': '#f5e9d4',
        popover: '#0f0c08', 'popover-foreground': '#f5e9d4', primary: '#f59e0b', 'primary-foreground': '#291506',
        secondary: '#d97706', 'secondary-foreground': '#291506', muted: '#2a1f10', 'muted-foreground': '#d6c29e',
        accent: '#fbbf24', 'accent-foreground': '#291506', destructive: '#ef4444', 'destructive-foreground': '#ffffff',
        border: '#2a1f10', input: '#2a1f10', ring: '#f59e0b',
        'sidebar-background': '#0f0c08', 'sidebar-foreground': '#f5e9d4', 'sidebar-primary': '#f59e0b',
        'sidebar-primary-foreground': '#291506', 'sidebar-accent': '#2a1f10', 'sidebar-accent-foreground': '#f5e9d4',
        'sidebar-border': '#2a1f10', 'sidebar-ring': '#f59e0b',
      },
      'deep-blue': {
        background: '#0a1830', foreground: '#dbe7ff', card: '#0f2147', 'card-foreground': '#dbe7ff',
        popover: '#0f2147', 'popover-foreground': '#dbe7ff', primary: '#3b82f6', 'primary-foreground': '#ffffff',
        secondary: '#2563eb', 'secondary-foreground': '#ffffff', muted: '#1e3a5f', 'muted-foreground': '#a8c1e8',
        accent: '#60a5fa', 'accent-foreground': '#061128', destructive: '#ef4444', 'destructive-foreground': '#ffffff',
        border: '#1e3a5f', input: '#1e3a5f', ring: '#3b82f6',
        'sidebar-background': '#0a1830', 'sidebar-foreground': '#dbe7ff', 'sidebar-primary': '#3b82f6',
        'sidebar-primary-foreground': '#ffffff', 'sidebar-accent': '#1e3a5f', 'sidebar-accent-foreground': '#dbe7ff',
        'sidebar-border': '#1e3a5f', 'sidebar-ring': '#3b82f6',
      },
    };

    Object.entries(themeColors[theme]).forEach(([key, value]) => {
      root.style.setProperty(`--${key}`, value);
    });
    root.style.setProperty('--radius', '0.5rem');

    localStorage.setItem('openvpn-admin-theme', theme);
  }, [theme]);

  const setTheme = (newTheme: ThemeKey) => {
    setThemeState(newTheme);
  };

  const toggleTheme = () => {
    const currentIndex = themes.indexOf(theme);
    const nextIndex = (currentIndex + 1) % themes.length;
    setThemeState(themes[nextIndex]);
  };

  return (
    <ThemeContext.Provider value={{ theme, setTheme, toggleTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
}