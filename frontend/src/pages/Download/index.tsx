import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Download,
  LogIn,
  Package,
  Monitor,
  Laptop,
  HardDrive,
  Smartphone,
  Apple,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { api } from '@/api';
import { Button } from '@/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Badge } from '@/ui/badge';

interface PublicClientPackage {
  id: number;
  platform: string;
  platformLabel: string;
  version: string;
  filename: string;
  fileSize: number;
  downloadUrl: string;
}

const PLATFORM_CONFIG: Record<string, { label: string; icon: LucideIcon; desc: string }> = {
  windows: { label: 'Windows', icon: Monitor, desc: 'Windows 10 / 11 (64 位)' },
  macos: { label: 'macOS', icon: Apple, desc: 'macOS 11 及以上 (Intel / Apple Silicon)' },
  linux: { label: 'Linux', icon: Laptop, desc: 'Ubuntu / Debian / RHEL / CentOS 等发行版' },
  android: { label: 'Android', icon: Smartphone, desc: 'Android 7.0 及以上' },
  ios: { label: 'iOS', icon: HardDrive, desc: 'iOS 14 及以上' },
};

function formatFileSize(bytes: number): string {
  if (bytes <= 0) return '—';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export default function DownloadPage() {
  const [packages, setPackages] = useState<PublicClientPackage[]>([]);
  const [loading, setLoading] = useState(true);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [searchParams] = useSearchParams();
  const nextParam = searchParams.get('next') || '/overview';

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    api
      .get<PublicClientPackage[]>('/ovpn/public/packages')
      .then((data) => {
        if (cancelled) return;
        setPackages(Array.isArray(data) ? data : []);
        setErrorMsg(null);
      })
      .catch((e) => {
        if (cancelled) return;
        console.error('[DownloadPage] 加载安装包列表失败', e);
        setErrorMsg('加载失败，请刷新页面重试或联系管理员');
        setPackages([]);
      })
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, []);

  const loginRedirectTo = `/login?next=${encodeURIComponent(nextParam)}`;

  return (
    <div className="min-h-screen w-full bg-background text-foreground relative overflow-hidden flex flex-col">
      {/* 动态背景层：移动端减小尺寸/降低 opacity 提升性能 */}
      <div aria-hidden className="pointer-events-none absolute inset-0 z-0">
        {/* 基础网格纹理 */}
        <div
          className="absolute inset-0 opacity-[0.035] dark:opacity-[0.05]"
          style={{
            backgroundImage:
              'linear-gradient(var(--foreground) 1px, transparent 1px), linear-gradient(90deg, var(--foreground) 1px, transparent 1px)',
            backgroundSize: '48px 48px',
            maskImage: 'radial-gradient(ellipse 70% 60% at 50% 40%, black 40%, transparent 100%)',
            WebkitMaskImage: 'radial-gradient(ellipse 70% 60% at 50% 40%, black 40%, transparent 100%)',
          }}
        />
        {/* 主光晕 - 右上（移动端缩小） */}
        <div
          className="absolute -top-24 -right-24 w-[32rem] h-[32rem] sm:w-[48rem] sm:h-[48rem] lg:w-[60rem] lg:h-[60rem] rounded-full blur-3xl opacity-30 dark:opacity-20 sm:opacity-35 sm:dark:opacity-25 lg:opacity-40 lg:dark:opacity-30 animate-pulse"
          style={{
            background:
              'radial-gradient(circle, color-mix(in srgb, var(--accent) 40%, transparent) 0%, transparent 70%)',
          }}
        />
        {/* 次光晕 - 左下（移动端缩小） */}
        <div
          className="absolute bottom-0 -left-32 w-[28rem] h-[28rem] sm:w-[40rem] sm:h-[40rem] lg:w-[50rem] lg:h-[50rem] rounded-full blur-3xl opacity-20 dark:opacity-15 sm:opacity-22 sm:dark:opacity-18 lg:opacity-25 lg:dark:opacity-20"
          style={{
            background:
              'radial-gradient(circle, color-mix(in srgb, var(--accent-2, var(--accent)) 35%, transparent) 0%, transparent 70%)',
          }}
        />
        {/* 微光斑点缀（移动端隐藏以节省性能） */}
        <div
          className="hidden sm:block absolute top-1/3 left-1/2 w-80 h-80 rounded-full blur-3xl opacity-20"
          style={{
            background:
              'radial-gradient(circle, color-mix(in srgb, var(--accent-3, var(--accent)) 40%, transparent) 0%, transparent 70%)',
            animation: 'float 12s ease-in-out infinite',
          }}
        />
      </div>

      {/* 顶栏 */}
      <header className="relative z-10 border-b border-border/40 backdrop-blur-xl bg-background/40 flex-shrink-0">
        <div className="mx-auto max-w-6xl px-4 sm:px-6 h-14 sm:h-16 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2.5 sm:gap-3 min-w-0">
            <div
              className="flex h-8 w-8 sm:h-9 sm:w-9 items-center justify-center rounded-xl shadow-lg shrink-0"
              style={{
                background:
                  'linear-gradient(135deg, color-mix(in srgb, var(--accent) 85%, white) 0%, var(--accent) 100%)',
                boxShadow:
                  '0 4px 14px color-mix(in srgb, var(--accent) 40%, transparent), inset 0 1px 0 rgba(255,255,255,0.2)',
              }}
            >
              <ShieldCheck className="h-4 w-4 sm:h-5 sm:w-5 text-white" strokeWidth={2.5} />
            </div>
            <div className="min-w-0">
              <div className="text-sm sm:text-base font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-[var(--text)] to-[color-mix(in_srgb,var(--text)_60%,var(--accent))] truncate">
                OpenVPN
              </div>
              <div className="text-xs text-muted-foreground -mt-0.5 hidden sm:block">Secure Client Portal</div>
            </div>
          </div>
          <Link to={loginRedirectTo}>
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5 backdrop-blur-sm border-[color-mix(in_srgb,var(--accent)_30%,transparent)] hover:bg-[color-mix(in_srgb,var(--accent)_10%,transparent)] hover:border-[color-mix(in_srgb,var(--accent)_50%,transparent)] transition-all duration-300 shrink-0"
            >
              <LogIn className="h-4 w-4" />
              <span className="hidden sm:inline">登录管理后台</span>
            </Button>
          </Link>
        </div>
      </header>

      {/* 主内容区 */}
      <main className="relative z-10 flex-1 flex flex-col">
        {/* Hero 介绍区：移动端紧凑 */}
        <section className="mx-auto max-w-6xl w-full px-4 sm:px-6 pt-12 sm:pt-20 pb-8 sm:pb-12">
          <div className="flex flex-col items-center text-center gap-4 sm:gap-6">
            <Badge
              variant="outline"
              className="gap-1.5 bg-[color-mix(in_srgb,var(--accent)_8%,transparent)] text-[var(--accent)] border-[color-mix(in_srgb,var(--accent)_25%,transparent)] px-3 py-1 sm:px-4 sm:py-1.5 rounded-full backdrop-blur-sm font-medium shadow-sm text-xs sm:text-sm"
            >
              <Sparkles className="h-3.5 w-3.5" />
              无需登录即可下载客户端
            </Badge>

            <h1 className="text-3xl sm:text-5xl lg:text-6xl font-black tracking-tight leading-[1.1]">
              <span className="bg-clip-text text-transparent bg-gradient-to-b from-[var(--text)] via-[var(--text)] to-[color-mix(in_srgb,var(--text)_40%,var(--accent))]">
                OpenVPN
              </span>
              <br />
              <span className="bg-clip-text text-transparent bg-gradient-to-r from-[var(--accent)] via-[color-mix(in_srgb,var(--accent)_70%,var(--accent-2,var(--accent)))] to-[color-mix(in_srgb,var(--accent)_50%,var(--accent-3,var(--accent)))]">
                客户端下载中心
              </span>
            </h1>

            <p className="max-w-2xl text-muted-foreground text-sm sm:text-base lg:text-lg leading-relaxed">
              根据您的设备选择对应的客户端安装包。下载并安装后，使用管理员发送给您的账号、
              密码及 <code className="font-mono text-xs sm:text-sm px-1.5 py-0.5 rounded bg-muted/80 border border-border/50">.ovpn</code> 配置文件即可连接。
              <br className="hidden sm:block" />
              若需下载个人配置、重置密码或修改 MFA，请点击右上角进入管理后台。
            </p>
          </div>
        </section>

        {/* 包列表卡片 */}
        <section className="mx-auto max-w-6xl w-full px-4 sm:px-6 pb-12 sm:pb-20 flex-1">
          {loading ? (
            <div className="grid gap-4 sm:gap-5 md:grid-cols-2 xl:grid-cols-3">
              {[...Array(6)].map((_, i) => (
                <Card
                  key={i}
                  className="border-border/40 bg-card/30 backdrop-blur-xl animate-pulse"
                >
                  <CardHeader className="pb-3">
                    <div className="h-6 w-24 rounded bg-muted/60" />
                    <div className="h-4 w-48 rounded bg-muted/40 mt-2" />
                  </CardHeader>
                  <CardContent>
                    <div className="h-10 w-full rounded bg-muted/50" />
                  </CardContent>
                </Card>
              ))}
            </div>
          ) : errorMsg ? (
            <Card className="border-destructive/30 bg-destructive/5 backdrop-blur-xl">
              <CardContent className="pt-6 pb-6 flex flex-col items-center justify-center text-center gap-3 min-h-[220px]">
                <Package className="h-10 w-10 text-destructive/70" />
                <p className="text-destructive font-medium">{errorMsg}</p>
                <Button variant="outline" onClick={() => window.location.reload()}>
                  重新加载
                </Button>
              </CardContent>
            </Card>
          ) : packages.length === 0 ? (
            <Card className="border-border/40 bg-card/20 backdrop-blur-xl">
              <CardContent className="pt-12 sm:pt-16 pb-12 sm:pb-16 flex flex-col items-center justify-center text-center gap-4">
                <Package className="h-10 w-10 sm:h-12 sm:w-12 text-muted-foreground/40" />
                <div>
                  <div className="font-semibold text-base sm:text-lg">暂无可用安装包</div>
                  <p className="text-muted-foreground mt-1.5 text-sm">
                    管理员尚未上传或启用客户端安装包，请稍后再访问，或联系您的管理员。
                  </p>
                </div>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 sm:gap-5 md:grid-cols-2 xl:grid-cols-3">
              {packages.map((pkg) => {
                const cfg = PLATFORM_CONFIG[pkg.platform] || {
                  label: pkg.platformLabel || pkg.platform,
                  icon: Package,
                  desc: '',
                };
                const Icon = cfg.icon;
                return (
                  <Card
                    key={pkg.id}
                    className="group relative overflow-hidden border border-border/40 bg-card/30 backdrop-blur-xl hover:border-[color-mix(in_srgb,var(--accent)_40%,transparent)] hover:bg-card/50 hover:-translate-y-1 hover:shadow-[0_20px_40px_-20px_color-mix(in_srgb,var(--accent)_40%,transparent)] transition-all duration-500"
                  >
                    {/* Hover 光晕 */}
                    <div
                      className="absolute -inset-px rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none"
                      style={{
                        background:
                          'radial-gradient(400px circle at var(--x,50%) var(--y,50%), color-mix(in srgb, var(--accent) 15%, transparent), transparent 40%)',
                      }}
                    />
                    <CardHeader className="pb-2 relative z-10">
                      <div className="flex items-start justify-between">
                        <div className="flex items-center gap-2.5 sm:gap-3">
                          <div
                            className="flex h-10 w-10 sm:h-11 sm:w-11 items-center justify-center rounded-xl shadow-md transition-all duration-300 group-hover:scale-110 shrink-0"
                            style={{
                              background:
                                'linear-gradient(135deg, color-mix(in srgb, var(--accent) 15%, var(--card)) 0%, var(--card) 100%)',
                              border:
                                '1px solid color-mix(in srgb, var(--accent) 25%, transparent)',
                            }}
                          >
                            <Icon
                              className="h-4 w-4 sm:h-5 sm:w-5 transition-colors duration-300"
                              style={{ color: 'var(--accent)' }}
                            />
                          </div>
                          <div className="min-w-0">
                            <CardTitle className="text-base sm:text-lg font-semibold bg-clip-text text-transparent bg-gradient-to-r from-[var(--text)] to-[color-mix(in_srgb,var(--text)_70%,var(--accent))] truncate">
                              {cfg.label}
                            </CardTitle>
                            {cfg.desc && (
                              <CardDescription className="mt-0.5 text-xs text-muted-foreground leading-snug">
                                {cfg.desc}
                              </CardDescription>
                            )}
                          </div>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="flex flex-col gap-3 relative z-10">
                      <div
                        className="rounded-lg border px-3 sm:px-3.5 py-3 flex items-center justify-between gap-3 transition-colors duration-300"
                        style={{
                          background:
                            'color-mix(in srgb, var(--accent) 5%, transparent)',
                          borderColor:
                            'color-mix(in srgb, var(--accent) 15%, transparent)',
                        }}
                      >
                        <div className="min-w-0">
                          <div className="text-sm font-medium truncate">{pkg.filename}</div>
                          <div className="mt-0.5 text-xs text-muted-foreground flex items-center gap-2 sm:gap-3">
                            <span>v{pkg.version}</span>
                            <span className="text-border/70">·</span>
                            <span>{formatFileSize(pkg.fileSize)}</span>
                          </div>
                        </div>
                        <Badge
                          variant="outline"
                          className="flex-shrink-0 bg-[color-mix(in_srgb,var(--accent)_12%,transparent)] border-[color-mix(in_srgb,var(--accent)_30%,transparent)] text-[var(--accent)]"
                        >
                          可用
                        </Badge>
                      </div>
                      <a
                        href={pkg.downloadUrl}
                        onClick={(e) => {
                          if (!pkg.downloadUrl) {
                            e.preventDefault();
                          }
                        }}
                        style={{ textDecoration: 'none' }}
                      >
                        <Button
                          className="w-full gap-2 transition-all duration-300 group-hover:shadow-[0_0_20px_-4px_var(--accent)] group-hover:scale-[1.02]"
                          style={{
                            background:
                              'linear-gradient(135deg, var(--accent) 0%, color-mix(in srgb, var(--accent) 70%, black) 100%)',
                          }}
                        >
                          <Download className="h-4 w-4" />
                          下载 {cfg.label} 客户端
                        </Button>
                      </a>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </section>
      </main>

      {/* 页脚 */}
      <footer className="relative z-10 border-t border-border/40 backdrop-blur-sm py-4 sm:py-6 flex-shrink-0 bg-background/20">
        <div className="mx-auto max-w-6xl px-4 sm:px-6 flex flex-col sm:flex-row items-center justify-between gap-2 text-xs text-muted-foreground text-center sm:text-left">
          <span>© {new Date().getFullYear()} OpenVPN Admin · 客户端下载门户</span>
          <span className="hidden sm:inline">本页面内容由系统自动生成，如遇问题请联系管理员</span>
        </div>
      </footer>
    </div>
  );
}
