import { Palette, User } from 'lucide-react';
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

export function TopBar() {
  const { theme, setTheme } = useTheme();
  const { user, logout } = useAuth();

  // 自定义头像优先；其次 dicebear 兜底
  const avatarSeed = user?.email || user?.username || 'U';
  const parsedAvatar = parseAvatarValue(user?.avatar, avatarSeed);
  const avatarSrc = parsedAvatar.kind === 'none' ? defaultAvatarUrl(avatarSeed) : parsedAvatar.url;

  return (
    <header className="h-16 border-b bg-card flex items-center px-6 sticky top-0 z-10 relative">
      {/* 左侧占位，保持主题切换 / 通知 / 头像靠右 */}
      <div className="flex items-center gap-3 min-w-[120px]">
        {/* 当前没有左侧导航；保留占位方便未来扩展 */}
      </div>

      {/* 顶部呼吸灯：OpenVPN Management 异常时实时提醒，居中显示更醒目 */}
      <div className="flex-1 flex items-center justify-center">
        <ManagementStatus />
      </div>

      <div className="flex items-center gap-4 min-w-[120px] justify-end">
        {/* GitHub 仓库入口 */}
        <a
          href="https://github.com/xinglinglove1029/openvpn"
          target="_blank"
          rel="noopener noreferrer"
          className="text-muted-foreground hover:text-foreground transition-colors"
          title="GitHub 仓库"
        >
          <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
            <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
          </svg>
        </a>

        {/* 主题切换：下拉选择，支持 6 个主题 */}
        <div className="flex items-center gap-2">
          <Palette className="w-4 h-4 text-muted-foreground" />
          <Select value={theme} onValueChange={(v) => setTheme(v as ThemeKey)}>
            <SelectTrigger className="w-[130px] h-9" aria-label="切换主题">
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

        {/* 通知按钮（真实未读数 + WebSocket 实时推送） */}
        <NotificationBell />

        {/* 用户菜单 */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="flex items-center gap-2 px-2">
              <Avatar className="w-8 h-8">
                <AvatarImage src={avatarSrc} alt="头像" />
                <AvatarFallback>
                  {user?.name?.[0] || user?.username?.[0] || 'U'}
                </AvatarFallback>
              </Avatar>
              <span className="text-sm font-medium">
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