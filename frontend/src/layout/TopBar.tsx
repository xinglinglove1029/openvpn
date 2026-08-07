import { Palette, User, Menu, PanelLeftClose, PanelLeftOpen, Check } from 'lucide-react';
import { Link } from 'react-router-dom';
import { Button } from '../ui/button';
import { Avatar, AvatarFallback, AvatarImage } from '../ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
import { useTheme, themeLabels, type ThemeKey } from '../store/theme';
import { useAuth } from '../store/auth';
import { defaultAvatarUrl, parseAvatarValue } from '../components/AvatarPicker';
import { NotificationBell } from '../components/NotificationBell';
import { ManagementStatus } from '../components/ManagementStatus';
import { useIsMobile } from '@/hooks/useIsMobile';

interface TopBarProps {
  /** 移动端汉堡菜单点击回调，用于切换 Sidebar 抽屉 */
  onMenuClick?: () => void;
  /** 桌面端 Sidebar 是否折叠（仅图标模式） */
  sidebarCollapsed?: boolean;
  /** 桌面端折叠/展开切换回调 */
  onToggleCollapse?: () => void;
}

export function TopBar({ onMenuClick, sidebarCollapsed, onToggleCollapse }: TopBarProps) {
  const { theme, setTheme } = useTheme();
  const { user, logout } = useAuth();
  const isMobile = useIsMobile();

  const avatarSeed = user?.email || user?.username || 'U';
  const parsedAvatar = parseAvatarValue(user?.avatar, avatarSeed);
  const avatarSrc = parsedAvatar.kind === 'none' ? defaultAvatarUrl(avatarSeed) : parsedAvatar.url;

  return (
    <header className={`${isMobile ? 'h-12' : 'h-14 sm:h-16'} border-b border-border/40 bg-card/60 backdrop-blur-xl flex items-center px-2 sm:px-6 sticky top-0 z-10 relative gap-1 sm:gap-2`}>
      {/* 左侧：菜单切换按钮
          - 移动端（<lg）：汉堡按钮控制抽屉
          - 桌面端（lg+）：折叠/展开 Sidebar */}
      <div className="flex items-center gap-1 sm:gap-2 min-w-0">
        <Button
          variant="ghost"
          size="icon"
          className={`lg:hidden shrink-0 ${isMobile ? 'h-11 w-11' : 'h-9 w-9'}`}
          aria-label="打开菜单"
          onClick={onMenuClick}
        >
          <Menu className={isMobile ? 'h-6 w-6' : 'h-5 w-5'} />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="hidden lg:inline-flex h-9 w-9 shrink-0"
          aria-label={sidebarCollapsed ? '展开菜单' : '收起菜单'}
          title={sidebarCollapsed ? '展开菜单' : '收起菜单'}
          onClick={onToggleCollapse}
        >
          {sidebarCollapsed ? (
            <PanelLeftOpen className="h-5 w-5" />
          ) : (
            <PanelLeftClose className="h-5 w-5" />
          )}
        </Button>
      </div>

      <div className="flex-1 flex items-center justify-center min-w-0">
        <ManagementStatus />
      </div>

      <div className="flex items-center gap-1 sm:gap-4 justify-end shrink-0">
        <a
          href="https://github.com/xinglinglove1029/openvpn"
          target="_blank"
          rel="noopener noreferrer"
          className="text-muted-foreground hover:text-foreground transition-colors hidden sm:inline-flex"
          title="GitHub 仓库"
          aria-label="GitHub 仓库"
        >
          <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
          </svg>
        </a>

        {isMobile ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-11 w-11"
                aria-label="切换主题"
              >
                <Palette className="h-5 w-5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              {(Object.keys(themeLabels) as ThemeKey[]).map((key) => (
                <DropdownMenuItem
                  key={key}
                  onClick={() => setTheme(key)}
                  className="flex items-center justify-between"
                >
                  <span>{themeLabels[key]}</span>
                  {theme === key && <Check className="h-4 w-4 text-[var(--accent)]" />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <div className="flex items-center gap-1.5 sm:gap-2">
            <Palette className="w-4 h-4 text-muted-foreground hidden sm:block" />
            <Select value={theme} onValueChange={(v) => setTheme(v as ThemeKey)}>
              <SelectTrigger className="w-[110px] sm:w-[130px] h-9" aria-label="切换主题">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(Object.keys(themeLabels) as ThemeKey[]).map((key) => (
                  <SelectItem key={key} value={key}>
                    {themeLabels[key]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        <NotificationBell />

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className={`flex items-center gap-2 px-2 ${isMobile ? 'h-11 min-h-[44px]' : ''}`}>
              <Avatar className={isMobile ? 'w-9 h-9' : 'w-8 h-8'}>
                <AvatarImage src={avatarSrc} alt="头像" />
                <AvatarFallback>
                  {user?.name?.[0] || user?.username?.[0] || 'U'}
                </AvatarFallback>
              </Avatar>
              <span className="text-sm font-medium hidden sm:inline">
                {user?.name || user?.username || '用户'}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel>
              <div className="flex flex-col space-y-1">
                <p className="text-sm font-medium">{user?.name || user?.username}</p>
                <p className="text-xs text-muted-foreground">{user?.email}</p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <Link to="/profile" className="flex items-center gap-2 cursor-pointer">
              <DropdownMenuItem>
                <User className="w-4 h-4" />
                <span>个人设置</span>
              </DropdownMenuItem>
            </Link>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={logout} className="text-red-600 cursor-pointer">
              退出登录
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}