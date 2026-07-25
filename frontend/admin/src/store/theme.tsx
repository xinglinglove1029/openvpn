import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';

export type ThemeKey = 'midnight' | 'aurora' | 'emerald' | 'daylight';

interface ThemeContextType {
  theme: ThemeKey;
  setTheme: (theme: ThemeKey) => void;
  toggleTheme: () => void;
}

const themes: ThemeKey[] = ['midnight', 'aurora', 'emerald', 'daylight'];

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeKey>(() => {
    const stored = localStorage.getItem('openvpn-admin-theme') as ThemeKey;
    return themes.includes(stored) ? stored : 'midnight';
  });

  useEffect(() => {
    const root = document.documentElement;
    root.setAttribute('data-theme', theme);

    // 浅色主题使用 .light，深色主题（midnight / aurora / emerald）使用 .dark，
    // 让 shadcn/ui 主题变量（--popover、--card、--background 等）正确切换。
    // 否则未在下方覆盖的 CSS 变量会停留在 :root 默认值（白色），下拉菜单会全白。
    if (theme === 'daylight') {
      root.classList.add('light');
      root.classList.remove('dark');
    } else {
      root.classList.add('dark');
      root.classList.remove('light');
    }

    // 应用主题颜色到 CSS 变量
    const themeColors = {
      midnight: {
        background: '#0a0e1a',
        foreground: '#e4e7f0',
        primary: '#3b82f6',
        secondary: '#6366f1',
        accent: '#06b6d4',
        muted: '#475569',
        border: '#1e293b',
        card: '#111827',
      },
      aurora: {
        background: '#0f0a1a',
        foreground: '#f0e7f4',
        primary: '#a855f7',
        secondary: '#ec4899',
        accent: '#06b6d4',
        muted: '#6b5b7a',
        border: '#2d1f3d',
        card: '#1a1125',
      },
      emerald: {
        background: '#0a1a14',
        foreground: '#e7f0e9',
        primary: '#10b981',
        secondary: '#06b6d4',
        accent: '#22c55e',
        muted: '#4b6b5a',
        border: '#1e3d2f',
        card: '#112a1e',
      },
      daylight: {
        background: '#f8fafc',
        foreground: '#0f172a',
        primary: '#3b82f6',
        secondary: '#6366f1',
        accent: '#06b6d4',
        muted: '#64748b',
        border: '#e2e8f0',
        card: '#ffffff',
      },
    };

    const colors = themeColors[theme];
    Object.entries(colors).forEach(([key, value]) => {
      root.style.setProperty(`--${key}`, value);
    });

    // 设置前景色
    root.style.setProperty('--primary-foreground', theme === 'daylight' ? '#ffffff' : '#ffffff');
    root.style.setProperty('--secondary-foreground', theme === 'daylight' ? '#ffffff' : '#ffffff');
    root.style.setProperty('--accent-foreground', theme === 'daylight' ? '#0f172a' : '#0f172a');
    root.style.setProperty('--muted-foreground', theme === 'daylight' ? '#64748b' : '#94a3b8');
    root.style.setProperty('--card-foreground', theme === 'daylight' ? '#0f172a' : '#e4e7f0');
    root.style.setProperty('--input', theme === 'daylight' ? '#e2e8f0' : '#1e293b');
    root.style.setProperty('--ring', colors.primary);
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