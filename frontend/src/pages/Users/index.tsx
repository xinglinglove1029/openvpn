import { useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import {
  Users,
  Plus,
  Search,
  Trash2,
  Edit,
  KeyRound,
  Upload,
  Download,
  FolderTree,
  Eye,
  EyeOff,
  CheckCircle2,
  XCircle,
  ShieldOff,
} from 'lucide-react';
import { api } from '@/api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { useIsMobile } from '@/hooks/useIsMobile';
import {
  expiryStatus,
  isUserExpired,
  isUserExpiring,
  messageOf,
  normalizeList,
  buildTree,
  getDescendantGroupIds,
} from '@/lib/format';
import {
  trimText,
  isValidAccount,
  isStrongPassword,
  isValidEmail,
  isValidIp,
} from '@/lib/validators';
import { cn } from '@/lib/utils';
import { DataTable, type Column } from '@/components/DataTable';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { DatePickerField } from '@/components/DatePickerField';
import { HasPermission } from '@/components/HasPermission';
import { useAuth } from '@/store/auth';
import { Card, CardContent, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Badge } from '@/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/ui/dialog';
import { Label } from '@/ui/label';
import { Switch } from '@/ui/switch';
import { Checkbox } from '@/ui/checkbox';
import { Textarea } from '@/ui/textarea';
import { Separator } from '@/ui/separator';
import type { UserRecord, GroupRecord, ClientRecord } from '@/types';

/* ───────── 常量 ───────── */

const userStatusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'enabled', label: '仅启用' },
  { value: 'disabled', label: '仅禁用' },
];

const mfaFilterOptions = [
  { value: 'all', label: '全部 MFA' },
  { value: 'enabled', label: 'MFA 已开' },
  { value: 'disabled', label: 'MFA 未开' },
];

const expireFilterOptions = [
  { value: 'all', label: '全部有效期' },
  { value: 'normal', label: '正常/长期' },
  { value: 'expiring', label: '即将过期' },
  { value: 'expired', label: '已过期' },
];

type FieldErrors = Record<string, string>;

/* ───────── Toast ───────── */

function useNotify() {
  const notify = (type: 'success' | 'error' | 'info', message: string) => {
    if (type === 'success') toast.success(message);
    else if (type === 'error') toast.error(message);
    else toast.info(message);
  };
  return { notify };
}

/* ───────── 主页面 ───────── */

export default function UsersPage() {
  const { notify } = useNotify();
  const { isAdmin } = useAuth();
  const [reloadKey, setReloadKey] = useState(0);
  // 用户列表独立刷新 key：编辑/启用/禁用/删除等用户操作只刷新用户列表，不影响分组树和客户端列表
  const [usersReloadKey, setUsersReloadKey] = useState(0);
  const [selectedGroupId, setSelectedGroupId] = useState(1);
  const [confirmState, setConfirmState] = useState<ConfirmState>();

  const refreshGroups = () => setReloadKey((v) => v + 1);
  const refreshUsers = () => setUsersReloadKey((v) => v + 1);

  const groupsState = useAsync(
    () => api.get<unknown>('/ovpn/group').then((v) => normalizeList<GroupRecord>(v, ['groups', 'data'])),
    [reloadKey],
  );
  const clientsState = useAsync(
    () => api.get<unknown>('/ovpn/client').then((v) => normalizeList<ClientRecord>(v, ['clients', 'data'])),
    [reloadKey],
  );
  const groups = groupsState.data || [];
  const groupIdSet = useMemo(() => new Set(groups.map((g) => g.id)), [groups]);
  const usersState = useAsync(
    () => {
      // 仅当选中的分组在可见列表中时才请求用户列表（避免普通用户请求无权限的分组）
      if (!groupIdSet.has(selectedGroupId)) {
        return Promise.resolve({ users: [], authUser: false });
      }
      return api
        .get<{ users?: UserRecord[]; authUser?: boolean }>(`/ovpn/group/${selectedGroupId}/users`)
        .then((v) => ({ users: normalizeList<UserRecord>(v, ['users', 'data']), authUser: v.authUser }));
    },
    [selectedGroupId, usersReloadKey, groupIdSet],
  );
  const clients = clientsState.data || [];

  useEffect(() => {
    if (!groups.length) return;
    if (!groups.some((g) => g.id === selectedGroupId)) setSelectedGroupId(groups[0].id);
  }, [groups, selectedGroupId]);

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Identity" title="账号管理" description="管理VPN用户账号和权限">
        <Button size="sm" onClick={refreshUsers}>
          刷新
        </Button>
      </PageHeader>

      <div className="grid grid-cols-1 md:grid-cols-[260px_1fr] gap-4">
        {/* 左侧分组树 */}
        <GroupTreePanel
          groups={groups}
          selectedGroupId={selectedGroupId}
          setSelectedGroupId={setSelectedGroupId}
          users={usersState.data?.users || []}
          usersLoading={usersState.loading}
          notify={notify}
          reload={refreshGroups}
          confirmAction={setConfirmState}
        />

        {/* 右侧用户表 */}
        <UserTablePanel
          groups={groups}
          selectedGroupId={selectedGroupId}
          usersState={usersState}
          clients={clients}
          isAdmin={isAdmin}
          notify={notify}
          reload={refreshUsers}
          confirmAction={setConfirmState}
        />
      </div>

      {/* 确认弹窗 */}
      {confirmState && (
        <ConfirmDialog state={confirmState} onClose={() => setConfirmState(undefined)} />
      )}
    </div>
  );
}

/* ───────── 分组树面板 ───────── */

function GroupTreePanel({
  groups,
  selectedGroupId,
  setSelectedGroupId,
  users,
  usersLoading,
  notify,
  reload,
  confirmAction,
}: {
  groups: GroupRecord[];
  selectedGroupId: number;
  setSelectedGroupId: (id: number) => void;
  users: UserRecord[];
  usersLoading: boolean;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
  confirmAction: (state: ConfirmState) => void;
}) {
  const tree = buildTree(groups);
  const selectedGroup = groups.find((g) => g.id === selectedGroupId);
  const [groupDialogOpen, setGroupDialogOpen] = useState(false);
  const [groupDialogMode, setGroupDialogMode] = useState<'add' | 'edit'>('add');
  const [groupConfigOpen, setGroupConfigOpen] = useState(false);
  const [groupConfigContent, setGroupConfigContent] = useState('');

  function openAddGroup() {
    setGroupDialogMode('add');
    setGroupDialogOpen(true);
  }

  function openEditGroup() {
    setGroupDialogMode('edit');
    setGroupDialogOpen(true);
  }

  function openGroupConfig() {
    if (selectedGroup) {
      setGroupConfigContent(selectedGroup.config || '');
      setGroupConfigOpen(true);
    }
  }

  async function deleteGroup() {
    if (!selectedGroup) return;
    if (selectedGroup.id === 1) {
      notify('error', '默认组不能删除');
      return;
    }
    const childGroups = groups.filter((g) => g.parent_id === selectedGroup.id);
    if (childGroups.length > 0) {
      notify('error', `分组「${selectedGroup.name}」下还有 ${childGroups.length} 个子分组，请先迁移或删除子分组`);
      return;
    }
    if (usersLoading) {
      notify('info', '正在读取分组用户，请稍后再删除');
      return;
    }
    if (users.length > 0) {
      notify('error', `分组「${selectedGroup.name}」下还有 ${users.length} 个用户，请先迁移用户到其他分组`);
      return;
    }
    confirmAction({
      title: '删除用户组',
      message: `确认删除分组 ${selectedGroup.name} 吗？请先确认该分组下账号和策略已迁移。`,
      danger: true,
      onConfirm: async () => {
        const result = await api.delete<{ message: string }>(`/ovpn/group/${selectedGroup.id}`);
        notify('success', result.message || '删除成功');
        setSelectedGroupId(1);
        reload();
      },
    });
  }

  return (
    <>
      <Card>
        <CardHeader className="pb-3">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <CardTitle className="flex flex-wrap items-center gap-2 text-base">
              <FolderTree className="h-4 w-4" />
              用户组
            </CardTitle>
            <HasPermission code="group:create">
              <Button size="sm" variant="outline" onClick={openAddGroup}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                新增
              </Button>
            </HasPermission>
          </div>
        </CardHeader>
        <CardContent className="pt-0 space-y-3">
          <div className="space-y-1">
            {tree.map((group) => {
              const isSelected = selectedGroupId === group.id;
              return (
                <button
                  key={group.id}
                  type="button"
                  aria-pressed={isSelected}
                  className={cn(
                    'group relative flex w-full items-center gap-2 rounded-lg border py-2 pl-3 pr-2 text-sm transition-all',
                    isSelected
                      ? 'border-[var(--accent)]/50 bg-[var(--accent)]/12 text-[var(--accent)] shadow-[0_1px_2px_rgba(0,0,0,0.04)] font-semibold'
                      : 'border-transparent hover:border-border/60 hover:bg-muted/70 text-foreground/80 hover:text-foreground',
                  )}
                  style={{ paddingLeft: 12 + group.depth * 16 }}
                  onClick={() => setSelectedGroupId(group.id)}
                >
                  {isSelected && (
                    <span className="absolute left-0 top-1/2 h-6 w-[3px] -translate-y-1/2 rounded-r-full bg-[var(--accent)] shadow-[0_0_6px_var(--accent)]" />
                  )}
                  <span
                    className={cn(
                      'flex h-5 w-5 shrink-0 items-center justify-center rounded transition-colors',
                      isSelected
                        ? 'bg-[var(--accent)]/20 text-[var(--accent)]'
                        : 'bg-muted/60 text-muted-foreground group-hover:bg-muted',
                    )}
                  >
                    <FolderTree className="h-3 w-3" />
                  </span>
                  <span className="truncate flex-1 text-left">{group.name}</span>
                  {isSelected && (
                    <span className="ml-auto text-[10px] font-semibold uppercase tracking-wider text-[var(--accent)]/80">
                      当前
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {selectedGroup && (
            <>
              <Separator />
              <div className="flex flex-col gap-1">
                <HasPermission code="group:update">
                  <Button size="sm" variant="ghost" className="justify-start w-full" onClick={openEditGroup}>
                    <Edit className="mr-2 h-3.5 w-3.5" />
                    编辑分组
                  </Button>
                </HasPermission>
                <HasPermission code="group:config">
                  <Button size="sm" variant="ghost" className="justify-start w-full" onClick={openGroupConfig}>
                    <KeyRound className="mr-2 h-3.5 w-3.5" />
                    组配置
                  </Button>
                </HasPermission>
                <HasPermission code="group:delete">
                  <Button size="sm" variant="ghost" className="justify-start text-destructive hover:text-destructive w-full" onClick={deleteGroup}>
                    <Trash2 className="mr-2 h-3.5 w-3.5" />
                    删除分组
                  </Button>
                </HasPermission>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* 分组表单对话框 */}
      <GroupFormDialog
        open={groupDialogOpen}
        onOpenChange={setGroupDialogOpen}
        mode={groupDialogMode}
        group={groupDialogMode === 'edit' ? selectedGroup : undefined}
        parentGroup={groupDialogMode === 'add' ? selectedGroup : undefined}
        groups={groups}
        selectedGroupId={selectedGroupId}
        notify={notify}
        reload={reload}
      />

      {/* 组配置编辑器 */}
      {selectedGroup && (
        <GroupConfigDialog
          open={groupConfigOpen}
          onOpenChange={setGroupConfigOpen}
          group={selectedGroup}
          initialContent={groupConfigContent}
          notify={notify}
          reload={reload}
        />
      )}
    </>
  );
}

/* ───────── 分组表单对话框 ───────── */

function GroupFormDialog({
  open,
  onOpenChange,
  mode,
  group,
  parentGroup,
  groups,
  selectedGroupId,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'add' | 'edit';
  group?: GroupRecord;
  parentGroup?: GroupRecord;
  groups: GroupRecord[];
  selectedGroupId: number;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const tree = buildTree(groups);
  const [name, setName] = useState('');
  const [parentId, setParentId] = useState('0');
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});

  useEffect(() => {
    if (open) {
      setName(mode === 'edit' && group ? group.name || '' : '');
      setParentId(
        mode === 'edit' && group
          ? String(group.parent_id ?? 0)
          : String(parentGroup?.id ?? selectedGroupId ?? 0),
      );
      setErrors({});
    }
  }, [open, mode, group, parentGroup, selectedGroupId]);

  const blockedParentIds =
    mode === 'edit' && group ? new Set([group.id, ...getDescendantGroupIds(groups, group.id)]) : new Set<number>();
  const isDefaultGroup = mode === 'edit' && group?.id === 1;
  const parentOptions = [
    ...(isDefaultGroup ? [{ value: '0', label: '— 无上级分组 —' }] : []),
    ...tree
      .filter((item) => !blockedParentIds.has(item.id))
      .map((item) => ({ value: String(item.id), label: `${'— '.repeat(item.depth)}${item.name}` })),
  ];

  function validate() {
    const next: FieldErrors = {};
    if (!trimText(name)) next.name = '分组名称不能为空';
    const normalizedParent = Number(parentId);
    if (!Number.isInteger(normalizedParent) || normalizedParent < 0) next.parentId = '请选择有效的上级分组';
    if (mode === 'edit' && group && String(group.id) === parentId) next.parentId = '上级分组不能选择自己';
    if (parentId === '0' && !(mode === 'edit' && group?.id === 1)) next.parentId = '只有默认分组可以设置为无上级分组';
    return next;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validate();
    setErrors(nextErrors);
    // 前端校验失败：仅在字段下方提示，不弹 toast
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    try {
      const form =
        mode === 'add'
          ? { name: trimText(name), parent_id: parentId === '0' || parentId === 'null' ? null : parentId }
          : { id: group!.id, name: trimText(name), parent_id: parentId === '0' || parentId === 'null' ? null : parentId };
      const action = mode === 'add' ? api.postForm : api.patchForm;
      const result = await action<{ message: string }>('/ovpn/group', form);
      notify('success', result.message || '用户组已保存');
      onOpenChange(false);
      reload();
    } catch (error) {
      // 后端错误：使用 toast 提醒
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{mode === 'add' ? '新增用户组' : `编辑用户组：${group?.name}`}</DialogTitle>
          <DialogDescription>
            {mode === 'add' ? '创建一个新的用户分组' : '修改分组的名称和上级分组'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="group-name" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              分组名称 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="group-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  if (errors.name) setErrors((prev) => { const n = { ...prev }; delete n.name; return n; });
                }}
                autoFocus
              />
              {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
            </div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              上级分组 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Select value={parentId} onValueChange={(v) => {
                setParentId(v);
                if (errors.parentId) setErrors((prev) => { const n = { ...prev }; delete n.parentId; return n; });
              }}>
                <SelectTrigger>
                  <SelectValue placeholder="请选择上级分组" />
                </SelectTrigger>
                <SelectContent>
                  {parentOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.parentId && <p className="text-xs text-destructive">{errors.parentId}</p>}
            </div>
          </div>
          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存分组'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 组配置对话框 ───────── */

function GroupConfigDialog({
  open,
  onOpenChange,
  group,
  initialContent,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group: GroupRecord;
  initialContent: string;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const [content, setContent] = useState(initialContent);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});

  useEffect(() => {
    setContent(initialContent);
    setErrors({});
  }, [initialContent]);

  function validate(): FieldErrors {
    const next: FieldErrors = {};
    if (!trimText(content)) next.content = '组配置内容不能为空';
    return next;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validate();
    setErrors(nextErrors);
    // 前端校验失败：仅在字段下方提示，不弹 toast
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    try {
      const form = { id: group.id, name: group.name, config: content };
      const result = await api.patchForm<{ message: string }>('/ovpn/group', form);
      notify('success', result.message || '组配置已保存');
      onOpenChange(false);
      reload();
    } catch (error) {
      // 后端错误：使用 toast 提醒
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>组配置：{group.name}</DialogTitle>
          <DialogDescription>编辑此分组的 OpenVPN 配置</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-2">
          <Textarea
            className={cn('min-h-[300px] font-mono text-sm', errors.content && 'border-destructive focus-visible:ring-destructive/40')}
            value={content}
            aria-invalid={errors.content ? 'true' : undefined}
            onChange={(e) => {
              setContent(e.target.value);
              if (errors.content) setErrors((prev) => { const n = { ...prev }; delete n.content; return n; });
            }}
          />
          {errors.content && <p className="text-xs text-destructive">{errors.content}</p>}
          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存组配置'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 用户表格面板 ───────── */

function UserTablePanel({
  groups,
  selectedGroupId,
  usersState,
  clients,
  notify,
  reload,
  confirmAction,
}: {
  groups: GroupRecord[];
  selectedGroupId: number;
  usersState: { loading: boolean; error?: string; data?: { users: UserRecord[]; authUser?: boolean } };
  clients: ClientRecord[];
  isAdmin: boolean;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
  confirmAction: (state: ConfirmState) => void;
}) {
  const isMobile = useIsMobile();
  const users = usersState.data?.users || [];
  const [selectedUserIds, setSelectedUserIds] = useState<number[]>([]);
  const [userSearch, setUserSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [mfaFilter, setMfaFilter] = useState('all');
  const [expireFilter, setExpireFilter] = useState('all');

  /* 对话框状态 */
  const [userFormOpen, setUserFormOpen] = useState(false);
  const [userFormMode, setUserFormMode] = useState<'add' | 'edit'>('add');
  const [editingUser, setEditingUser] = useState<UserRecord>();
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false);
  const [resetPasswordUser, setResetPasswordUser] = useState<UserRecord>();

  /* 过滤 */
  const filteredUsers = users.filter((user) => {
    const keyword = userSearch.toLowerCase().trim();
    const matchesKeyword =
      !keyword ||
      [user.username, user.name, user.email, user.ipAddr, user.ovpnConfig]
        .filter(Boolean)
        .some((v) => String(v).toLowerCase().includes(keyword));
    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'enabled' ? user.isEnable !== false : user.isEnable === false);
    const matchesMfa =
      mfaFilter === 'all' ||
      (mfaFilter === 'enabled' ? Boolean(user.mfaSecret || user.mfaEnabled) : !(user.mfaSecret || user.mfaEnabled));
    const matchesExpire =
      expireFilter === 'all' ||
      (expireFilter === 'expired'
        ? isUserExpired(user)
        : expireFilter === 'expiring'
          ? isUserExpiring(user)
          : !isUserExpired(user) && !isUserExpiring(user));
    return matchesKeyword && matchesStatus && matchesMfa && matchesExpire;
  });

  const userPagination = usePagination(
    filteredUsers,
    `${selectedGroupId}|${userSearch}|${statusFilter}|${mfaFilter}|${expireFilter}`,
  );

  const visibleIds = userPagination.pagedItems
    .map((u) => u.id)
    .filter((id): id is number => Boolean(id));
  const selectedUsers = users.filter((u) => u.id && selectedUserIds.includes(u.id));
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedUserIds.includes(id));
  const hasSelectedMfaUser = selectedUsers.some((u) => u.mfaSecret || u.mfaEnabled);

  useEffect(() => {
    setSelectedUserIds((ids) => ids.filter((id) => users.some((u) => u.id === id)));
  }, [users]);

  /* 操作函数 */
  async function patchUser(user: UserRecord, form: Record<string, unknown>, success: string) {
    try {
      const result = await api.patchForm<{ message: string }>('/ovpn/user', { id: user.id, ...form });
      notify('success', result.message || success);
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  function deleteUser(user: UserRecord) {
    confirmAction({
      title: '删除 VPN 用户',
      message: `确认删除用户 ${user.username} 吗？该操作不可恢复。`,
      danger: true,
      onConfirm: async () => {
        const result = await api.delete<{ message: string }>(`/ovpn/user/${user.id}`);
        notify('success', result.message || '删除成功');
        reload();
      },
    });
  }

  function resetMfa(user: UserRecord) {
    confirmAction({
      title: '重置 MFA',
      message: `确认重置 ${user.username} 的 MFA 吗？用户下次需要重新绑定。`,
      danger: true,
      onConfirm: async () => {
        await api.delete(`/client/mfa/${user.id}`);
        notify('success', 'MFA 已重置');
        reload();
      },
    });
  }

  // 账号密码认证开关已迁至【系统设置 → 服务管理】

  function toggleSelectedUser(user: UserRecord, checked: boolean) {
    if (!user.id) return;
    setSelectedUserIds((ids) =>
      checked ? [...new Set([...ids, user.id!])] : ids.filter((id) => id !== user.id),
    );
  }

  function toggleVisibleUsers(checked: boolean) {
    setSelectedUserIds((ids) =>
      checked
        ? [...new Set([...ids, ...visibleIds])]
        : ids.filter((id) => !visibleIds.includes(id)),
    );
  }

  function batchAction(
    title: string,
    message: string,
    action: (user: UserRecord) => Promise<unknown>,
    success: string,
    danger = false,
  ) {
    if (!selectedUsers.length) {
      notify('error', '请先选择要操作的账号');
      return;
    }
    confirmAction({
      title,
      message: `${message}（已选择 ${selectedUsers.length} 个账号）`,
      danger,
      onConfirm: async () => {
        for (const user of selectedUsers) {
          await action(user);
        }
        notify('success', success);
        setSelectedUserIds([]);
        reload();
      },
    });
  }

  async function importUsers(file?: File) {
    if (!file) return;
    try {
      const form = new FormData();
      form.set('file', file);
      form.set('gid', String(selectedGroupId));
      const result = await api.multipart<{ message: string }>('/ovpn/user', form);
      notify('success', result.message || '导入成功');
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  function exportSelectedUsers() {
    if (!selectedUsers.length) {
      notify('error', '请先选择要导出的账号');
      return;
    }
    const header = ['username', 'name', 'email', 'ipAddr', 'ovpnConfig', 'expireDate', 'isEnable', 'mfa'];
    const rows = selectedUsers.map((u) => [
      u.username,
      u.name || '',
      u.email || '',
      u.ipAddr || '',
      u.ovpnConfig || '',
      u.expireDate || '',
      u.isEnable === false ? 'disabled' : 'enabled',
      u.mfaSecret ? 'enabled' : 'disabled',
    ]);
    const csv = [header, ...rows]
      .map((row) => row.map((v) => `"${String(v).replace(/"/g, '""')}"`).join(','))
      .join('\n');
    const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `openvpn-users-${Date.now()}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  }

  function openAddUser() {
    setUserFormMode('add');
    setEditingUser(undefined);
    setUserFormOpen(true);
  }

  function openEditUser(user: UserRecord) {
    setUserFormMode('edit');
    setEditingUser(user);
    setUserFormOpen(true);
  }

  function openResetPassword(user: UserRecord) {
    setResetPasswordUser(user);
    setResetPasswordOpen(true);
  }

  /* 文件上传 ref */
  const fileInputRef = useRef<HTMLInputElement>(null);

  /* 表格列 */
  const columns: Column<UserRecord>[] = [
    {
      key: 'select',
      header: (
        <Checkbox
          checked={allVisibleSelected}
          onCheckedChange={(checked) => toggleVisibleUsers(!!checked)}
        />
      ),
      mobileHeader: '选择',
      cardPlacement: 'header-action',
      render: (user) => (
        <Checkbox
          checked={Boolean(user.id && selectedUserIds.includes(user.id))}
          onCheckedChange={(checked) => toggleSelectedUser(user, !!checked)}
        />
      ),
      className: 'w-10',
    },
    {
      key: 'username',
      header: '账号',
      sortable: true,
      sortAccessor: (user) => user.username ?? '',
      cardPlacement: 'header-left',
      mobileRender: (user) => (
        <div className="flex flex-col items-start">
          <div className="flex items-center gap-1.5 min-w-0 max-w-full">
            <span className="text-sm font-semibold leading-5 truncate">{user.username}</span>
          </div>
          {user.name && (
            <span className="text-xs text-muted-foreground truncate max-w-full">
              {user.name}
            </span>
          )}
        </div>
      ),
      render: (user) => <span className="font-medium">{user.username}</span>,
    },
    {
      key: 'name',
      header: '姓名',
      sortable: true,
      sortAccessor: (user) => user.name ?? '',
      render: (user) => user.name || '-',
    },
    {
      key: 'email',
      header: '邮箱',
      sortable: true,
      sortAccessor: (user) => user.email ?? '',
      render: (user) => user.email || '-',
    },
    {
      key: 'ipAddr',
      header: '固定 IP',
      sortable: true,
      sortAccessor: (user) => user.ipAddr ?? '',
      className: 'min-w-[5.5rem] whitespace-nowrap',
      render: (user) => user.ipAddr || '-',
    },
    {
      key: 'ipRegion',
      header: 'IP 归属地',
      sortable: true,
      sortAccessor: (user) => user.ipRegion ?? '',
      className: 'min-w-[5.5rem] whitespace-nowrap',
      render: (user) => user.ipRegion || '-',
    },
    {
      key: 'ovpnConfig',
      header: '配置文件',
      sortable: true,
      sortAccessor: (user) => user.ovpnConfig ?? '',
      className: 'min-w-[6rem] whitespace-nowrap',
      render: (user) => user.ovpnConfig || '-',
    },
    {
      key: 'mfa',
      header: 'MFA 状态',
      sortable: true,
      sortAccessor: (user) => (user.mfaSecret || user.mfaEnabled ? 1 : 0),
      className: 'min-w-[4.5rem] whitespace-nowrap',
      render: (user) =>
        user.mfaSecret || user.mfaEnabled ? (
          <Badge variant="secondary" className="whitespace-nowrap">开启</Badge>
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
    {
      key: 'status',
      header: '状态',
      sortable: true,
      sortAccessor: (user) => (user.isEnable === false ? 0 : 1),
      cardPlacement: 'header-right',
      className: 'min-w-[4rem] whitespace-nowrap',
      render: (user) => (
        <StatusBadge status={user.isEnable === false ? 'danger' : 'success'} className="whitespace-nowrap">
          {user.isEnable === false ? '禁用' : '启用'}
        </StatusBadge>
      ),
    },
    {
      key: 'expiry',
      header: '有效期',
      sortable: true,
      sortAccessor: (user) => {
        if (!user.expireDate) return Number.MAX_SAFE_INTEGER;
        const ts = new Date(user.expireDate).getTime();
        return Number.isNaN(ts) ? Number.MAX_SAFE_INTEGER : ts;
      },
      className: 'min-w-[6rem] whitespace-nowrap',
      render: (user) => {
        const expire = expiryStatus(user);
        const statusType = expire.className as 'success' | 'warning' | 'danger' | 'neutral';
        return (
          <StatusBadge status={statusType} className="whitespace-nowrap">
            {user.expireDate || expire.label}
          </StatusBadge>
        );
      },
    },
    {
      key: 'actions',
      header: '操作',
      render: (user) => (
        // 桌面（pc）模式强制单行不换行：紧凑 padding + 较小 gap + whitespace-nowrap。
        // 仅极窄屏（移动端）才允许 wrap 兜底，避免被裁切隐藏。
        <div className="flex flex-wrap items-center gap-0.5 sm:flex-nowrap sm:gap-1">
          <HasPermission code="user:update">
            <Button size="sm" variant="ghost" className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5', isMobile && 'min-w-[2.25rem] p-0')} onClick={() => openEditUser(user)}>
              {isMobile ? <Edit className="h-4 w-4" /> : '编辑'}
            </Button>
          </HasPermission>
          <HasPermission
            code={user.isEnable === false ? 'user:enable' : 'user:disable'}
            fallback={
              <Button size="sm" variant="ghost" className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5', isMobile && 'min-w-[2.25rem] p-0')} disabled>
                {isMobile ? (
                  user.isEnable === false ? <CheckCircle2 className="h-4 w-4" /> : <XCircle className="h-4 w-4" />
                ) : (user.isEnable === false ? '启用' : '禁用')}
              </Button>
            }
          >
            <Button
              size="sm"
              variant="ghost"
              className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5', isMobile && 'min-w-[2.25rem] p-0')}
              onClick={() => patchUser(user, { isEnable: user.isEnable === false }, '状态已更新')}
            >
              {isMobile ? (
                user.isEnable === false ? <CheckCircle2 className="h-4 w-4 text-emerald-500" /> : <XCircle className="h-4 w-4 text-amber-500" />
              ) : (user.isEnable === false ? '启用' : '禁用')}
            </Button>
          </HasPermission>
          <HasPermission code="user:reset_password">
            <Button size="sm" variant="ghost" className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5', isMobile && 'min-w-[2.25rem] p-0')} onClick={() => openResetPassword(user)}>
              {isMobile ? <KeyRound className="h-4 w-4" /> : '重置密码'}
            </Button>
          </HasPermission>
          {(user.mfaSecret || user.mfaEnabled) && (
            <HasPermission code="user:reset_mfa">
              <Button size="sm" variant="ghost" className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5', isMobile && 'min-w-[2.25rem] p-0')} onClick={() => resetMfa(user)}>
                {isMobile ? <ShieldOff className="h-4 w-4" /> : '重置 MFA'}
              </Button>
            </HasPermission>
          )}
          <HasPermission code="user:delete">
            {!user.isBuiltin && (
              <Button
                size="sm"
                variant="ghost"
                className={cn('h-8 px-1.5 whitespace-nowrap sm:h-7 sm:px-1.5 text-destructive hover:text-destructive', isMobile && 'min-w-[2.25rem] p-0')}
                onClick={() => deleteUser(user)}
              >
                {isMobile ? <Trash2 className="h-4 w-4" /> : '删除'}
              </Button>
            )}
          </HasPermission>
        </div>
      ),
    },
  ];

  return (
    <>
      <Card>
        <CardHeader className="pb-4">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <CardTitle className="flex flex-wrap items-center gap-2 text-base">
              <Users className="h-4 w-4" />
              VPN 账号
            </CardTitle>
            <div className="grid grid-cols-2 sm:flex sm:flex-nowrap gap-2 w-full sm:w-auto">
              <HasPermission code="user:import">
                <Button size="sm" variant="outline" className="w-full sm:w-auto" onClick={() => fileInputRef.current?.click()}>
                  <Upload className="mr-1 h-3.5 w-3.5" />
                  导入 CSV
                </Button>
              </HasPermission>
              <input
                ref={fileInputRef}
                type="file"
                accept=".csv"
                className="hidden"
                onChange={(e) => importUsers(e.target.files?.[0])}
              />
              <HasPermission code="user:export">
                <Button size="sm" variant="outline" className="w-full sm:w-auto" asChild>
                  <a href="/user/template">
                    <Download className="mr-1 h-3.5 w-3.5" />
                    模板
                  </a>
                </Button>
              </HasPermission>
              <HasPermission code="user:export">
                <Button size="sm" variant="outline" className="w-full sm:w-auto" asChild>
                  <a href={`/ovpn/user/export?gid=${selectedGroupId}`}>
                    <Download className="mr-1 h-3.5 w-3.5" />
                    导出分组
                  </a>
                </Button>
              </HasPermission>
              <HasPermission code="user:create">
                <Button size="sm" className="w-full sm:w-auto" onClick={openAddUser}>
                  <Plus className="mr-1 h-3.5 w-3.5" />
                  添加用户
                </Button>
              </HasPermission>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2 sm:space-y-4">
          {/* 过滤栏 */}
          <div className="flex flex-col sm:flex-row sm:flex-wrap items-stretch sm:items-center gap-2">
            <div className="relative flex-1 min-w-0 sm:min-w-[200px]">
              <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder="搜索账号 / 姓名 / 邮箱 / 固定 IP"
                value={userSearch}
                onChange={(e) => setUserSearch(e.target.value)}
              />
            </div>
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-full sm:w-[130px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {userStatusOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={mfaFilter} onValueChange={setMfaFilter}>
              <SelectTrigger className="w-full sm:w-[130px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {mfaFilterOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={expireFilter} onValueChange={setExpireFilter}>
              <SelectTrigger className="w-full sm:w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {expireFilterOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* 批量操作栏 */}
          {selectedUserIds.length > 0 && (
            <div className="flex flex-col sm:flex-row sm:items-center gap-2 rounded-lg border border-[var(--accent)]/40 bg-[var(--accent)]/[0.08] px-3 py-2">
              <span className="text-sm font-semibold text-[var(--accent)] flex items-center gap-1.5">
                <CheckCircle2 className="h-4 w-4" />
                已选择 {selectedUserIds.length} 个账号
              </span>
              <Separator orientation="vertical" className="hidden sm:block h-4" />
              <div className="flex flex-wrap gap-1.5 sm:gap-2 sm:ml-auto w-full sm:w-auto">
                <HasPermission code="user:enable">
                  <Button
                    size="sm"
                    variant="outline"
                    className="w-full sm:w-auto"
                    onClick={() =>
                      batchAction(
                        '批量启用账号',
                        '确认启用这些账号吗？',
                        (u) => api.patchForm('/ovpn/user', { id: u.id, isEnable: true }),
                        '批量启用完成',
                      )
                    }
                  >
                    <CheckCircle2 className="mr-1 h-3.5 w-3.5 text-emerald-500" />
                    启用
                  </Button>
                </HasPermission>
                <HasPermission code="user:disable">
                  <Button
                    size="sm"
                    variant="outline"
                    className="w-full sm:w-auto"
                    onClick={() =>
                      batchAction(
                        '批量禁用账号',
                        '确认禁用这些账号吗？',
                        (u) => api.patchForm('/ovpn/user', { id: u.id, isEnable: false }),
                        '批量禁用完成',
                        true,
                      )
                    }
                  >
                    <XCircle className="mr-1 h-3.5 w-3.5 text-amber-500" />
                    禁用
                  </Button>
                </HasPermission>
                {hasSelectedMfaUser && (
                  <HasPermission code="user:reset_mfa">
                    <Button
                      size="sm"
                      variant="outline"
                      className="w-full sm:w-auto"
                      onClick={() =>
                        batchAction(
                          '批量重置 MFA',
                          '确认重置这些账号的 MFA 吗？',
                          (u) => api.delete(`/client/mfa/${u.id}`),
                          '批量 MFA 重置完成',
                          true,
                        )
                      }
                    >
                      <ShieldOff className="mr-1 h-3.5 w-3.5" />
                      重置 MFA
                    </Button>
                  </HasPermission>
                )}
                <HasPermission code="user:export">
                  <Button size="sm" variant="outline" onClick={exportSelectedUsers} className="w-full sm:w-auto">
                    <Download className="mr-1 h-3.5 w-3.5" />
                    导出选中
                  </Button>
                </HasPermission>
                <HasPermission code="user:delete">
                  <Button
                    size="sm"
                    variant="outline"
                    className="w-full sm:w-auto text-destructive hover:text-destructive"
                    onClick={() =>
                      batchAction(
                        '批量删除账号',
                        '确认删除这些账号吗？该操作不可恢复。',
                        (u) => api.delete(`/ovpn/user/${u.id}`),
                        '批量删除完成',
                        true,
                      )
                    }
                  >
                    <Trash2 className="mr-1 h-3.5 w-3.5" />
                    删除
                  </Button>
                </HasPermission>
              </div>
            </div>
          )}

          {/* 表格 */}
          {users.length ? (
            filteredUsers.length ? (
              <DataTable
                columns={columns}
                data={userPagination.pagedItems}
                fullData={filteredUsers}
                page={userPagination.page}
                pageSize={userPagination.pageSize}
                pageCount={userPagination.pageCount}
                total={userPagination.total}
                start={userPagination.start}
                end={userPagination.end}
                onPageChange={userPagination.setPage}
                onPageSizeChange={userPagination.setPageSize}
                keyFn={(user) => user.id || user.username}
                isCardSelected={(user) => Boolean(user.id && selectedUserIds.includes(user.id))}
                mobileToolbar={
                  <div className="flex flex-wrap items-center gap-3 w-full">
                    <label className="inline-flex items-center gap-2 cursor-pointer select-none">
                      <Checkbox
                        checked={allVisibleSelected}
                        onCheckedChange={(checked) => toggleVisibleUsers(!!checked)}
                      />
                      <span className="text-sm font-medium">全选当前页</span>
                    </label>
                    {visibleIds.length > 0 && (
                      <span className="text-xs text-muted-foreground ml-auto">
                        已选 {selectedUserIds.filter((id) => visibleIds.includes(id)).length} / {visibleIds.length}
                      </span>
                    )}
                  </div>
                }
              />
            ) : (
              <div className="rounded-lg border bg-card p-8 text-center">
                <p className="text-lg font-medium">没有匹配的 VPN 账号</p>
                <p className="text-muted-foreground mt-1">调整关键词、状态、MFA 或有效期筛选后再试。</p>
              </div>
            )
          ) : (
            <div className="rounded-lg border bg-card p-8 text-center">
              <p className="text-lg font-medium">当前分组暂无用户</p>
              <p className="text-muted-foreground mt-1">
                {clients.length
                  ? '点击添加用户即可绑定客户端配置。'
                  : '可以先添加客户端配置，再创建账号。'}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 用户表单对话框 */}
      <UserFormDialog
        open={userFormOpen}
        onOpenChange={setUserFormOpen}
        mode={userFormMode}
        user={editingUser}
        clients={clients}
        groups={groups}
        selectedGroupId={selectedGroupId}
        notify={notify}
        reload={reload}
      />

      {/* 重置密码对话框 */}
      <ResetPasswordDialog
        open={resetPasswordOpen}
        onOpenChange={setResetPasswordOpen}
        user={resetPasswordUser}
        notify={notify}
        reload={reload}
      />
    </>
  );
}

/* ───────── 用户表单对话框 ───────── */

function UserFormDialog({
  open,
  onOpenChange,
  mode,
  user,
  clients,
  groups,
  selectedGroupId,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'add' | 'edit';
  user?: UserRecord;
  clients: ClientRecord[];
  groups: GroupRecord[];
  selectedGroupId: number;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const [username, setUsername] = useState('');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [ipAddr, setIpAddr] = useState('');
  const [ovpnConfig, setOvpnConfig] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [expireDate, setExpireDate] = useState('');
  const [sendNotifyEmail, setSendNotifyEmail] = useState(false);
  const [autoCreateClient, setAutoCreateClient] = useState(true);
  const [isFirstLogin, setIsFirstLogin] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  // 分组选择：编辑模式下可切换用户所属分组
  const [gid, setGid] = useState<string>(String(selectedGroupId));

  // 分组树（用于下拉展示层级关系）
  const groupTree = useMemo(() => buildTree(groups), [groups]);

  useEffect(() => {
    if (open) {
      setUsername(mode === 'edit' && user ? user.username || '' : '');
      setName(mode === 'edit' && user ? user.name || '' : '');
      setEmail(mode === 'edit' && user ? user.email || '' : '');
      setIpAddr(mode === 'edit' && user ? user.ipAddr || '' : '');
      setOvpnConfig(mode === 'edit' && user ? user.ovpnConfig || '' : '');
      setPassword('');
      setShowPassword(false);
      setExpireDate(mode === 'edit' && user ? user.expireDate || '' : '');
      setSendNotifyEmail(false);
      setAutoCreateClient(true);
      setIsFirstLogin(false);
      // 编辑模式下使用用户当前分组，新增模式下使用当前选中的分组
      setGid(mode === 'edit' && user ? String(user.gid ?? selectedGroupId) : String(selectedGroupId));
      setErrors({});
    }
  }, [open, mode, user, clients, selectedGroupId]);

  function validate() {
    const next: FieldErrors = {};
    if (!isValidAccount(username)) next.username = '账号不能为空，长度需在 2-64 个字符内';
    if (!trimText(name)) next.name = '姓名不能为空';
    if (mode === 'add' && !isStrongPassword(password)) next.password = '初始密码需至少 12 位，包含大小写字母、数字和特殊字符';
    if (mode === 'add' && !trimText(email)) next.email = '邮箱不能为空';
    if (trimText(email) && !isValidEmail(email)) next.email = '邮箱格式不正确';
    if (trimText(ipAddr) && !isValidIp(ipAddr)) next.ipAddr = '固定 IP 请输入合法 IPv4 或 IPv6';
    return next;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validate();
    setErrors(nextErrors);
    // 前端校验失败：仅在字段下方提示，不弹 toast
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    try {
      if (mode === 'add') {
        const result = await api.postForm<{ message: string }>('/ovpn/user', {
          username: trimText(username),
          name: trimText(name),
          password: trimText(password),
          email: trimText(email),
          ipAddr: trimText(ipAddr),
          gid: Number(gid) || selectedGroupId,
          ovpnConfig,
          expireDate,
          sendNotifyEmail,
          autoCreateClient,
          isFirstLogin,
        });
        notify('success', result.message || '用户已创建');
      } else {
        const result = await api.patchForm<{ message: string }>('/ovpn/user', {
          id: user?.id,
          username: trimText(username),
          name: trimText(name),
          email: trimText(email),
          ipAddr: trimText(ipAddr),
          gid: Number(gid) || user?.gid || selectedGroupId,
          ovpnConfig,
          expireDate,
        });
        notify('success', result.message || '用户已保存');
      }
      onOpenChange(false);
      reload();
    } catch (error) {
      // 后端错误：使用 toast 提醒
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === 'add' ? '添加 VPN 用户' : `编辑用户：${user?.username}`}
          </DialogTitle>
          <DialogDescription>
            {mode === 'add' ? '创建一个新的 VPN 账号' : '修改账号信息'}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="user-username" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              账号 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="user-username"
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value);
                  if (errors.username) setErrors((prev) => { const n = { ...prev }; delete n.username; return n; });
                }}
                autoFocus
                placeholder="请输入账号"
              />
              {errors.username && <p className="text-xs text-destructive">{errors.username}</p>}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="user-name" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              姓名 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="user-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  if (errors.name) setErrors((prev) => { const n = { ...prev }; delete n.name; return n; });
                }}
                placeholder="请输入姓名"
              />
              {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="user-email" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">邮箱 {mode === 'add' && <span className="text-destructive">*</span>}</Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="user-email"
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  if (errors.email) setErrors((prev) => { const n = { ...prev }; delete n.email; return n; });
                }}
                placeholder="请输入邮箱"
              />
              {errors.email && <p className="text-xs text-destructive">{errors.email}</p>}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="user-ip" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">固定 IP</Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="user-ip"
                value={ipAddr}
                onChange={(e) => {
                  setIpAddr(e.target.value);
                  if (errors.ipAddr) setErrors((prev) => { const n = { ...prev }; delete n.ipAddr; return n; });
                }}
                placeholder="留空则自动分配"
              />
              {errors.ipAddr && <p className="text-xs text-destructive">{errors.ipAddr}</p>}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">所属分组</Label>
            <div className="space-y-1.5 min-w-0">
              <Select value={gid} onValueChange={setGid}>
                <SelectTrigger>
                  <SelectValue placeholder="选择分组" />
                </SelectTrigger>
                <SelectContent>
                  {groupTree.map((g) => (
                    <SelectItem key={g.id} value={String(g.id)}>
                      {`${'— '.repeat(g.depth)}${g.name}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">客户端配置</Label>
            <div className="space-y-1.5 min-w-0">
              <Select value={ovpnConfig || '__none__'} onValueChange={(v) => setOvpnConfig(v === '__none__' ? '' : v)}>
                <SelectTrigger>
                  <SelectValue placeholder="选择客户端配置" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">不绑定</SelectItem>
                  {clients.map((c) => (
                    <SelectItem key={c.name} value={c.name}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {mode === 'add' && (
            <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
              <Label htmlFor="user-password" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
                初始密码 <span className="text-destructive ml-0.5">*</span>
              </Label>
              <div className="space-y-1.5 min-w-0">
                <div className="relative">
                  <Input
                    clearable={false}
                    id="user-password"
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => {
                      setPassword(e.target.value);
                      if (errors.password) setErrors((prev) => { const n = { ...prev }; delete n.password; return n; });
                    }}
                    placeholder="至少 12 位强密码"
                    className="pr-12"
                  />
                  <button
                    type="button"
                    aria-label={showPassword ? '隐藏密码' : '显示密码'}
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-1 top-1/2 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center text-muted-foreground hover:text-foreground"
                  >
                    {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                {errors.password && <p className="text-xs text-destructive">{errors.password}</p>}
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">过期日期</Label>
            <div className="space-y-1.5 min-w-0">
              <DatePickerField value={expireDate} onChange={setExpireDate} />
            </div>
          </div>

          {mode === 'add' && (
            <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-center gap-2 sm:gap-4">
              <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">自动创建客户端</Label>
              <div className="flex items-center gap-3">
                <Switch checked={autoCreateClient} onCheckedChange={setAutoCreateClient} />
                <span className="text-xs text-muted-foreground">开启后将基于用户名生成 .ovpn 配置文件</span>
              </div>
            </div>
          )}

          {mode === 'add' && (
            <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-center gap-2 sm:gap-4">
              <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">发送通知邮件</Label>
              <div className="flex items-center gap-3">
                <Switch checked={sendNotifyEmail} onCheckedChange={setSendNotifyEmail} />
              </div>
            </div>
          )}

          {mode === 'add' && (
            <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-center gap-2 sm:gap-4">
              <Label className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">首次登录修改密码</Label>
              <div className="flex items-center gap-3">
                <Switch checked={isFirstLogin} onCheckedChange={setIsFirstLogin} />
              </div>
            </div>
          )}

          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存用户'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 重置密码对话框（默认开启邮件推送） ───────── */

function ResetPasswordDialog({
  open,
  onOpenChange,
  user,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user?: UserRecord;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [sendNotifyEmail, setSendNotifyEmail] = useState(true);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});

  useEffect(() => {
    if (open) {
      setPassword('');
      setShowPassword(false);
      setSendNotifyEmail(true);
      setErrors({});
    }
  }, [open]);

  function validate() {
    const next: FieldErrors = {};
    if (!isStrongPassword(password)) next.password = '新密码需至少 12 位，包含大小写字母、数字和特殊字符';
    return next;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validate();
    setErrors(nextErrors);
    // 前端校验失败：仅在字段下方提示，不弹 toast
    if (Object.keys(nextErrors).length) return;
    if (!user) return;
    setSaving(true);
    try {
      const result = await api.patchForm<{ message: string }>('/ovpn/user', {
        id: user.id,
        password: trimText(password),
        sendNotifyEmail,
      });
      notify('success', result.message || '密码已重置');
      onOpenChange(false);
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>重置密码：{user?.username}</DialogTitle>
          <DialogDescription>设置新的登录密码</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="reset-password">
              新密码 <span className="text-destructive">*</span>
            </Label>
            <div className="relative">
              <Input
                clearable={false}
                id="reset-password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  if (errors.password) setErrors((prev) => { const n = { ...prev }; delete n.password; return n; });
                }}
                autoFocus
                placeholder="至少 12 位强密码"
                className="pr-12"
              />
              <button
                type="button"
                aria-label={showPassword ? '隐藏密码' : '显示密码'}
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-1 top-1/2 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center text-muted-foreground hover:text-foreground"
              >
                {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
            {errors.password && <p className="text-sm text-destructive">{errors.password}</p>}
          </div>

          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 rounded-md border px-4 py-3">
            <div>
              <p className="font-medium">发送通知邮件</p>
            </div>
            <Switch checked={sendNotifyEmail} onCheckedChange={setSendNotifyEmail} />
          </div>

          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? '处理中...' : '确认重置'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
