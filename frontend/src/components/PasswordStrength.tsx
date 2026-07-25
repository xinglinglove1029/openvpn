import { Progress } from '@/ui/progress';

const checks = [
  { key: 'length', label: '12 位以上', test: (v: string) => v.length >= 12 },
  { key: 'case', label: '大小写字母', test: (v: string) => /[a-z]/.test(v) && /[A-Z]/.test(v) },
  { key: 'digit', label: '数字', test: (v: string) => /\d/.test(v) },
  { key: 'special', label: '特殊字符', test: (v: string) => /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(v) },
];

export function PasswordStrength({ value }: { value: string }) {
  const results = checks.map((c) => ({ ...c, ok: c.test(value) }));
  const score = results.filter((c) => c.ok).length;
  const percent = (score / checks.length) * 100;

  return (
    <div className="space-y-2">
      <Progress value={percent} className="h-1.5" />
      <div className="flex gap-2 flex-wrap">
        {results.map((c) => (
          <span
            key={c.key}
            className={`text-xs ${c.ok ? 'text-emerald-600' : 'text-muted-foreground'}`}
          >
            {c.ok ? '✓' : '○'} {c.label}
          </span>
        ))}
      </div>
    </div>
  );
}
