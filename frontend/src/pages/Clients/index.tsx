import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Search, Plus, Download, Copy, FileText, FolderOpen, Trash2 } from 'lucide-react';
import { api } from '@/api';
import type { ClientRecord, SettingsResponse } from '@/types';
import { normalizeList, messageOf } from '@/lib/format';
import { trimText, isValidSafeName, isValidHost, isValidPort } from '@/lib/validators';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { PageHeader } from '@/components/PageHeader';
import { DataTable, type Column } from '@/components/DataTable';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { Card, CardContent } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Textarea } from '@/ui/textarea';
import { useAuth } from '@/store/auth';
import { useIsMobile } from '@/hooks/useIsMobile';
import { HasPermission } from '@/components/HasPermission';

import { cn } from '@/lib/utils';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/ui/dialog';

type FieldErrors = Record<string, string>;

export default function ClientsPage() {
  const { hasPermission } = useAuth();
  const isMobile = useIsMobile();
  const [reloadKey, setReloadKey] = useState(0);
  const [search, setSearch] = useState('');
  const [confirmState, setConfirmState] = useState<ConfirmState>();

  // Dialog states
  const [addOpen, setAddOpen] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorTarget, setEditorTarget] = useState<{ client: ClientRecord; editor: 'config' | 'ccd'; content: string }>();

  // Data
  const clientsState = useAsync(
    () => api.get<unknown>('/ovpn/client').then((v) => normalizeList<ClientRecord>(v, ['clients', 'data'])),
    [reloadKey],
  );
  const canViewSettings = hasPermission('settings:view');
  const settingsState = useAsync(
    () => {
      if (!canViewSettings) return Promise.resolve(null as SettingsResponse | null);
      return api.get<SettingsResponse>('/ovpn/settings').catch(() => null as SettingsResponse | null);
    },
    [reloadKey, canViewSettings],
  );

  const clients = clientsState.data || [];
  const settings = settingsState.data;

  const reload = () => setReloadKey((k) => k + 1);
  const notify = (type: 'success' | 'error' | 'info', message: string) => {
    if (type === 'success') toast.success(message);
    else if (type === 'error') toast.error(message);
    else toast.info(message);
  };

  // Filtering & pagination
  const filteredClients = clients.filter((c) => {
    const kw = search.toLowerCase().trim();
    if (!kw) return true;
    return [c.name, c.fullName, c.file].filter(Boolean).some((v) => String(v).toLowerCase().includes(kw));
  });
  const pagination = usePagination(filteredClients, search);

  // --- Actions ---
  function deleteClient(client: ClientRecord) {
    setConfirmState({
      title: '删除客户端配置',
      message: `确认删除客户端 ${client.name} 吗？这会吊销证书并删除配置文件。`,
      danger: true,
      onConfirm: async () => {
        const result = await api.delete<{ message: string }>(`/ovpn/client/${encodeURIComponent(client.name)}`);
        notify('success', result.message || '删除成功');
        reload();
      },
    });
  }

  async function openEditor(client: ClientRecord, editor: 'config' | 'ccd') {
    try {
      const result = await api.get<{ content: string }>(`/ovpn/client/${encodeURIComponent(client.name)}/${editor}`);
      setEditorTarget({ client, editor, content: result.content || '' });
      setEditorOpen(true);
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function copyClientFile(client: ClientRecord) {
    if (!navigator.clipboard) {
      notify('error', '当前浏览器不支持剪贴板复制');
      return;
    }
    try {
      const result = await api.get<{ content: string }>(
        `/ovpn/client/${encodeURIComponent(client.name)}/config`,
      );
      await navigator.clipboard.writeText(result.content || '');
      notify('success', '客户端配置已复制到剪贴板');
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  // --- Table columns ---
  const columns: Column<ClientRecord>[] = [
    {
      key: 'name',
      header: '名称',
      sortable: true,
      sortAccessor: (c) => c.name ?? '',
      render: (c) => <span className="font-medium">{c.name}</span>,
    },
    {
      key: 'file',
      header: '文件',
      sortable: true,
      sortAccessor: (c) => c.fullName ?? c.file ?? '',
      render: (c) => c.fullName || c.file || '-',
    },
    {
      key: 'date',
      header: '更新时间',
      sortable: true,
      sortAccessor: (c) => {
        if (!c.date) return 0;
        const ts = new Date(c.date).getTime();
        return Number.isNaN(ts) ? 0 : ts;
      },
      render: (c) => c.date || '-',
    },
    {
      key: 'actions',
      header: '操作',
      className: 'w-[200px]',
      render: (c) => (
        <div className="flex items-center gap-1">
          <HasPermission code="client:download">
            {c.file && (
              <Button variant="ghost" size="sm" className={cn(isMobile && 'h-10 w-10 p-0')} asChild>
                <a href={c.file} download={c.fullName}>
                  <Download className={cn('h-4 w-4', !isMobile && 'mr-1')} />
                  {!isMobile && '下载'}
                </a>
              </Button>
            )}
          </HasPermission>
          <HasPermission code="client:download">
            <Button variant="ghost" size="sm" className={cn(isMobile && 'h-10 w-10 p-0')} onClick={() => copyClientFile(c)}>
              <Copy className={cn('h-4 w-4', !isMobile && 'mr-1')} />
              {!isMobile && '复制'}
            </Button>
          </HasPermission>
          <HasPermission code="client:regenerate">
            <Button variant="ghost" size="sm" className={cn(isMobile && 'h-10 w-10 p-0')} onClick={() => openEditor(c, 'config')}>
              <FileText className={cn('h-4 w-4', !isMobile && 'mr-1')} />
              {!isMobile && '配置'}
            </Button>
          </HasPermission>
          <HasPermission code="client:regenerate">
            <Button variant="ghost" size="sm" className={cn(isMobile && 'h-10 w-10 p-0')} onClick={() => openEditor(c, 'ccd')}>
              <FolderOpen className={cn('h-4 w-4', !isMobile && 'mr-1')} />
              {!isMobile && 'CCD'}
            </Button>
          </HasPermission>
          <HasPermission code="client:delete">
            <Button variant="ghost" size="sm" className={cn('text-destructive', isMobile && 'h-10 w-10 p-0')} onClick={() => deleteClient(c)}>
              <Trash2 className="h-4 w-4" />
            </Button>
          </HasPermission>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Device" title="客户端配置" description="管理 VPN 客户端证书和 .ovpn 配置文件">
        <HasPermission code="client:create">
          <Button onClick={() => setAddOpen(true)}>
            <Plus className="h-4 w-4 mr-1" />
            添加客户端
          </Button>
        </HasPermission>
      </PageHeader>

      {/* Search */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3">
        <div className="relative flex-1 w-full sm:max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder="搜索客户端名称"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
      </div>

      {/* Client download links */}
      {settings?.client?.client_url && Object.keys(settings.client.client_url).length > 0 && (
        <Card>
          <CardContent className="pt-4">
            <div className="flex flex-col sm:flex-row sm:flex-wrap sm:items-center gap-2 sm:gap-4">
              <span className="text-sm font-medium text-muted-foreground">客户端下载：</span>
              {Object.entries(settings.client.client_url).map(([key, url]) => (
                <a
                  key={key}
                  href={String(url)}
                  target="_blank"
                  rel="noreferrer"
                  className="text-sm text-primary underline-offset-4 hover:underline"
                >
                  {key.toUpperCase()} 客户端
                </a>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Data table or empty states */}
      {clientsState.loading ? (
        <div className="flex items-center justify-center py-12 text-muted-foreground">加载中...</div>
      ) : clientsState.error ? null : clients.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">还没有客户端</p>
            <p className="text-sm text-muted-foreground mt-1">
              点击添加客户端生成 .ovpn 配置；本地非 Docker 环境可能缺少 easyrsa。
            </p>
          </CardContent>
        </Card>
      ) : filteredClients.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">没有匹配的客户端配置</p>
            <p className="text-sm text-muted-foreground mt-1">换个客户端名称或配置文件关键词再试。</p>
          </CardContent>
        </Card>
      ) : (
        <DataTable
          columns={columns}
          data={pagination.pagedItems}
          fullData={filteredClients}
          page={pagination.page}
          pageSize={pagination.pageSize}
          pageCount={pagination.pageCount}
          total={pagination.total}
          start={pagination.start}
          end={pagination.end}
          onPageChange={pagination.setPage}
          onPageSizeChange={pagination.setPageSize}
          keyFn={(c) => c.name}
        />
      )}

      {/* Add client dialog */}
      <AddClientDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        notify={notify}
        reload={reload}
        settings={settings ?? null}
      />

      {/* Config/CCD editor dialog */}
      {editorTarget && (
        <ClientEditorDialog
          open={editorOpen}
          onOpenChange={setEditorOpen}
          client={editorTarget.client}
          editor={editorTarget.editor}
          initialContent={editorTarget.content}
          notify={notify}
          reload={reload}
        />
      )}

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

// ==================== Add Client Dialog ====================

function AddClientDialog({
  open,
  onOpenChange,
  notify,
  reload,
  settings,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
  settings: SettingsResponse | null;
}) {
  const [saving, setSaving] = useState(false);
  const [name, setName] = useState('');
  const [serverAddr, setServerAddr] = useState('');
  const [serverPort, setServerPort] = useState('');
  const [ccdConfig, setCcdConfig] = useState('');
  const [clientConfig, setClientConfig] = useState('');
  const [errors, setErrors] = useState<FieldErrors>({});

  // 从系统设置中取默认 VPN 服务器地址和端口
  const defaultAddr = settings?.system?.base?.server_addr ?? '';
  const defaultPort = settings?.openvpn?.ovpn_port ?? '';

  function resetForm() {
    setName('');
    setServerAddr(String(defaultAddr ?? ''));
    setServerPort(String(defaultPort ?? ''));
    setCcdConfig('');
    setClientConfig('');
    setErrors({});
  }

  // 当对话框打开时，用 settings 中的默认值初始化 serverAddr/serverPort
  useEffect(() => {
    if (open) {
      setServerAddr(String(defaultAddr ?? ''));
      setServerPort(String(defaultPort ?? ''));
    }
  }, [open, defaultAddr, defaultPort]);

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
    if (!isValidSafeName(name)) e.name = '客户端名称只能包含字母、数字、点、下划线、中划线，长度 2-64 位';
    if (!isValidHost(serverAddr)) e.serverAddr = 'VPN 服务器地址不能为空，请填写域名或 IP';
    if (!isValidPort(serverPort)) e.serverPort = 'VPN 端口必须是 1-65535 的整数';
    return e;
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length) return;

    setSaving(true);
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/client', {
        name: trimText(name),
        serverAddr: trimText(serverAddr),
        serverPort: trimText(serverPort),
        ccdConfig,
        config: clientConfig,
      });
      notify('success', result.message || '客户端已创建');
      onOpenChange(false);
      resetForm();
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) resetForm(); onOpenChange(v); }}>
      <DialogContent className="w-full sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>添加客户端配置</DialogTitle>
          <DialogDescription>名称会用于生成证书与 .ovpn 文件；地址、端口、CCD、自定义配置会写入当前客户端配置。MFA 验证将根据用户绑定状态自动启用。</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Name */}
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="client-name" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              客户端名称 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="client-name"
                value={name}
                onChange={(e) => { setName(e.target.value); clearError('name'); }}
                autoFocus
                placeholder="例如 user1-client"
              />
              {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
            </div>
          </div>

          {/* Server address */}
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="server-addr" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              VPN 服务器地址 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="server-addr"
                value={serverAddr}
                onChange={(e) => { setServerAddr(e.target.value); clearError('serverAddr'); }}
                placeholder="例如 vpn.example.com"
              />
              {errors.serverAddr && <p className="text-xs text-destructive">{errors.serverAddr}</p>}
            </div>
          </div>

          {/* Server port */}
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="server-port" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              VPN 端口 <span className="text-destructive ml-0.5">*</span>
            </Label>
            <div className="space-y-1.5 min-w-0">
              <Input
                id="server-port"
                value={serverPort}
                onChange={(e) => { setServerPort(e.target.value); clearError('serverPort'); }}
                placeholder="例如 1194"
              />
              {errors.serverPort && <p className="text-xs text-destructive">{errors.serverPort}</p>}
            </div>
          </div>

          {/* CCD config */}
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="ccd-config" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">CCD 配置</Label>
            <div className="space-y-1.5 min-w-0">
              <Textarea
                id="ccd-config"
                value={ccdConfig}
                onChange={(e) => setCcdConfig(e.target.value)}
                rows={4}
                className="font-mono text-xs"
                placeholder="iroute 10.8.0.0 255.255.255.0"
              />
            </div>
          </div>

          {/* Custom config */}
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
            <Label htmlFor="custom-config" className="pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">自定义配置</Label>
            <div className="space-y-1.5 min-w-0">
              <Textarea
                id="custom-config"
                value={clientConfig}
                onChange={(e) => setClientConfig(e.target.value)}
                rows={4}
                className="font-mono text-xs"
                placeholder="pull-filter ignore redirect-gateway"
              />
            </div>
          </div>

          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => { resetForm(); onOpenChange(false); }}>取消</Button>
            <Button type="submit" disabled={saving}>
              {saving ? '生成中...' : '生成客户端'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// ==================== Client Editor Dialog ====================

function ClientEditorDialog({
  open,
  onOpenChange,
  client,
  editor,
  initialContent,
  notify,
  reload,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  client: ClientRecord;
  editor: 'config' | 'ccd';
  initialContent: string;
  notify: (type: 'success' | 'error' | 'info', message: string) => void;
  reload: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const [content, setContent] = useState(initialContent);
  const [errors, setErrors] = useState<FieldErrors>({});

  useEffect(() => {
    setContent(initialContent);
    setErrors({});
  }, [initialContent]);

  function validate(): FieldErrors {
    const next: FieldErrors = {};
    if (!trimText(content)) next.content = '配置内容不能为空';
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
      const result = await api.putForm<{ message: string }>(
        `/ovpn/client/${encodeURIComponent(client.name)}/${editor}`,
        { content },
      );
      notify('success', result.message || '配置已保存');
      onOpenChange(false);
      reload();
    } catch (error) {
      // 后端错误：使用 toast 提醒
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  const title = editor === 'config' ? '客户端配置' : 'CCD 配置';

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}：{client.name}</DialogTitle>
          <DialogDescription>编辑并保存 {editor === 'config' ? '.ovpn 配置文件' : 'CCD 路由配置'} 内容</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-2">
          <Textarea
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              if (errors.content) setErrors((prev) => { const n = { ...prev }; delete n.content; return n; });
            }}
            rows={18}
            aria-invalid={errors.content ? 'true' : undefined}
            className={cn('font-mono text-xs min-h-[300px]', errors.content && 'border-destructive focus-visible:ring-destructive/40')}
          />
          {errors.content && <p className="text-xs text-destructive">{errors.content}</p>}
          <DialogFooter className="flex-col sm:flex-row sm:justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>取消</Button>
            <Button type="submit" disabled={saving}>
              {saving ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
