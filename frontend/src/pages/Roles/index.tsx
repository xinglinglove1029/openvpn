import { useEffect, useMemo, useState } from 'react';
import {
  ShieldCheck,
  Plus,
  RefreshCw,
  Trash2,
  Edit,
  Lock,
  KeyRound,
  ChevronRight,
  ChevronDown,
} from 'lucide-react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Badge } from '@/ui/badge';
import { Switch } from '@/ui/switch';
import { Textarea } from '@/ui/textarea';
import { Checkbox } from '@/ui/checkbox';
import { Separator } from '@/ui/separator';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/ui/dialog';
import { DataTable, type Column } from '@/components/DataTable';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { HasPermission } from '@/components/HasPermission';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { api } from '@/api';
import { messageOf, normalizeList } from '@/lib/format';
import { cn } from '@/lib/utils';
import type { Role, PermissionTreeNode } from '@/types';

/* ───────── 工具：收集权限树所有 code ───────── */

function collectCodes(nodes: PermissionTreeNode[]): string[] {
  const codes: string[] = [];
  function walk(list: PermissionTreeNode[]) {
    for (const n of list) {
      codes.push(n.code);
      if (n.children && n.children.length) walk(n.children);
    }
  }
  walk(nodes);
  return codes;
}

/** 收集叶子节点 code（用于父节点全选/取消全选） */
function collectLeafCodes(node: PermissionTreeNode): string[] {
  const codes: string[] = [];
  function walk(n: PermissionTreeNode) {
    if (n.children && n.children.length) {
      for (const c of n.children) walk(c);
    } else {
      codes.push(n.code);
    }
  }
  walk(node);
  return codes;
}

/**
 * 递归查找指定 code 的所有祖先菜单 code（从根到父节点，不含自身）
 * 用于选中叶子时自动添加父菜单，保证菜单可见性
 */
function getAncestorCodes(tree: PermissionTreeNode[], targetCode: string): string[] {
  function findPath(nodes: PermissionTreeNode[], path: string[]): string[] | null {
    for (const n of nodes) {
      // 当前节点加入路径
      const currentPath = [...path, n.code];
      if (n.code === targetCode) {
        // 命中：返回路径中除自身外的祖先
        return currentPath.slice(0, -1);
      }
      if (n.children && n.children.length) {
        const result = findPath(n.children, currentPath);
        if (result) return result;
      }
    }
    return null;
  }
  return findPath(tree, []) ?? [];
}

/* ───────── 权限树组件 ───────── */

function PermissionTree({
  tree,
  selected,
  onChange,
  disabled,
}: {
  tree: PermissionTreeNode[];
  selected: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
}) {
  const [expanded, setExpanded] = useState<Set<string>>(() => {
    // 默认展开所有菜单节点
    const set = new Set<string>();
    for (const n of tree) {
      if (n.type === 'menu' && n.children && n.children.length) set.add(n.code);
    }
    return set;
  });

  function toggleExpand(code: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
  }

  function toggleLeaf(code: string) {
    if (disabled) return;
    if (selected.includes(code)) {
      // 取消叶子：只移除当前 code，保留父菜单（兄弟叶子可能仍选中）
      onChange(selected.filter((c) => c !== code));
    } else {
      // 选中叶子：自动添加所有祖先菜单 code，保证菜单可见性
      const ancestors = getAncestorCodes(tree, code);
      const next = new Set(selected);
      next.add(code);
      ancestors.forEach((c) => next.add(c));
      onChange(Array.from(next));
    }
  }

  function toggleParent(node: PermissionTreeNode) {
    if (disabled) return;
    const leafCodes = collectLeafCodes(node);
    const allChecked = leafCodes.every((c) => selected.includes(c));
    if (allChecked) {
      // 取消所有子节点 + 父节点本身
      onChange(selected.filter((c) => !leafCodes.includes(c) && c !== node.code));
    } else {
      // 选中所有子节点（含父节点本身）
      const next = new Set(selected);
      next.add(node.code);
      leafCodes.forEach((c) => next.add(c));
      onChange(Array.from(next));
    }
  }

  function renderNode(node: PermissionTreeNode, depth: number): React.ReactNode {
    const isMenu = node.type === 'menu';
    const hasChildren = !!node.children && node.children.length > 0;
    const isExpanded = expanded.has(node.code);
    const leafCodes = hasChildren ? collectLeafCodes(node) : [node.code];
    const checkedCount = leafCodes.filter((c) => selected.includes(c)).length;
    const checked = checkedCount === leafCodes.length;
    const indeterminate = checkedCount > 0 && !checked;

    return (
      <div key={node.code} className="select-none">
        <div
          className={cn(
            'flex items-center gap-2 rounded-md py-1.5 pr-2 transition-colors',
            disabled ? 'opacity-60' : 'hover:bg-muted/50',
          )}
          style={{ paddingLeft: depth * 20 + 4 }}
        >
          {hasChildren ? (
            <button
              type="button"
              onClick={() => toggleExpand(node.code)}
              className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
              aria-label={isExpanded ? '收起' : '展开'}
            >
              {isExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            </button>
          ) : (
            <span className="inline-block h-4 w-4 shrink-0" />
          )}
          <Checkbox
            checked={indeterminate ? 'indeterminate' : checked}
            onCheckedChange={() => (hasChildren ? toggleParent(node) : toggleLeaf(node.code))}
            disabled={disabled}
          />
          <span className={cn('text-sm', isMenu ? 'font-medium' : 'text-foreground/80')}>
            {node.name}
          </span>
          <span className="text-[10px] font-mono text-muted-foreground/70">{node.code}</span>
          {hasChildren && (
            <span className="ml-auto text-[10px] text-muted-foreground">
              {checkedCount}/{leafCodes.length}
            </span>
          )}
        </div>
        {hasChildren && isExpanded && (
          <div className="space-y-0.5">
            {node.children!.map((c) => renderNode(c, depth + 1))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="max-h-[420px] overflow-y-auto rounded-lg border bg-muted/20 p-2">
      {tree.length === 0 ? (
        <div className="py-6 text-center text-sm text-muted-foreground">暂无权限定义</div>
      ) : (
        tree.map((n) => renderNode(n, 0))
      )}
    </div>
  );
}

/* ───────── 角色表单对话框 ───────── */

interface RoleFormState {
  open: boolean;
  mode: 'create' | 'edit';
  role?: Role;
  name: string;
  code: string;
  description: string;
  isEnable: boolean;
  sort: number;
  errors: Record<string, string>;
}

const emptyForm: RoleFormState = {
  open: false,
  mode: 'create',
  name: '',
  code: '',
  description: '',
  isEnable: true,
  sort: 0,
  errors: {},
};

function RoleFormDialog({
  state,
  setForm,
  onSaved,
}: {
  state: RoleFormState;
  setForm: React.Dispatch<React.SetStateAction<RoleFormState>>;
  onSaved: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const isBuiltin = state.role?.isBuiltin ?? false;

  function updateField<K extends keyof RoleFormState>(key: K, value: RoleFormState[K]) {
    setForm((prev) => ({
      ...prev,
      [key]: value,
      errors: { ...prev.errors, [key]: '' },
    }));
  }

  function validate(): Record<string, string> {
    const errors: Record<string, string> = {};
    if (!state.name.trim()) errors.name = '请输入角色名称';
    if (!state.code.trim()) errors.code = '请输入角色编码';
    else if (!/^[a-zA-Z][a-zA-Z0-9_]*$/.test(state.code.trim())) {
      errors.code = '编码需以字母开头，仅支持字母、数字、下划线';
    }
    return errors;
  }

  async function handleSubmit() {
    const errors = validate();
    if (Object.keys(errors).length) {
      setForm((prev) => ({ ...prev, errors }));
      return;
    }

    setSaving(true);
    try {
      const payload = {
        name: state.name.trim(),
        code: state.code.trim(),
        description: state.description.trim(),
        isEnable: state.isEnable,
        sort: state.sort,
      };
      if (state.mode === 'create') {
        await api.postForm<{ message: string }>('/ovpn/role', payload);
        toast.success('角色已创建');
      } else {
        await api.patchForm<{ message: string }>(`/ovpn/role/${state.role?.id}`, payload);
        toast.success('角色已更新');
      }
      setForm((prev) => ({ ...prev, open: false }));
      onSaved();
    } catch (e) {
      toast.error(`保存失败：${messageOf(e)}`);
    } finally {
      setSaving(false);
    }
  }

  function close() {
    setForm((prev) => ({ ...prev, open: false }));
  }

  return (
    <Dialog open={state.open} onOpenChange={(open) => !open && close()}>
      <DialogContent className="max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5 text-[var(--accent)]" />
            {state.mode === 'create' ? '新建角色' : '编辑角色'}
          </DialogTitle>
          <DialogDescription>
            {isBuiltin
              ? '内置角色：编码不可修改，可调整名称、描述、启用状态与排序'
              : '自定义角色：可编辑所有字段；角色创建后请在列表中点击"分配权限"按钮配置权限'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
              角色名称<span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                value={state.name}
                onChange={(e) => updateField('name', e.target.value)}
                placeholder="请输入角色名称"
                maxLength={64}
                className={cn(state.errors.name && 'border-destructive focus-visible:ring-destructive/40')}
              />
              {state.errors.name && (
                <p className="text-xs font-medium text-destructive">{state.errors.name}</p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
              角色编码<span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                value={state.code}
                onChange={(e) => updateField('code', e.target.value)}
                placeholder="如 editor、auditor"
                maxLength={64}
                disabled={isBuiltin}
                className={cn(
                  'font-mono',
                  isBuiltin && 'bg-muted/40 cursor-not-allowed',
                  state.errors.code && 'border-destructive focus-visible:ring-destructive/40',
                )}
              />
              {state.errors.code && (
                <p className="text-xs font-medium text-destructive">{state.errors.code}</p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-right text-sm font-medium text-foreground/80">描述</Label>
            <Textarea
              value={state.description}
              onChange={(e) => updateField('description', e.target.value)}
              placeholder="角色职责说明"
              rows={2}
              maxLength={255}
            />
          </div>

          <div className="grid grid-cols-[140px_1fr] items-start gap-4">
            <Label className="pt-2 text-right text-sm font-medium text-foreground/80">排序</Label>
            <Input
              type="number"
              value={String(state.sort)}
              onChange={(e) => updateField('sort', Number(e.target.value) || 0)}
              placeholder="数字越小越靠前"
            />
          </div>

          <div className="grid grid-cols-[140px_1fr] items-center gap-4">
            <Label className="text-right text-sm font-medium text-foreground/80">启用状态</Label>
            <div className="flex items-center gap-2">
              <Switch
                checked={state.isEnable}
                onCheckedChange={(v) => updateField('isEnable', v)}
                disabled={isBuiltin}
              />
              <span className="text-sm text-muted-foreground">
                {state.isEnable ? '已启用' : '已禁用'}
              </span>
              {isBuiltin && (
                <span className="text-xs text-muted-foreground">（内置角色不允许禁用）</span>
              )}
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={close} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving}>
            {saving ? '保存中...' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 权限分配对话框 ───────── */

function PermissionAssignDialog({
  open,
  role,
  tree,
  onOpenChange,
  onSaved,
}: {
  open: boolean;
  role: Role | null;
  tree: PermissionTreeNode[];
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open && role) {
      setSelected(role.permissions ?? []);
    }
  }, [open, role]);

  async function handleSubmit() {
    if (!role) return;
    setSaving(true);
    try {
      await api.putJson<{ message: string }>(`/ovpn/role/${role.id}/permissions`, {
        permissions: selected,
      });
      toast.success('权限已更新');
      onOpenChange(false);
      onSaved();
    } catch (e) {
      toast.error(`保存失败：${messageOf(e)}`);
    } finally {
      setSaving(false);
    }
  }

  function selectAll() {
    setSelected(collectCodes(tree));
  }
  function clearAll() {
    setSelected([]);
  }

  const totalCodes = useMemo(() => collectCodes(tree), [tree]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[680px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5 text-[var(--accent)]" />
            分配权限：{role?.name}
          </DialogTitle>
          <DialogDescription>
            勾选权限后保存，该角色下用户下次登录后菜单与按钮可见性将立即生效
          </DialogDescription>
        </DialogHeader>

        <div className="mb-2 flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            已选 {selected.length} 项 / 共 {totalCodes.length} 项
          </span>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="outline" onClick={selectAll} disabled={saving}>
              全选
            </Button>
            <Button size="sm" variant="outline" onClick={clearAll} disabled={saving}>
              清空
            </Button>
          </div>
        </div>

        <PermissionTree
          tree={tree}
          selected={selected}
          onChange={setSelected}
          disabled={saving}
        />

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving}>
            {saving ? '保存中...' : '保存权限'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 主页面 ───────── */

export default function RolesPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [form, setForm] = useState<RoleFormState>(emptyForm);
  const [permOpen, setPermOpen] = useState(false);
  const [permRole, setPermRole] = useState<Role | null>(null);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);

  // 拉取角色列表
  const rolesState = useAsync(async () => {
    const result = await api.get<unknown>('/ovpn/role');
    return normalizeList<Role>(result, ['data']);
  }, [reloadKey]);

  // 拉取权限树（仅加载一次）
  const treeState = useAsync(async () => {
    const result = await api.get<{ data?: PermissionTreeNode[] } | PermissionTreeNode[]>(
      '/ovpn/permission/tree',
    );
    return normalizeList<PermissionTreeNode>(result, ['data']);
  }, []);

  const reload = () => setReloadKey((k) => k + 1);

  useEffect(() => {
    if (rolesState.error) toast.error(`加载角色失败：${messageOf(rolesState.error)}`);
  }, [rolesState.error]);
  useEffect(() => {
    if (treeState.error) toast.error(`加载权限树失败：${messageOf(treeState.error)}`);
  }, [treeState.error]);

  function openCreate() {
    setForm({
      ...emptyForm,
      open: true,
      mode: 'create',
      isEnable: true,
      sort: (rolesState.data?.length ?? 0) + 1,
      errors: {},
    });
  }

  function openEdit(role: Role) {
    setForm({
      open: true,
      mode: 'edit',
      role,
      name: role.name,
      code: role.code,
      description: role.description ?? '',
      isEnable: role.isEnable,
      sort: role.sort,
      errors: {},
    });
  }

  function openAssign(role: Role) {
    setPermRole(role);
    setPermOpen(true);
  }

  function askDelete(role: Role) {
    if (role.isBuiltin) {
      toast.error('内置角色不允许删除');
      return;
    }
    setConfirm({
      title: '删除角色',
      message: `确认要删除角色「${role.name}」吗？删除后无法恢复，且若角色下仍有用户将拒绝删除。`,
      danger: true,
      onConfirm: async () => {
        try {
          await api.delete<{ message: string }>(`/ovpn/role/${role.id}`);
          toast.success('删除成功');
          reload();
          setConfirm(null);
        } catch (e) {
          toast.error(`删除失败：${messageOf(e)}`);
        }
      },
    });
  }

  const items = rolesState.data ?? [];
  const tree = treeState.data ?? [];
  const pagination = usePagination(items, `roles-${reloadKey}`, 10);

  const columns: Column<Role>[] = useMemo(
    () => [
      {
        key: 'name',
        header: '角色',
        sortable: true,
        sortAccessor: (item) => item.name,
        render: (item) => (
          <div className="flex items-center gap-2.5">
            <span
              className={cn(
                'flex h-7 w-7 items-center justify-center rounded-md',
                item.isBuiltin
                  ? 'bg-[var(--accent)]/15 text-[var(--accent)]'
                  : 'bg-muted text-muted-foreground',
              )}
            >
              <ShieldCheck className="h-4 w-4" />
            </span>
            <div className="min-w-0">
              <div className="flex items-center gap-1.5">
                <span className="font-medium leading-tight">{item.name}</span>
                {item.isBuiltin && (
                  <Badge
                    variant="outline"
                    className="bg-[var(--accent)]/10 border-[var(--accent)]/30 text-[var(--accent)] text-[10px] px-1.5 py-0"
                  >
                    <Lock className="h-2.5 w-2.5 mr-0.5" />
                    内置
                  </Badge>
                )}
              </div>
              <div className="text-xs font-mono text-muted-foreground">{item.code}</div>
            </div>
          </div>
        ),
      },
      {
        key: 'description',
        header: '描述',
        render: (item) => (
          <span className="text-sm text-muted-foreground line-clamp-2 max-w-[280px]">
            {item.description || '-'}
          </span>
        ),
      },
      {
        key: 'permCount',
        header: '权限数',
        className: 'text-center',
        render: (item) => (
          <Badge variant="outline" className="font-mono">
            {item.permissions?.length ?? 0}
          </Badge>
        ),
      },
      {
        key: 'isEnable',
        header: '状态',
        className: 'text-center',
        render: (item) => (
          <StatusBadge status={item.isEnable ? 'success' : 'neutral'}>
            {item.isEnable ? '启用' : '禁用'}
          </StatusBadge>
        ),
      },
      {
        key: 'sort',
        header: '排序',
        className: 'text-center',
        sortable: true,
        sortAccessor: (item) => item.sort,
        render: (item) => <span className="font-mono text-sm">{item.sort}</span>,
      },
      {
        key: 'actions',
        header: '操作',
        render: (item) => (
          <div className="flex items-center gap-1">
            <HasPermission code="role:assign_permissions">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => openAssign(item)}
                title={item.isBuiltin ? '内置角色权限由系统管理' : '分配权限'}
              >
                <KeyRound className="h-3.5 w-3.5 mr-1" />
                分配权限
                {item.isBuiltin && (
                  <span className="ml-1 text-[10px] text-muted-foreground">（系统管理）</span>
                )}
              </Button>
            </HasPermission>
            <HasPermission code="role:update">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => openEdit(item)}
              >
                <Edit className="h-3.5 w-3.5 mr-1" />
                编辑
              </Button>
            </HasPermission>
            <HasPermission
              code="role:delete"
              fallback={
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 px-2 text-destructive hover:text-destructive"
                  disabled
                  title={item.isBuiltin ? '内置角色不允许删除' : '无权限'}
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  删除
                </Button>
              }
            >
              <Button
                size="sm"
                variant="ghost"
                className={cn(
                  'h-7 px-2 text-destructive hover:text-destructive',
                  item.isBuiltin && 'opacity-40 cursor-not-allowed',
                )}
                disabled={item.isBuiltin}
                onClick={() => !item.isBuiltin && askDelete(item)}
                title={item.isBuiltin ? '内置角色不允许删除' : '删除角色'}
              >
                <Trash2 className="h-3.5 w-3.5 mr-1" />
                删除
              </Button>
            </HasPermission>
          </div>
        ),
      },
    ],
    [],
  );

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Access Control" title="角色管理" description="管理角色与权限分配，控制菜单与按钮可见性">
        <HasPermission code="role:view">
          <Button size="sm" variant="outline" onClick={reload}>
            <RefreshCw className="h-3.5 w-3.5 mr-1" />
            刷新
          </Button>
        </HasPermission>
        <HasPermission code="role:create">
          <Button size="sm" onClick={openCreate}>
            <Plus className="h-3.5 w-3.5 mr-1" />
            新建角色
          </Button>
        </HasPermission>
      </PageHeader>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck className="h-4 w-4" />
            角色列表
          </CardTitle>
        </CardHeader>
        <CardContent>
          {rolesState.loading ? (
            <div className="py-10 text-center text-sm text-muted-foreground">正在加载...</div>
          ) : items.length === 0 ? (
            <div className="py-10 text-center">
              <ShieldCheck className="mx-auto h-10 w-10 text-muted-foreground/50" />
              <p className="mt-2 text-sm font-medium">暂无角色</p>
              <p className="mt-1 text-xs text-muted-foreground">
                系统启动时会自动创建「系统超管」与「普通用户」两个内置角色
              </p>
            </div>
          ) : (
            <DataTable
              columns={columns}
              data={pagination.pagedItems}
              fullData={items}
              page={pagination.page}
              pageSize={pagination.pageSize}
              pageCount={pagination.pageCount}
              total={pagination.total}
              start={pagination.start}
              end={pagination.end}
              onPageChange={pagination.setPage}
              onPageSizeChange={pagination.setPageSize}
              keyFn={(item) => item.id}
            />
          )}
        </CardContent>
      </Card>

      <RoleFormDialog state={form} setForm={setForm} onSaved={reload} />

      <PermissionAssignDialog
        open={permOpen}
        role={permRole}
        tree={tree}
        onOpenChange={setPermOpen}
        onSaved={reload}
      />

      {confirm && <ConfirmDialog state={confirm} onClose={() => setConfirm(null)} />}

      <Separator className="my-4" />
      <p className="text-xs text-muted-foreground">
        提示：内置角色（系统超管 / 普通用户）由系统维护，可查看与分配权限但不允许删除；
        新建角色后请点击「分配权限」按钮为其配置菜单与按钮权限。
      </p>
    </div>
  );
}
