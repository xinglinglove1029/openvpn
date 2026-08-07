import { useEffect, useMemo, useRef, useState } from 'react';
import {
  KeyRound,
  Plus,
  RefreshCw,
  Trash2,
  Edit,
  Lock,
  ChevronRight,
  ChevronDown,
  ArrowUp,
  ArrowDown,
  FolderPlus,
  Search,
  ChevronsDownUp,
  ChevronsUpDown,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Badge } from '@/ui/badge';
import { Card, CardContent } from '@/ui/card';
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/ui/table';
import { PageHeader } from '@/components/PageHeader';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { HasPermission } from '@/components/HasPermission';
import { useAuth } from '@/store/auth';
import { api } from '@/api';
import { messageOf } from '@/lib/format';
import { cn } from '@/lib/utils';
import type { PermissionTreeNode } from '@/types';

/* ───────── 工具：扁平化权限树（保留层级信息） ───────── */

/** 收集所有后代节点（含自身）的 ID */
function collectDescendantIds(node: PermissionTreeNode): number[] {
  const ids = [node.id];
  if (node.children) {
    for (const c of node.children) {
      ids.push(...collectDescendantIds(c));
    }
  }
  return ids;
}

/* ───────── 权限表单对话框 ───────── */

interface PermFormState {
  open: boolean;
  mode: 'create' | 'edit';
  node?: PermissionTreeNode;
  parent?: PermissionTreeNode | null;
  name: string;
  code: string;
  type: 'menu' | 'button';
  path: string;
  icon: string;
  sort: number;
  parentId: number;
  errors: Record<string, string>;
}

const emptyForm: PermFormState = {
  open: false,
  mode: 'create',
  name: '',
  code: '',
  type: 'menu',
  path: '',
  icon: '',
  sort: 0,
  parentId: 0,
  errors: {},
};

function PermissionFormDialog({
  state,
  setForm,
  tree,
  onSaved,
}: {
  state: PermFormState;
  setForm: React.Dispatch<React.SetStateAction<PermFormState>>;
  tree: PermissionTreeNode[];
  onSaved: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const isBuiltin = state.node?.isBuiltin ?? false;

  function updateField<K extends keyof PermFormState>(key: K, value: PermFormState[K]) {
    setForm((prev) => ({
      ...prev,
      [key]: value,
      errors: { ...prev.errors, [key]: '' },
    }));
  }

  function validate(): Record<string, string> {
    const errors: Record<string, string> = {};
    if (!state.name.trim()) errors.name = '请输入权限名称';
    if (state.mode === 'create' || (!isBuiltin && state.code !== state.node?.code)) {
      if (!state.code.trim()) errors.code = '请输入权限编码';
      else if (!/^[a-zA-Z][a-zA-Z0-9_:]*$/.test(state.code.trim())) {
        errors.code = '编码需以字母开头，仅支持字母、数字、下划线、冒号';
      }
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
      if (state.mode === 'create') {
        await api.postJson<{ message: string }>('/ovpn/permission', {
          name: state.name.trim(),
          code: state.code.trim(),
          type: state.type,
          path: state.path.trim(),
          icon: state.icon.trim(),
          sort: state.sort,
          parentId: state.parentId,
        });
        toast.success('权限已创建');
      } else {
        const payload: Record<string, unknown> = {
          name: state.name.trim(),
          path: state.path.trim(),
          icon: state.icon.trim(),
          sort: state.sort,
          parentId: state.parentId,
        };
        // 内置权限不允许改 code/type，非内置权限才传 code/type
        if (!isBuiltin) {
          payload.code = state.code.trim();
          payload.type = state.type;
        }
        await api.putJson<{ message: string }>(`/ovpn/permission/${state.node?.id}`, payload);
        toast.success('权限已更新');
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

  // 父节点选择列表：仅 menu 类型可选作父节点；编辑模式下排除自身及子树
  const parentOptions = useMemo(() => {
    const excludedIds = new Set<number>();
    if (state.mode === 'edit' && state.node) {
      excludedIds.add(state.node.id);
      for (const id of collectDescendantIds(state.node)) {
        excludedIds.add(id);
      }
    }
    const menus: PermissionTreeNode[] = [];
    function walk(nodes: PermissionTreeNode[], depth = 0) {
      for (const n of nodes) {
        if (n.type === 'menu' && !excludedIds.has(n.id)) {
          menus.push({ ...n, name: depth === 0 ? n.name : `${'　'.repeat(depth)}└ ${n.name}` });
        }
        if (n.children && n.children.length) {
          walk(n.children, depth + 1);
        }
      }
    }
    walk(tree);
    return menus;
  }, [tree, state.mode, state.node]);

  return (
    <Dialog open={state.open} onOpenChange={(open) => !open && close()}>
      <DialogContent className="max-w-[560px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="h-5 w-5 text-[var(--accent)]" />
            {state.mode === 'create' ? '新建权限' : '编辑权限'}
          </DialogTitle>
          <DialogDescription>
            {isBuiltin
              ? '内置权限：编码与类型不可修改，可调整名称、路径、图标、排序与父节点'
              : '自定义权限：可编辑所有字段；菜单类型可作为父节点挂载子权限'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              名称<span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                value={state.name}
                onChange={(e) => updateField('name', e.target.value)}
                placeholder="如 概览、创建用户"
                maxLength={64}
                className={cn(state.errors.name && 'border-destructive focus-visible:ring-destructive/40')}
              />
              {state.errors.name && (
                <p className="text-xs font-medium text-destructive">{state.errors.name}</p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              编码<span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                value={state.code}
                onChange={(e) => updateField('code', e.target.value)}
                placeholder="如 menu:overview、user:create"
                maxLength={64}
                disabled={isBuiltin || (state.mode === 'edit' && isBuiltin)}
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

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">类型</Label>
            <Select
              value={state.type}
              onValueChange={(v) => updateField('type', v as 'menu' | 'button')}
              disabled={isBuiltin}
            >
              <SelectTrigger className={cn(isBuiltin && 'bg-muted/40 cursor-not-allowed')}>
                <SelectValue placeholder="选择类型" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="menu">菜单</SelectItem>
                <SelectItem value="button">按钮</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">路径</Label>
            <Input
              value={state.path}
              onChange={(e) => updateField('path', e.target.value)}
              placeholder="菜单：/overview；按钮留空"
              maxLength={255}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">图标</Label>
            <Input
              value={state.icon}
              onChange={(e) => updateField('icon', e.target.value)}
              placeholder="如 LayoutDashboard、Users（仅菜单需要）"
              maxLength={64}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">排序</Label>
            <Input
              type="number"
              value={String(state.sort)}
              onChange={(e) => updateField('sort', Number(e.target.value) || 0)}
              placeholder="数字越小越靠前"
              min={0}
              max={9999}
            />
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-[100px_1fr] items-start gap-2 sm:gap-4">
            <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">父节点</Label>
            <Select
              value={String(state.parentId)}
              onValueChange={(v) => updateField('parentId', Number(v))}
            >
              <SelectTrigger>
                <SelectValue placeholder="（无）作为根菜单" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">（无）作为根菜单</SelectItem>
                {parentOptions.map((m) => (
                  <SelectItem key={m.id} value={String(m.id)}>
                    {m.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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

/* ───────── 主页面 ───────── */

export default function PermissionsPage() {
  const { permissionTree, reloadPermissionTree } = useAuth();
  const [reloadKey, setReloadKey] = useState(0);
  const [form, setForm] = useState<PermFormState>(emptyForm);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  // 搜索与筛选
  const [searchText, setSearchText] = useState('');
  const [filterType, setFilterType] = useState<'all' | 'menu' | 'button'>('all');
  // 展开节点集合：默认折叠（空 Set），用户点击 chevron 或搜索命中祖先链时才展开
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());
  // 全部展开/折叠切换状态：默认折叠
  const [isAllExpanded, setIsAllExpanded] = useState(false);

  // 重新拉取权限树（保存/删除后刷新）
  const reload = () => {
    setReloadKey((k) => k + 1);
    reloadPermissionTree();
  };

  useEffect(() => {
    if (reloadKey > 0) {
      reloadPermissionTree();
    }
  }, [reloadKey, reloadPermissionTree]);

  // 首次加载权限树：默认保持折叠状态（不自动展开）
  // 使用 ref 避免初始化逻辑重复覆盖用户手动展开的节点
  const initializedRef = useRef(false);
  useEffect(() => {
    if (permissionTree.length && !initializedRef.current) {
      initializedRef.current = true;
      // 默认折叠：不做任何 expanded 修改（保留初始空 Set）
    }
  }, [permissionTree]);

  // 搜索过滤：对权限树进行搜索，匹配节点的路径上所有祖先都保留
  const filteredTree = useMemo(() => {
    const kw = searchText.trim().toLowerCase();
    if (!kw && filterType === 'all') return permissionTree;

    function matchNode(node: PermissionTreeNode): boolean {
      const kwMatch = !kw ||
        node.name.toLowerCase().includes(kw) ||
        node.code.toLowerCase().includes(kw) ||
        (node.path ?? '').toLowerCase().includes(kw);
      const typeMatch = filterType === 'all' || node.type === filterType;
      return kwMatch && typeMatch;
    }

    // 递归过滤：保留匹配节点及其祖先链
    function filterNodes(nodes: PermissionTreeNode[]): PermissionTreeNode[] {
      const result: PermissionTreeNode[] = [];
      for (const node of nodes) {
        const children = node.children ? filterNodes(node.children) : [];
        if (matchNode(node) || children.length > 0) {
          result.push({ ...node, children });
        }
      }
      return result;
    }
    return filterNodes(permissionTree);
  }, [permissionTree, searchText, filterType]);

  // 一键全部展开
  function expandAll() {
    const set = new Set<number>();
    function walk(nodes: PermissionTreeNode[]) {
      for (const n of nodes) {
        if (n.children && n.children.length) {
          set.add(n.id);
          walk(n.children);
        }
      }
    }
    walk(permissionTree);
    setExpanded(set);
  }

  // 一键全部折叠
  function collapseAll() {
    setExpanded(new Set());
  }

  // 切换全部展开/折叠：点击一次展开，再点击一次折叠
  function toggleExpandAll() {
    if (isAllExpanded) {
      collapseAll();
      setIsAllExpanded(false);
    } else {
      expandAll();
      setIsAllExpanded(true);
    }
  }

  function toggleExpand(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function openCreate(parent?: PermissionTreeNode | null) {
    setForm({
      ...emptyForm,
      open: true,
      mode: 'create',
      parent: parent ?? null,
      type: parent ? 'button' : 'menu',
      parentId: parent?.id ?? 0,
      sort: parent?.children?.length ?? permissionTree.length + 1,
      errors: {},
    });
  }

  function openEdit(node: PermissionTreeNode) {
    setForm({
      open: true,
      mode: 'edit',
      node,
      name: node.name,
      code: node.code,
      type: node.type,
      path: node.path ?? '',
      icon: node.icon ?? '',
      sort: node.sort,
      parentId: node.parentId ?? 0,
      errors: {},
    });
  }

  function askDelete(node: PermissionTreeNode) {
    if (node.isBuiltin) {
      toast.error('内置权限不允许删除');
      return;
    }
    const childCount = node.children?.length ?? 0;
    setConfirm({
      title: '删除权限',
      message:
        childCount > 0
          ? `确认要删除权限「${node.name}」吗？将同时删除其 ${childCount} 个子节点及所有关联的角色权限，删除后无法恢复。`
          : `确认要删除权限「${node.name}」吗？将同时删除所有关联的角色权限，删除后无法恢复。`,
      danger: true,
      onConfirm: async () => {
        try {
          await api.delete<{ message: string }>(`/ovpn/permission/${node.id}`);
          toast.success('删除成功');
          reload();
          setConfirm(null);
        } catch (e) {
          toast.error(`删除失败：${messageOf(e)}`);
        }
      },
    });
  }

  // 上移/下移：在同一父节点下找兄弟节点，交换 sort 值，批量 PUT /permission/sort
  async function moveNode(node: PermissionTreeNode, direction: 'up' | 'down') {
    // 找到同父节点的兄弟列表
    let siblings: PermissionTreeNode[] = [];
    if (node.parentId && node.parentId > 0) {
      const parent = findNodeById(permissionTree, node.parentId);
      siblings = parent?.children ?? [];
    } else {
      siblings = permissionTree.filter((n) => !n.parentId || n.parentId === 0);
    }
    // 按 sort 排序
    const sorted = [...siblings].sort((a, b) => a.sort - b.sort);
    const idx = sorted.findIndex((n) => n.id === node.id);
    if (idx < 0) return;
    const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
    if (swapIdx < 0 || swapIdx >= sorted.length) {
      toast.error(direction === 'up' ? '已是第一个' : '已是最后一个');
      return;
    }
    const swapNode = sorted[swapIdx];
    // 交换 sort 值，批量更新
    const payload = [
      { id: node.id, sort: swapNode.sort },
      { id: swapNode.id, sort: node.sort },
    ];
    try {
      await api.putJson<{ message: string }>('/ovpn/permission/sort', payload);
      toast.success('排序已更新');
      reload();
    } catch (e) {
      toast.error(`排序失败：${messageOf(e)}`);
    }
  }

  function findNodeById(nodes: PermissionTreeNode[], id: number): PermissionTreeNode | null {
    for (const n of nodes) {
      if (n.id === id) return n;
      if (n.children) {
        const found = findNodeById(n.children, id);
        if (found) return found;
      }
    }
    return null;
  }

  // 渲染权限树表格
  function renderRows(nodes: PermissionTreeNode[], depth = 0): React.ReactNode {
    const sorted = [...nodes].sort((a, b) => a.sort - b.sort);
    return sorted.map((node) => {
      const hasChildren = !!node.children && node.children.length > 0;
      const isExpanded = expanded.has(node.id);
      const isMenu = node.type === 'menu';
      return (
        <React.Fragment key={node.id}>
          <TableRow className="hover:bg-muted/40">
            <TableCell className="font-medium">
              <div className="flex items-center gap-2" style={{ paddingLeft: depth * 20 }}>
                {hasChildren ? (
                  <button
                    type="button"
                    onClick={() => toggleExpand(node.id)}
                    className="flex h-4 w-4 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
                    aria-label={isExpanded ? '收起' : '展开'}
                  >
                    {isExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                  </button>
                ) : (
                  <span className="inline-block h-4 w-4 shrink-0" />
                )}
                <span className={cn(isMenu ? 'font-semibold' : 'text-foreground/80')}>{node.name}</span>
                {node.isBuiltin && (
                  <Badge
                    variant="outline"
                    className="bg-[var(--accent)]/10 border-[var(--accent)]/30 text-[var(--accent)] text-[10px] px-1.5 py-0"
                  >
                    <Lock className="h-2.5 w-2.5 mr-0.5" />
                    内置
                  </Badge>
                )}
              </div>
            </TableCell>
            <TableCell>
              <span className="font-mono text-xs text-muted-foreground">{node.code}</span>
            </TableCell>
            <TableCell>
              <Badge variant="outline" className={cn(isMenu ? 'border-blue-500/30 text-blue-600' : 'border-amber-500/30 text-amber-600')}>
                {isMenu ? '菜单' : '按钮'}
              </Badge>
            </TableCell>
            <TableCell>
              <span className="text-xs text-muted-foreground">{node.path || '-'}</span>
            </TableCell>
            <TableCell>
              <span className="text-xs font-mono text-muted-foreground">{node.icon || '-'}</span>
            </TableCell>
            <TableCell className="text-center">
              <span className="font-mono text-sm">{node.sort}</span>
            </TableCell>
            <TableCell>
              <div className="flex items-center gap-1">
                <HasPermission code="permission:manage">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-7 px-2"
                    onClick={() => moveNode(node, 'up')}
                    title="上移"
                  >
                    <ArrowUp className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-7 px-2"
                    onClick={() => moveNode(node, 'down')}
                    title="下移"
                  >
                    <ArrowDown className="h-3.5 w-3.5" />
                  </Button>
                  {isMenu && (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-7 px-2"
                      onClick={() => openCreate(node)}
                      title="新增子节点"
                    >
                      <FolderPlus className="h-3.5 w-3.5 mr-1" />
                      子节点
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-7 px-2"
                    onClick={() => openEdit(node)}
                    title="编辑"
                  >
                    <Edit className="h-3.5 w-3.5 mr-1" />
                    编辑
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className={cn(
                      'h-7 px-2 text-destructive hover:text-destructive',
                      node.isBuiltin && 'opacity-40 cursor-not-allowed',
                    )}
                    disabled={node.isBuiltin}
                    onClick={() => !node.isBuiltin && askDelete(node)}
                    title={node.isBuiltin ? '内置权限不允许删除' : '删除'}
                  >
                    <Trash2 className="h-3.5 w-3.5 mr-1" />
                    删除
                  </Button>
                </HasPermission>
              </div>
            </TableCell>
          </TableRow>
          {hasChildren && isExpanded && renderRows(node.children!, depth + 1)}
        </React.Fragment>
      );
    });
  }

  function renderMobileCards(nodes: PermissionTreeNode[], depth = 0): React.ReactNode {
    return [...nodes].sort((a, b) => a.sort - b.sort).map((node) => {
      const hasChildren = Boolean(node.children?.length);
      const isExpanded = expanded.has(node.id);
      const isMenu = node.type === 'menu';
      return (
        <div key={node.id} className="space-y-2">
          <Card data-testid="permissions-mobile-card" className="overflow-hidden" style={{ marginLeft: depth ? `${Math.min(depth, 3) * 12}px` : undefined }}>
            <CardContent className="space-y-3 p-3 sm:p-4">
              <div className="flex min-w-0 items-start gap-2">
                {hasChildren ? (
                  <Button type="button" size="icon" variant="ghost" className="h-11 w-11 shrink-0" onClick={() => toggleExpand(node.id)} aria-label={isExpanded ? '\u6536\u8d77\u5b50\u6743\u9650' : '\u5c55\u5f00\u5b50\u6743\u9650'} aria-expanded={isExpanded}>
                    {isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                  </Button>
                ) : <span className="h-11 w-11 shrink-0" aria-hidden />}
                <div className="min-w-0 flex-1 space-y-1 pt-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={cn('min-w-0 break-words text-sm', isMenu && 'font-semibold')}>{node.name}</span>
                    <Badge variant="outline" className={cn(isMenu ? 'border-blue-500/30 text-blue-600' : 'border-amber-500/30 text-amber-600')}>{isMenu ? '\u83dc\u5355' : '\u6309\u94ae'}</Badge>
                    {depth > 2 && <Badge variant="outline" className="text-xs">{'\u5c42\u7ea7'} {depth + 1}</Badge>}
                    {node.isBuiltin && <Badge variant="outline" className="text-xs"><Lock className="mr-1 h-3 w-3" />{'\u5185\u7f6e'}</Badge>}
                  </div>
                  <p className="break-all font-mono text-xs text-muted-foreground">{node.code}</p>
                </div>
              </div>
              <dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-2 gap-y-1 text-xs">
                <dt className="text-muted-foreground">{'\u8def\u5f84'}</dt><dd className="min-w-0 break-words">{node.path || '-'}</dd>
                <dt className="text-muted-foreground">{'\u56fe\u6807'}</dt><dd className="min-w-0 break-words font-mono">{node.icon || '-'}</dd>
                <dt className="text-muted-foreground">{'\u6392\u5e8f'}</dt><dd>{node.sort}</dd>
              </dl>
              <HasPermission code="permission:manage"><div className="flex flex-wrap gap-2 border-t pt-2">
                <Button size="sm" variant="outline" className="min-h-11" onClick={() => moveNode(node, 'up')}><ArrowUp className="mr-1 h-4 w-4" /> {'\u4e0a\u79fb'}</Button>
                <Button size="sm" variant="outline" className="min-h-11" onClick={() => moveNode(node, 'down')}><ArrowDown className="mr-1 h-4 w-4" /> {'\u4e0b\u79fb'}</Button>
                {isMenu && <Button size="sm" variant="outline" className="min-h-11" onClick={() => openCreate(node)}><FolderPlus className="mr-1 h-4 w-4" /> {'\u65b0\u589e\u5b50\u8282\u70b9'}</Button>}
                <Button size="sm" variant="outline" className="min-h-11" onClick={() => openEdit(node)}><Edit className="mr-1 h-4 w-4" /> {'\u7f16\u8f91'}</Button>
                <Button size="sm" variant="outline" className="min-h-11 text-destructive" disabled={node.isBuiltin} onClick={() => !node.isBuiltin && askDelete(node)}><Trash2 className="mr-1 h-4 w-4" /> {'\u5220\u9664'}</Button>
              </div></HasPermission>
            </CardContent>
          </Card>
          {hasChildren && isExpanded && renderMobileCards(node.children!, depth + 1)}
        </div>
      );
    });
  }

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Access Control" title="权限管理" description="管理菜单与按钮权限，调整菜单顺序、名称、路径" />

      {/* 操作工具栏：搜索、筛选 在左，刷新、新增、展开/折叠切换 在右 */}
      <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:flex-wrap sm:items-center">
        <div className="relative w-full sm:w-48">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="搜索名称、编码、路径"
            className="h-11 pl-9 sm:h-8"
          />
        </div>
        <Select value={filterType} onValueChange={(v) => setFilterType(v as 'all' | 'menu' | 'button')}>
          <SelectTrigger className="h-11 w-full sm:h-8 sm:w-[110px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部类型</SelectItem>
            <SelectItem value="menu">菜单</SelectItem>
            <SelectItem value="button">按钮</SelectItem>
          </SelectContent>
        </Select>
        <div className="flex flex-wrap items-center gap-2 sm:ml-auto">
          <HasPermission code="permission:view">
            <Button size="sm" variant="outline" onClick={reload}>
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新
            </Button>
          </HasPermission>
          <HasPermission code="permission:manage">
            <Button size="sm" onClick={() => openCreate(null)}>
              <Plus className="h-3.5 w-3.5 mr-1" />
              新增菜单
            </Button>
          </HasPermission>
          <Button size="sm" variant="outline" onClick={toggleExpandAll}>
            {isAllExpanded ? (
              <>
                <ChevronsDownUp className="h-3.5 w-3.5 mr-1" />
                全部折叠
              </>
            ) : (
              <>
                <ChevronsUpDown className="h-3.5 w-3.5 mr-1" />
                全部展开
              </>
            )}
          </Button>
        </div>
      </div>

      {filteredTree.length === 0 ? (
        <div className="py-10 text-center">
          <KeyRound className="mx-auto h-10 w-10 text-muted-foreground/50" />
          <p className="mt-2 text-sm font-medium">暂无权限数据</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {searchText || filterType !== 'all'
              ? '没有匹配的权限节点，请调整搜索条件'
              : '系统启动时会自动初始化内置权限；也可点击"新增菜单"添加自定义权限'}
          </p>
        </div>
      ) : (
        <>
          <div data-testid="permissions-mobile-list" className="lg:hidden">{renderMobileCards(filteredTree)}</div>
          <div data-testid="permissions-desktop-tree" className="hidden rounded-md border lg:block">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="min-w-[200px]">名称</TableHead><TableHead>编码</TableHead><TableHead className="text-center">类型</TableHead><TableHead>路径</TableHead><TableHead>图标</TableHead><TableHead className="text-center">排序</TableHead><TableHead className="min-w-[280px]">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>{renderRows(filteredTree)}</TableBody>
          </Table>
          </div>
        </>
      )}

      <PermissionFormDialog state={form} setForm={setForm} tree={permissionTree} onSaved={reload} />

      {confirm && <ConfirmDialog state={confirm} onClose={() => setConfirm(null)} />}

      <p className="text-xs text-muted-foreground">
        提示：内置权限（标记"内置"）由系统维护，编码与类型不可修改，不可删除；
        自定义权限支持完整增删改查，删除菜单时将级联删除其下所有子节点与角色关联。
      </p>
    </div>
  );
}

// 引入 React 以便 JSX 使用 React.Fragment
import * as React from 'react';
