import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Settings, Shield, Server, Wrench, RefreshCw, FileCode2, Save, RotateCcw, Package, Upload, Trash2, CheckCircle2, Eye, EyeOff } from 'lucide-react';

import { api } from '@/api';
import type { SettingsResponse } from '@/types';
import { messageOf } from '@/lib/format';
import { useAuth } from '@/store/auth';
import {
  trimText,
  isValidUrl,
  isValidPort,
  isValidAccount,
  isStrongPassword,
  isNonNegativeInteger,
  isPositiveInteger,
  isValidCidr,
  isValidHostPort,
  isValidIp,
} from '@/lib/validators';

import { PageHeader } from '@/components/PageHeader';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/ui/card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/ui/tabs';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Switch } from '@/ui/switch';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/ui/select';
import { Separator } from '@/ui/separator';
import { Button } from '@/ui/button';
import { Textarea } from '@/ui/textarea';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { HasPermission } from '@/components/HasPermission';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/ui/dialog';
import { GlowButton } from '@/components/GlowButton';
import { cn } from '@/lib/utils';

/* ---------- 验证 ---------- */

function validateSetting(key: string, value: unknown): string | undefined {
  const text = trimText(value);
  if (key === 'system.base.site_url' && !isValidUrl(text)) return '站点地址请输入完整的 http/https 地址';
  if (key === 'system.base.web_port' && !isValidPort(text)) return 'Web 端口必须是 1-65535 的整数';
  if (key === 'system.base.admin_username' && !isValidAccount(text)) return '管理员账号不能为空，长度需在 2-64 个字符内';
  if (key === 'system.base.admin_password' && text && !isStrongPassword(text)) return '管理员新密码需至少 12 位，包含大小写字母、数字和特殊字符';
  if (key === 'system.base.history_max_days' && !isNonNegativeInteger(text)) return '历史保留天数必须是非负整数';
  if (key === 'system.base.max_duplicate_login' && !isNonNegativeInteger(text)) return '重复登录数必须是非负整数';
  if (key === 'system.ldap.ldap_url' && text && !isValidUrl(text, ['ldap:', 'ldaps:'])) return 'LDAP URL 请输入 ldap:// 或 ldaps:// 地址';
  if (key === 'openvpn.ovpn_port' && !isValidPort(text)) return 'OpenVPN 端口必须是 1-65535 的整数';
  if (key === 'openvpn.ovpn_subnet' && !isValidCidr(text, 4)) return 'IPv4 网段请输入 CIDR 格式，例如 10.8.0.0/24';
  if (key === 'openvpn.ovpn_subnet6' && text && !isValidCidr(text, 6)) return 'IPv6 网段请输入 CIDR 格式，例如 fd00::/64';
  if (key === 'openvpn.ovpn_max_clients' && !isPositiveInteger(text)) return '最大客户端数量必须是正整数';
  if (key === 'openvpn.ovpn_management' && !isValidHostPort(text)) return 'Management 请输入 host:port，例如 127.0.0.1:7505';
  if ((key === 'openvpn.ovpn_push_dns1' || key === 'openvpn.ovpn_push_dns2') && text && !isValidIp(text)) return 'DNS 地址请输入合法 IPv4 或 IPv6';
  return undefined;
}

/* ---------- 整页统一保存：草稿仓库 ---------- */

interface DraftStore {
  /** 当前所有字段的草稿（key -> value） */
  drafts: Record<string, string>;
  /** 服务端值（key -> value） */
  originals: Record<string, string>;
  /** 字段级错误（key -> error） */
  errors: Record<string, string | undefined>;
  /** 设置某 key 的草稿 */
  setDraft: (key: string, value: string) => void;
  /** 重置全部草稿到 originals */
  reset: () => void;
  /** 判断是否有任何字段被修改 */
  isDirty: () => boolean;
  /** 获取修改过的 key 列表 */
  dirtyKeys: () => string[];
  /** 校验所有草稿并收集错误；返回是否通过 */
  validateAll: () => boolean;
}

/* ---------- 字段组件（无独立保存按钮） ---------- */

interface SettingFieldProps {
  label: string;
  description?: string;
  value: string | number | undefined | null;
  settingKey: string;
  type?: string;
  placeholder?: string;
  /** 为 true 时仅在值非空时才参与保存（密码字段） */
  saveOnlyIfValue?: boolean;
  /** 为 true 时在右侧显示"显示/隐藏密码"按钮，type 会在 text/password 间切换 */
  visibilityToggle?: boolean;
  store: DraftStore;
}

/**
 * 设置项：把改动写入统一的草稿仓库，由父组件的"保存"按钮统一提交。
 * 字段下方的红色错误提示仅用于本地校验。
 */
function SettingField({ label, description, value, settingKey, type = 'text', placeholder, saveOnlyIfValue, visibilityToggle, store }: SettingFieldProps) {
  const initial = saveOnlyIfValue ? '' : String(value ?? '');
  const draft = store.drafts[settingKey] ?? initial;
  const error = store.errors[settingKey];
  const [visible, setVisible] = useState(false);
  const actualType = visibilityToggle ? (visible ? 'text' : 'password') : type;

  return (
    <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
      <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">{label}</Label>
      <div className="space-y-1.5 min-w-0">
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        <div className="relative">
          <Input
            type={actualType}
            value={draft}
            placeholder={placeholder}
            aria-invalid={error ? 'true' : undefined}
            className={cn(
              error && 'border-destructive focus-visible:ring-destructive/40',
              visibilityToggle && 'pr-9',
            )}
            onChange={(e) => {
              store.setDraft(settingKey, e.target.value);
            }}
          />
          {visibilityToggle && (
            <button
              type="button"
              tabIndex={-1}
              aria-label={visible ? '隐藏密码' : '显示密码'}
              onClick={() => setVisible((v) => !v)}
              className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
            >
              {visible ? <EyeOff size={16} /> : <Eye size={16} />}
            </button>
          )}
        </div>
        {error && <p className="text-xs text-destructive">{error}</p>}
      </div>
    </div>
  );
}

interface SettingSwitchProps {
  label: string;
  description?: string;
  checked: boolean;
  settingKey: string;
  store: DraftStore;
}

function SettingSwitch({ label, description, checked, settingKey, store }: SettingSwitchProps) {
  const draft = store.drafts[settingKey] ?? String(checked);
  const isChecked = draft === 'true';

  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 sm:gap-0">
      <div className="space-y-0.5 flex-1">
        <Label>{label}</Label>
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
      </div>
      <Switch
        checked={isChecked}
        onCheckedChange={(next) => {
          store.setDraft(settingKey, String(next));
        }}
      />
    </div>
  );
}

interface SettingSelectProps {
  label: string;
  description?: string;
  value: string;
  settingKey: string;
  options: Array<{ value: string; label: string }>;
  store: DraftStore;
}

function SettingSelectField({ label, description, value, settingKey, options, store }: SettingSelectProps) {
  const draft = store.drafts[settingKey] ?? String(value);

  return (
    <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4">
      <Label className="pt-0 sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">{label}</Label>
      <div className="space-y-1.5 min-w-0">
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        <Select
          value={draft}
          onValueChange={(next) => {
            store.setDraft(settingKey, next);
          }}
        >
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {options.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

/* ---------- 页面 ---------- */

/** 把 SettingsResponse 扁平化为 key->value 的字符串映射 */
function flattenSettings(settings: SettingsResponse): Record<string, string> {
  const out: Record<string, string> = {};
  const base = settings?.system?.base ?? ({} as SettingsResponse['system']['base']);
  out['system.base.site_url'] = String(base.site_url ?? '');
  out['system.base.server_addr'] = String(base.server_addr ?? '');
  out['system.base.web_port'] = String(base.web_port ?? '');
  out['system.base.admin_username'] = String(base.admin_username ?? '');
  // 密码字段永远不存草稿，留空表示不修改
  out['system.base.admin_password'] = '';
  out['system.base.history_max_days'] = String(base.history_max_days ?? '');
  out['system.base.max_duplicate_login'] = String(base.max_duplicate_login ?? '');

  const ldap = settings?.system?.ldap ?? ({} as SettingsResponse['system']['ldap']);
  out['system.ldap.ldap_url'] = String(ldap.ldap_url ?? '');
  out['system.ldap.ldap_base_dn'] = String(ldap.ldap_base_dn ?? '');
  out['system.ldap.ldap_user_attribute'] = String(ldap.ldap_user_attribute ?? '');
  out['system.ldap.ldap_bind_user_dn'] = String(ldap.ldap_bind_user_dn ?? '');
  out['system.ldap.ldap_bind_password'] = '';
  out['system.ldap.ldap_user_attr_ipaddr_name'] = String(ldap.ldap_user_attr_ipaddr_name ?? '');
  out['system.ldap.ldap_user_attr_config_name'] = String(ldap.ldap_user_attr_config_name ?? '');
  out['system.ldap.ldap_user_group_dn'] = String(ldap.ldap_user_group_dn ?? '');

  const ovpn = settings?.openvpn ?? ({} as SettingsResponse['openvpn']);
  out['openvpn.ovpn_port'] = String(ovpn.ovpn_port ?? '');
  out['openvpn.ovpn_proto'] = String(ovpn.ovpn_proto ?? '');
  out['openvpn.ovpn_subnet'] = String(ovpn.ovpn_subnet ?? '');
  out['openvpn.ovpn_max_clients'] = String(ovpn.ovpn_max_clients ?? '');
  out['openvpn.ovpn_management'] = String(ovpn.ovpn_management ?? '');
  out['openvpn.ovpn_push_dns1'] = String(ovpn.ovpn_push_dns1 ?? '');
  out['openvpn.ovpn_push_dns2'] = String(ovpn.ovpn_push_dns2 ?? '');
  out['openvpn.ovpn_subnet6'] = String(ovpn.ovpn_subnet6 ?? '');
  return out;
}

export default function SettingsPage() {
  const navigate = useNavigate();
  const { hasPermission } = useAuth();
  const [settings, setSettings] = useState<SettingsResponse>();
  const [loading, setLoading] = useState(true);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [originals, setOriginals] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | undefined>>({});
  const [saving, setSaving] = useState(false);
  const [activeTab, setActiveTab] = useState<string>('');

  // 检查 Tab 权限
  const canViewBase = hasPermission('settings:base');
  const canViewLdap = hasPermission('settings:ldap');
  const canViewOpenvpn = hasPermission('settings:openvpn');
  const canViewService = hasPermission('settings:service');
  const canViewPackages = hasPermission('settings:packages');

  // 检查各 Tab 的保存权限
  const canSaveBase = hasPermission('settings:base:update');
  const canSaveLdap = hasPermission('settings:ldap:update');
  const canSaveOvpn = hasPermission('settings:openvpn:update');
  // service.auth_user 的保存需要 server:manage 权限
  const canSaveServiceAuth = hasPermission('server:manage');
  // SaveBar 仅在用户拥有至少一个 Tab 的保存权限时显示
  const canShowSaveBar = !activeTab.startsWith('packages') && (canSaveBase || canSaveLdap || canSaveOvpn || canSaveServiceAuth);

  // 检查是否至少有一个 Tab 权限，否则重定向到概览页
  const hasAnyTabPermission = canViewBase || canViewLdap || canViewOpenvpn || canViewService || canViewPackages;

  useEffect(() => {
    if (!loading && !hasAnyTabPermission) {
      navigate('/overview', { replace: true });
    }
  }, [loading, hasAnyTabPermission, navigate]);

  function loadSettings() {
    setLoading(true);
    Promise.all([
      api.get<SettingsResponse>('/ovpn/settings'),
      api.get<{ authUser?: boolean }>('/ovpn/auth-status').catch(() => ({ authUser: false })),
    ])
      .then(([data, authData]) => {
        setSettings(data);
        const flat = flattenSettings(data);
        flat['service.auth_user'] = String(authData?.authUser ?? false);
        setOriginals(flat);
        setDrafts(flat);
        setErrors({});
      })
      .catch((err) => toast.error(messageOf(err)))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    loadSettings();
  }, []);

  const setDraft = useCallback((key: string, value: string) => {
    setDrafts((prev) => ({ ...prev, [key]: value }));
    // 编辑时清掉该字段的错误
    setErrors((prev) => (prev[key] ? { ...prev, [key]: undefined } : prev));
  }, []);

  const dirtyKeys = useMemo(() => {
    return Object.keys(drafts).filter((k) => {
      // 密码字段：原始值是密码原文时不做 diff（永远视为"留空不改"），否则与原值比对
      if (k === 'system.base.admin_password' || k === 'system.ldap.ldap_bind_password') {
        return trimText(drafts[k]) !== '';
      }
      return trimText(drafts[k]) !== trimText(originals[k] ?? '');
    });
  }, [drafts, originals]);

  const isDirty = dirtyKeys.length > 0;

  // 过滤出用户有保存权限的 dirty keys（仅这些会被提交保存）
  const saveableDirtyKeys = useMemo(() => {
    return dirtyKeys.filter((key) => {
      if (key.startsWith('system.base.') && !canSaveBase) return false;
      if (key.startsWith('system.ldap.') && !canSaveLdap) return false;
      if (key.startsWith('openvpn.') && !canSaveOvpn) return false;
      // service.auth_user 的保存需要 server:manage 权限
      if (key === 'service.auth_user' && !canSaveServiceAuth) return false;
      return true;
    });
  }, [dirtyKeys, canSaveBase, canSaveLdap, canSaveOvpn, canSaveServiceAuth]);

  const validateAll = useCallback((): boolean => {
    const next: Record<string, string | undefined> = {};
    let ok = true;
    for (const k of Object.keys(drafts)) {
      const v = drafts[k];
      // 密码字段留空跳过校验
      if ((k === 'system.base.admin_password' || k === 'system.ldap.ldap_bind_password') && !trimText(v)) {
        next[k] = undefined;
        continue;
      }
      const err = validateSetting(k, v);
      next[k] = err;
      if (err) ok = false;
    }
    setErrors(next);
    return ok;
  }, [drafts]);

  async function handleSave() {
    if (saveableDirtyKeys.length === 0 || saving) return;
    if (!validateAll()) {
      toast.error('请先修正页面上红色高亮的字段');
      return;
    }
    setSaving(true);
    try {
      // 分离 auth-user（走 /ovpn/server）和普通设置（走 /settings）
      const authUserDirty = saveableDirtyKeys.includes('service.auth_user');
      const payload: Record<string, string> = {};
      for (const k of saveableDirtyKeys) {
        if (k === 'service.auth_user') continue;
        payload[k] = trimText(drafts[k]);
      }

      // 提交账号密码认证开关（实时生效到 OpenVPN 服务）
      if (authUserDirty) {
        await api.postForm<{ message: string }>('/ovpn/server', {
          action: 'settings',
          key: 'auth-user',
          value: drafts['service.auth_user'] === 'true',
        });
      }

      // 提交普通设置
      if (Object.keys(payload).length > 0) {
        await api.postForm<{ message: string }>('/ovpn/settings', payload);
      }

      toast.success('设置已保存');
      // 保存成功后用 drafts 覆盖 originals（实现"保存即生效"的语义）
      setOriginals((prev) => {
        const next = { ...prev };
        for (const k of saveableDirtyKeys) next[k] = trimText(drafts[k]);
        return next;
      });
      // 重新拉一次数据，确保与服务端同步
      await loadSettings();
    } catch (err) {
      toast.error(messageOf(err));
    } finally {
      setSaving(false);
    }
  }

  function handleReset() {
    setDrafts(originals);
    setErrors({});
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader eyebrow="Control" title="系统设置" description="配置系统参数、LDAP认证和OpenVPN参数" />
        <Card>
          <CardContent className="p-8 text-center text-muted-foreground">
            正在加载设置…
          </CardContent>
        </Card>
      </div>
    );
  }

  if (!settings) {
    return (
      <div className="space-y-6">
        <PageHeader eyebrow="Control" title="系统设置" description="配置系统参数、LDAP认证和OpenVPN参数" />
        <Card>
          <CardContent className="p-8 text-center text-muted-foreground">
            无法加载设置，请检查网络或刷新重试。
          </CardContent>
        </Card>
      </div>
    );
  }

  const base = settings.system.base;
  const ldap = settings.system.ldap;
  const ovpn = settings.openvpn;

  const store: DraftStore = {
    drafts,
    originals,
    errors,
    setDraft,
    reset: handleReset,
    isDirty: () => isDirty,
    dirtyKeys: () => dirtyKeys,
    validateAll,
  };

  // 确定默认 Tab 值：取第一个有权限的 Tab
  const defaultTab = canViewBase ? 'base' : canViewLdap ? 'ldap' : canViewOpenvpn ? 'openvpn' : canViewService ? 'service' : canViewPackages ? 'packages' : 'base';

  return (
    <div className="space-y-6">
      <PageHeader eyebrow="Control" title="系统设置" description="配置系统参数、LDAP认证和OpenVPN参数" />

      <Tabs
        value={activeTab || defaultTab}
        onValueChange={setActiveTab}
        defaultValue={defaultTab}
        className="w-full"
      >
        <TabsList>
          {canViewBase && (
            <TabsTrigger value="base" className="gap-1.5">
              <Settings className="h-4 w-4" />
              基础控制
            </TabsTrigger>
          )}
          {canViewLdap && (
            <TabsTrigger value="ldap" className="gap-1.5">
              <Shield className="h-4 w-4" />
              LDAP认证
            </TabsTrigger>
          )}
          {canViewOpenvpn && (
            <TabsTrigger value="openvpn" className="gap-1.5">
              <Server className="h-4 w-4" />
              OpenVPN参数
            </TabsTrigger>
          )}
          {canViewService && (
            <TabsTrigger value="service" className="gap-1.5">
              <Wrench className="h-4 w-4" />
              服务管理
            </TabsTrigger>
          )}
          {canViewPackages && (
            <TabsTrigger value="packages" className="gap-1.5">
              <Package className="h-4 w-4" />
              客户端安装包
            </TabsTrigger>
          )}
        </TabsList>

        {/* ====== Tab 1: 基础控制 ====== */}
        {canViewBase && (
          <TabsContent value="base">
            <Card>
              <CardHeader>
                <CardTitle>基础控制</CardTitle>
                <CardDescription>站点地址、管理员、系统行为等核心参数</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid gap-6 sm:grid-cols-2">
                  <SettingField
                    label="站点地址"
                    value={base.site_url}
                    settingKey="system.base.site_url"
                    placeholder="https://example.com"
                    store={store}
                  />
                  <SettingField
                    label="VPN 服务器地址"
                    value={base.server_addr}
                    settingKey="system.base.server_addr"
                    placeholder="公网 IP 或域名，留空则从站点地址解析"
                    store={store}
                  />
                  <SettingField
                    label="Web 端口"
                    value={base.web_port}
                    settingKey="system.base.web_port"
                    placeholder="8080"
                    store={store}
                  />
                  <SettingField
                    label="管理员账号"
                    value={base.admin_username}
                    settingKey="system.base.admin_username"
                    store={store}
                  />
                  <SettingField
                    label="管理员新密码"
                    value=""
                    settingKey="system.base.admin_password"
                    type="password"
                    placeholder="留空不改；建议 12 位强密码"
                    saveOnlyIfValue
                    visibilityToggle
                    store={store}
                  />
                  <SettingField
                    label="历史保留天数"
                    value={base.history_max_days}
                    settingKey="system.base.history_max_days"
                    store={store}
                  />
                  <SettingField
                    label="重复登录数"
                    value={base.max_duplicate_login}
                    settingKey="system.base.max_duplicate_login"
                    store={store}
                  />
                </div>

                <Separator />

                <div className="space-y-4">
                  <SettingSwitch
                    label="自动更新 OpenVPN 配置"
                    description="保存设置后自动同步 server.conf"
                    checked={base.auto_update_ovpn_config}
                    settingKey="system.base.auto_update_ovpn_config"
                    store={store}
                  />
                  <SettingSwitch
                    label="校验客户端配置"
                    description="登录时校验用户绑定的配置文件"
                    checked={base.validate_client_config}
                    settingKey="system.base.validate_client_config"
                    store={store}
                  />
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {/* ====== Tab 2: LDAP认证 ====== */}
        {canViewLdap && (
          <TabsContent value="ldap">
            <Card>
              <CardHeader>
                <CardTitle>LDAP 认证</CardTitle>
                <CardDescription>配置 LDAP 外部认证源，启用后本地 VPN 账号不参与认证</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <SettingSwitch
                  label="启用 LDAP"
                  description="启用后本地 VPN 账号不参与认证"
                  checked={ldap.ldap_auth}
                  settingKey="system.ldap.ldap_auth"
                  store={store}
                />

                <Separator />

                <div className="grid gap-6 sm:grid-cols-2">
                  <SettingField
                    label="LDAP URL"
                    value={ldap.ldap_url}
                    settingKey="system.ldap.ldap_url"
                    placeholder="ldap://ldap.example.com:389"
                    store={store}
                  />
                  <SettingField
                    label="Base DN"
                    value={ldap.ldap_base_dn}
                    settingKey="system.ldap.ldap_base_dn"
                    placeholder="dc=example,dc=com"
                    store={store}
                  />
                  <SettingField
                    label="用户属性"
                    value={ldap.ldap_user_attribute}
                    settingKey="system.ldap.ldap_user_attribute"
                    placeholder="uid"
                    store={store}
                  />
                  <SettingField
                    label="绑定 DN"
                    value={ldap.ldap_bind_user_dn}
                    settingKey="system.ldap.ldap_bind_user_dn"
                    store={store}
                  />
                  <SettingField
                    label="绑定密码"
                    value={ldap.ldap_bind_password}
                    settingKey="system.ldap.ldap_bind_password"
                    type="password"
                    visibilityToggle
                    store={store}
                  />
                  <SettingField
                    label="IP 属性名"
                    value={ldap.ldap_user_attr_ipaddr_name}
                    settingKey="system.ldap.ldap_user_attr_ipaddr_name"
                    store={store}
                  />
                  <SettingField
                    label="配置属性名"
                    value={ldap.ldap_user_attr_config_name}
                    settingKey="system.ldap.ldap_user_attr_config_name"
                    store={store}
                  />
                </div>

                <Separator />

                <div className="space-y-4">
                  <SettingSwitch
                    label="用户组过滤"
                    description="只允许指定 memberOf 登录"
                    checked={ldap.ldap_user_group_filter}
                    settingKey="system.ldap.ldap_user_group_filter"
                    store={store}
                  />
                  <SettingField
                    label="用户组 DN"
                    value={ldap.ldap_user_group_dn}
                    settingKey="system.ldap.ldap_user_group_dn"
                    store={store}
                  />
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {/* ====== Tab 3: OpenVPN参数 ====== */}
        {canViewOpenvpn && (
          <TabsContent value="openvpn">
            <Card>
              <CardHeader>
                <CardTitle>OpenVPN 参数</CardTitle>
                <CardDescription>端口、协议、网段、DNS 等核心 OpenVPN 服务配置</CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="grid gap-6 sm:grid-cols-2">
                  <SettingField
                    label="端口"
                    value={ovpn.ovpn_port}
                    settingKey="openvpn.ovpn_port"
                    store={store}
                  />
                  <SettingSelectField
                    label="协议"
                    value={ovpn.ovpn_proto}
                    settingKey="openvpn.ovpn_proto"
                    options={[
                      { value: 'udp', label: 'UDP（低延迟，常用推荐）' },
                      { value: 'tcp', label: 'TCP（穿透性更好，稳定优先）' },
                    ]}
                    store={store}
                  />
                  <SettingField
                    label="IPv4 网段"
                    value={ovpn.ovpn_subnet}
                    settingKey="openvpn.ovpn_subnet"
                    placeholder="10.8.0.0/24"
                    store={store}
                  />
                  <SettingField
                    label="最大客户端"
                    value={ovpn.ovpn_max_clients}
                    settingKey="openvpn.ovpn_max_clients"
                    store={store}
                  />
                  <SettingField
                    label="Management"
                    value={ovpn.ovpn_management}
                    settingKey="openvpn.ovpn_management"
                    placeholder="127.0.0.1:7505"
                    store={store}
                  />
                  <SettingField
                    label="DNS1"
                    value={ovpn.ovpn_push_dns1}
                    settingKey="openvpn.ovpn_push_dns1"
                    placeholder="8.8.8.8"
                    store={store}
                  />
                  <SettingField
                    label="DNS2"
                    value={ovpn.ovpn_push_dns2}
                    settingKey="openvpn.ovpn_push_dns2"
                    placeholder="8.8.4.4"
                    store={store}
                  />
                </div>

                <Separator />

                <div className="space-y-4">
                  <SettingSwitch
                    label="全局网关"
                    description="推送默认路由"
                    checked={ovpn.ovpn_gateway}
                    settingKey="openvpn.ovpn_gateway"
                    store={store}
                  />
                  <SettingSwitch
                    label="IPv6"
                    description="启用 IPv6 地址池"
                    checked={ovpn.ovpn_ipv6}
                    settingKey="openvpn.ovpn_ipv6"
                    store={store}
                  />
                  <SettingField
                    label="IPv6 网段"
                    value={ovpn.ovpn_subnet6}
                    settingKey="openvpn.ovpn_subnet6"
                    placeholder="fd00::/64"
                    store={store}
                  />
                </div>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {/* ====== Tab 4: 服务管理 ====== */}
        {canViewService && (
          <TabsContent value="service">
            <ServiceTab store={store} />
          </TabsContent>
        )}

        {/* ====== Tab 5: 客户端安装包管理 ====== */}
        {canViewPackages && (
          <TabsContent value="packages">
            <ClientPackagesTab />
          </TabsContent>
        )}
      </Tabs>

      {/* 整页统一的保存条：固定在底部，所有 Tab 共享；拥有至少一个 Tab:update 权限时显示 */}
      {canShowSaveBar && (
        <SaveBar
          dirtyCount={saveableDirtyKeys.length}
          saving={saving}
          disabled={saveableDirtyKeys.length === 0}
          onSave={handleSave}
          onReset={handleReset}
        />
      )}
    </div>
  );
}

/* ---------- 整页统一保存条 ---------- */

function SaveBar({
  dirtyCount,
  saving,
  disabled,
  onSave,
  onReset,
}: {
  dirtyCount: number;
  saving: boolean;
  disabled: boolean;
  onSave: () => void | Promise<void>;
  onReset: () => void;
}) {
  return (
    <div
      className={cn(
        'sticky bottom-4 z-30 mt-2 flex flex-col sm:flex-row items-center justify-between gap-3 rounded-2xl border p-3 shadow-xl backdrop-blur-md',
        'border-[color-mix(in_srgb,var(--accent)_30%,transparent)]',
        'bg-[color-mix(in_srgb,var(--surface-strong)_82%,transparent)]',
      )}
    >
      <div className="flex items-center gap-2 px-1 text-sm text-muted-foreground w-full sm:w-auto">
        {disabled ? (
          <span>所有设置已是最新</span>
        ) : (
          <span>
            <strong className="text-[var(--accent)]">{dirtyCount}</strong> 项设置待保存
          </span>
        )}
      </div>
      <div className="flex flex-col sm:flex-row items-center gap-2 w-full sm:w-auto">
        <button
          type="button"
          onClick={onReset}
          disabled={disabled || saving}
          className="secondary-action"
        >
          <RotateCcw className="h-4 w-4" />
          撤销改动
        </button>
        <GlowButton
          type="button"
          onClick={onSave}
          loading={saving}
          loadingText="保存中…"
          disabled={disabled}
          icon={<Save className="h-4 w-4" />}
        >
          保存全部
        </GlowButton>
      </div>
    </div>
  );
}

/* ========== 服务管理：账号密码认证 / 重启 / 编辑 server.conf ========== */

function ServiceTab({ store }: { store: DraftStore }) {
  const [restartConfirm, setRestartConfirm] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [configContent, setConfigContent] = useState('');
  const [configLoading, setConfigLoading] = useState(false);
  const [configSaving, setConfigSaving] = useState(false);
  const [configError, setConfigError] = useState<string | undefined>();

  async function handleRestart() {
    setRestarting(true);
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', { action: 'restartSrv' });
      toast.success(result.message || 'OpenVPN 服务已重启');
    } catch (error) {
      toast.error(`重启失败：${messageOf(error)}`);
    } finally {
      setRestarting(false);
      setRestartConfirm(false);
    }
  }

  async function openServerConfig() {
    setConfigOpen(true);
    setConfigLoading(true);
    setConfigError(undefined);
    try {
      const result = await api.postForm<{ content: string }>('/ovpn/server', { action: 'getConfig' });
      setConfigContent(result.content || '');
    } catch (error) {
      toast.error(`加载配置失败：${messageOf(error)}`);
      setConfigOpen(false);
    } finally {
      setConfigLoading(false);
    }
  }

  function validateConfig(): string | undefined {
    if (!configContent.trim()) return '服务端配置内容不能为空';
    return undefined;
  }

  async function handleSaveConfig(e: React.FormEvent) {
    e.preventDefault();
    const err = validateConfig();
    if (err) {
      setConfigError(err);
      return;
    }
    setConfigError(undefined);
    setConfigSaving(true);
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', {
        action: 'updateConfig',
        content: configContent,
      });
      toast.success(result.message || '服务端配置已保存');
      setConfigOpen(false);
    } catch (error) {
      toast.error(`保存失败：${messageOf(error)}`);
    } finally {
      setConfigSaving(false);
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>服务管理</CardTitle>
          <CardDescription>账号密码认证开关、服务重启与 server.conf 维护</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="flex items-center justify-between rounded-md border border-[color-mix(in_srgb,var(--accent)_22%,transparent)] px-4 py-3">
            <div>
              <p className="font-medium">账号密码认证</p>
              <p className="text-sm text-muted-foreground">控制 auth-user-pass-verify 认证开关</p>
            </div>
            <Switch
              checked={store.drafts['service.auth_user'] === 'true'}
              onCheckedChange={(next) => store.setDraft('service.auth_user', String(next))}
            />
          </div>

          <Separator />

          <div className="space-y-3">
            <p className="text-sm font-medium">服务控制</p>
            <p className="text-xs text-muted-foreground">重启会向 OpenVPN 进程发送 SIGHUP 信号，重新加载 server.conf。</p>
            <div className="flex flex-wrap gap-2">
              <HasPermission code="settings:service:restart">
                <Button onClick={() => setRestartConfirm(true)} disabled={restarting}>
                  <RefreshCw className="h-4 w-4 mr-2" />
                  {restarting ? '重启中...' : '重启 OpenVPN'}
                </Button>
              </HasPermission>
              <HasPermission code="settings:service:config">
                <Button variant="outline" onClick={openServerConfig} disabled={configLoading}>
                  <FileCode2 className="h-4 w-4 mr-2" />
                  编辑 server.conf
                </Button>
              </HasPermission>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* 重启确认 */}
      {restartConfirm && (
        <ConfirmDialog
          state={{
            title: '重启 OpenVPN',
            message: '确认重启 OpenVPN 服务吗？所有在线连接会暂时中断并自动重连。',
            danger: false,
            onConfirm: handleRestart,
          }}
          onClose={() => !restarting && setRestartConfirm(false)}
        />
      )}

      {/* 编辑 server.conf */}
      {configOpen && (
        <Dialog open onOpenChange={(open) => !open && !configSaving && setConfigOpen(false)}>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>编辑 server.conf</DialogTitle>
              <DialogDescription>修改 OpenVPN 服务端配置文件</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleSaveConfig} className="space-y-2">
              {configLoading ? (
                <div className="min-h-[300px] flex items-center justify-center text-muted-foreground text-sm">
                  正在加载配置...
                </div>
              ) : (
                <>
                  <Textarea
                    className={cn(
                      'min-h-[300px] font-mono text-sm',
                      configError && 'border-destructive focus-visible:ring-destructive/40',
                    )}
                    value={configContent}
                    aria-invalid={configError ? 'true' : undefined}
                    onChange={(e) => {
                      setConfigContent(e.target.value);
                      if (configError) setConfigError(undefined);
                    }}
                  />
                  {configError && <p className="text-xs font-medium text-destructive">{configError}</p>}
                </>
              )}
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setConfigOpen(false)}
                  disabled={configSaving}
                >
                  取消
                </Button>
                <Button type="submit" disabled={configSaving || configLoading}>
                  {configSaving ? '保存中...' : '保存配置'}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

/* ========== 客户端安装包管理 ========== */

interface ClientPackageItem {
  id: number;
  platform: string;
  version: string;
  filename: string;
  storedName: string;
  fileSize: number;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  downloadUrl: string;
}

const PLATFORM_LABELS: Record<string, { label: string; className: string }> = {
  windows: { label: 'Windows', className: 'bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] text-[var(--accent)] border-[color-mix(in_srgb,var(--accent)_25%,transparent)]' },
  macos: { label: 'macOS', className: 'bg-[color-mix(in_srgb,var(--muted)_80%,transparent)] text-[color-mix(in_srgb,var(--text)_70%,transparent)] border-border' },
  linux: { label: 'Linux', className: 'bg-[color-mix(in_srgb,var(--accent)_8%,transparent)] text-[color-mix(in_srgb,var(--accent)_90%,transparent)] border-[color-mix(in_srgb,var(--accent)_20%,transparent)]' },
  android: { label: 'Android', className: 'bg-[color-mix(in_srgb,var(--accent)_10%,transparent)] text-[var(--accent)] border-[color-mix(in_srgb,var(--accent)_22%,transparent)]' },
  ios: { label: 'iOS', className: 'bg-[color-mix(in_srgb,var(--muted)_75%,transparent)] text-[color-mix(in_srgb,var(--text)_65%,transparent)] border-border' },
};

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function ClientPackagesTab() {
  const [packages, setPackages] = useState<ClientPackageItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadForm, setUploadForm] = useState({ platform: '', version: '' });
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<ClientPackageItem | null>(null);

  async function loadPackages() {
    setLoading(true);
    try {
      const data = await api.clientPackages.list();
      setPackages(data);
    } catch (err) {
      toast.error(messageOf(err));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadPackages();
  }, []);

  function openUpload() {
    setUploadForm({ platform: '', version: '' });
    setSelectedFile(null);
    setUploadOpen(true);
  }

  async function handleUpload() {
    if (!selectedFile) {
      toast.error('请选择安装包文件');
      return;
    }
    if (!uploadForm.platform) {
      toast.error('请选择平台');
      return;
    }
    if (!uploadForm.version) {
      toast.error('请输入版本号');
      return;
    }

    setUploading(true);
    try {
      const formData = new FormData();
      formData.append('file', selectedFile);
      formData.append('platform', uploadForm.platform);
      formData.append('version', uploadForm.version);

      await api.clientPackages.upload(formData);
      toast.success('安装包上传成功');
      setUploadOpen(false);
      await loadPackages();
    } catch (err) {
      toast.error(messageOf(err));
    } finally {
      setUploading(false);
    }
  }

  async function handleActivate(pkg: ClientPackageItem) {
    try {
      await api.clientPackages.enable(pkg.id);
      toast.success(`已启用 ${pkg.platform} v${pkg.version}`);
      await loadPackages();
    } catch (err) {
      toast.error(messageOf(err));
    }
  }

  async function handleDelete() {
    if (!confirmDelete) return;
    try {
      await api.clientPackages.remove(confirmDelete.id);
      toast.success('删除成功');
      setConfirmDelete(null);
      await loadPackages();
    } catch (err) {
      toast.error(messageOf(err));
    }
  }

  const groupedByPlatform = useMemo(() => {
    const grouped: Record<string, ClientPackageItem[]> = {};
    for (const pkg of packages) {
      if (!grouped[pkg.platform]) grouped[pkg.platform] = [];
      grouped[pkg.platform].push(pkg);
    }
    return grouped;
  }, [packages]);

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>客户端安装包管理</CardTitle>
            <CardDescription>上传各平台的 OpenVPN 客户端安装包，用户开通邮件会自动附带下载链接</CardDescription>
          </div>
          <HasPermission code="settings:packages:upload">
            <Button onClick={openUpload} disabled={loading}>
              <Upload className="h-4 w-4 mr-2" />
              上传安装包
            </Button>
          </HasPermission>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="py-12 text-center text-muted-foreground text-sm">加载中...</div>
          ) : packages.length === 0 ? (
            <div className="py-12 text-center">
              <Package className="h-12 w-12 mx-auto text-muted-foreground mb-3" />
              <p className="text-muted-foreground text-sm">暂无安装包，点击"上传安装包"开始添加</p>
            </div>
          ) : (
            <div className="space-y-6">
              {Object.entries(PLATFORM_LABELS).map(([platform, info]) => {
                const pkgs = groupedByPlatform[platform] || [];
                if (pkgs.length === 0) return null;
                return (
                  <div key={platform}>
                    <div className="flex items-center gap-2 mb-3">
                      <span className={`px-2 py-0.5 rounded text-xs font-medium border ${info.className}`}>{info.label}</span>
                      <span className="text-xs text-muted-foreground">{pkgs.length} 个版本</span>
                    </div>
                    <div className="space-y-2">
                      {pkgs.map((pkg) => (
                        <div
                          key={pkg.id}
                          className={cn(
                            'flex items-center justify-between rounded-lg border p-3 transition-colors',
                            pkg.isActive
                              ? 'border-[color-mix(in_srgb,var(--accent)_35%,transparent)] bg-[color-mix(in_srgb,var(--accent)_8%,transparent)]'
                              : 'border-border'
                          )}
                        >
                          <div className="flex items-center gap-3 min-w-0">
                            <div className="flex-shrink-0">
                              {pkg.isActive ? (
                                <CheckCircle2 className="h-5 w-5 text-[var(--accent)]" />
                              ) : (
                                <Package className="h-5 w-5 text-muted-foreground" />
                              )}
                            </div>
                            <div className="min-w-0">
                              <div className="flex flex-col sm:flex-row items-center gap-2 w-full sm:w-auto">
                                <span className="font-medium text-sm">{pkg.filename}</span>
                                {pkg.isActive && (
                                  <span className="px-1.5 py-0.5 rounded text-[10px] font-medium border bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] text-[var(--accent)] border-[color-mix(in_srgb,var(--accent)_25%,transparent)]">
                                    已启用
                                  </span>
                                )}
                              </div>
                              <div className="flex items-center gap-3 text-xs text-muted-foreground mt-0.5">
                                <span>v{pkg.version}</span>
                                <span>{formatFileSize(pkg.fileSize)}</span>
                                <span>{new Date(pkg.createdAt).toLocaleString('zh-CN')}</span>
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            {!pkg.isActive && (
                              <HasPermission code="settings:packages:enable">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() => handleActivate(pkg)}
                                >
                                  <CheckCircle2 className="h-3.5 w-3.5 mr-1" />
                                  启用
                                </Button>
                              </HasPermission>
                            )}
                            <HasPermission code="settings:packages:delete">
                              <Button
                                variant="outline"
                                size="sm"
                                onClick={() => setConfirmDelete(pkg)}
                                className="text-destructive hover:bg-destructive hover:text-destructive-foreground hover:border-destructive"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            </HasPermission>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* 上传对话框 */}
      {uploadOpen && (
        <Dialog open onOpenChange={(open) => !uploading && setUploadOpen(open)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>上传客户端安装包</DialogTitle>
              <DialogDescription>选择要上传的安装包文件并填写相关信息</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-2">
              <div>
                <Label className="mb-1.5 block">平台</Label>
                <Select
                  value={uploadForm.platform}
                  onValueChange={(v) => setUploadForm((prev) => ({ ...prev, platform: v }))}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="选择平台" />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(PLATFORM_LABELS).map(([key, val]) => (
                      <SelectItem key={key} value={key}>{val.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label className="mb-1.5 block">版本号</Label>
                <Input
                  placeholder="如 v10.0.0"
                  value={uploadForm.version}
                  onChange={(e) => setUploadForm((prev) => ({ ...prev, version: e.target.value }))}
                />
              </div>
              <div>
                <Label className="mb-1.5 block">安装包文件</Label>
                <div className="border-2 border-dashed border-border rounded-lg p-4 text-center hover:border-accent transition-colors">
                  <input
                    type="file"
                    id="pkg-file"
                    className="hidden"
                    onChange={(e) => setSelectedFile(e.target.files?.[0] ?? null)}
                  />
                  <label htmlFor="pkg-file" className="cursor-pointer">
                    <Upload className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
                    {selectedFile ? (
                      <span className="text-sm font-medium">{selectedFile.name}</span>
                    ) : (
                      <span className="text-sm text-muted-foreground">点击选择文件或拖拽到此处</span>
                    )}
                    <p className="text-xs text-muted-foreground mt-1">最大支持 500MB</p>
                  </label>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => setUploadOpen(false)}
                disabled={uploading}
              >
                取消
              </Button>
              <Button onClick={handleUpload} disabled={uploading}>
                {uploading ? '上传中...' : '上传'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {/* 删除确认 */}
      {confirmDelete && (
        <ConfirmDialog
          state={{
            title: '删除安装包',
            message: `确定要删除 ${confirmDelete.filename} 吗？此操作不可恢复。`,
            danger: true,
            onConfirm: handleDelete,
          }}
          onClose={() => !loading && setConfirmDelete(null)}
        />
      )}
    </>
  );
}