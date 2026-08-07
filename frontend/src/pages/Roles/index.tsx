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
  Search,
  ChevronsDownUp,
  ChevronsUpDown,
  Users,
  Network,
  ArrowRight,
  ArrowLeft,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Badge } from '@/ui/badge';
import { Switch } from '@/ui/switch';
import { Textarea } from '@/ui/textarea';
import { Checkbox } from '@/ui/checkbox';
import { Card } from '@/ui/card';
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
import { messageOf, normalizeList, buildTree } from '@/lib/format';
import { cn } from '@/lib/utils';
import type { Role, PermissionTreeNode, GroupRecord } from '@/types';

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

/** 收集所有拥有子节点的 code（用于全部展开/折叠） */
function collectExpandableCodes(nodes: PermissionTreeNode[]): string[] {
  const codes: string[] = [];
  function walk(list: PermissionTreeNode[]) {
    for (const n of list) {
      if (n.children && n.children.length) {
        codes.push(n.code);
        walk(n.children);
      }
    }
  }
  walk(nodes);
  return codes;
}

/** 按关键词过滤权限树：命中节点及其所有祖先保留 */
function filterTreeByKeyword(nodes: PermissionTreeNode[], kw: string): PermissionTreeNode[] {
  function walk(list: PermissionTreeNode[]): PermissionTreeNode[] {
    const result: PermissionTreeNode[] = [];
    for (const n of list) {
      const hit =
        n.name.toLowerCase().includes(kw) || n.code.toLowerCase().includes(kw);
      const filteredChildren = n.children ? walk(n.children) : [];
      if (hit || filteredChildren.length > 0) {
        result.push({ ...n, children: filteredChildren });
      }
    }
    return result;
  }
  return walk(nodes);
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
  // 默认展开所有可展开节点
  const [expanded, setExpanded] = useState<Set<string>>(() => {
    const set = new Set<string>();
    function walk(nodes: PermissionTreeNode[]) {
      for (const n of nodes) {
        if (n.children && n.children.length) {
          set.add(n.code);
          walk(n.children);
        }
      }
    }
    walk(tree);
    return set;
  });

  // 搜索关键词
  const [searchText, setSearchText] = useState('');

  // 所有权限 code（全选用）
  const allCodes = useMemo(() => collectCodes(tree), [tree]);

  // 搜索过滤后的树（命中节点及其祖先保留）
  const displayTree = useMemo(() => {
    const kw = searchText.trim().toLowerCase();
    if (!kw) return tree;
    return filterTreeByKeyword(tree, kw);
  }, [tree, searchText]);

  // 所有可展开节点的 code（基于原始 tree）
  const allExpandableCodes = useMemo(() => collectExpandableCodes(tree), [tree]);

  // 搜索时自动展开所有命中节点的祖先：直接展开 displayTree 中所有有子节点的 code
  const effectiveExpanded = useMemo(() => {
    const kw = searchText.trim().toLowerCase();
    if (kw) {
      const set = new Set<string>();
      function walk(nodes: PermissionTreeNode[]) {
        for (const n of nodes) {
          if (n.children && n.children.length) {
            set.add(n.code);
            walk(n.children);
          }
        }
      }
      walk(displayTree);
      return set;
    }
    return expanded;
  }, [searchText, displayTree, expanded]);

  // 是否全部展开（用于切换按钮文案）
  const isAllExpanded =
    allExpandableCodes.length > 0 && allExpandableCodes.every((c) => expanded.has(c));

  function toggleExpand(code: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(code)) next.delete(code);
      else next.add(code);
      return next;
    });
  }

  function toggleExpandAll() {
    if (isAllExpanded) {
      setExpanded(new Set());
    } else {
      setExpanded(new Set(allExpandableCodes));
    }
  }

  function handleSelectAll() {
    if (disabled) return;
    onChange([...allCodes]);
  }

  function handleClearAll() {
    if (disabled) return;
    onChange([]);
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
    const isExpanded = effectiveExpanded.has(node.code);
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
    <div className="space-y-2">
      {/* 工具栏：左侧搜索框 + 右侧一排操作按钮（全选/清空/展开折叠） */}
      <div className="flex flex-nowrap items-center gap-2">
        <div className="relative flex-1 min-w-0">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="搜索权限名称或编码"
            className="pl-8 h-11 sm:h-8 text-xs"
          />
        </div>
        <div className="flex items-center gap-1.5 flex-none">
          <Button
            size="sm"
            variant="outline"
            onClick={handleSelectAll}
            disabled={disabled}
            className="h-8 px-2.5"
          >
            全选
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={handleClearAll}
            disabled={disabled || selected.length === 0}
            className="h-8 px-2.5"
          >
            清空
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={toggleExpandAll}
            disabled={!!searchText}
            className="h-8 px-2.5"
          >
            {isAllExpanded ? (
              <>
                <ChevronsDownUp className="h-3.5 w-3.5 mr-1" />
                折叠
              </>
            ) : (
              <>
                <ChevronsUpDown className="h-3.5 w-3.5 mr-1" />
                展开
              </>
            )}
          </Button>
        </div>
      </div>

      {/* 树容器 */}
      <div className="max-h-[420px] overflow-y-auto rounded-lg border bg-muted/20 p-2">
        {displayTree.length === 0 ? (
          <div className="py-6 text-center text-sm text-muted-foreground">
            {searchText ? '没有匹配的权限' : '暂无权限定义'}
          </div>
        ) : (
          displayTree.map((n) => renderNode(n, 0))
        )}
      </div>

      {/* 已选数量（放到树容器下方，避免和工具栏挤在一起） */}
      <div className="flex items-center justify-between text-[11px] text-muted-foreground px-1">
        <span>
          已选 <span className="font-medium text-foreground">{selected.length}</span> 项
          <span className="mx-1">/</span>
          共 {allCodes.length} 项
        </span>
        {searchText && <span>搜索命中 {displayTree.length ? '有结果' : '无结果'}</span>}
      </div>
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
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 text-left sm:pt-2 sm:text-right text-sm font-medium text-foreground/80">
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

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 text-left sm:pt-2 sm:text-right text-sm font-medium text-foreground/80">
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

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 text-left sm:pt-2 sm:text-right text-sm font-medium text-foreground/80">描述</Label>
            <Textarea
              value={state.description}
              onChange={(e) => updateField('description', e.target.value)}
              placeholder="角色职责说明"
              rows={2}
              maxLength={255}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 text-left sm:pt-2 sm:text-right text-sm font-medium text-foreground/80">排序</Label>
            <Input
              type="number"
              value={String(state.sort)}
              onChange={(e) => updateField('sort', Number(e.target.value) || 0)}
              placeholder="数字越小越靠前"
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-center gap-2 sm:gap-4">
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

/* ───────── 用户分配穿梭框 ───────── */

interface RoleAssignUser {
  id: number;
  username: string;
  name?: string;
  gid?: number;
  groupName?: string;
  roleIds?: number[];
  roleNames?: string[];
}

interface RoleAssignUsersResponse {
  allUsers: RoleAssignUser[];
  assignedUserIds: number[];
}

function UserAssignDialog({
  open,
  role,
  onOpenChange,
  onSaved,
}: {
  open: boolean;
  role: Role | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [allUsers, setAllUsers] = useState<RoleAssignUser[]>([]);
  // 已选用户 ID 集合（即右侧列表）
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  // 左侧列表选中（用于穿梭按钮）
  const [leftChecked, setLeftChecked] = useState<Set<number>>(new Set());
  // 右侧列表选中
  const [rightChecked, setRightChecked] = useState<Set<number>>(new Set());
  // 左右两侧搜索框
  const [leftSearch, setLeftSearch] = useState('');
  const [rightSearch, setRightSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // 加载失败标记：失败时禁用保存按钮，避免空列表提交导致清空所有绑定
  const [loadError, setLoadError] = useState(false);

  // 打开时加载数据
  useEffect(() => {
    if (!open || !role) return;
    setLoading(true);
    setLoadError(false);
    api
      .get<RoleAssignUsersResponse>(`/ovpn/role/${role.id}/users`)
      .then((res) => {
        const data = res ?? { allUsers: [], assignedUserIds: [] };
        setAllUsers(data.allUsers ?? []);
        setSelectedIds(new Set(data.assignedUserIds ?? []));
        setLeftChecked(new Set());
        setRightChecked(new Set());
        setLeftSearch('');
        setRightSearch('');
      })
      .catch((e) => {
        toast.error(`加载用户列表失败：${messageOf(e)}`);
        setLoadError(true);
      })
      .finally(() => setLoading(false));
  }, [open, role]);

  // 左侧可选用户 = 未在 selectedIds 中的用户，按搜索词过滤
  const leftList = useMemo(() => {
    const kw = leftSearch.trim().toLowerCase();
    return allUsers
      .filter((u) => !selectedIds.has(u.id))
      .filter((u) => {
        if (!kw) return true;
        return (
          u.username.toLowerCase().includes(kw) ||
          (u.name ?? '').toLowerCase().includes(kw)
        );
      });
  }, [allUsers, selectedIds, leftSearch]);

  // 右侧已选用户
  const rightList = useMemo(() => {
    const kw = rightSearch.trim().toLowerCase();
    return allUsers
      .filter((u) => selectedIds.has(u.id))
      .filter((u) => {
        if (!kw) return true;
        return (
          u.username.toLowerCase().includes(kw) ||
          (u.name ?? '').toLowerCase().includes(kw)
        );
      });
  }, [allUsers, selectedIds, rightSearch]);

  function toggleLeftChecked(id: number) {
    setLeftChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  function toggleRightChecked(id: number) {
    setRightChecked((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  // 把左侧选中的用户移到右侧
  function moveToRight() {
    if (leftChecked.size === 0) return;
    setSelectedIds((prev) => {
      const next = new Set(prev);
      leftChecked.forEach((id) => next.add(id));
      return next;
    });
    setLeftChecked(new Set());
  }
  // 把右侧选中的用户移回左侧
  function moveToLeft() {
    if (rightChecked.size === 0) return;
    setSelectedIds((prev) => {
      const next = new Set(prev);
      rightChecked.forEach((id) => next.delete(id));
      return next;
    });
    setRightChecked(new Set());
  }

  async function handleSubmit() {
    if (!role) return;
    setSaving(true);
    try {
      await api.putJson<{ message: string }>(`/ovpn/role/${role.id}/users`, {
        userIds: Array.from(selectedIds),
      });
      toast.success('用户已分配');
      onOpenChange(false);
      onSaved();
    } catch (e) {
      toast.error(`保存失败：${messageOf(e)}`);
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!saving) onOpenChange(v); }}>
      <DialogContent
        className="max-w-[860px]"
        onPointerDownOutside={(e) => { if (saving) e.preventDefault(); }}
        onEscapeKeyDown={(e) => { if (saving) e.preventDefault(); }}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Users className="h-5 w-5 text-[var(--accent)]" />
            分配用户：{role?.name}
          </DialogTitle>
          <DialogDescription>
            左侧勾选用户后点击"→"加入右侧；右侧为该角色已绑定用户，保存后立即生效
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="py-10 text-center text-sm text-muted-foreground">正在加载...</div>
        ) : (
          <div className="space-y-2">
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] lg:items-stretch">
              {/* 左栏：可选用户 */}
              <Card className="p-2">
                <div className="flex items-center justify-between mb-2 px-1">
                  <span className="text-xs font-medium text-muted-foreground">
                    可选用户（{leftList.length}）
                  </span>
                </div>
                <Input
                  value={leftSearch}
                  onChange={(e) => setLeftSearch(e.target.value)}
                  placeholder="搜索用户名"
                  className="h-11 sm:h-8 text-xs mb-2"
                />
                <div className="max-h-[45dvh] min-h-[14rem] lg:max-h-[360px] lg:min-h-[360px] overflow-y-auto rounded-md border bg-muted/10">
                  {leftList.length === 0 ? (
                    <div className="py-6 text-center text-xs text-muted-foreground">
                      {leftSearch ? '没有匹配的用户' : '暂无可选用户'}
                    </div>
                  ) : (
                    leftList.map((u) => {
                      const checked = leftChecked.has(u.id);
                      // 多角色模式下：列出用户已有但不在当前角色中的角色名（灰色 Badge）
                      const otherRoleNames = (u.roleIds ?? [])
                        .map((rid, idx) => ({ rid, name: u.roleNames?.[idx] }))
                        .filter((r) => r.rid !== role?.id && r.name);
                      return (
                        <label
                          key={u.id}
                          className={cn(
                            'flex items-center gap-2 px-2 py-1.5 cursor-pointer hover:bg-muted/40 transition-colors',
                            checked && 'bg-[var(--accent)]/10',
                          )}
                        >
                          <Checkbox
                            checked={checked}
                            onCheckedChange={() => toggleLeftChecked(u.id)}
                          />
                          <div className="min-w-0 flex-1">
                            <div className="text-sm leading-tight">{u.username}</div>
                            <div className="flex items-center gap-1 mt-0.5 flex-wrap">
                              {u.groupName && (
                                <span className="text-[11px] text-muted-foreground truncate">
                                  {u.groupName}
                                </span>
                              )}
                              {otherRoleNames.map((r) => (
                                <Badge
                                  key={r.rid}
                                  variant="outline"
                                  className="bg-muted/40 border-border text-muted-foreground text-[10px] px-1.5 py-0 shrink-0"
                                >
                                  {r.name}
                                </Badge>
                              ))}
                            </div>
                          </div>
                        </label>
                      );
                    })
                  )}
                </div>
              </Card>

              {/* 中间穿梭按钮 */}
              <div className="flex flex-col items-center justify-center gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  className="h-11 w-full p-0 lg:h-8 lg:w-8"
                  onClick={moveToRight}
                  disabled={leftChecked.size === 0 || saving}
                  title="加入右侧"
                >
                  <ArrowRight className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-11 w-full p-0 lg:h-8 lg:w-8"
                  onClick={moveToLeft}
                  disabled={rightChecked.size === 0 || saving}
                  title="移回左侧"
                >
                  <ArrowLeft className="h-3.5 w-3.5" />
                </Button>
              </div>

              {/* 右栏：已选用户 */}
              <Card className="p-2">
                <div className="flex items-center justify-between mb-2 px-1">
                  <span className="text-xs font-medium text-muted-foreground">
                    已选用户（{rightList.length}）
                  </span>
                </div>
                <Input
                  value={rightSearch}
                  onChange={(e) => setRightSearch(e.target.value)}
                  placeholder="搜索用户名"
                  className="h-11 sm:h-8 text-xs mb-2"
                />
                <div className="max-h-[45dvh] min-h-[14rem] lg:max-h-[360px] lg:min-h-[360px] overflow-y-auto rounded-md border bg-muted/10">
                  {rightList.length === 0 ? (
                    <div className="py-6 text-center text-xs text-muted-foreground">
                      {rightSearch ? '没有匹配的用户' : '暂未选择用户'}
                    </div>
                  ) : (
                    rightList.map((u) => {
                      const checked = rightChecked.has(u.id);
                      // 多角色模式下：列出用户已有但不在当前角色中的角色名（灰色 Badge）
                      const otherRoleNames = (u.roleIds ?? [])
                        .map((rid, idx) => ({ rid, name: u.roleNames?.[idx] }))
                        .filter((r) => r.rid !== role?.id && r.name);
                      return (
                        <label
                          key={u.id}
                          className={cn(
                            'flex items-center gap-2 px-2 py-1.5 cursor-pointer hover:bg-muted/40 transition-colors',
                            checked && 'bg-[var(--accent)]/10',
                          )}
                        >
                          <Checkbox
                            checked={checked}
                            onCheckedChange={() => toggleRightChecked(u.id)}
                          />
                          <div className="min-w-0 flex-1">
                            <div className="text-sm leading-tight">{u.username}</div>
                            <div className="flex items-center gap-1 mt-0.5 flex-wrap">
                              {u.groupName && (
                                <span className="text-[11px] text-muted-foreground truncate">
                                  {u.groupName}
                                </span>
                              )}
                              {otherRoleNames.map((r) => (
                                <Badge
                                  key={r.rid}
                                  variant="outline"
                                  className="bg-muted/40 border-border text-muted-foreground text-[10px] px-1.5 py-0 shrink-0"
                                >
                                  {r.name}
                                </Badge>
                              ))}
                            </div>
                          </div>
                        </label>
                      );
                    })
                  )}
                </div>
              </Card>
            </div>

            <div className="flex items-center justify-between text-[11px] text-muted-foreground px-1">
              <span>
                已选 <span className="font-medium text-foreground">{selectedIds.size}</span> / 共{' '}
                {allUsers.length} 人
              </span>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving || loading || loadError}>
            {saving ? '保存中...' : '保存用户'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/* ───────── 用户组分配树形对话框 ───────── */

interface RoleAssignGroup {
  id: number;
  name: string;
  parentId?: number | null;
}

interface RoleAssignGroupsResponse {
  allGroups: RoleAssignGroup[];
  assignedGroupIds: number[];
}

function GroupAssignDialog({
  open,
  role,
  onOpenChange,
  onSaved,
}: {
  open: boolean;
  role: Role | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [allGroups, setAllGroups] = useState<RoleAssignGroup[]>([]);
  // 已勾选的组 ID 集合（包含父子节点）
  const [checkedIds, setCheckedIds] = useState<Set<number>>(new Set());
  // 展开节点集合
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  // 加载失败标记：失败时禁用保存按钮，避免空列表提交导致清空所有绑定
  const [loadError, setLoadError] = useState(false);

  useEffect(() => {
    if (!open || !role) return;
    setLoading(true);
    setLoadError(false);
    api
      .get<RoleAssignGroupsResponse>(`/ovpn/role/${role.id}/groups`)
      .then((res) => {
        const data = res ?? { allGroups: [], assignedGroupIds: [] };
        setAllGroups(data.allGroups ?? []);
        setCheckedIds(new Set(data.assignedGroupIds ?? []));
        // 默认展开所有有子节点的组
        const expSet = new Set<number>();
        (data.allGroups ?? []).forEach((g) => {
          if ((data.allGroups ?? []).some((c) => c.parentId === g.id)) {
            expSet.add(g.id);
          }
        });
        setExpanded(expSet);
      })
      .catch((e) => {
        toast.error(`加载用户组失败：${messageOf(e)}`);
        setLoadError(true);
      })
      .finally(() => setLoading(false));
  }, [open, role]);

  // 收集指定节点的整个子树 ID（含自身，用于点击父节点时整体勾选/取消）
  function collectSubtreeIds(groupId: number): number[] {
    const result: number[] = [groupId];
    const visit = (pid: number) => {
      const children = allGroups.filter((g) => g.parentId === pid);
      children.forEach((c) => {
        result.push(c.id);
        visit(c.id);
      });
    };
    visit(groupId);
    return result;
  }

  function toggleExpand(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleGroupCheck(group: RoleAssignGroup) {
    // Default 组（ID=1）拒绝勾选
    if (group.id === 1) return;
    // 判断点击节点当前是否已勾选（基于节点本身是否在 checkedIds 中）
    const isChecked = checkedIds.has(group.id);
    // 收集整个子树 ID（含中间节点），确保勾选/取消时整棵子树一致
    const subtreeIds = collectSubtreeIds(group.id);
    setCheckedIds((prev) => {
      const next = new Set(prev);
      if (isChecked) {
        // 取消整个子树
        subtreeIds.forEach((id) => next.delete(id));
      } else {
        // 选中整个子树
        subtreeIds.forEach((id) => next.add(id));
      }
      return next;
    });
  }

  // 提交时发送所有勾选的非 Default 组 ID（含父子节点）
  // checkedIds 存储用户明确勾选的节点，UI 显示与提交数据完全一致
  function getSubmitIds(): number[] {
    return Array.from(checkedIds).filter((id) => id !== 1);
  }

  async function handleSubmit() {
    if (!role) return;
    setSaving(true);
    try {
      await api.putJson<{ message: string }>(`/ovpn/role/${role.id}/groups`, {
        groupIds: getSubmitIds(),
      });
      toast.success('用户组已分配');
      onOpenChange(false);
      onSaved();
    } catch (e) {
      toast.error(`保存失败：${messageOf(e)}`);
    } finally {
      setSaving(false);
    }
  }

  // 构建树形展示：使用 buildTree 把扁平数组按 depth 排序
  const displayList = useMemo(() => {
    // buildTree 接收 GroupRecord[]，我们转换一下
    const groups: GroupRecord[] = allGroups.map((g) => ({
      id: g.id,
      name: g.name,
      parent_id: g.parentId,
    }));
    return buildTree(groups);
  }, [allGroups]);

  function renderGroupNode(item: GroupRecord & { depth: number }): React.ReactNode {
    const original = allGroups.find((g) => g.id === item.id);
    if (!original) return null;
    const isDefault = item.id === 1;
    const hasChildren = allGroups.some((g) => g.parentId === item.id);
    const isExpanded = expanded.has(item.id);
    // 选中状态：基于节点本身是否在 checkedIds 中（与提交数据一致）
    const checked = checkedIds.has(item.id);
    // 半选状态：自身未选中但有后代节点被选中（仅用于 UI 提示）
    const subtreeIds = collectSubtreeIds(item.id);
    const descendantCheckedCount = subtreeIds.filter(
      (id) => id !== item.id && checkedIds.has(id),
    ).length;
    const indeterminate = !checked && descendantCheckedCount > 0;

    return (
      <div key={item.id} className="select-none">
        <div
          className={cn(
            'flex items-center gap-2 rounded-md py-1.5 pr-2 transition-colors',
            isDefault ? 'opacity-70' : 'hover:bg-muted/50',
          )}
          style={{ paddingLeft: item.depth * 20 + 4 }}
        >
          {hasChildren ? (
            <button
              type="button"
              onClick={() => toggleExpand(item.id)}
              className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
              aria-label={isExpanded ? '收起' : '展开'}
            >
              {isExpanded ? (
                <ChevronDown className="h-3.5 w-3.5" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5" />
              )}
            </button>
          ) : (
            <span className="inline-block h-4 w-4 shrink-0" />
          )}
          <Checkbox
            checked={indeterminate ? 'indeterminate' : checked}
            onCheckedChange={() => toggleGroupCheck(original)}
            disabled={isDefault}
            title={isDefault ? '默认组不支持绑定角色' : undefined}
          />
          <span className={cn('text-sm', 'font-medium')}>{item.name}</span>
          {/* Default 组 Badge */}
          {isDefault && (
            <Badge variant="outline" className="text-[10px] px-1.5 py-0">
              默认组
            </Badge>
          )}
          {hasChildren && (
            <span className="ml-auto text-[10px] text-muted-foreground">
              {descendantCheckedCount}/{subtreeIds.length - 1}
            </span>
          )}
        </div>
        {hasChildren && isExpanded && (
          <div className="space-y-0.5">
            {displayList
              .filter((c) => c.parent_id === item.id)
              .map((c) => renderGroupNode(c))}
          </div>
        )}
      </div>
    );
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!saving) onOpenChange(v); }}>
      <DialogContent
        className="max-w-[680px]"
        onPointerDownOutside={(e) => { if (saving) e.preventDefault(); }}
        onEscapeKeyDown={(e) => { if (saving) e.preventDefault(); }}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Network className="h-5 w-5 text-[var(--accent)]" />
            分配用户组：{role?.name}
          </DialogTitle>
          <DialogDescription>
            勾选用户组后保存，该组将成为角色的默认组；新建用户未指定角色时自动继承所在组角色
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="py-10 text-center text-sm text-muted-foreground">正在加载...</div>
        ) : (
          <div className="space-y-2">
            <div className="max-h-[480px] overflow-y-auto rounded-lg border bg-muted/20 p-2">
              {displayList.length === 0 ? (
                <div className="py-6 text-center text-sm text-muted-foreground">暂无用户组</div>
              ) : (
                displayList
                  .filter((n) => n.parent_id === null || !displayList.some((p) => p.id === n.parent_id))
                  .map((n) => renderGroupNode(n))
              )}
            </div>
            <div className="flex items-center justify-between text-[11px] text-muted-foreground px-1">
              <span>
                已选 <span className="font-medium text-foreground">{getSubmitIds().length}</span> 个用户组
              </span>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving || loading || loadError}>
            {saving ? '保存中...' : '保存用户组'}
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
  // 用户分配对话框状态
  const [userAssignOpen, setUserAssignOpen] = useState(false);
  const [userAssignRole, setUserAssignRole] = useState<Role | null>(null);
  // 用户组分配对话框状态
  const [groupAssignOpen, setGroupAssignOpen] = useState(false);
  const [groupAssignRole, setGroupAssignRole] = useState<Role | null>(null);
  // 搜索与筛选
  const [searchText, setSearchText] = useState('');
  const [filterStatus, setFilterStatus] = useState<'all' | 'enabled' | 'disabled'>('all');

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

  function openUserAssign(role: Role) {
    setUserAssignRole(role);
    setUserAssignOpen(true);
  }

  function openGroupAssign(role: Role) {
    setGroupAssignRole(role);
    setGroupAssignOpen(true);
  }

  function askDelete(role: Role) {
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

  // 搜索过滤：按名称、编码、描述匹配；按启用状态筛选
  const filteredItems = useMemo(() => {
    const kw = searchText.trim().toLowerCase();
    return (rolesState.data ?? []).filter((item) => {
      // 关键词过滤
      if (kw) {
        const hit =
          item.name.toLowerCase().includes(kw) ||
          item.code.toLowerCase().includes(kw) ||
          (item.description ?? '').toLowerCase().includes(kw);
        if (!hit) return false;
      }
      // 状态过滤
      if (filterStatus === 'enabled' && !item.isEnable) return false;
      if (filterStatus === 'disabled' && item.isEnable) return false;
      return true;
    });
  }, [rolesState.data, searchText, filterStatus]);

  const items = filteredItems;
  const tree = treeState.data ?? [];
  const pagination = usePagination(items, `roles-${reloadKey}-${searchText}-${filterStatus}`, 10);

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
        key: 'userCount',
        header: '用户数',
        className: 'text-center',
        render: (item) => (
          <Badge variant="outline" className="font-mono">
            {item.userCount ?? 0}
          </Badge>
        ),
      },
      {
        key: 'groupCount',
        header: '用户组数',
        className: 'text-center',
        render: (item) => (
          <Badge variant="outline" className="font-mono">
            {item.groupCount ?? 0}
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
            <HasPermission code="role:assign_users">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => openUserAssign(item)}
                title="分配用户"
              >
                <Users className="h-3.5 w-3.5 mr-1" />
                分配用户
              </Button>
            </HasPermission>
            <HasPermission code="role:assign_groups">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => openGroupAssign(item)}
                title="分配用户组"
              >
                <Network className="h-3.5 w-3.5 mr-1" />
                分配用户组
              </Button>
            </HasPermission>
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
            <HasPermission code="role:delete">
              {!item.isBuiltin && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 px-2 text-destructive hover:text-destructive"
                  onClick={() => askDelete(item)}
                  title="删除角色"
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  删除
                </Button>
              )}
            </HasPermission>
          </div>
        ),
      },
    ],
    [],
  );

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Access Control" title="角色管理" description="管理角色与权限分配，控制菜单与按钮可见性" />

      {/* 操作工具栏：搜索、筛选 在左，刷新、新建 在右 */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-48">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="搜索名称、编码、描述"
            className="pl-9 h-11 sm:h-8"
          />
        </div>
        <Select value={filterStatus} onValueChange={(v) => setFilterStatus(v as 'all' | 'enabled' | 'disabled')}>
          <SelectTrigger className="w-[110px] h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="enabled">启用</SelectItem>
            <SelectItem value="disabled">禁用</SelectItem>
          </SelectContent>
        </Select>
        <div className="ml-auto flex items-center gap-2">
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
        </div>
      </div>

      {rolesState.loading ? (
        <div className="py-10 text-center text-sm text-muted-foreground">正在加载...</div>
      ) : items.length === 0 ? (
        <div className="py-10 text-center">
          <ShieldCheck className="mx-auto h-10 w-10 text-muted-foreground/50" />
          <p className="mt-2 text-sm font-medium">暂无角色</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {searchText || filterStatus !== 'all'
              ? '没有匹配的角色，请调整搜索条件'
              : '系统启动时会自动创建「系统超管」与「普通用户」两个内置角色'}
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

      <RoleFormDialog state={form} setForm={setForm} onSaved={reload} />

      <PermissionAssignDialog
        open={permOpen}
        role={permRole}
        tree={tree}
        onOpenChange={setPermOpen}
        onSaved={reload}
      />

      <UserAssignDialog
        open={userAssignOpen}
        role={userAssignRole}
        onOpenChange={setUserAssignOpen}
        onSaved={reload}
      />

      <GroupAssignDialog
        open={groupAssignOpen}
        role={groupAssignRole}
        onOpenChange={setGroupAssignOpen}
        onSaved={reload}
      />

      {confirm && <ConfirmDialog state={confirm} onClose={() => setConfirm(null)} />}
    </div>
  );
}
