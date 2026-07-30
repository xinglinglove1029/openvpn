import { useState, useEffect } from 'react';
import { Lock, User, KeyRound, ShieldCheck, Eye, EyeOff } from 'lucide-react';
import { useNavigate, useSearchParams, Navigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Checkbox } from '@/ui/checkbox';
import { HeroOrbitScene } from '@/components/HeroOrbitScene';
import { PasswordStrength } from '@/components/PasswordStrength';
import { useAuth } from '@/store/auth';
import { cn } from '@/lib/utils';

type LoginMode = 'login' | 'mfa' | 'first-password';
type FieldErrors = Record<string, string>;

function safeNextPath(raw: string | null): string {
  // 仅允许跳转到站内相对路径，避免开放重定向风险
  if (!raw) return '/overview';
  const decoded = (() => {
    try {
      return decodeURIComponent(raw);
    } catch {
      return raw;
    }
  })();
  if (!decoded.startsWith('/') || decoded.startsWith('//')) return '/overview';
  return decoded;
}

export default function LoginPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { loginWithCredentials, login, user } = useAuth();
  const [mode, setMode] = useState<LoginMode>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [remember7d, setRemember7d] = useState(true);
  const [passcode, setPasscode] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newPasswordAgain, setNewPasswordAgain] = useState('');
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [pendingUser, setPendingUser] = useState<{ id: number; username: string; isFirstLogin?: boolean } | null>(null);
  const [showPassword, setShowPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showNewPasswordAgain, setShowNewPasswordAgain] = useState(false);

  useEffect(() => {
    if (user) {
      const next = safeNextPath(searchParams.get('next'));
      navigate(next, { replace: true });
    }
  }, [user, navigate, searchParams]);

  if (user) {
    return null;
  }

  function validateFields(): FieldErrors {
    const next: FieldErrors = {};
    if (mode === 'login') {
      if (!username.trim()) next.username = '请输入账号';
      if (!password) next.password = '请输入密码';
    } else if (mode === 'mfa') {
      if (!passcode.trim()) next.passcode = '请输入验证码';
    } else if (mode === 'first-password') {
      if (!newPassword || newPassword.length < 12) next.newPassword = '新密码至少 12 位';
      if (newPassword !== newPasswordAgain) next.newPasswordAgain = '两次密码不一致';
    }
    return next;
  }

  function enterPortal() {
    const next = safeNextPath(searchParams.get('next'));
    navigate(next, { replace: true });
  }

  function clearError(field: string) {
    setErrors((prev) => {
      if (!prev[field]) return prev;
      const n = { ...prev };
      delete n[field];
      return n;
    });
  }

  async function submitLogin() {
    const nextErrors = validateFields();
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    try {
      const result = await loginWithCredentials(username, password, remember7d);
      // 后端返回 mfaRequired 表示需要 MFA 验证
      if (result?.mfaRequired) {
        setMode('mfa');
        return;
      }
      if (result?.user?.isFirstLogin) {
        setPendingUser(result.user);
        setMode('first-password');
        return;
      }
      enterPortal();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  }

  async function submitMfa(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validateFields();
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    try {
      // MFA 验证：向 /login 提交用户名 + passcode（后端从缓存中取出 valid_user 进行验证）
      const response = await fetch('/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8' },
        credentials: 'same-origin',
        body: new URLSearchParams({
          username,
          passcode,
          remember7d: remember7d ? 'on' : '',
        }).toString(),
      });
      if (!response.ok) {
        const data = await response.json().catch(() => ({ message: response.statusText }));
        throw new Error(data?.message || 'MFA 验证失败');
      }
      const data = await response.json();
      if (data?.user) {
        login(data.user, remember7d);
      }
      enterPortal();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  }

  async function submitFirstPassword(e: React.FormEvent) {
    e.preventDefault();
    const nextErrors = validateFields();
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length || !pendingUser) return;
    setSaving(true);
    try {
      const response = await fetch('/client/modifyPass', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8' },
        credentials: 'same-origin',
        body: new URLSearchParams({
          id: String(pendingUser.id),
          password: newPassword,
          isFirstLogin: 'false',
        }).toString(),
      });
      if (!response.ok) {
        const data = await response.json().catch(() => ({ message: response.statusText }));
        throw new Error(data?.message || '密码修改失败');
      }
      // 首次登录修改密码成功后，直接进入门户
      login(
        {
          id: pendingUser.id,
          username: pendingUser.username,
          isFirstLogin: false,
        },
        remember7d,
      );
      enterPortal();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="reference-login-shell">
      {/* 特效层：网格 + 星星 */}
      <div className="reference-login-mesh" aria-hidden="true" />
      <div className="reference-login-stars" aria-hidden="true">
        <i /><i /><i /><i /><i /><i /><i /><i />
      </div>

      {/* 左侧品牌区 */}
      <aside className="reference-login-brand">
        <div className="reference-login-gradient" />
        <div className="reference-login-glow glow-a" />
        <div className="reference-login-glow glow-b" />
        <div className="reference-login-glow glow-c" />
        <div className="reference-login-orbit" aria-hidden="true">
          <HeroOrbitScene />
        </div>
        <div className="reference-brand-content">
          <div className="reference-brand-logo">
            <div className="reference-brand-mark">OV</div>
            <div>
              <strong>OpenVPN</strong>
              <span>Secure Console</span>
            </div>
          </div>
          <section className="reference-brand-copy">
            <span className="reference-chip">OpenVPN Secure Gateway</span>
            <h1>
              优雅地管理<br />
              <em>你的 VPN 访问网络</em>
            </h1>
            <p>统一管理账号、客户端、分组、防火墙和消息告警，让安全接入更清晰、更可靠、更适合日常运维。</p>
            <div className="reference-stats">
              <div><strong>MFA</strong><span>动态口令认证</span></div>
              <div><strong>Notify</strong><span>上线下线通知</span></div>
              <div><strong>Audit</strong><span>操作留痕审计</span></div>
            </div>
          </section>
          <div className="reference-brand-badges">
            <span>实时在线监控</span>
            <span>Webhook 告警</span>
            <span>证书生命周期</span>
          </div>
          <footer>OpenVPN Web Admin · Local Secure Operations</footer>
        </div>
      </aside>

      {/* 右侧登录表单区 */}
      <section className="reference-login-form">
        <div className="reference-mobile-bg" />
        <div className="reference-login-card">
          {/* 卡片标题 */}
          <div className="reference-card-heading">
            <div className="reference-card-icon" aria-hidden="true">
              <Lock size={22} color="#fff" />
            </div>
            <div>
              <strong>
                {mode === 'first-password' ? '首次登录' : mode === 'mfa' ? '安全验证' : '欢迎回来'}
              </strong>
              <span>
                {saving
                  ? '处理中...'
                  : mode === 'mfa'
                    ? '请完成 MFA 验证'
                    : '请使用管理员账号登录'}
              </span>
            </div>
          </div>

          {/* 登录表单 */}
          {mode === 'login' && (
            <form
              className="reference-login-fields"
              noValidate
              onSubmit={(e) => { e.preventDefault(); void submitLogin(); }}
            >
              <div className={cn('field-line reference-field-line', errors.username && 'has-error')}>
                <Label className="text-[var(--login-label-text)] text-sm">账号</Label>
                <div className="field-input-wrap has-icon">
                  <Input
                    value={username}
                    onChange={(e) => {
                      setUsername(e.target.value);
                      clearError('username');
                    }}
                    placeholder="请输入 OpenVPN 管理账号"
                    autoFocus
                    aria-invalid={errors.username ? 'true' : undefined}
                    className="h-[46px] rounded-lg bg-[var(--login-input-bg)] border-[var(--login-input-border)] text-[var(--login-heading-text)] placeholder:text-[var(--login-placeholder-text)] focus:bg-[var(--login-input-focus-bg)] focus:border-[color-mix(in_srgb,var(--accent)_64%,var(--login-input-border))]"
                  />
                  <div className="field-icon-wrap">
                    <User className="field-icon-svg w-4 h-4" />
                  </div>
                </div>
                {errors.username && (
                  <p className="text-sm font-medium text-destructive">{errors.username}</p>
                )}
              </div>

              <div className={cn('field-line reference-field-line', errors.password && 'has-error')}>
                <Label className="text-[var(--login-label-text)] text-sm">密码</Label>
                <div className="field-input-wrap has-icon relative">
                  <Input
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => {
                      setPassword(e.target.value);
                      clearError('password');
                    }}
                    placeholder="请输入登录密码"
                    aria-invalid={errors.password ? 'true' : undefined}
                    className="h-[46px] rounded-lg bg-[var(--login-input-bg)] border-[var(--login-input-border)] text-[var(--login-heading-text)] placeholder:text-[var(--login-placeholder-text)] focus:bg-[var(--login-input-focus-bg)] focus:border-[color-mix(in_srgb,var(--accent)_64%,var(--login-input-border))] pr-10"
                  />
                  <div className="field-icon-wrap">
                    <Lock className="field-icon-svg w-4 h-4" />
                  </div>
                  <button
                    type="button"
                    tabIndex={-1}
                    aria-label={showPassword ? '隐藏密码' : '显示密码'}
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-[var(--login-muted-text)] hover:text-[var(--login-heading-text)]"
                  >
                    {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                {errors.password && (
                  <p className="text-sm font-medium text-destructive">{errors.password}</p>
                )}
              </div>

              <div className="switch-line reference-switch-line">
                <Label className="text-[var(--login-muted-text)] text-[13px] font-semibold cursor-pointer">
                  7 天内保持登录
                </Label>
                <Checkbox
                  checked={remember7d}
                  onCheckedChange={(checked) => setRemember7d(checked === true)}
                />
              </div>

              <div className="reference-login-actions">
                <Button
                  type="submit"
                  disabled={saving}
                  className="w-full h-[44px] mt-1.5 border-0 rounded-lg text-white bg-[var(--login-button-bg)] shadow-[0_14px_28px_color-mix(in_srgb,var(--accent)_24%,transparent)] text-[15px] font-extrabold tracking-[8px] hover:brightness-108 hover:-translate-y-px"
                >
                  {saving ? '登录中...' : '登 录'}
                </Button>
              </div>
            </form>
          )}

          {/* MFA 验证表单 */}
          {mode === 'mfa' && (
            <form className="reference-login-fields" noValidate onSubmit={submitMfa}>
              <div className={cn('field-line reference-field-line', errors.passcode && 'has-error')}>
                <Label className="text-[var(--login-label-text)] text-sm">MFA 动态验证码</Label>
                <div className="field-input-wrap has-icon">
                  <Input
                    value={passcode}
                    onChange={(e) => {
                      setPasscode(e.target.value);
                      clearError('passcode');
                    }}
                    placeholder="请输入 6 位动态验证码"
                    autoFocus
                    aria-invalid={errors.passcode ? 'true' : undefined}
                    className="h-[46px] rounded-lg bg-[var(--login-input-bg)] border-[var(--login-input-border)] text-[var(--login-heading-text)] placeholder:text-[var(--login-placeholder-text)] focus:bg-[var(--login-input-focus-bg)]"
                  />
                  <div className="field-icon-wrap">
                    <KeyRound className="field-icon-svg w-4 h-4" />
                  </div>
                </div>
                {errors.passcode && (
                  <p className="text-sm font-medium text-destructive">{errors.passcode}</p>
                )}
              </div>
              <p className="modal-hint text-[var(--login-label-text)] text-sm">请输入认证器 App 中当前 6 位动态验证码。</p>
              <div className="reference-login-actions">
                <Button
                  type="submit"
                  disabled={saving}
                  className="w-full h-[44px] border-0 rounded-lg text-white bg-[var(--login-button-bg)] shadow-[0_14px_28px_color-mix(in_srgb,var(--accent)_24%,transparent)] text-[15px] font-extrabold tracking-[8px] hover:brightness-108 hover:-translate-y-px"
                >
                  {saving ? '验证中...' : '完成验证'}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setMode('login')}
                  disabled={saving}
                  className="mt-1.5 border-[rgba(255,255,255,0.12)] text-[rgba(255,255,255,0.84)] bg-[rgba(255,255,255,0.06)]"
                >
                  返回登录
                </Button>
              </div>
            </form>
          )}

          {/* 首次登录修改密码表单 */}
          {mode === 'first-password' && (
            <form className="reference-login-fields" noValidate onSubmit={submitFirstPassword}>
              <div className={cn('field-line reference-field-line', errors.newPassword && 'has-error')}>
                <Label className="text-[var(--login-label-text)] text-sm">新密码</Label>
                <div className="field-input-wrap has-icon relative">
                  <Input
                    type={showNewPassword ? 'text' : 'password'}
                    value={newPassword}
                    onChange={(e) => {
                      setNewPassword(e.target.value);
                      clearError('newPassword');
                    }}
                    placeholder="至少 12 位强密码"
                    autoFocus
                    aria-invalid={errors.newPassword ? 'true' : undefined}
                    className="h-[46px] rounded-lg bg-[var(--login-input-bg)] border-[var(--login-input-border)] text-[var(--login-heading-text)] placeholder:text-[var(--login-placeholder-text)] focus:bg-[var(--login-input-focus-bg)] pr-10"
                  />
                  <div className="field-icon-wrap">
                    <ShieldCheck className="field-icon-svg w-4 h-4" />
                  </div>
                  <button
                    type="button"
                    tabIndex={-1}
                    aria-label={showNewPassword ? '隐藏密码' : '显示密码'}
                    onClick={() => setShowNewPassword(!showNewPassword)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-[var(--login-muted-text)] hover:text-[var(--login-heading-text)]"
                  >
                    {showNewPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                <PasswordStrength value={newPassword} />
                {errors.newPassword && (
                  <p className="text-sm font-medium text-destructive">{errors.newPassword}</p>
                )}
              </div>

              <div className={cn('field-line reference-field-line', errors.newPasswordAgain && 'has-error')}>
                <Label className="text-[var(--login-label-text)] text-sm">确认新密码</Label>
                <div className="field-input-wrap has-icon relative">
                  <Input
                    type={showNewPasswordAgain ? 'text' : 'password'}
                    value={newPasswordAgain}
                    onChange={(e) => {
                      setNewPasswordAgain(e.target.value);
                      clearError('newPasswordAgain');
                    }}
                    placeholder="请再次输入新密码"
                    aria-invalid={errors.newPasswordAgain ? 'true' : undefined}
                    className="h-[46px] rounded-lg bg-[var(--login-input-bg)] border-[var(--login-input-border)] text-[var(--login-heading-text)] placeholder:text-[var(--login-placeholder-text)] focus:bg-[var(--login-input-focus-bg)] pr-10"
                  />
                  <div className="field-icon-wrap">
                    <Lock className="field-icon-svg w-4 h-4" />
                  </div>
                  <button
                    type="button"
                    tabIndex={-1}
                    aria-label={showNewPasswordAgain ? '隐藏密码' : '显示密码'}
                    onClick={() => setShowNewPasswordAgain(!showNewPasswordAgain)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-[var(--login-muted-text)] hover:text-[var(--login-heading-text)]"
                  >
                    {showNewPasswordAgain ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                {errors.newPasswordAgain && (
                  <p className="text-sm font-medium text-destructive">{errors.newPasswordAgain}</p>
                )}
              </div>

              <div className="reference-login-actions">
                <Button
                  type="submit"
                  disabled={saving}
                  className="w-full h-[44px] mt-1.5 border-0 rounded-lg text-white bg-[var(--login-button-bg)] shadow-[0_14px_28px_color-mix(in_srgb,var(--accent)_24%,transparent)] text-[15px] font-extrabold tracking-[8px] hover:brightness-108 hover:-translate-y-px"
                >
                  {saving ? '保存中...' : '保存并进入门户'}
                </Button>
              </div>
            </form>
          )}
        </div>
      </section>
    </main>
  );
}
