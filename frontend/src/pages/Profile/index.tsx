import { useEffect, useState, useCallback } from 'react';
import { Lock, User as UserIcon, ShieldCheck, Save, Eye, EyeOff } from 'lucide-react';
import QRCode from 'qrcode';
import { toast } from 'sonner';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Button } from '@/ui/button';
import { Badge } from '@/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Separator } from '@/ui/separator';
import { api } from '@/api';
import { messageOf } from '@/lib/format';
import { isStrongPassword, isValidEmail, trimText } from '@/lib/validators';
import { useAuth } from '@/store/auth';
import { PasswordStrength } from '@/components/PasswordStrength';
import { GlowButton } from '@/components/GlowButton';
import { AvatarPicker } from '@/components/AvatarPicker';
import { cn } from '@/lib/utils';
import type { UserRecord } from '@/types';

type FieldErrors = Record<string, string>;

// 表单行：Label 在左、右对齐；输入控件在右
function FormField({
  id,
  label,
  required,
  error,
  children,
}: {
  id?: string;
  label: React.ReactNode;
  required?: boolean;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[140px_1fr] items-start gap-4">
      <Label htmlFor={id} className="pt-2 text-right text-sm font-medium text-foreground/80">
        {label}
        {required && <span className="text-destructive ml-0.5">*</span>}
      </Label>
      <div className="space-y-1.5 min-w-0">
        {children}
        {error && <p className="text-xs font-medium text-destructive">{error}</p>}
      </div>
    </div>
  );
}

export default function ProfilePage() {
  const { user, logout, updateUser } = useAuth();
  const [profile, setProfile] = useState<UserRecord | null>(null);
  const [loadingProfile, setLoadingProfile] = useState(true);

  // 基本资料表单字段
  const [editName, setEditName] = useState('');
  const [editEmail, setEditEmail] = useState('');
  const [profileErrors, setProfileErrors] = useState<FieldErrors>({});
  const [savingProfile, setSavingProfile] = useState(false);

  // 修改密码表单字段
  const [currentPass, setCurrentPass] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newPasswordAgain, setNewPasswordAgain] = useState('');
  const [showCurrentPass, setShowCurrentPass] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showNewPasswordAgain, setShowNewPasswordAgain] = useState(false);

  const [savingPassword, setSavingPassword] = useState(false);
  const [passwordErrors, setPasswordErrors] = useState<FieldErrors>({});

  // MFA 绑定状态
  const [mfaBound, setMfaBound] = useState(false);
  const [mfaBinding, setMfaBinding] = useState(false);
  const [mfaSecret, setMfaSecret] = useState('');
  const [qrDataUrl, setQrDataUrl] = useState('');
  const [mfaPasscode, setMfaPasscode] = useState('');
  const [mfaError, setMfaError] = useState('');
  const [mfaSaving, setMfaSaving] = useState(false);
  const [mfaUnbinding, setMfaUnbinding] = useState(false);

  // 加载当前用户信息
  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!user?.username) {
        setLoadingProfile(false);
        return;
      }
      try {
        const data = await api.get<UserRecord>('/ovpn/user/me');
        if (!cancelled) {
          setProfile(data);
          setEditName(data?.name ?? user?.name ?? '');
          setEditEmail(data?.email ?? user?.email ?? '');
          setProfileErrors({});
          setMfaBound(!!data?.mfaSecret || !!data?.mfaEnabled);
        }
      } catch (error) {
        if (!cancelled) {
          toast.error(`加载用户信息失败：${messageOf(error)}`);
        }
      } finally {
        if (!cancelled) setLoadingProfile(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [user?.id, user?.username]);

  function validateProfile(): FieldErrors {
    const next: FieldErrors = {};
    const name = trimText(editName);
    if (!name) {
      next.name = '请输入姓名';
    } else if (name.length > 64) {
      next.name = '姓名长度不能超过 64 个字符';
    }
    const email = trimText(editEmail);
    if (!email) {
      next.email = '请输入邮箱';
    } else if (!isValidEmail(email)) {
      next.email = '邮箱格式不正确';
    } else if (email.length > 128) {
      next.email = '邮箱长度不能超过 128 个字符';
    }
    return next;
  }

  function clearProfileError(key: string) {
    setProfileErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  async function handleSaveProfile(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validateProfile();
    setProfileErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;

    const trimmedName = trimText(editName);
    const trimmedEmail = trimText(editEmail);

    setSavingProfile(true);
    try {
      // 统一调用 PATCH /ovpn/user/profile（不需要 user:update 权限，只改自己的 name/email）
      const result = await api.patchForm<{ message: string }>('/ovpn/user/profile', {
        name: trimmedName,
        email: trimmedEmail,
      });
      setProfile((prev) =>
        prev
          ? { ...prev, name: trimmedName, email: trimmedEmail }
          : prev,
      );
      // 同步更新顶部导航展示的用户信息
      updateUser({ name: trimmedName, email: trimmedEmail });
      toast.success(result.message || '基本资料已更新');
    } catch (error) {
      toast.error(`保存失败：${messageOf(error)}`);
    } finally {
      setSavingProfile(false);
    }
  }

  function validatePassword(): FieldErrors {
    const next: FieldErrors = {};
    if (!currentPass.trim()) next.currentPass = '请输入当前密码';
    if (!newPassword) {
      next.newPassword = '请输入新密码';
    } else if (!isStrongPassword(newPassword)) {
      next.newPassword = '密码至少 12 位，且需包含大小写字母、数字、特殊字符';
    }
    if (!newPasswordAgain) {
      next.newPasswordAgain = '请再次输入新密码';
    } else if (newPassword && newPassword !== newPasswordAgain) {
      next.newPasswordAgain = '两次密码不一致';
    }
    return next;
  }

  function clearError(key: string) {
    setPasswordErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  async function handleChangePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!user?.username) return;
    const nextErrors = validatePassword();
    setPasswordErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;

    setSavingPassword(true);
    try {
      await api.postForm<{ message: string }>('/client/modifyPass', {
        id: user.id ?? 0,
        username: user.username,
        currentPass,
        password: newPassword,
        isFirstLogin: false,
      });
      toast.success('密码已修改，请使用新密码重新登录');
      setCurrentPass('');
      setNewPassword('');
      setNewPasswordAgain('');
      setPasswordErrors({});
      // 密码修改后清除本地登录态并跳转到登录页
      setTimeout(() => {
        void logout();
      }, 600);
    } catch (error) {
      toast.error(`修改失败：${messageOf(error)}`);
    } finally {
      setSavingPassword(false);
    }
  }

  // 开始 MFA 绑定：调用 GET /client/mfa 获取 secret 和 otpauthUrl
  const handleStartBindMfa = useCallback(async () => {
    setMfaBinding(true);
    setMfaPasscode('');
    setMfaError('');
    try {
      const data = await api.get<{ mfaEnable: boolean; user: UserRecord; otpauthUrl?: string }>('/client/mfa');
      if (data.otpauthUrl && data.user?.mfaSecret) {
        setMfaSecret(data.user.mfaSecret);
        const url = await QRCode.toDataURL(data.otpauthUrl, { width: 200, margin: 2 });
        setQrDataUrl(url);
      }
    } catch (error) {
      toast.error(`获取 MFA 信息失败：${messageOf(error)}`);
      setMfaBinding(false);
    }
  }, []);

  // 确认绑定 MFA：调用 POST /client/mfa 验证 passcode 并保存 secret
  async function handleConfirmBindMfa(e: React.FormEvent) {
    e.preventDefault();
    if (!mfaPasscode.trim()) {
      setMfaError('请输入验证码');
      return;
    }
    setMfaSaving(true);
    try {
      await api.postForm('/client/mfa', {
        id: user?.id ?? 0,
        mfaSecret,
        passcode: mfaPasscode,
      });
      toast.success('MFA 绑定成功，新的客户端配置文件已发送至您的邮箱，请使用新配置连接 VPN');
      setMfaBound(true);
      setMfaBinding(false);
      setMfaPasscode('');
      setMfaSecret('');
      setQrDataUrl('');
      setProfile((prev) => (prev ? { ...prev, mfaSecret: '***', mfaEnabled: true } : prev));
    } catch (error) {
      setMfaError(messageOf(error));
    } finally {
      setMfaSaving(false);
    }
  }

  // 取消 MFA 绑定
  function handleCancelBindMfa() {
    setMfaBinding(false);
    setMfaPasscode('');
    setMfaError('');
    setMfaSecret('');
    setQrDataUrl('');
  }

  // 解除 MFA 绑定
  async function handleUnbindMfa() {
    setMfaUnbinding(true);
    try {
      await api.delete(`/client/mfa/${user?.id ?? 0}`);
      toast.success('MFA 已解除绑定');
      setMfaBound(false);
      setProfile((prev) => (prev ? { ...prev, mfaSecret: '', mfaEnabled: false } : prev));
    } catch (error) {
      toast.error(`解除绑定失败：${messageOf(error)}`);
    } finally {
      setMfaUnbinding(false);
    }
  }

  const avatarSeed = profile?.email || user?.email || user?.username || 'U';
  const displayName = profile?.name || user?.name || user?.username || '用户';
  const displayUsername = profile?.username || user?.username || '-';
  const displayEmail = profile?.email || user?.email || '-';
  const displayUserId = user?.id && user.id > 0 ? String(user.id) : '系统内置';
  // RBAC：admin 用户由后端下发的 isAdmin 字段决定，兼容历史数据（id<=0）
  const isAdmin = !!user?.isAdmin || !user?.id || user.id <= 0;

  function handleAvatarChange(next: string | undefined) {
    updateUser({ avatar: next });
  }

  return (
    <div className="space-y-6 max-w-4xl">
      <div>
        <h1 className="text-3xl font-bold">个人设置</h1>
        <p className="text-muted-foreground mt-1">维护基本资料并修改登录密码</p>
      </div>

      {/* 基本资料：可编辑，提交前进行校验 */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-4">
            <AvatarPicker
              value={user?.avatar}
              fallbackSeed={avatarSeed}
              displayName={displayUsername}
              onChange={handleAvatarChange}
              size={64}
            />
            <div className="min-w-0">
              <CardTitle className="flex items-center gap-2">
                <UserIcon className="w-4 h-4" />
                基本资料
              </CardTitle>
              <CardDescription>可在此更新您的姓名、邮箱等信息；点击头像可自定义</CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {loadingProfile ? (
            <div className="py-8 text-center text-muted-foreground text-sm">正在加载...</div>
          ) : (
            <form onSubmit={handleSaveProfile} className="space-y-4">
              <FormField id="profile-displayUsername" label="账号">
                <Input
                  id="profile-displayUsername"
                  value={displayUsername}
                  readOnly
                  disabled
                  className="bg-muted/40 cursor-not-allowed"
                />
              </FormField>

              <FormField id="profile-editName" label="姓名" required error={profileErrors.name}>
                <Input
                  id="profile-editName"
                  value={editName}
                  onChange={(e) => {
                    setEditName(e.target.value);
                    clearProfileError('name');
                  }}
                  placeholder="请输入姓名"
                  maxLength={64}
                  aria-invalid={profileErrors.name ? 'true' : undefined}
                  className={cn(
                    profileErrors.name && 'border-destructive focus-visible:ring-destructive/40',
                  )}
                />
              </FormField>

              <FormField id="profile-editEmail" label="邮箱" required error={profileErrors.email}>
                <Input
                  id="profile-editEmail"
                  type="email"
                  value={editEmail}
                  onChange={(e) => {
                    setEditEmail(e.target.value);
                    clearProfileError('email');
                  }}
                  placeholder="请输入邮箱"
                  maxLength={128}
                  aria-invalid={profileErrors.email ? 'true' : undefined}
                  className={cn(
                    profileErrors.email && 'border-destructive focus-visible:ring-destructive/40',
                  )}
                />
              </FormField>

              <FormField id="profile-displayUserId" label="用户 ID">
                <Input
                  id="profile-displayUserId"
                  value={displayUserId}
                  readOnly
                  disabled
                  className="bg-muted/40 cursor-not-allowed"
                />
              </FormField>

              <div className="flex justify-end pt-2">
                <GlowButton
                  type="submit"
                  loading={savingProfile}
                  loadingText="保存中…"
                  icon={<Save className="w-4 h-4" />}
                >
                  保存资料
                </GlowButton>
              </div>
            </form>
          )}
        </CardContent>
      </Card>

      {/* 修改密码 */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="w-4 h-4" />
            修改密码
          </CardTitle>
          <CardDescription>为了账号安全，建议每 90 天更换一次密码</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleChangePassword} className="space-y-4">
            <FormField id="profile-currentPass" label="当前密码" required error={passwordErrors.currentPass}>
              <div className="relative">
                <Input
                  id="profile-currentPass"
                  type={showCurrentPass ? 'text' : 'password'}
                  value={currentPass}
                  onChange={(e) => {
                    setCurrentPass(e.target.value);
                    clearError('currentPass');
                  }}
                  placeholder="请输入当前密码"
                  autoComplete="current-password"
                  aria-invalid={passwordErrors.currentPass ? 'true' : undefined}
                  className={cn(
                    passwordErrors.currentPass && 'border-destructive focus-visible:ring-destructive/40',
                    'pr-9',
                  )}
                />
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={showCurrentPass ? '隐藏密码' : '显示密码'}
                  onClick={() => setShowCurrentPass(!showCurrentPass)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
                >
                  {showCurrentPass ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </FormField>

            <Separator />

            <FormField id="profile-newPassword" label="新密码" required error={passwordErrors.newPassword}>
              <div className="relative">
                <Input
                  id="profile-newPassword"
                  type={showNewPassword ? 'text' : 'password'}
                  value={newPassword}
                  onChange={(e) => {
                    setNewPassword(e.target.value);
                    clearError('newPassword');
                    if (newPasswordAgain && newPasswordAgain === e.target.value) {
                      clearError('newPasswordAgain');
                    }
                  }}
                  placeholder="至少 12 位，包含大小写字母、数字、特殊字符"
                  autoComplete="new-password"
                  aria-invalid={passwordErrors.newPassword ? 'true' : undefined}
                  className={cn(
                    passwordErrors.newPassword && 'border-destructive focus-visible:ring-destructive/40',
                    'pr-9',
                  )}
                />
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={showNewPassword ? '隐藏密码' : '显示密码'}
                  onClick={() => setShowNewPassword(!showNewPassword)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
                >
                  {showNewPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
              <PasswordStrength value={newPassword} />
            </FormField>

            <FormField
              id="profile-newPasswordAgain"
              label="确认新密码"
              required
              error={passwordErrors.newPasswordAgain}
            >
              <div className="relative">
                <Input
                  id="profile-newPasswordAgain"
                  type={showNewPasswordAgain ? 'text' : 'password'}
                  value={newPasswordAgain}
                  onChange={(e) => {
                    setNewPasswordAgain(e.target.value);
                    clearError('newPasswordAgain');
                  }}
                  placeholder="请再次输入新密码"
                  autoComplete="new-password"
                  aria-invalid={passwordErrors.newPasswordAgain ? 'true' : undefined}
                  className={cn(
                    passwordErrors.newPasswordAgain && 'border-destructive focus-visible:ring-destructive/40',
                    'pr-9',
                  )}
                />
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={showNewPasswordAgain ? '隐藏密码' : '显示密码'}
                  onClick={() => setShowNewPasswordAgain(!showNewPasswordAgain)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-muted-foreground hover:text-foreground"
                >
                  {showNewPasswordAgain ? <EyeOff size={16} /> : <Eye size={16} />}
                </button>
              </div>
            </FormField>

            <div className="flex items-center gap-2 pl-[156px] text-xs text-muted-foreground">
              <ShieldCheck className="w-3.5 h-3.5" />
              密码要求：12 位以上 · 大小写字母 · 数字 · 特殊字符
            </div>

            <div className="flex justify-end pt-2">
              <GlowButton
                type="submit"
                loading={savingPassword}
                loadingText="提交中…"
                icon={<Lock className="w-4 h-4" />}
              >
                更新密码
              </GlowButton>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* MFA 双因素认证：仅非系统内置用户可绑定 */}
      {!isAdmin && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="w-4 h-4" />
              MFA 双因素认证
            </CardTitle>
            <CardDescription>
              使用 TOTP 认证 App（如 Google Authenticator）绑定动态口令，绑定后登录管理后台和连接 OpenVPN 均需输入验证码
            </CardDescription>
          </CardHeader>
          <CardContent>
            {/* 未绑定 — 初始状态 */}
            {!mfaBound && !mfaBinding && (
              <div className="space-y-4">
                <FormField id="mfa-status" label="当前状态">
                  <div className="pt-2">
                    <Badge variant="outline">未绑定</Badge>
                  </div>
                </FormField>
                <div className="flex justify-end pt-2">
                  <GlowButton
                    type="button"
                    icon={<ShieldCheck className="w-4 h-4" />}
                    onClick={handleStartBindMfa}
                  >
                    绑定 MFA
                  </GlowButton>
                </div>
              </div>
            )}

            {/* 绑定流程 — 显示二维码和验证码输入 */}
            {!mfaBound && mfaBinding && (
              <form onSubmit={handleConfirmBindMfa} className="space-y-4">
                <div className="flex flex-col items-center gap-3 py-2">
                  {qrDataUrl && (
                    <img src={qrDataUrl} alt="MFA 二维码" className="w-[200px] h-[200px] rounded-lg" />
                  )}
                  <p className="text-xs text-muted-foreground text-center">
                    请使用 TOTP 认证 App 扫描二维码
                  </p>
                  {mfaSecret && (
                    <div className="text-xs text-muted-foreground">
                      无法扫码？手动输入密钥：
                      <code className="ml-2 font-mono bg-muted/40 px-2 py-1 rounded">{mfaSecret}</code>
                    </div>
                  )}
                </div>

                <FormField id="mfa-passcode" label="验证码" required error={mfaError}>
                  <Input
                    id="mfa-passcode"
                    value={mfaPasscode}
                    onChange={(e) => {
                      setMfaPasscode(e.target.value);
                      setMfaError('');
                    }}
                    placeholder="请输入 6 位动态验证码"
                    maxLength={6}
                    autoFocus
                    className={cn(mfaError && 'border-destructive focus-visible:ring-destructive/40')}
                  />
                </FormField>

                <div className="flex justify-end gap-2 pt-2">
                  <Button type="button" variant="outline" onClick={handleCancelBindMfa}>
                    取消
                  </Button>
                  <GlowButton
                    type="submit"
                    loading={mfaSaving}
                    loadingText="验证中…"
                    icon={<ShieldCheck className="w-4 h-4" />}
                  >
                    确认绑定
                  </GlowButton>
                </div>
              </form>
            )}

            {/* 已绑定 */}
            {mfaBound && (
              <div className="space-y-4">
                <FormField id="mfa-status-bound" label="当前状态">
                  <div className="pt-2">
                    <Badge variant="secondary">已绑定</Badge>
                  </div>
                </FormField>
                <div className="flex justify-end pt-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={handleUnbindMfa}
                    disabled={mfaUnbinding}
                  >
                    {mfaUnbinding ? '解除中…' : '解除绑定'}
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
