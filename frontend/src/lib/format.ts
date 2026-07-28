/** 格式化工具函数 */
import type { OnlineClient, UserRecord, GroupRecord } from '../types';

export function formatBytes(value?: number | string | null) {
  if (value === null || value === undefined || value === '') return '0 B';
  const numValue = typeof value === 'number' ? value : Number(String(value).replace(/[^\d.eE+\-]/g, ''));
  if (!Number.isFinite(numValue) || numValue <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = numValue;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const fixed = Number(size.toFixed(2));
  return `${fixed} ${units[unitIndex]}`;
}

export function getClientName(client: OnlineClient) {
  return client.username && client.username !== 'UNDEF' ? client.username : client.commonName || client.common_name || '未知用户';
}

export function getClientBytes(client: OnlineClient, direction: 'received' | 'sent') {
  const candidates = direction === 'received'
    ? [client.bytesReceived, client.bytes_received, client.recvBytes]
    : [client.bytesSent, client.bytes_sent, client.sendBytes];
  for (const value of candidates) {
    if (value == null) continue;
    const num = Number(value);
    if (Number.isFinite(num)) return num;
  }
  return 0;
}

export function clientVips(client: OnlineClient) {
  return [client.vip, client.vip6].filter(Boolean).join(',');
}

export function parseDateOnly(value?: string) {
  if (!value) return undefined;
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

export function expiryStatus(user: UserRecord) {
  const date = parseDateOnly(user.expireDate);
  if (!date) return { label: '长期', className: 'neutral' };
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const diffDays = Math.ceil((date.getTime() - today.getTime()) / 86400000);
  if (diffDays < 0) return { label: '已过期', className: 'danger' };
  if (diffDays <= 7) return { label: '即将过期', className: 'warning' };
  return { label: '正常', className: 'success' };
}

export function isUserExpired(user: UserRecord) {
  return expiryStatus(user).className === 'danger';
}

export function isUserExpiring(user: UserRecord) {
  return expiryStatus(user).className === 'warning';
}

export function messageOf(error: unknown) {
  // RBAC：api.ts 已对 403 等错误主动 toast，此处返回空字符串避免调用方再次 toast 导致双重提示
  // 调用方 toast.error(`prefix：${messageOf(error)}`) 会显示 "prefix："，不再追加错误消息
  if (error && typeof error === 'object' && 'handled' in error && (error as { handled: boolean }).handled) {
    return '';
  }
  return error instanceof Error ? error.message : String(error);
}

export function normalizeList<T>(value: unknown, candidates: string[] = []): T[] {
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    for (const candidate of candidates) {
      if (Array.isArray(record[candidate])) return record[candidate] as T[];
    }
  }
  return [];
}

export function buildTree(groups: GroupRecord[], parentId: number | null = null, depth = 0): Array<GroupRecord & { depth: number }> {
  const idSet = new Set(groups.map((g) => g.id));
  return groups
    .filter((item) => {
      const pid = item.parent_id === item.id ? null : item.parent_id ?? null;
      if (parentId !== null) return pid === parentId;
      // 根节点：parent_id 为 null，或父节点不在可见列表中（数据权限过滤后的子树）
      return pid === null || !idSet.has(pid);
    })
    .flatMap((item) => [{ ...item, depth }, ...buildTree(groups, item.id, depth + 1)]);
}

export function getDescendantGroupIds(groups: GroupRecord[], groupId: number) {
  const ids = new Set<number>();
  const visit = (parentId: number) => {
    groups.filter((item) => item.parent_id === parentId).forEach((child) => {
      if (child.id === groupId || ids.has(child.id)) return;
      ids.add(child.id);
      visit(child.id);
    });
  };
  visit(groupId);
  return ids;
}

export function toDateInputValue(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}
