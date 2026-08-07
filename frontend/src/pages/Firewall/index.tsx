import { useState } from 'react';
import { toast } from 'sonner';
import { Plus, Pencil, Trash2, Power, PowerOff } from 'lucide-react';
import { api } from '@/api';
import type { FirewallRecord, GroupRecord } from '@/types';
import { normalizeList, buildTree, messageOf } from '@/lib/format';
import { trimText, isValidIpOrCidrList } from '@/lib/validators';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { PageHeader } from '@/components/PageHeader';
import { DataTable, type Column } from '@/components/DataTable';
import { StatusBadge } from '@/components/StatusBadge';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { Card, CardContent } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Textarea } from '@/ui/textarea';
import { Switch } from '@/ui/switch';
import { Checkbox } from '@/ui/checkbox';
import { Separator } from '@/ui/separator';
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
  DialogDescription,
  DialogFooter,
} from '@/ui/dialog';

type FieldErrors = Record<string, string>;

export default function FirewallPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [confirmState, setConfirmState] = useState<ConfirmState>();

  // Dialog states
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [editTarget, setEditTarget] = useState<FirewallRecord>();

  // Data
  const firewallsState = useAsync(
    () => api.get<unknown>('/ovpn/firewall').then((v) => normalizeList<FirewallRecord>(v, ['firewalls', 'data'])),
    [reloadKey],
  );
  const groupsState = useAsync(
    () => api.get<unknown>('/ovpn/group').then((v) => normalizeList<GroupRecord>(v, ['groups', 'data'])),
    [reloadKey],
  );

  const firewalls = firewallsState.data || [];
  const groups = groupsState.data || [];
  const tree = buildTree(groups);

  const reload = () => setReloadKey((k) => k + 1);
  const notify = (type: 'success' | 'error' | 'info', message: string) => {
    if (type === 'success') toast.success(message);
    else if (type === 'error') toast.error(message);
    else toast.info(message);
  };

  const pagination = usePagination(firewalls, String(firewalls.length));

  // Helper to render group names
  const groupNames = (items?: GroupRecord[]) => items?.map((g) => g.name).join(' / ') || '';

  // --- Actions ---
  function patchFirewall(firewall: FirewallRecord, nextStatus: boolean) {
    api.patchForm<{ message: string }>('/ovpn/firewall', {
      id: firewall.id,
      sip: firewall.sip || '',
      dip: firewall.dip || '',
      sg: firewall.sg?.map((g) => g.id).join(',') || '',
      dg: firewall.dg?.map((g) => g.id).join(',') || '',
      policy: firewall.policy,
      comment: firewall.comment || '',
      status: nextStatus,
    })
      .then((result) => {
        notify('success', result.message || '更新成功');
        reload();
      })
      .catch((error) => notify('error', messageOf(error)));
  }

  function deleteFirewall(firewall: FirewallRecord) {
    setConfirmState({
      title: '删除防火墙规则',
      message: `确认删除防火墙规则 #${firewall.id} 吗？`,
      danger: true,
      onConfirm: async () => {
        const result = await api.delete<{ message: string }>(`/ovpn/firewall/${firewall.id}`);
        notify('success', result.message || '删除成功');
        reload();
      },
    });
  }

  function openAddForm() {
    setFormMode('add');
    setEditTarget(undefined);
    setFormOpen(true);
  }

  function openEditForm(firewall: FirewallRecord) {
    setFormMode('edit');
    setEditTarget(firewall);
    setFormOpen(true);
  }

  // --- Table columns ---
  const columns: Column<FirewallRecord>[] = [
    {
      key: 'id',
      header: 'ID',
      className: 'w-[60px]',
      sortable: true,
      sortAccessor: (f) => f.id ?? 0,
      render: (f) => f.id,
    },
    {
      key: 'source',
      header: '源',
      sortable: true,
      sortAccessor: (f) => `${f.sip ?? ''}${groupNames(f.sg)}`,
      render: (f) => {
        const parts = [f.sip, groupNames(f.sg)].filter(Boolean);
        return parts.join(' / ') || '-';
      },
    },
    {
      key: 'dest',
      header: '目的',
      sortable: true,
      sortAccessor: (f) => `${f.dip ?? ''}${groupNames(f.dg)}`,
      render: (f) => {
        const parts = [f.dip, groupNames(f.dg)].filter(Boolean);
        return parts.join(' / ') || '-';
      },
    },
    {
      key: 'policy',
      header: '策略',
      className: 'w-[80px]',
      sortable: true,
      sortAccessor: (f) => (f.policy === 'accept' ? 1 : 0),
      render: (f) => (
        <StatusBadge status={f.policy === 'accept' ? 'success' : 'danger'}>
          {f.policy === 'accept' ? '允许' : '拒绝'}
        </StatusBadge>
      ),
    },
    {
      key: 'status',
      header: '状态',
      className: 'w-[80px]',
      sortable: true,
      sortAccessor: (f) => (f.status === false ? 0 : 1),
      render: (f) => (
        <StatusBadge status={f.status === false ? 'danger' : 'success'}>
          {f.status === false ? '禁用' : '启用'}
        </StatusBadge>
      ),
    },
    {
      key: 'comment',
      header: '备注',
      sortable: true,
      sortAccessor: (f) => f.comment ?? '',
      render: (f) => f.comment || '-',
    },
    {
      key: 'actions',
      header: '操作',
      className: 'w-[180px]',
      render: (f) => (
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => openEditForm(f)}>
            <Pencil className="h-4 w-4 mr-1" />
            编辑
          </Button>
          <Button variant="ghost" size="sm" onClick={() => patchFirewall(f, f.status === false)}>
            {f.status === false ? (
              <><Power className="h-4 w-4 mr-1" />启用</>
            ) : (
              <><PowerOff className="h-4 w-4 mr-1" />禁用</>
            )}
          </Button>
          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => deleteFirewall(f)}>
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Policy" title="防火墙规则" description="配置网络访问控制策略">
        <Button onClick={openAddForm}>
          <Plus className="h-4 w-4 mr-1" />
          添加规则
        </Button>
      </PageHeader>

      {/* Data table or empty states */}
      {firewallsState.loading ? (
        <div className="flex items-center justify-center py-12 text-muted-foreground">加载中...</div>
      ) : firewallsState.error ? (
        <div className="flex items-center justify-center py-12 text-destructive">{firewallsState.error}</div>
      ) : firewalls.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">暂无防火墙规则</p>
            <p className="text-sm text-muted-foreground mt-1">
              {groups.length
                ? '点击添加规则，可选择 IP、用户组、允许/拒绝策略。'
                : '先创建用户组，再配置访问策略。'}
            </p>
          </CardContent>
        </Card>
      ) : (
        <DataTable
          columns={columns}
          data={pagination.pagedItems}
          fullData={firewalls}
          page={pagination.page}
          pageSize={pagination.pageSize}
          pageCount={pagination.pageCount}
          total={pagination.total}
          start={pagination.start}
          end={pagination.end}
          onPageChange={pagination.setPage}
          onPageSizeChange={pagination.setPageSize}
          keyFn={(f) => f.id}
        />
      )}

      {/* Add/Edit firewall dialog */}
      <FirewallFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        target={editTarget}
        groups={tree}
        notify={notify}
        reload={reload}
      />

      {/* Confirm dialog */}
      {confirmState && (
        <ConfirmDialog
          state={confirmState}
          onClose={() => setConfirmState(undefined)}
        />
      )}
    </div>
  );
}

// ==================== Firewall Form Dialog ====================

function FirewallFormDialog({
  open,
  onOpenChange,
  mode,
  target,
  groups,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'add' | 'edit';
  target?: FirewallRecord;
  groups: Array<GroupRecord & { depth: number }>;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const [sip, setSip] = useState(target?.sip || '');
  const [dip, setDip] = useState(target?.dip || '');
  const [sg, setSg] = useState(target?.sg?.map((g) => String(g.id)).join(',') || '');
  const [dg, setDg] = useState(target?.dg?.map((g) => String(g.id)).join(',') || '');
  const [policy, setPolicy] = useState(target?.policy || 'accept');
  const [status, setStatus] = useState(target?.status !== false);
  const [comment, setComment] = useState(target?.comment || '');
  const [errors, setErrors] = useState<FieldErrors>({});

  // Reset form when dialog opens with new target
  const resetForm = () => {
    setSip(target?.sip || '');
    setDip(target?.dip || '');
    setSg(target?.sg?.map((g) => String(g.id)).join(',') || '');
    setDg(target?.dg?.map((g) => String(g.id)).join(',') || '');
    setPolicy(target?.policy || 'accept');
    setStatus(target?.status !== false);
    setComment(target?.comment || '');
    setErrors({});
  };

  function clearError(key: string) {
    setErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function validate(): FieldErrors {
    const e: FieldErrors = {};
    if (!trimText(sip) && !trimText(sg)) {
      e.sip = '请至少填写源 IP/CIDR 或选择源用户组';
    }
    if (!trimText(dip) && !trimText(dg)) {
      e.dip = '请至少填写目的 IP/CIDR 或选择目的用户组';
    }
    if (!isValidIpOrCidrList(sip)) e.sip = '源 IP/CIDR 支持逗号分隔，例如 10.8.0.0/24,10.8.0.10';
    if (!isValidIpOrCidrList(dip)) e.dip = '目的 IP/CIDR 支持逗号分隔，例如 192.168.1.0/24';
    if (!['accept', 'drop'].includes(policy)) e.policy = '策略只能选择允许或拒绝';
    return e;
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length) return;

    setSaving(true);
    const form = {
      id: target?.id,
      sip: trimText(sip),
      dip: trimText(dip),
      sg,
      dg,
      policy,
      status,
      comment: trimText(comment),
    };

    try {
      const result = mode === 'add'
        ? await api.postForm<{ message: string }>('/ovpn/firewall', form)
        : await api.patchForm<{ message: string }>('/ovpn/firewall', form);
      notify('success', result.message || '防火墙规则已保存');
      onOpenChange(false);
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  const title = mode === 'add' ? '添加防火墙规则' : `编辑防火墙规则 #${target?.id}`;

  // Group multi-select helpers
  const sgSet = new Set(sg.split(',').filter(Boolean));
  const dgSet = new Set(dg.split(',').filter(Boolean));

  function toggleGroup(setter: typeof setSg, currentSet: Set<string>, groupId: number) {
    const next = new Set(currentSet);
    if (next.has(String(groupId))) next.delete(String(groupId));
    else next.add(String(groupId));
    setter([...next].join(','));
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) resetForm(); onOpenChange(v); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>配置源/目的 IP 或用户组，选择允许或拒绝策略</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Source IP/CIDR */}
          <div className="space-y-2">
            <Label htmlFor="fw-sip">源 IP/CIDR</Label>
            <Input
              id="fw-sip"
              value={sip}
              onChange={(e) => { setSip(e.target.value); clearError('sip'); }}
              placeholder="例如 10.8.0.0/24 或 10.8.0.10"
            />
            {errors.sip && <p className="text-sm text-destructive">{errors.sip}</p>}
          </div>

          {/* Dest IP/CIDR */}
          <div className="space-y-2">
            <Label htmlFor="fw-dip">目的 IP/CIDR</Label>
            <Input
              id="fw-dip"
              value={dip}
              onChange={(e) => { setDip(e.target.value); clearError('dip'); }}
              placeholder="例如 192.168.1.0/24"
            />
            {errors.dip && <p className="text-sm text-destructive">{errors.dip}</p>}
          </div>

          {/* Source group multi-select */}
          {groups.length > 0 && (
            <div className="space-y-2">
              <Label>源用户组</Label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 rounded-md border p-3 max-h-40 overflow-y-auto">
                {groups.map((group) => (
                  <label
                    key={group.id}
                    className="flex items-center gap-2 text-sm cursor-pointer"
                    style={{ paddingLeft: group.depth * 12 }}
                  >
                    <Checkbox
                      checked={sgSet.has(String(group.id))}
                      onCheckedChange={() => toggleGroup(setSg, sgSet, group.id)}
                    />
                    <span>{group.depth ? `${'— '.repeat(group.depth)}${group.name}` : group.name}</span>
                  </label>
                ))}
              </div>
              {errors.sg && <p className="text-sm text-destructive">{errors.sg}</p>}
            </div>
          )}

          {/* Dest group multi-select */}
          {groups.length > 0 && (
            <div className="space-y-2">
              <Label>目的用户组</Label>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 rounded-md border p-3 max-h-40 overflow-y-auto">
                {groups.map((group) => (
                  <label
                    key={group.id}
                    className="flex items-center gap-2 text-sm cursor-pointer"
                    style={{ paddingLeft: group.depth * 12 }}
                  >
                    <Checkbox
                      checked={dgSet.has(String(group.id))}
                      onCheckedChange={() => toggleGroup(setDg, dgSet, group.id)}
                    />
                    <span>{group.depth ? `${'— '.repeat(group.depth)}${group.name}` : group.name}</span>
                  </label>
                ))}
              </div>
              {errors.dg && <p className="text-sm text-destructive">{errors.dg}</p>}
            </div>
          )}

          <Separator />

          {/* Policy select */}
          <div className="space-y-2">
            <Label>策略 <span className="text-destructive">*</span></Label>
            <Select value={policy} onValueChange={(v) => { setPolicy(v); clearError('policy'); }}>
              <SelectTrigger>
                <SelectValue placeholder="选择策略" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="accept">允许 accept</SelectItem>
                <SelectItem value="drop">拒绝 drop</SelectItem>
              </SelectContent>
            </Select>
            {errors.policy && <p className="text-sm text-destructive">{errors.policy}</p>}
          </div>

          {/* Status switch */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
            <Label htmlFor="fw-status">启用规则</Label>
            <Switch id="fw-status" checked={status} onCheckedChange={setStatus} />
          </div>

          {/* Comment */}
          <div className="space-y-2">
            <Label htmlFor="fw-comment">备注</Label>
            <Textarea
              id="fw-comment"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              rows={2}
              placeholder="规则备注说明"
            />
          </div>

          <DialogFooter className="flex-col sm:flex-row">
            <Button type="button" variant="outline" onClick={() => { resetForm(); onOpenChange(false); }}>取消</Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存规则'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
