import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Bell,
  Plus,
  RefreshCw,
  Trash2,
  Edit,
  Send,
  Power,
  Webhook,
  Mail,
  BellRing,
  MessageSquare,
  MessageCircle,
  Hash,
  Radio,
  Eye,
  EyeOff,
} from 'lucide-react';
import { toast } from 'sonner';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Badge } from '@/ui/badge';
import { Switch } from '@/ui/switch';
import { Textarea } from '@/ui/textarea';
import { Separator } from '@/ui/separator';
import { RadioGroup, RadioGroupItem } from '@/ui/radio-group';
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
import { StatusBadge } from '@/components/StatusBadge';
import { ConfirmDialog, type ConfirmState } from '@/components/ConfirmDialog';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { api } from '@/api';
import { messageOf } from '@/lib/format';
import { cn } from '@/lib/utils';
import type {
  ChannelType,
  ChannelTypeMeta,
  NotificationChannel,
} from '@/types';

/* ───────── 工具：图标映射 ───────── */

const iconMap: Record<string, React.ComponentType<{ className?: string }>> = {
  Webhook,
  Mail,
  BellRing,
  MessageSquare,
  MessageCircle,
  Hash,
  Send,
  Radio,
  Bell,
};

function ChannelIcon({ name, className }: { name?: string; className?: string }) {
  const Cmp = (name && iconMap[name]) || Bell;
  return <Cmp className={className} />;
}

/* ───────── 渠道类型 config 字段定义（前端 UI 渲染用） ───────── */

type FieldDef =
  | { kind: 'text'; key: string; label: string; placeholder?: string; required?: boolean }
  | { kind: 'password'; key: string; label: string; placeholder?: string; required?: boolean }
  | { kind: 'textarea'; key: string; label: string; placeholder?: string; rows?: number; required?: boolean }
  | { kind: 'number'; key: string; label: string; min?: number; max?: number; required?: boolean }
  | { kind: 'select'; key: string; label: string; options: { value: string; label: string }[]; required?: boolean }
  | { kind: 'switch'; key: string; label: string };

const configSchemas: Record<ChannelType, FieldDef[]> = {
  webhook: [
    { kind: 'text', key: 'url', label: 'Webhook 地址', placeholder: 'https://example.com/hook', required: true },
    { kind: 'select', key: 'method', label: '请求方法', options: [
      { value: 'POST', label: 'POST' },
      { value: 'PUT', label: 'PUT' },
    ] },
    { kind: 'textarea', key: 'headers', label: '自定义请求头（JSON 字符串，可选）', placeholder: '{"X-Token":"xxx"}', rows: 3 },
    { kind: 'textarea', key: 'body_template', label: '请求体模板（JSON 字符串，可选）', placeholder: '{"title":"{{title}}","content":"{{content}}"}', rows: 4 },
  ],
  email: [
    { kind: 'text', key: 'host', label: 'SMTP 主机', placeholder: 'smtp.example.com', required: true },
    { kind: 'number', key: 'port', label: 'SMTP 端口', min: 1, max: 65535, required: true },
    { kind: 'select', key: 'security', label: '加密方式', options: [
      { value: '', label: '自动（STARTTLS）' },
      { value: 'tls', label: '强制 TLS' },
      { value: 'ssl', label: 'SSL' },
    ] },
    { kind: 'text', key: 'username', label: 'SMTP 用户名' },
    { kind: 'password', key: 'password', label: 'SMTP 密码' },
    { kind: 'text', key: 'from', label: '发件人邮箱', placeholder: 'no-reply@example.com', required: true },
    { kind: 'textarea', key: 'to', label: '收件人（多个换行 / 逗号分隔）', placeholder: 'ops@example.com\nalerts@example.com', rows: 3, required: true },
    { kind: 'text', key: 'subject_prefix', label: '主题前缀（可选）', placeholder: '[OpenVPN]' },
  ],
  dingtalk: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://oapi.dingtalk.com/robot/send?access_token=...', required: true },
    { kind: 'text', key: 'secret', label: '加签密钥（可选）' },
    { kind: 'switch', key: 'mention_all', label: '@所有人' },
  ],
  feishu: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...', required: true },
    { kind: 'text', key: 'secret', label: '加签密钥（可选）' },
  ],
  wecom: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...', required: true },
    { kind: 'switch', key: 'mention_all', label: '@所有人' },
  ],
  discord: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://discord.com/api/webhooks/...', required: true },
    { kind: 'text', key: 'username', label: '机器人显示名（可选）' },
  ],
  slack: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://hooks.slack.com/services/...', required: true },
    { kind: 'text', key: 'channel', label: '频道（可选，例：#alerts）' },
    { kind: 'text', key: 'username', label: '机器人显示名（可选）' },
  ],
  telegram: [
    { kind: 'text', key: 'bot_token', label: 'Bot Token', placeholder: '123456:ABC-DEF...', required: true },
    { kind: 'text', key: 'chat_id', label: 'Chat ID', placeholder: '-1001234567890', required: true },
    { kind: 'select', key: 'parse_mode', label: '解析模式', options: [
      { value: 'Markdown', label: 'Markdown' },
      { value: 'MarkdownV2', label: 'MarkdownV2' },
      { value: 'HTML', label: 'HTML' },
    ] },
    { kind: 'switch', key: 'disable_web_page_preview', label: '禁用链接预览' },
  ],
  mattermost: [
    { kind: 'text', key: 'webhook', label: 'Webhook 地址', placeholder: 'https://mattermost.example.com/hooks/...', required: true },
    { kind: 'text', key: 'username', label: '机器人显示名（可选）' },
    { kind: 'text', key: 'channel', label: '频道（可选）' },
  ],
};

/* ───────── 抽屉内字段渲染 ───────── */

function ConfigFields({
  type,
  value,
  onChange,
  errors,
}: {
  type: ChannelType;
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  errors: Record<string, string>;
}) {
  const schema = configSchemas[type] || [];
  const [visibleKeys, setVisibleKeys] = useState<Record<string, boolean>>({});
  const toggleVisible = (key: string) =>
    setVisibleKeys((prev) => ({ ...prev, [key]: !prev[key] }));
  return (
    <div className="space-y-3">
      {schema.map((field) => {
        const v = (value as Record<string, unknown>)[field.key];
        const err = errors[field.key];
        const setVal = (next: unknown) => onChange({ ...value, [field.key]: next });
        if (field.kind === 'text') {
          return (
            <div key={field.key} className="grid grid-cols-[140px_1fr] items-start gap-4">
              <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
                {field.label}
                {field.required && <span className="text-red-500 ml-0.5">*</span>}
              </Label>
              <div className="space-y-1.5 min-w-0">
                <Input
                  value={typeof v === 'string' ? v : ''}
                  placeholder={field.placeholder}
                  onChange={(e) => setVal(e.target.value)}
                  className={cn(err && 'border-red-500 focus-visible:ring-red-500')}
                />
                {err && <p className="text-xs text-red-500">{err}</p>}
              </div>
            </div>
          );
        }
        if (field.kind === 'password') {
          const visible = !!visibleKeys[field.key];
          return (
            <div key={field.key} className="grid grid-cols-[140px_1fr] items-start gap-4">
              <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
                {field.label}
                {field.required && <span className="text-red-500 ml-0.5">*</span>}
              </Label>
              <div className="space-y-1.5 min-w-0">
                <div className="relative">
                  <Input
                    type={visible ? 'text' : 'password'}
                    value={typeof v === 'string' ? v : ''}
                    placeholder={field.placeholder}
                    onChange={(e) => setVal(e.target.value)}
                    className={cn('pr-9', err && 'border-red-500 focus-visible:ring-red-500')}
                  />
                  <button
                    type="button"
                    tabIndex={-1}
                    aria-label={visible ? '隐藏密码' : '显示密码'}
                    onClick={() => toggleVisible(field.key)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
                  >
                    {visible ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                {err && <p className="text-xs text-red-500">{err}</p>}
              </div>
            </div>
          );
        }
        if (field.kind === 'textarea') {
          return (
            <div key={field.key} className="grid grid-cols-[140px_1fr] items-start gap-4">
              <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
                {field.label}
                {field.required && <span className="text-red-500 ml-0.5">*</span>}
              </Label>
              <div className="space-y-1.5 min-w-0">
                <Textarea
                  rows={field.rows || 3}
                  value={typeof v === 'string' ? v : ''}
                  placeholder={field.placeholder}
                  onChange={(e) => setVal(e.target.value)}
                  className={cn('font-mono text-xs', err && 'border-red-500 focus-visible:ring-red-500')}
                />
                {err && <p className="text-xs text-red-500">{err}</p>}
              </div>
            </div>
          );
        }
        if (field.kind === 'number') {
          return (
            <div key={field.key} className="grid grid-cols-[140px_1fr] items-start gap-4">
              <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
                {field.label}
                {field.required && <span className="text-red-500 ml-0.5">*</span>}
              </Label>
              <div className="space-y-1.5 min-w-0">
                <Input
                  type="number"
                  min={field.min}
                  max={field.max}
                  value={v === undefined || v === null ? '' : String(v)}
                  onChange={(e) => setVal(e.target.value === '' ? '' : Number(e.target.value))}
                  className={cn(err && 'border-red-500 focus-visible:ring-red-500')}
                />
                {err && <p className="text-xs text-red-500">{err}</p>}
              </div>
            </div>
          );
        }
        if (field.kind === 'select') {
          const cur = typeof v === 'string' ? v : (field.options[0]?.value ?? '');
          return (
            <div key={field.key} className="grid grid-cols-[140px_1fr] items-start gap-4">
              <Label className="pt-2 text-right text-sm font-medium text-foreground/80">
                {field.label}
                {field.required && <span className="text-red-500 ml-0.5">*</span>}
              </Label>
              <div className="space-y-1.5 min-w-0">
                <Select value={cur} onValueChange={setVal}>
                  <SelectTrigger className={cn(err && 'border-red-500 focus:ring-red-500')}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {field.options.map((o) => (
                      <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {err && <p className="text-xs text-red-500">{err}</p>}
              </div>
            </div>
          );
        }
        // switch
        const on = v === true;
        return (
          <div key={field.key} className="flex items-center justify-between rounded-lg border px-3 py-2">
            <Label className="cursor-pointer">{field.label}</Label>
            <Switch checked={on} onCheckedChange={setVal} />
          </div>
        );
      })}
    </div>
  );
}

/* ───────── 工具：list（to 字段 / headers 字符串 → 数组） ───────── */

function toStringList(input: string): string[] {
  if (!input) return [];
  return input
    .split(/[\n,;]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/* ───────── 主页面 ───────── */

type FormMode = 'create' | 'edit';

interface FormState {
  mode: FormMode;
  open: boolean;
  id?: number;
  name: string;
  type: ChannelType;
  enabled: boolean;
  config: Record<string, unknown>;
  errors: Record<string, string>;
}

const emptyForm: FormState = {
  mode: 'create',
  open: false,
  name: '',
  type: 'webhook',
  enabled: true,
  config: {},
  errors: {},
};

export default function ChannelProvidersPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [confirm, setConfirm] = useState<ConfirmState | null>(null);
  const [testingId, setTestingId] = useState<number | null>(null);

  /* 拉取所有渠道类型（中文 label / icon） */
  const typesState = useAsync(async () => {
    const result = await api.get<{ data: ChannelTypeMeta[] }>('/ovpn/channel-types');
    return Array.isArray(result?.data) ? result.data : [];
  }, []);

  /* 拉取所有已配置渠道 */
  const state = useAsync(async () => {
    const result = await api.get<{ data: NotificationChannel[] }>('/ovpn/channel');
    return Array.isArray(result?.data) ? result.data : [];
  }, [reloadKey]);

  const handleReload = useCallback(() => setReloadKey((k) => k + 1), []);

  useEffect(() => {
    if (state.error) toast.error(`加载渠道失败：${messageOf(state.error)}`);
  }, [state.error]);
  useEffect(() => {
    if (typesState.error) toast.error(`加载渠道类型失败：${messageOf(typesState.error)}`);
  }, [typesState.error]);

  /* 抽屉打开 / 关闭 */
  const openCreate = () => setForm({ ...emptyForm, open: true, mode: 'create' });
  const openEdit = (ch: NotificationChannel) => {
    let cfgObj: Record<string, unknown> = {};
    if (ch.config && typeof ch.config === 'object' && !Array.isArray(ch.config)) {
      cfgObj = { ...(ch.config as Record<string, unknown>) };
    } else if (typeof ch.config === 'string') {
      try { cfgObj = JSON.parse(ch.config); } catch { cfgObj = {}; }
    }
    // 邮件渠道的 to 在 UI 上以多行字符串展示
    if (ch.type === 'email' && Array.isArray(cfgObj.to)) {
      cfgObj = { ...cfgObj, to: (cfgObj.to as string[]).join('\n') };
    }
    setForm({
      mode: 'edit',
      open: true,
      id: ch.id,
      name: ch.name,
      type: ch.type as ChannelType,
      enabled: !!ch.enabled,
      config: cfgObj,
      errors: {},
    });
  };
  const closeForm = () => setForm((prev) => ({ ...prev, open: false }));

  /* 提交 */
  const submitForm = async () => {
    const errors: Record<string, string> = {};
    if (!form.name.trim()) errors.name = '请输入渠道名称';

    // 必填项校验
    const schema = configSchemas[form.type] || [];
    for (const f of schema) {
      if (f.kind === 'switch') continue;
      const raw = form.config[f.key];
      const isEmpty =
        raw === undefined ||
        raw === null ||
        (typeof raw === 'string' && raw.trim() === '');
      if (f.required && isEmpty) {
        errors[f.key] = `请填写${f.label}`;
      }
    }

    if (Object.keys(errors).length > 0) {
      setForm((prev) => ({ ...prev, errors }));
      return;
    }

    // 组装要提交给后端的 config：把 to 的多行字符串转成数组
    const configToSend: Record<string, unknown> = { ...form.config };
    if (form.type === 'email' && typeof configToSend.to === 'string') {
      configToSend.to = toStringList(configToSend.to as string);
    }

    try {
      if (form.mode === 'create') {
        await api.postJson<{ message: string }>('/ovpn/channel', {
          name: form.name.trim(),
          type: form.type,
          enabled: form.enabled,
          config: configToSend,
        });
        toast.success('添加渠道成功');
      } else {
        await api.putJson<{ message: string }>(`/ovpn/channel/${form.id}`, {
          name: form.name.trim(),
          type: form.type,
          enabled: form.enabled,
          config: configToSend,
        });
        toast.success('更新渠道成功');
      }
      closeForm();
      handleReload();
    } catch (e) {
      toast.error(`保存失败：${messageOf(e)}`);
    }
  };

  /* 启用 / 禁用 */
  const toggleEnabled = async (ch: NotificationChannel) => {
    try {
      let cfgObj: Record<string, unknown> = {};
      if (ch.config && typeof ch.config === 'object' && !Array.isArray(ch.config)) {
        cfgObj = ch.config as Record<string, unknown>;
      }
      await api.putJson<{ message: string }>(`/ovpn/channel/${ch.id}`, {
        name: ch.name,
        type: ch.type,
        enabled: !ch.enabled,
        config: cfgObj,
      });
      toast.success(ch.enabled ? '已禁用' : '已启用');
      handleReload();
    } catch (e) {
      toast.error(`操作失败：${messageOf(e)}`);
    }
  };

  /* 删除 */
  const askDelete = (ch: NotificationChannel) => {
    setConfirm({
      title: '删除渠道',
      message: `确认要删除渠道 “${ch.name}” 吗？删除后无法恢复。`,
      danger: true,
      onConfirm: async () => {
        try {
          await api.delete<{ message: string }>(`/ovpn/channel/${ch.id}`);
          toast.success('删除成功');
          handleReload();
          setConfirm(null);
        } catch (e) {
          toast.error(`删除失败：${messageOf(e)}`);
        }
      },
    });
  };

  /* 测试发送 */
  const sendTest = async (ch: NotificationChannel) => {
    if (!ch.enabled) {
      toast.error('请先启用该渠道');
      return;
    }
    setTestingId(ch.id);
    try {
      await api.postForm<{ message: string }>(`/ovpn/channel/${ch.id}/test`, {});
      toast.success('测试消息已发送，请到对应渠道查收');
    } catch (e) {
      toast.error(`测试失败：${messageOf(e)}`);
    } finally {
      setTestingId(null);
    }
  };

  /* 列表渲染 */
  const typeMeta = (t: string) => typesState.data?.find((m) => m.type === t);
  const items = state.data ?? [];
  const pagination = usePagination(items, `channels-${reloadKey}`, 10);

  const columns: Column<NotificationChannel>[] = [
    {
      key: 'name',
      header: '渠道',
      sortable: true,
      sortAccessor: (item) => item.name ?? '',
      render: (item) => {
        const meta = typeMeta(item.type);
        return (
          <div className="flex items-center gap-2.5">
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-[var(--accent)]/12 text-[var(--accent)]">
              <ChannelIcon name={meta?.icon} className="h-4 w-4" />
            </span>
            <div>
              <div className="font-medium leading-tight">{item.name}</div>
              <div className="text-xs text-muted-foreground">{meta?.label || item.type}</div>
            </div>
          </div>
        );
      },
    },
    {
      key: 'type',
      header: '类型',
      render: (item) => {
        const meta = typeMeta(item.type);
        return (
          <Badge variant="outline" className="bg-[var(--accent)]/10 border-[var(--accent)]/30 text-[var(--accent)]">
            {meta?.label || item.type}
          </Badge>
        );
      },
    },
    {
        key: 'enabled',
        header: '状态',
        render: (item) =>
          item.enabled ? (
            <StatusBadge status="success">已启用</StatusBadge>
          ) : (
            <StatusBadge status="neutral">未启用</StatusBadge>
          ),
      },
    {
      key: 'updatedAt',
      header: '更新时间',
      sortable: true,
      sortAccessor: (item) => (item.updatedAt ? new Date(item.updatedAt).getTime() : 0),
      render: (item) => {
        if (!item.updatedAt) return '-';
        const d = new Date(item.updatedAt);
        if (Number.isNaN(d.getTime())) return item.updatedAt;
        return d.toLocaleString('zh-CN', { hour12: false });
      },
    },
    {
      key: 'actions',
      header: '操作',
      render: (item) => (
        <div className="flex items-center justify-start gap-1">
          <Button
            size="icon"
            variant="ghost"
            onClick={() => sendTest(item)}
            disabled={testingId === item.id}
            title="发送测试"
            className="h-8 w-8"
          >
            <Send className={`h-4 w-4 ${testingId === item.id ? 'animate-pulse' : ''}`} />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => toggleEnabled(item)}
            title={item.enabled ? '禁用' : '启用'}
            className="h-8 w-8"
          >
            <Power className={`h-4 w-4 ${item.enabled ? 'text-emerald-500' : 'text-muted-foreground'}`} />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => openEdit(item)}
            title="编辑"
            className="h-8 w-8"
          >
            <Edit className="h-4 w-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => askDelete(item)}
            title="删除"
            className="h-8 w-8 text-red-500 hover:text-red-600"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ),
    },
  ];

  const enabledCount = useMemo(() => items.filter((i) => i.enabled).length, [items]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold flex items-center gap-2">
            <Bell className="w-7 h-7" />
            通知渠道
          </h1>
          <p className="text-muted-foreground mt-1">
            配置 Webhook / 邮件 / 钉钉 / 飞书 / 企业微信 / Discord / Slack / Telegram / Mattermost 渠道，启用后会在用户上下线时自动派发
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={handleReload} disabled={state.loading}>
            <RefreshCw className={`w-4 h-4 mr-2 ${state.loading ? 'animate-spin' : ''}`} />
            刷新
          </Button>
          <Button onClick={openCreate}>
            <Plus className="w-4 h-4 mr-2" />
            新增渠道
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>已配置渠道</CardDescription>
            <CardTitle className="text-2xl">{items.length}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>启用中</CardDescription>
            <CardTitle className="text-2xl text-emerald-500">{enabledCount}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>支持的渠道类型</CardDescription>
            <CardTitle className="text-2xl">{typesState.data?.length ?? 0}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardContent className="pt-6">
          <DataTable<NotificationChannel>
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
            emptyTitle={state.loading ? '加载中...' : '暂无渠道'}
            emptyDescription={state.loading ? '正在拉取通知渠道' : '点击右上角"新增渠道"开始配置'}
            keyFn={(item) => item.id}
          />
        </CardContent>
      </Card>

      <Dialog open={form.open} onOpenChange={(o) => !o && closeForm()}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{form.mode === 'create' ? '新增通知渠道' : '编辑通知渠道'}</DialogTitle>
            <DialogDescription>
              {form.mode === 'create'
                ? '选择渠道类型并填写对应配置，保存后可在列表中启用并测试'
                : '修改当前渠道的配置，保存后立即生效'}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label className="flex items-center gap-1">
                  渠道名称<span className="text-red-500">*</span>
                </Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((p) => ({ ...p, name: e.target.value, errors: { ...p.errors, name: '' } }))}
                  placeholder="例：运维值班群"
                  className={cn(form.errors.name && 'border-red-500 focus-visible:ring-red-500')}
                />
                {form.errors.name && <p className="text-xs text-red-500">{form.errors.name}</p>}
              </div>
              <div className="space-y-1.5">
                <Label>渠道类型</Label>
                <Select
                  value={form.type}
                  onValueChange={(v) => setForm((p) => ({ ...p, type: v as ChannelType, config: {}, errors: {} }))}
                  disabled={form.mode === 'edit'}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(typesState.data || []).map((m) => (
                      <SelectItem key={m.type} value={m.type}>
                        <span className="flex items-center gap-2">
                          <ChannelIcon name={m.icon} className="h-3.5 w-3.5" />
                          {m.label}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="rounded-lg border px-3 py-2.5">
              <div className="flex items-center justify-between">
                <Label className="cursor-pointer">状态</Label>
                <RadioGroup
                  className="flex items-center gap-4"
                  value={form.enabled ? 'enabled' : 'disabled'}
                  onValueChange={(v) => setForm((p) => ({ ...p, enabled: v === 'enabled' }))}
                >
                  <label className="flex items-center gap-1.5 cursor-pointer text-sm">
                    <RadioGroupItem value="enabled" id="channel-enabled-on" />
                    <span>启用</span>
                  </label>
                  <label className="flex items-center gap-1.5 cursor-pointer text-sm">
                    <RadioGroupItem value="disabled" id="channel-enabled-off" />
                    <span>禁用</span>
                  </label>
                </RadioGroup>
              </div>
            </div>

            <Separator />

            <ConfigFields
              type={form.type}
              value={form.config}
              onChange={(next) => setForm((p) => ({ ...p, config: next, errors: { ...p.errors, ...Object.fromEntries(Object.keys(p.errors).map((k) => [k, ''])) } }))}
              errors={form.errors}
            />
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={closeForm}>取消</Button>
            <Button onClick={submitForm}>{form.mode === 'create' ? '保存' : '更新'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {confirm && <ConfirmDialog state={confirm} onClose={() => setConfirm(null)} />}
    </div>
  );
}
