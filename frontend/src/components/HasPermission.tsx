import { type ReactNode } from 'react';
import { useAuth } from '@/store/auth';

/**
 * RBAC 按钮级权限包裹组件
 *
 * @example
 * <HasPermission code="user:create">
 *   <Button>添加用户</Button>
 * </HasPermission>
 *
 * @example
 * // 自定义无权限时的回退内容
 * <HasPermission code="user:delete" fallback={<span className="text-muted-foreground">无权限</span>}>
 *   <Button>删除</Button>
 * </HasPermission>
 */
export function HasPermission({
  code,
  children,
  fallback = null,
}: {
  code: string;
  children: ReactNode;
  fallback?: ReactNode;
}) {
  const { hasPermission } = useAuth();
  if (!hasPermission(code)) return <>{fallback}</>;
  return <>{children}</>;
}
