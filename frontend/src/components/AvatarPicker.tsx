import { useEffect, useMemo, useRef, useState } from 'react';
import { Camera, Image as ImageIcon, RefreshCcw, Sparkles, Upload, X } from 'lucide-react';
import { toast } from 'sonner';

import { Avatar, AvatarFallback, AvatarImage } from '@/ui/avatar';
import { Button } from '@/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/ui/tabs';
import { cn } from '@/lib/utils';

const MAX_FILE_SIZE = 2 * 1024 * 1024; // 2MB
const ALLOWED_TYPES = ['image/png', 'image/jpeg', 'image/jpg', 'image/webp', 'image/gif', 'image/svg+xml'];

const PRESET_STYLES = [
  { id: 'avataaars', label: '卡通' },
  { id: 'personas', label: '面具' },
  { id: 'lorelei', label: '洛雷莱' },
  { id: 'notionists', label: '极简' },
  { id: 'fun-emoji', label: '趣味' },
  { id: 'thumbs', label: '拇指' },
] as const;

const PRESET_VARIANTS = 12;

function generateAvatarSvg(seed: string, style: string): string {
  const hash = seed.split('').reduce((acc, char) => acc + char.charCodeAt(0), 0);
  
  const colors: Record<string, string[]> = {
    avataaars: ['#FF6B6B', '#4ECDC4', '#45B7D1', '#96CEB4', '#FFEAA7', '#DDA0DD', '#98D8C8', '#F7DC6F'],
    personas: ['#3498DB', '#E74C3C', '#2ECC71', '#F39C12', '#9B59B6', '#1ABC9C', '#E67E22', '#16A085'],
    lorelei: ['#E91E63', '#FF5722', '#FF9800', '#4CAF50', '#2196F3', '#3F51B5', '#00BCD4', '#FFEB3B'],
    notionists: ['#667EEA', '#764BA2', '#F093FB', '#F5576C', '#4FACFE', '#00F2FE', '#43E97B', '#38F9D7'],
    'fun-emoji': ['#FF0000', '#FF69B4', '#FFD700', '#00FF00', '#00BFFF', '#9370DB', '#FF6347', '#00CED1'],
    thumbs: ['#7C4DFF', '#651FFF', '#536DFE', '#448AFF', '#40C4FF', '#18FFFF', '#64FFDA', '#69F0AE'],
  };
  
  const palette = colors[style] || colors.avataaars;
  const color = palette[hash % palette.length];
  
  const shapes = ['circle', 'square', 'triangle'];
  const shape = shapes[hash % shapes.length];
  
  const letters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
  const letter = letters[hash % letters.length];
  
  let shapePath = '';
  if (shape === 'circle') {
    shapePath = '<circle cx="24" cy="24" r="20" />';
  } else if (shape === 'square') {
    shapePath = '<rect x="4" y="4" width="40" height="40" rx="8" />';
  } else {
    shapePath = '<polygon points="24,4 44,44 4,44" />';
  }
  
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
      <rect width="48" height="48" rx="24" fill="white" />
      <g fill="${color}">
        ${shapePath}
      </g>
      <text x="24" y="29" text-anchor="middle" font-size="16" font-weight="bold" fill="white" font-family="system-ui, sans-serif">
        ${letter}
      </text>
    </svg>
  `.trim();
  
  return `data:image/svg+xml;base64,${btoa(svg)}`;
}

export function defaultAvatarUrl(seed: string, style: string = 'avataaars'): string {
  return generateAvatarSvg(seed, style);
}

/**
 * 解析当前头像：
 * - 空：未设置
 * - 包含 ":"：视为 preset 标识，形如 "preset:<style>:<seed>"
 * - 其他：视为 data URL / 远程 URL（用户上传）
 */
export function parseAvatarValue(avatar: string | undefined, fallbackSeed: string) {
  if (!avatar) {
    return { kind: 'none' as const, url: '' };
  }
  if (avatar.startsWith('preset:')) {
    const [, style = 'avataaars', ...rest] = avatar.split(':');
    const seed = rest.join(':') || fallbackSeed;
    return { kind: 'preset' as const, url: defaultAvatarUrl(seed, style), style, seed };
  }
  if (avatar.startsWith('http') || avatar.startsWith('data:')) {
    return { kind: 'custom' as const, url: avatar };
  }
  return { kind: 'none' as const, url: '' };
}

interface AvatarPickerProps {
  /** 当前头像值：data URL / 远程 URL / "preset:<style>:<seed>" / 空 */
  value: string | undefined;
  /** 头像预览/默认 seed：通常是 email 或 username */
  fallbackSeed: string;
  /** 展示名，用于首字母兜底 */
  displayName?: string;
  /** 保存回调：返回新的 avatar 值；返回 undefined 表示未改动 */
  onChange: (next: string | undefined) => void;
  /** 头像尺寸，默认 64 */
  size?: number;
}

export function AvatarPicker({
  value,
  fallbackSeed,
  displayName,
  onChange,
  size = 64,
}: AvatarPickerProps) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<string | undefined>(value);
  const [activeStyle, setActiveStyle] = useState<string>('avataaars');
  const fileInputRef = useRef<HTMLInputElement>(null);

  // 打开弹窗时初始化草稿
  useEffect(() => {
    if (!open) return;
    const parsed = parseAvatarValue(value, fallbackSeed);
    setDraft(value);
    if (parsed.kind === 'preset' && parsed.style) {
      setActiveStyle(parsed.style);
    }
  }, [open, value, fallbackSeed]);

  const preview = useMemo(() => {
    const parsed = parseAvatarValue(draft, fallbackSeed);
    if (parsed.kind === 'none') return defaultAvatarUrl(fallbackSeed, activeStyle);
    return parsed.url;
  }, [draft, fallbackSeed, activeStyle]);

  // 是否与当前保存值有差异，未改动时禁用"应用"按钮
  const isDirty = (draft ?? null) !== (value ?? null);
  const firstLetter = (displayName || 'U')[0]?.toUpperCase() || 'U';

  function handlePickPreset(style: string, seed: string) {
    setActiveStyle(style);
    setDraft(`preset:${style}:${seed}`);
  }

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = ''; // 允许重复选择同一文件
    if (!file) return;

    if (!ALLOWED_TYPES.includes(file.type)) {
      toast.error('仅支持 PNG / JPG / WEBP / GIF / SVG 图片');
      return;
    }
    if (file.size > MAX_FILE_SIZE) {
      toast.error('图片大小不能超过 2MB');
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : '';
      if (!result) {
        toast.error('图片读取失败');
        return;
      }
      setDraft(result);
    };
    reader.onerror = () => toast.error('图片读取失败');
    reader.readAsDataURL(file);
  }

  function handleApply() {
    if (!isDirty) {
      setOpen(false);
      return;
    }
    onChange(draft);
    setOpen(false);
    toast.success('头像已更新');
  }

  function handleReset() {
    setDraft(undefined);
  }

  function handleCancel() {
    setOpen(false);
  }

  return (
    <>
      {/* 触发器：圆形头像 + 悬浮编辑提示 */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="group relative inline-flex items-center justify-center rounded-full outline-none ring-offset-background focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        style={{ width: size, height: size }}
        aria-label="修改头像"
      >
        <Avatar className="h-full w-full border border-border/60">
          {value ? (
            <AvatarImage src={preview} alt="头像" />
          ) : null}
          <AvatarFallback className="bg-muted text-muted-foreground text-lg font-semibold">
            {firstLetter}
          </AvatarFallback>
        </Avatar>
        <span
          className={cn(
            'absolute inset-0 flex items-center justify-center rounded-full',
            'bg-black/45 text-white opacity-0 transition-opacity duration-200',
            'group-hover:opacity-100 group-focus-visible:opacity-100',
          )}
        >
          <Camera className="h-5 w-5" />
        </span>
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-accent" />
              自定义头像
            </DialogTitle>
            <DialogDescription>
              从预设风格中选择，或上传本地图片（不超过 2MB）
            </DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="preset" className="w-full">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="preset" className="gap-1.5">
                <Sparkles className="h-3.5 w-3.5" />
                预设风格
              </TabsTrigger>
              <TabsTrigger value="upload" className="gap-1.5">
                <Upload className="h-3.5 w-3.5" />
                上传图片
              </TabsTrigger>
            </TabsList>

            <TabsContent value="preset" className="mt-4 space-y-3">
              <div className="flex flex-wrap gap-1.5">
                {PRESET_STYLES.map((style) => (
                  <button
                    key={style.id}
                    type="button"
                    onClick={() => setActiveStyle(style.id)}
                    className={cn(
                      'rounded-full border px-3 py-1 text-xs font-medium transition-colors',
                      activeStyle === style.id
                        ? 'border-accent bg-accent/10 text-accent'
                        : 'border-border/60 text-muted-foreground hover:border-accent/40 hover:text-foreground',
                    )}
                  >
                    {style.label}
                  </button>
                ))}
              </div>
              <div className="grid grid-cols-6 gap-3">
                {Array.from({ length: PRESET_VARIANTS }).map((_, index) => {
                  const seed = `${fallbackSeed}-${index}`;
                  const url = defaultAvatarUrl(seed, activeStyle);
                  const isActive =
                    draft === `preset:${activeStyle}:${seed}` ||
                    (draft === undefined && activeStyle === 'avataaars' && index === 0 && !value);
                  return (
                    <button
                      key={`${activeStyle}-${index}`}
                      type="button"
                      onClick={() => handlePickPreset(activeStyle, seed)}
                      className={cn(
                        'relative aspect-square overflow-hidden rounded-full border-2 transition-all',
                        isActive
                          ? 'border-accent shadow-[0_0_0_3px_color-mix(in_srgb,var(--accent)_25%,transparent)]'
                          : 'border-transparent hover:border-accent/50',
                      )}
                    >
                      <img
                        src={url}
                        alt={`预设头像 ${index + 1}`}
                        className="h-full w-full bg-muted object-cover"
                        loading="lazy"
                      />
                    </button>
                  );
                })}
              </div>
            </TabsContent>

            <TabsContent value="upload" className="mt-4 space-y-3">
              <div
                className={cn(
                  'flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border/60 bg-muted/30 px-4 py-8 text-center transition-colors',
                  'hover:border-accent/40 hover:bg-muted/40',
                )}
              >
                {draft && (draft.startsWith('data:') || draft.startsWith('http')) ? (
                  <div className="relative">
                    <Avatar className="h-24 w-24 border border-border/60">
                      <AvatarImage src={draft} alt="已选头像" />
                      <AvatarFallback>
                        <ImageIcon className="h-6 w-6" />
                      </AvatarFallback>
                    </Avatar>
                    <button
                      type="button"
                      onClick={handleReset}
                      className="absolute -right-2 -top-2 rounded-full border border-border/60 bg-background p-1 text-muted-foreground shadow-sm transition-colors hover:text-foreground"
                      title="清除已选图片"
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ) : (
                  <>
                    <ImageIcon className="h-7 w-7 text-muted-foreground" />
                    <p className="text-sm text-muted-foreground">点击下方按钮选择本地图片</p>
                  </>
                )}
                <input
                  ref={fileInputRef}
                  type="file"
                  accept={ALLOWED_TYPES.join(',')}
                  className="hidden"
                  onChange={handleFileChange}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => fileInputRef.current?.click()}
                  className="gap-1.5"
                >
                  <Upload className="h-3.5 w-3.5" />
                  选择图片
                </Button>
                <p className="text-xs text-muted-foreground">支持 PNG / JPG / WEBP / GIF / SVG，最大 2MB</p>
              </div>
            </TabsContent>
          </Tabs>

          <DialogFooter className="gap-2 sm:gap-2">
            <Button type="button" variant="ghost" onClick={handleReset} className="gap-1.5">
              <RefreshCcw className="h-3.5 w-3.5" />
              恢复默认
            </Button>
            <div className="flex items-center gap-2">
              <Button type="button" variant="outline" onClick={handleCancel}>
                取消
              </Button>
              <Button type="button" onClick={handleApply} disabled={!isDirty}>
                应用
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
