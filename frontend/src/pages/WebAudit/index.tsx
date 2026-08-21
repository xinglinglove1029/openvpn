import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Activity, AlertTriangle, Download, Globe2, Network, RefreshCw, Search, ShieldAlert, ShieldCheck, Users } from 'lucide-react';
import { api } from '../../api';
import { PageHeader } from '@/components/PageHeader';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/ui/select';
import { Badge } from '@/ui/badge';
import { formatDateTime } from '@/lib/utils';
import type { WebsiteAccessRecord, WebsiteAuditRecordsResponse, WebsiteAuditStatus, WebsiteAuditSummary, WebsiteAuditUsersResponse } from '../../types';

const ranges = [
  ['1h', '近 1 小时'], ['6h', '近 6 小时'], ['24h', '近 24 小时'], ['7d', '近 7 天'], ['30d', '近 30 天'],
] as const;
type RangeName = typeof ranges[number][0];
type AppliedQuery = { range: RangeName; username: string; domain: string; end: number };

const pageSize = 20;
const rangeSeconds = (range: RangeName) => ({ '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800, '30d': 2592000 } as Record<RangeName, number>)[range];
const formatCount = (value?: number) => new Intl.NumberFormat('zh-CN').format(value || 0);
const statusTone = (ok: boolean) => ok ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-600' : 'border-amber-500/25 bg-amber-500/10 text-amber-600';

function buildParams(query: AppliedQuery) {
  const params = new URLSearchParams({ start: String(query.end - rangeSeconds(query.range)), end: String(query.end) });
  if (query.username) params.set('username', query.username);
  if (query.domain) params.set('domain', query.domain);
  return params;
}

export default function WebAuditPage() {
  const [draftRange, setDraftRange] = useState<RangeName>('24h');
  const [draftUsername, setDraftUsername] = useState('');
  const [draftDomain, setDraftDomain] = useState('');
  const [query, setQuery] = useState<AppliedQuery>(() => ({ range: '24h', username: '', domain: '', end: Math.floor(Date.now() / 1000) }));
  const [page, setPage] = useState(0);
  const [summary, setSummary] = useState<WebsiteAuditSummary>();
  const [records, setRecords] = useState<WebsiteAuditRecordsResponse>();
  const [status, setStatus] = useState<WebsiteAuditStatus>();
  const [auditUsers, setAuditUsers] = useState<WebsiteAuditUsersResponse['data']>([]);
  const [loadingSummary, setLoadingSummary] = useState(true);
  const [loadingRecords, setLoadingRecords] = useState(true);
  const [loadingStatus, setLoadingStatus] = useState(true);
  const [error, setError] = useState('');
  const summaryRequest = useRef(0);
  const recordsRequest = useRef(0);
  const statusRequest = useRef(0);

  const params = useMemo(() => buildParams(query), [query]);
  const applyFilters = useCallback(() => {
    setError('');
    setPage(0);
    setQuery({ range: draftRange, username: draftUsername.trim(), domain: draftDomain.trim(), end: Math.floor(Date.now() / 1000) });
  }, [draftDomain, draftRange, draftUsername]);

  const refreshStatus = useCallback(async () => {
    const requestID = ++statusRequest.current;
    setLoadingStatus(true);
    try {
      const next = await api.get<WebsiteAuditStatus>('/ovpn/web-audit/status');
      if (requestID === statusRequest.current) setStatus(next);
    } catch (err) {
      if (requestID === statusRequest.current) setError(err instanceof Error ? err.message : '读取域名审计状态失败');
    } finally {
      if (requestID === statusRequest.current) setLoadingStatus(false);
    }
  }, []);

  const refreshAll = useCallback(() => {
    applyFilters();
    void refreshStatus();
  }, [applyFilters, refreshStatus]);

  useEffect(() => { void refreshStatus(); }, [refreshStatus]);
  useEffect(() => {
    void api.get<WebsiteAuditUsersResponse>('/ovpn/web-audit/users')
      .then((next) => setAuditUsers(Array.isArray(next?.data) ? next.data : []))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : '加载网站审计用户失败'));
  }, []);
  useEffect(() => {
    const requestID = ++summaryRequest.current;
    setLoadingSummary(true);
    void api.get<WebsiteAuditSummary>(`/ovpn/web-audit/summary?${params.toString()}`)
      .then((next) => { if (requestID === summaryRequest.current) setSummary(next); })
      .catch((err: unknown) => { if (requestID === summaryRequest.current) setError(err instanceof Error ? err.message : '加载网站访问统计失败'); })
      .finally(() => { if (requestID === summaryRequest.current) setLoadingSummary(false); });
  }, [params]);
  useEffect(() => {
    const requestID = ++recordsRequest.current;
    setLoadingRecords(true);
    void api.get<WebsiteAuditRecordsResponse>(`/ovpn/web-audit/records?${params.toString()}&offset=${page * pageSize}&limit=${pageSize}`)
      .then((next) => { if (requestID === recordsRequest.current) setRecords(next); })
      .catch((err: unknown) => { if (requestID === recordsRequest.current) setError(err instanceof Error ? err.message : '加载网站访问明细失败'); })
      .finally(() => { if (requestID === recordsRequest.current) setLoadingRecords(false); });
  }, [page, params]);

  const exportRecords = () => window.open(`/ovpn/web-audit/export?${params.toString()}`, '_blank', 'noopener,noreferrer');
  const totalPages = Math.max(1, Math.ceil((records?.total || 0) / pageSize));
  const maxTrend = Math.max(...(summary?.trend || []).map((item) => item.queries), 1);

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Privacy-preserving telemetry"
        title="访问域名审计"
        description="记录 VPN 隧道中可观测的域名元数据；不会解密 HTTPS，不记录 URL、网页内容、Cookie 或凭据。"
      >
        <Button variant="outline" onClick={refreshAll} disabled={loadingStatus || loadingSummary}>
          <RefreshCw className={loadingStatus || loadingSummary ? 'animate-spin' : ''} /> 刷新
        </Button>
        <Button variant="outline" onClick={exportRecords}><Download /> 导出当前筛选</Button>
      </PageHeader>

      <Card className="border-amber-500/25 bg-amber-500/[0.04]">
        <CardContent className="flex gap-3 p-4 text-sm text-muted-foreground">
          <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" />
          <div>
            <p className="font-medium text-foreground">覆盖范围与隐私边界</p>
            <p className="mt-1">{status?.coverageNote || '正在读取审计覆盖状态。'} 不会通过 HTTPS MITM 获取完整网页浏览内容。</p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle className="text-base">筛选条件</CardTitle><CardDescription>按当前账号的数据权限筛选已归属 VPN 用户的普通 DNS 域名事件。</CardDescription></CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-[180px_1fr_1fr_auto]">
          <select className="h-10 rounded-md border border-input bg-background px-3 text-sm" value={draftRange} onChange={(event) => setDraftRange(event.target.value as RangeName)}>
            {ranges.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
          <Select value={draftUsername || '__all__'} onValueChange={(value) => setDraftUsername(value === '__all__' ? '' : value)}>
            <SelectTrigger aria-label="VPN 用户">
              <SelectValue placeholder="VPN 用户名（可选）" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">全部 VPN 用户</SelectItem>
              {auditUsers.map((user) => <SelectItem key={user.id} value={user.username}>{user.username}</SelectItem>)}
            </SelectContent>
          </Select>
          <Input value={draftDomain} onChange={(event) => setDraftDomain(event.target.value)} onKeyDown={(event) => event.key === 'Enter' && applyFilters()} placeholder="域名关键字，例如 youtube.com" />
          <Button onClick={applyFilters}><Search /> 应用筛选</Button>
        </CardContent>
      </Card>

      {error && <Card className="border-destructive/30"><CardContent className="flex gap-2 p-4 text-sm text-destructive"><AlertTriangle className="h-4 w-4 shrink-0" />{error}</CardContent></Card>}

      <div className="grid gap-4 md:grid-cols-3">
        <Metric icon={<Activity className="h-5 w-5" />} label="DNS 域名事件" value={formatCount(summary?.totalQueries)} hint="当前筛选时间范围" />
        <Metric icon={<Users className="h-5 w-5" />} label="活跃用户" value={formatCount(summary?.activeUsers)} hint="仅统计可归属的 VPN 用户" />
        <Metric icon={<Globe2 className="h-5 w-5" />} label="唯一域名" value={formatCount(summary?.uniqueDomains)} hint="事件来源：普通 DNS" />
      </div>

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Activity className="h-4 w-4 text-[var(--accent)]" />查询趋势</CardTitle><CardDescription>按数据库时间桶聚合；无查询的时间段显示为零。</CardDescription></CardHeader>
        <CardContent><div className="flex h-44 items-end gap-1.5">{summary?.trend.map((item) => <div key={item.time} className="group relative flex h-full min-w-0 flex-1 items-end"><div className="w-full rounded-t bg-[var(--accent)]/75 transition-all group-hover:bg-[var(--accent)]" style={{ height: `${Math.max(item.queries ? 5 : 0, item.queries / maxTrend * 100)}%` }} /><span className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-2 hidden -translate-x-1/2 whitespace-nowrap rounded bg-foreground px-2 py-1 text-xs text-background group-hover:block">{formatDateTime(new Date(item.time * 1000))} · {formatCount(item.queries)} 次</span></div>)}</div></CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-3">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Network className="h-4 w-4 text-[var(--accent)]" />运行与策略状态</CardTitle></CardHeader>
          <CardContent className="space-y-3 text-sm">
            <StatusRow label="DNS 监听绑定" value={status?.ingressRestricted ? '5353 已绑定 tun0' : '未就绪，服务保持故障开放'} ready={Boolean(status?.ingressRestricted)} />
            <StatusRow label="IPv4 DNS 截获" value={status?.ipv4RedirectInstalled ? (status.strictDnsCaptureEnabled ? '严格：tun0 全部 UDP/TCP 53 → 5353' : '下发 DNS → 5353') : '未安装'} ready={Boolean(status?.ipv4RedirectInstalled)} />
            <StatusRow label="IPv6 DNS 截获" value={status?.ipv6RedirectInstalled ? (status.strictDnsCaptureEnabled ? '严格 DNS 已就绪' : '下发 DNS 已就绪') : '未就绪，IPv6 DNS 不审计'} ready={Boolean(status?.ipv6RedirectInstalled)} />
            <StatusRow label="DoT 阻断" value={status?.dotBlockEnabled ? (status.ipv4DotBlockInstalled ? 'tun0 TCP/853 已阻断' : '已请求但 IPv4 规则未就绪') : '未启用（可经 DoT 绕过）'} ready={Boolean(status?.dotBlockEnabled && status?.ipv4DotBlockInstalled)} />
            <StatusRow label="HTTP/3 回退策略" value={status?.udp443BlockEnabled ? (status.ipv4Udp443BlockInstalled ? 'tun0 UDP/443 已阻断，浏览器通常回退 TCP' : '已请求但 IPv4 规则未就绪') : '未启用（QUIC/DoH/缓存仍可能漏记）'} ready={Boolean(status?.udp443BlockEnabled && status?.ipv4Udp443BlockInstalled)} />
            <StatusRow label="上游 DNS" value={status?.upstreamDns?.join(' · ') || '未配置'} ready={Boolean(status?.upstreamDns?.length)} />
            {Boolean(status?.droppedAuditEvents || status?.droppedDnsRequests) && <StatusRow label="丢弃事件 / 请求" value={`${formatCount(status?.droppedAuditEvents)} / ${formatCount(status?.droppedDnsRequests)}`} ready={false} />}
          </CardContent>
        </Card>
        <TopList title="访问最多的域名" data={summary?.topDomains || []} kind="domain" />
        <TopList title="查询最多的用户" data={summary?.topUsers || []} kind="user" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <DiagnosticList title="当前可能漏报的原因" icon={<AlertTriangle className="h-4 w-4 text-amber-500" />} items={status?.detectedGaps || []} empty="当前未返回诊断信息。" />
        <DiagnosticList title="建议操作" icon={<ShieldCheck className="h-4 w-4 text-emerald-500" />} items={status?.recommendedActions || []} empty="当前没有额外建议。" />
      </div>

      {status?.lastError && <Card className="border-destructive/30"><CardHeader><CardTitle className="text-base text-destructive">最后错误</CardTitle><CardDescription>审计错误会撤销本功能的重定向或阻断规则，不会用于阻断 OpenVPN 基本连接。</CardDescription></CardHeader><CardContent><code className="block break-all rounded bg-muted p-3 text-xs">{status.lastError}</code></CardContent></Card>}

      <Card>
        <CardHeader><CardTitle>DNS 域名事件明细</CardTitle><CardDescription>仅显示当前账号有权查看的数据。事件来源为普通 DNS，不代表完整网页、URL 或页面内容。</CardDescription></CardHeader>
        <CardContent className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-sm"><thead className="border-b text-left text-xs text-muted-foreground"><tr><th className="pb-3 font-medium">查询时间</th><th className="pb-3 font-medium">用户</th><th className="pb-3 font-medium">VPN IP</th><th className="pb-3 font-medium">域名</th><th className="pb-3 font-medium">类型</th><th className="pb-3 font-medium">响应</th></tr></thead><tbody>{records?.data.map((item) => <RecordRow key={item.id} item={item} />)}</tbody></table>
          {!loadingRecords && !records?.data.length && <div className="py-12 text-center text-muted-foreground">当前筛选条件下还没有普通 DNS 域名事件。</div>}
          <div className="mt-4 flex items-center justify-between border-t pt-4 text-sm text-muted-foreground"><span>共 {formatCount(records?.total)} 条</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 0 || loadingRecords} onClick={() => setPage((current) => current - 1)}>上一页</Button><span>{page + 1} / {totalPages}</span><Button size="sm" variant="outline" disabled={page + 1 >= totalPages || loadingRecords} onClick={() => setPage((current) => current + 1)}>下一页</Button></div></div>
        </CardContent>
      </Card>
    </div>
  );
}

function Metric({ icon, label, value, hint }: { icon: ReactNode; label: string; value: string; hint: string }) {
  return <Card interactive><CardContent className="flex items-center gap-4 p-5"><div className="rounded-xl bg-[var(--accent)]/12 p-3 text-[var(--accent)]">{icon}</div><div><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold tracking-tight">{value}</p><p className="mt-1 text-xs text-muted-foreground">{hint}</p></div></CardContent></Card>;
}
function StatusRow({ label, value, ready }: { label: string; value: string; ready?: boolean }) {
  return <div className="flex items-start justify-between gap-3"><div><p className="text-muted-foreground">{label}</p><p className="mt-0.5 break-all font-medium">{value}</p></div><span className={`mt-1 h-2.5 w-2.5 shrink-0 rounded-full ${ready ? 'bg-emerald-500 shadow-[0_0_9px_rgba(16,185,129,.8)]' : 'bg-amber-500'}`} /></div>;
}
function DiagnosticList({ title, icon, items, empty }: { title: string; icon: ReactNode; items: string[]; empty: string }) {
  return <Card><CardHeader><CardTitle className="flex items-center gap-2 text-base">{icon}{title}</CardTitle></CardHeader><CardContent><ul className="space-y-2 text-sm text-muted-foreground">{items.length ? items.map((item) => <li key={item} className="flex gap-2"><span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-current" />{item}</li>) : <li>{empty}</li>}</ul></CardContent></Card>;
}
function TopList({ title, data, kind }: { title: string; data: NonNullable<WebsiteAuditSummary['topDomains']>; kind: 'domain' | 'user' }) {
  const max = Math.max(...data.map((item) => item.queries), 1);
  return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle></CardHeader><CardContent className="space-y-3">{data.length ? data.map((item, index) => <div key={`${item.domain}-${item.username}-${index}`}><div className="mb-1 flex justify-between gap-3 text-sm"><span className="truncate font-medium">{kind === 'domain' ? item.domain : (item.username || item.commonName || '未知用户')}</span><span className="shrink-0 text-muted-foreground">{formatCount(item.queries)} 次</span></div><div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-[var(--accent)]" style={{ width: `${item.queries / max * 100}%` }} /></div></div>) : <p className="py-4 text-sm text-muted-foreground">暂无数据</p>}</CardContent></Card>;
}
function RecordRow({ item }: { item: WebsiteAccessRecord }) {
  const success = item.responseCode === 'RCodeSuccess' || item.responseCode === 'NOERROR';
  return <tr className="border-b border-border/40 last:border-0 hover:bg-muted/25"><td className="py-3 text-muted-foreground">{formatDateTime(new Date(item.queriedAt * 1000))}</td><td className="py-3"><div className="font-medium">{item.username || '-'}</div>{item.commonName && <div className="text-xs text-muted-foreground">{item.commonName}</div>}</td><td className="py-3 font-mono text-xs">{item.vpnIp || '-'}</td><td className="py-3 font-medium">{item.domain}</td><td className="py-3"><Badge variant="outline">{item.queryType}</Badge></td><td className="py-3"><Badge className={success ? 'bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15' : 'bg-amber-500/10 text-amber-600 hover:bg-amber-500/15'}>{item.responseCode}</Badge></td></tr>;
}
