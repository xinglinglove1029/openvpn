import { Moon, Sun, User } from 'lucide-react';
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
import { useTheme } from '../store/theme';
import { useAuth } from '../store/auth';
import { defaultAvatarUrl, parseAvatarValue } from '../components/AvatarPicker';
import { NotificationBell } from '../components/NotificationBell';
import { ManagementStatus } from '../components/ManagementStatus';

export function TopBar() {
  const { theme, toggleTheme } = useTheme();
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
        {/* 主题切换 */}
        <Button
          variant="ghost"
          size="icon"
          onClick={toggleTheme}
          title={theme === 'daylight' ? '切换到深色模式' : '切换到浅色模式'}
        >
          {theme === 'daylight' ? (
            <Moon className="w-5 h-5" />
          ) : (
            <Sun className="w-5 h-5" />
          )}
        </Button>

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