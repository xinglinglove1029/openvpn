import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Activity, AlertTriangle, Download, Globe2, Network, RefreshCw, Search, ShieldCheck, Users } from 'lucide-react';
import { api } from '../../api';
import { PageHeader } from '@/components/PageHeader';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/ui/card';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Badge } from '@/ui/badge';
import { formatDateTime } from '@/lib/utils';
import type { WebsiteAccessRecord, WebsiteAuditRecordsResponse, WebsiteAuditStatus, WebsiteAuditSummary } from '../../types';

const ranges = [
  ['1h', '近 1 小时'], ['6h', '近 6 小时'], ['24h', '近 24 小时'], ['7d', '近 7 天'], ['30d', '近 30 天'],
] as const;
type RangeName = typeof ranges[number][0];
type AppliedQuery = { range: RangeName; username: string; domain: string; end: number };

const pageSize = 20;
function rangeSeconds(range: RangeName) { return ({ '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800, '30d': 2592000 } as Record<RangeName, number>)[range]; }
function formatCount(value?: number) { return new Intl.NumberFormat('zh-CN').format(value || 0); }
function statusTone(ok: boolean) { return ok ? 'text-emerald-500 bg-emerald-500/10 border-emerald-500/25' : 'text-amber-500 bg-amber-500/10 border-amber-500/25'; }
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
      if (requestID === statusRequest.current) setError(err instanceof Error ? err.message : '读取 DNS 审计状态失败');
    } finally {
      if (requestID === statusRequest.current) setLoadingStatus(false);
    }
  }, []);
  const refreshAll = useCallback(() => {
    setError('');
    setPage(0);
    setQuery({ range: draftRange, username: draftUsername.trim(), domain: draftDomain.trim(), end: Math.floor(Date.now() / 1000) });
    void refreshStatus();
  }, [draftDomain, draftRange, draftUsername, refreshStatus]);

  useEffect(() => { void refreshStatus(); }, [refreshStatus]);
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

  const exportCsv = () => window.open(`/ovpn/web-audit/export?${params.toString()}`, '_blank', 'noopener,noreferrer');
  const maxTrend = Math.max(...(summary?.trend.map((item) => item.queries) || [1]), 1);
  const totalPages = Math.max(1, Math.ceil((records?.total || 0) / pageSize));
  const loading = loadingSummary || loadingRecords || loadingStatus;

  return <div className="space-y-6">
    <PageHeader eyebrow="Privacy-preserving telemetry" title="网站访问审计" description="按 VPN 用户统计普通 DNS 域名查询；不解密 HTTPS，也不记录 URL、网页内容或凭据。">
      <Button variant="outline" onClick={refreshAll} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} /> 刷新</Button>
      <Button onClick={exportCsv}><Download /> 导出 CSV</Button>
    </PageHeader>

    <Card className="overflow-hidden border-[color-mix(in_srgb,var(--accent)_30%,transparent)]">
      <CardContent className="p-0">
        <div className="flex flex-col gap-4 bg-gradient-to-r from-[color-mix(in_srgb,var(--accent)_13%,transparent)] to-transparent p-5 lg:flex-row lg:items-end">
          <div className="min-w-0 flex-1"><div className="mb-2 flex items-center gap-2 font-medium"><ShieldCheck className="h-5 w-5 text-[var(--accent)]" />透明且有边界的审计</div><p className="text-sm leading-6 text-muted-foreground">{status?.coverageNote || '正在读取审计覆盖范围…'}</p></div>
          <div className={`rounded-full border px-3 py-1.5 text-sm font-medium ${statusTone(Boolean(status?.enabled && status?.listenerReady && status?.redirectInstalled))}`}>{status?.enabled ? (status?.redirectInstalled ? 'DNS 审计运行中' : '审计已启用，等待安全就绪') : 'DNS 审计已关闭'}</div>
        </div>
        {status?.lastError && <div className="flex gap-2 border-t border-amber-500/20 bg-amber-500/5 px-5 py-3 text-sm text-amber-700 dark:text-amber-400"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" /><span>{status.lastError}</span></div>}
      </CardContent>
    </Card>

    <Card><CardContent className="grid gap-3 p-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:grid-cols-[auto_minmax(0,1fr)_minmax(0,1fr)_auto]">
      <div className="flex flex-wrap gap-2">{ranges.map(([value, label]) => <Button key={value} size="sm" variant={draftRange === value ? 'default' : 'outline'} onClick={() => setDraftRange(value)}>{label}</Button>)}</div>
      <Input value={draftUsername} onChange={(event) => setDraftUsername(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') applyFilters(); }} placeholder="按用户名筛选" />
      <Input value={draftDomain} onChange={(event) => setDraftDomain(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') applyFilters(); }} placeholder="按域名筛选（支持文字）" />
      <Button onClick={applyFilters}><Search /> 应用筛选</Button>
    </CardContent></Card>

    {error && <Card className="border-destructive/30"><CardContent className="p-4 text-sm text-destructive">{error}</CardContent></Card>}
    <div className="grid gap-4 md:grid-cols-3"><Metric icon={<Activity className="h-5 w-5"/>} label="DNS 查询" value={formatCount(summary?.totalQueries)} hint="当前筛选时间范围"/><Metric icon={<Users className="h-5 w-5"/>} label="活跃用户" value={formatCount(summary?.activeUsers)} hint="仅统计已归属 VPN 用户"/><Metric icon={<Globe2 className="h-5 w-5"/>} label="唯一域名" value={formatCount(summary?.uniqueDomains)} hint="普通 DNS 查询域名"/></div>

    <Card><CardHeader><CardTitle className="flex items-center gap-2"><Activity className="h-4 w-4 text-[var(--accent)]"/>查询趋势</CardTitle><CardDescription>趋势由数据库按时间桶聚合；没有查询的时间段会显示为零。</CardDescription></CardHeader><CardContent><div className="flex h-44 items-end gap-1.5">{summary?.trend.map((item) => <div key={item.time} className="group relative flex h-full min-w-0 flex-1 items-end"><div className="w-full rounded-t bg-[var(--accent)]/75 transition-all group-hover:bg-[var(--accent)]" style={{height:`${Math.max(item.queries ? 5 : 0, item.queries / maxTrend * 100)}%`}}/><span className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-2 hidden -translate-x-1/2 whitespace-nowrap rounded bg-foreground px-2 py-1 text-xs text-background group-hover:block">{formatDateTime(new Date(item.time * 1000))} · {formatCount(item.queries)} 次</span></div>)}</div></CardContent></Card>

    <div className="grid gap-4 xl:grid-cols-3"><Card><CardHeader><CardTitle className="flex items-center gap-2 text-base"><Network className="h-4 w-4 text-[var(--accent)]"/>运行状态</CardTitle></CardHeader><CardContent className="space-y-3 text-sm"><StatusRow label="IPv4 UDP/TCP 监听" value={status?.listenAddress || '127.0.0.1:5353'} ready={status?.ipv4ListenerReady ?? status?.listenerReady}/><StatusRow label="IPv4 隧道 DNS 重定向" value={(status?.ipv4RedirectInstalled ?? status?.redirectInstalled) ? 'tun0 UDP/TCP 53 → 5353' : '未安装'} ready={status?.ipv4RedirectInstalled ?? status?.redirectInstalled}/><StatusRow label="IPv6 DNS 覆盖" value={status?.ipv6RedirectInstalled ? '::1 UDP/TCP 监听及 ip6tables 已就绪' : '未就绪，IPv6 普通 DNS 不审计'} ready={status?.ipv6RedirectInstalled}/><StatusRow label="上游 DNS" value={status?.upstreamDns?.join(' · ') || '未配置'} ready={Boolean(status?.upstreamDns?.length)}/>{Boolean(status?.droppedAuditEvents) && <StatusRow label="过载丢弃的审计事件" value={formatCount(status?.droppedAuditEvents)} ready={false}/>}</CardContent></Card><TopList title="访问最多的域名" data={summary?.topDomains || []} kind="domain"/><TopList title="查询最多的用户" data={summary?.topUsers || []} kind="user"/></div>

    <Card><CardHeader><CardTitle>DNS 查询明细</CardTitle><CardDescription>仅显示当前账号有权查看的用户数据；分页只刷新明细，不会重复拉取统计或状态。</CardDescription></CardHeader><CardContent className="overflow-x-auto"><table className="w-full min-w-[760px] text-sm"><thead className="border-b text-left text-xs text-muted-foreground"><tr><th className="pb-3 font-medium">查询时间</th><th className="pb-3 font-medium">用户</th><th className="pb-3 font-medium">VPN IP</th><th className="pb-3 font-medium">域名</th><th className="pb-3 font-medium">类型</th><th className="pb-3 font-medium">响应</th></tr></thead><tbody>{records?.data.map((item) => <RecordRow key={item.id} item={item}/>)}</tbody></table>{!loadingRecords && !records?.data.length && <div className="py-12 text-center text-muted-foreground">当前筛选条件下还没有普通 DNS 审计记录。</div>}<div className="mt-4 flex items-center justify-between border-t pt-4 text-sm text-muted-foreground"><span>共 {formatCount(records?.total)} 条</span><div className="flex items-center gap-2"><Button size="sm" variant="outline" disabled={page <= 0 || loadingRecords} onClick={() => setPage((current) => current - 1)}>上一页</Button><span>{page + 1} / {totalPages}</span><Button size="sm" variant="outline" disabled={page + 1 >= totalPages || loadingRecords} onClick={() => setPage((current) => current + 1)}>下一页</Button></div></div></CardContent></Card>
  </div>;
}

function Metric({icon,label,value,hint}:{icon:React.ReactNode;label:string;value:string;hint:string}) { return <Card interactive><CardContent className="flex items-center gap-4 p-5"><div className="rounded-xl bg-[var(--accent)]/12 p-3 text-[var(--accent)]">{icon}</div><div><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 text-2xl font-semibold tracking-tight">{value}</p><p className="mt-1 text-xs text-muted-foreground">{hint}</p></div></CardContent></Card>; }
function StatusRow({label,value,ready}:{label:string;value:string;ready?:boolean}) { return <div className="flex items-start justify-between gap-3"><div><p className="text-muted-foreground">{label}</p><p className="mt-0.5 break-all font-medium">{value}</p></div><span className={`mt-1 h-2.5 w-2.5 shrink-0 rounded-full ${ready ? 'bg-emerald-500 shadow-[0_0_9px_rgba(16,185,129,.8)]' : 'bg-amber-500'}`}/></div>; }
function TopList({title,data,kind}:{title:string;data:NonNullable<WebsiteAuditSummary['topDomains']>;kind:'domain'|'user'}) { const max=Math.max(...data.map((item) => item.queries),1); return <Card><CardHeader><CardTitle className="text-base">{title}</CardTitle></CardHeader><CardContent className="space-y-3">{data.length ? data.map((item,index) => <div key={`${item.domain}-${item.username}-${index}`}><div className="mb-1 flex justify-between gap-3 text-sm"><span className="truncate font-medium">{kind === 'domain' ? item.domain : (item.username || item.commonName || '未知用户')}</span><span className="shrink-0 text-muted-foreground">{formatCount(item.queries)} 次</span></div><div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-[var(--accent)]" style={{width:`${item.queries/max*100}%`}}/></div></div>) : <p className="py-4 text-sm text-muted-foreground">暂无数据</p>}</CardContent></Card>; }
function RecordRow({item}:{item:WebsiteAccessRecord}) { return <tr className="border-b border-border/40 last:border-0 hover:bg-muted/25"><td className="py-3 text-muted-foreground">{formatDateTime(new Date(item.queriedAt*1000))}</td><td className="py-3"><div className="font-medium">{item.username || '-'}</div>{item.commonName && <div className="text-xs text-muted-foreground">{item.commonName}</div>}</td><td className="py-3 font-mono text-xs">{item.vpnIp || '-'}</td><td className="py-3 font-medium">{item.domain}</td><td className="py-3"><Badge variant="outline">{item.queryType}</Badge></td><td className="py-3"><Badge className={item.responseCode === 'RCodeSuccess' || item.responseCode === 'NOERROR' ? 'bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15' : 'bg-amber-500/10 text-amber-600 hover:bg-amber-500/15'}>{item.responseCode}</Badge></td></tr>; }
