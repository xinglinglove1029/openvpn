import { useEffect, useMemo, useState } from 'react';
import { RefreshCw, Search, ShieldCheck, RotateCw, CalendarClock, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { messageOf, normalizeList } from '@/lib/format';
import { isPositiveInteger } from '@/lib/validators';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { DataTable, type Column } from '@/components/DataTable';
import { HasPermission } from '@/components/HasPermission';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Label } from '@/ui/label';
import { Checkbox } from '@/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/ui/alert-dialog';
import type { CertRecord, SettingsResponse } from '../../types';

function certStatus(status?: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (status === '已过期') return 'danger';
  if (status === '即将过期') return 'warning';
  if (status === '正常' || status === 'Valid') return 'success';
  return 'neutral';
}

export default function CertsPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [selectedCerts, setSelectedCerts] = useState<Set<string>>(new Set());
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteTargets, setDeleteTargets] = useState<string[]>([]);

  // 单证书续签弹窗
  const [rowRenewOpen, setRowRenewOpen] = useState(false);
  const [rowRenewing, setRowRenewing] = useState(false);
  const [rowRenewTarget, setRowRenewTarget] = useState<CertRecord | null>(null);
  const [rowRenewDays, setRowRenewDays] = useState<string>('365');

  // 搜索与筛选
  const [searchText, setSearchText] = useState('');
  const [filterStatus, setFilterStatus] = useState<'all' | 'normal' | 'expiring' | 'expired'>('all');

  const state = useAsync(
    () => api.get<unknown>('/ovpn/certs').then((v) => normalizeList<CertRecord>(v, ['certs', 'data'])),
    [reloadKey],
  );

  const settingsState = useAsync(
    () => api.get<SettingsResponse>('/ovpn/settings'),
    [],
  );

  // 用系统配置的默认续签天数初始化弹窗
  useEffect(() => {
    if (settingsState.data?.system?.base?.renew_days && settingsState.data.system.base.renew_days > 0) {
      const defaultDays = String(settingsState.data.system.base.renew_days);
      setRowRenewDays((prev) => (prev === '365' ? defaultDays : prev));
    }
  }, [settingsState.data]);

  useEffect(() => {
    if (state.error) {
      toast.error(`加载证书失败：${messageOf(state.error)}`);
    }
  }, [state.error]);

  // 搜索过滤：按名称、类型匹配；按状态筛选
  const filteredCerts = useMemo(() => {
    const kw = searchText.trim().toLowerCase();
    return (state.data ?? []).filter((cert) => {
      // 关键词过滤
      if (kw) {
        const hit =
          (cert.name ?? '').toLowerCase().includes(kw) ||
          (cert.type ?? '').toLowerCase().includes(kw);
        if (!hit) return false;
      }
      // 状态过滤
      const s = certStatus(cert.status);
      if (filterStatus === 'normal' && s !== 'success') return false;
      if (filterStatus === 'expiring' && s !== 'warning') return false;
      if (filterStatus === 'expired' && s !== 'danger') return false;
      return true;
    });
  }, [state.data, searchText, filterStatus]);

  const certs = filteredCerts;
  const pagination = usePagination(certs, `certs-${reloadKey}-${searchText}-${filterStatus}`);

  // A selection must never survive a search/status scope change. Keeping hidden names
  // would make the destructive bulk action include certificates the operator cannot see.
  useEffect(() => {
    setSelectedCerts(new Set());
  }, [searchText, filterStatus]);

  // 表格勾选仅用于安全批量删除客户端证书。
  const deletableCerts = useMemo(
    () => certs.filter((cert) => cert.deletable === true && Boolean(cert.name)),
    [certs],
  );
  const allDeleteSelected =
    deletableCerts.length > 0 && deletableCerts.every((cert) => selectedCerts.has(cert.name ?? ''));
  function toggleCert(name: string) {
    setSelectedCerts((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  function toggleAllCerts() {
    if (allDeleteSelected) {
      setSelectedCerts(new Set());
    } else {
      setSelectedCerts(new Set(deletableCerts.map((cert) => cert.name ?? '').filter(Boolean)));
    }
  }
  function openDeleteDialog(names: string[]) {
    const byName = new Map(certs.map((cert) => [cert.name ?? '', cert]));
    const targets = Array.from(new Set(names.filter(Boolean))).filter((name) => byName.get(name)?.deletable === true);
    if (targets.length === 0) {
      toast.error('没有可删除的客户端证书；CA、服务端证书和 CRL 已受保护。');
      return;
    }
    setDeleteTargets(targets);
    setDeleteOpen(true);
  }

  async function handleDelete() {
    if (deleteTargets.length === 0) return;
    setDeleting(true);
    try {
      const response = await api.deleteJson<{ results?: Array<{ name: string; success: boolean; message: string }>; successCount?: number; total?: number }>(
        '/ovpn/certs',
        { names: deleteTargets },
      );
      const results = response.results ?? [];
      const failed = results.filter((result) => !result.success);
      const successCount = response.successCount ?? results.length - failed.length;
      if (successCount > 0) toast.success(`已清理 ${successCount} 个客户端证书`);
      if (failed.length > 0) toast.warning(`部分清理失败：${failed.map((result) => `${result.name}: ${result.message}`).join('；')}`);
      setSelectedCerts((prev) => {
        const next = new Set(prev);
        deleteTargets.forEach((name) => next.delete(name));
        return next;
      });
      setReloadKey((value) => value + 1);
      setDeleteOpen(false);
      setDeleteTargets([]);
    } catch (error) {
      toast.error(`删除证书失败：${messageOf(error)}`);
      // Refresh after a rejected request: server-side revocation may already be durable
      // while the running daemon still needs an operator to restore CRL reload.
      setReloadKey((value) => value + 1);
    } finally {
      setDeleting(false);
    }
  }

  function openRowRenew(cert: CertRecord) {
    setRowRenewTarget(cert);
    if (settingsState.data?.system?.base?.renew_days && settingsState.data.system.base.renew_days > 0) {
      setRowRenewDays(String(settingsState.data.system.base.renew_days));
    } else if (!isPositiveInteger(rowRenewDays)) {
      setRowRenewDays('365');
    }
    setRowRenewOpen(true);
  }

  async function handleRowRenew() {
    if (!rowRenewTarget) return;
    if (!isPositiveInteger(rowRenewDays)) {
      toast.error('续签天数必须是大于 0 的整数');
      return;
    }
    setRowRenewing(true);
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', {
        action: 'renewCertByName',
        name: rowRenewTarget.name,
        day: rowRenewDays,
      });
      toast.success(result.message || '续签成功');
      setReloadKey((v) => v + 1);
    } catch (error) {
      toast.error(`续签失败：${messageOf(error)}`);
    } finally {
      setRowRenewing(false);
      setRowRenewOpen(false);
      setRowRenewTarget(null);
    }
  }

  const columns: Column<CertRecord>[] = useMemo(
    () => [
      {
        key: 'select',
        header: (
          <Checkbox
            checked={allDeleteSelected ? true : selectedCerts.size > 0 ? 'indeterminate' : false}
            onCheckedChange={toggleAllCerts}
            aria-label="全选可删除客户端证书"
          />
        ),
        mobileHeader: null,
        render: (cert) => {
          const name = cert.name ?? '';
          return (
            <Checkbox
              checked={selectedCerts.has(name)}
              onCheckedChange={() => toggleCert(name)}
              disabled={!cert.deletable}
              aria-label={cert.deletable ? `选择 ${name}` : cert.protectedReason || `${name} 不可删除`}
              title={cert.protectedReason}
            />
          );
        },
        mobileRender: (cert) => {
          const name = cert.name ?? '';
          return (
            <Checkbox
              checked={selectedCerts.has(name)}
              onCheckedChange={() => toggleCert(name)}
              disabled={!cert.deletable}
              aria-label={cert.deletable ? `选择 ${name}` : cert.protectedReason || `${name} 不可删除`}
              title={cert.protectedReason}
            />
          );
        },
        sortable: false,
        cardPlacement: 'header-action',
        className: 'w-10 text-center',
      },
      {
        key: 'name',
        header: '名称',
        sortable: true,
        sortAccessor: (cert) => cert.name ?? '',
        cardPlacement: 'header-left',
        render: (cert) => cert.name || '-',
      },
      {
        key: 'type',
        header: '类型',
        sortable: true,
        sortAccessor: (cert) => cert.type ?? '',
        render: (cert) => cert.type || '-',
      },
      {
        key: 'status',
        header: '状态',
        sortable: true,
        cardPlacement: 'header-right',
        sortAccessor: (cert) => certStatus(cert.status),
        render: (cert) => (
          <StatusBadge status={certStatus(cert.status)}>
            {cert.status || '-'}
          </StatusBadge>
        ),
      },
      {
        key: 'notBefore',
        header: '颁发时间',
        sortable: true,
        sortAccessor: (cert) => {
          if (!cert.notBefore) return 0;
          const ts = new Date(cert.notBefore).getTime();
          return Number.isNaN(ts) ? 0 : ts;
        },
        render: (cert) => cert.notBefore || '-',
      },
      {
        key: 'notAfter',
        header: '过期时间',
        sortable: true,
        sortAccessor: (cert) => {
          if (!cert.notAfter) return 0;
          const ts = new Date(cert.notAfter).getTime();
          return Number.isNaN(ts) ? 0 : ts;
        },
        render: (cert) => cert.notAfter || '-',
      },
      {
        key: 'expiresIn',
        header: '剩余天数',
        sortable: true,
        sortAccessor: (cert) => {
          if (!cert.notAfter) return -Infinity;
          const diff = new Date(cert.notAfter).getTime() - Date.now();
          return Math.floor(diff / (1000 * 60 * 60 * 24));
        },
        render: (cert) => cert.expiresIn || '-',
      },
      {
        key: 'actions',
        header: '操作',
        mobileHeader: '',
        mobileClassName: 'grid-cols-1',
        render: (cert) => (
          <div className="flex items-center gap-1">
            <HasPermission code="cert:delete">
              {cert.deletable ? (
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-7 px-2 text-destructive hover:text-destructive"
                  onClick={() => openDeleteDialog([cert.name ?? ''])}
                  title="删除客户端证书"
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  删除
                </Button>
              ) : (
                <span className="text-xs text-muted-foreground" title={cert.protectedReason}>受保护</span>
              )}
            </HasPermission>
            <HasPermission code="cert:renew">
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2"
                onClick={() => openRowRenew(cert)}
                title="续签证书"
              >
                <RotateCw className="h-3.5 w-3.5 mr-1" />
                续签
              </Button>
            </HasPermission>
          </div>
        ),
        mobileRender: (cert) => (
          <div className="grid grid-cols-1 gap-2">
            <HasPermission code="cert:delete">
              {cert.deletable ? (
                <Button
                  size="sm"
                  variant="outline"
                  className="h-9 w-full border-destructive/40 text-destructive hover:text-destructive"
                  onClick={() => openDeleteDialog([cert.name ?? ''])}
                >
                  <Trash2 className="mr-1 h-4 w-4" />
                  删除 ({cert.name || '-'})
                </Button>
              ) : (
                <p className="text-xs text-muted-foreground">{cert.protectedReason || '系统证书不可删除'}</p>
              )}
            </HasPermission>
            <HasPermission code="cert:renew">
              <Button size="sm" variant="outline" className="h-9 w-full" onClick={() => openRowRenew(cert)}>
                <RotateCw className="mr-1 h-4 w-4" />
                续签 ({cert.name || '-'})
              </Button>
            </HasPermission>
          </div>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [settingsState.data, selectedCerts, allDeleteSelected],
  );

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Trust" title="证书管理" description="管理 CA 证书与客户端证书生命周期" />

      {/* 操作工具栏：搜索、筛选 在左，刷新、更新证书 在右 */}
      <div className="flex flex-col sm:flex-row flex-wrap items-stretch sm:items-center gap-3">
        <div className="relative w-full sm:w-48">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            placeholder="搜索名称、类型"
            className="pl-9 h-11 sm:h-8"
          />
        </div>
        <Select value={filterStatus} onValueChange={(v) => setFilterStatus(v as 'all' | 'normal' | 'expiring' | 'expired')}>
          <SelectTrigger className="w-full sm:w-[110px] h-11 sm:h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="normal">正常</SelectItem>
            <SelectItem value="expiring">即将过期</SelectItem>
            <SelectItem value="expired">已过期</SelectItem>
          </SelectContent>
        </Select>
        <div className="grid grid-cols-2 sm:flex sm:flex-nowrap items-center gap-2 w-full sm:w-auto sm:ml-auto">
          <HasPermission code="cert:view">
            <Button size="sm" variant="outline" onClick={() => setReloadKey((v) => v + 1)} className="w-full sm:w-auto">
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新
            </Button>
          </HasPermission>
          <HasPermission code="cert:delete">
            <Button size="sm" variant="destructive" onClick={() => openDeleteDialog(Array.from(selectedCerts))} className="w-full sm:w-auto" disabled={selectedCerts.size === 0}>
              <Trash2 className="h-3.5 w-3.5 mr-1" />
              删除证书
              {selectedCerts.size > 0 && <span className="ml-1 text-[10px]">{selectedCerts.size}</span>}
            </Button>
          </HasPermission>
        </div>
      </div>

      {/* 加载中 */}
      {state.loading && (
        <p className="text-sm text-muted-foreground text-center py-8">
          加载中...
        </p>
      )}

      {/* 表格 */}
      {!state.loading && certs.length > 0 && (
        <DataTable
          columns={columns}
          data={pagination.pagedItems}
          fullData={certs}
          page={pagination.page}
          pageSize={pagination.pageSize}
          pageCount={pagination.pageCount}
          total={pagination.total}
          start={pagination.start}
          end={pagination.end}
          onPageChange={pagination.setPage}
          onPageSizeChange={pagination.setPageSize}
          keyFn={(cert, index) => `${cert.name}-${pagination.start + index}`}
          isCardSelected={(cert) => selectedCerts.has(cert.name ?? '')}
        />
      )}

      {/* 空状态 */}
      {!state.loading && certs.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground font-medium">暂无证书信息</p>
          <p className="text-sm text-muted-foreground mt-1">
            {searchText || filterStatus !== 'all'
              ? '没有匹配的证书，请调整搜索条件'
              : 'Docker 完整环境会挂载 EasyRSA 数据并展示证书状态。'}
          </p>
        </div>
      )}
      {/* 批量删除客户端证书确认 */}
      <AlertDialog open={deleteOpen} onOpenChange={(open) => !deleting && setDeleteOpen(open)}>
        <AlertDialogContent className="sm:max-w-xl">
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2 text-destructive"><Trash2 className="h-5 w-5" />删除客户端证书</AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <p>活跃客户端证书会先吊销并刷新 CRL，再删除对应的私钥、.ovpn、CCD 和请求文件。CA、OpenVPN Server 和 CRL 受保护，不能删除。</p>
              <div className="max-h-[200px] overflow-y-auto rounded-md border bg-muted/20 px-3 py-2 space-y-1">
                {deleteTargets.map((name) => <div key={name} className="font-medium break-all text-sm">{name}</div>)}
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} disabled={deleting || deleteTargets.length === 0} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              {deleting ? '删除中...' : `确认删除 (${deleteTargets.length})`}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={rowRenewOpen} onOpenChange={(open) => !rowRenewing && setRowRenewOpen(open)}>
        <AlertDialogContent className="sm:max-w-xl">
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2">
              <RotateCw className="h-5 w-5 text-[var(--accent)]" />
              续签证书
            </AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <div className="grid grid-cols-[minmax(5rem,32%)_1fr] gap-2 rounded-md border bg-muted/20 px-3 py-2 text-sm">
                <div className="text-muted-foreground">证书名称</div>
                <div className="font-medium break-all">{rowRenewTarget?.name || '-'}</div>
                <div className="text-muted-foreground">类型</div>
                <div>{rowRenewTarget?.type || '-'}</div>
                <div className="text-muted-foreground">过期时间</div>
                <div>{rowRenewTarget?.notAfter || '-'}</div>
              </div>
              <p className="text-xs text-muted-foreground">
                续签后该证书将从当前时间起重新计算有效期；内置 CA 续签会自动同步服务器证书和 CRL。
              </p>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid grid-cols-1 sm:grid-cols-[140px_1fr] items-start gap-4 py-2">
            <Label htmlFor="row-renew-days" className="flex items-center gap-1.5 pt-0 sm:justify-end sm:pt-2 text-left sm:text-right text-sm font-medium text-foreground/80">
              <CalendarClock className="h-3.5 w-3.5 text-muted-foreground" />
              续签天数
            </Label>
            <div className="min-w-0 space-y-2">
              <Input
                id="row-renew-days"
                type="number"
                min={1}
                inputMode="numeric"
                value={rowRenewDays}
                onChange={(e) => setRowRenewDays(e.target.value)}
                placeholder="必须是大于 0 的整数，例如 365"
              />
              {rowRenewDays && !isPositiveInteger(rowRenewDays) && (
                <p className="text-xs text-destructive">续签天数必须是大于 0 的整数</p>
              )}
            </div>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={rowRenewing}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleRowRenew} disabled={rowRenewing || !isPositiveInteger(rowRenewDays) || !rowRenewTarget}>
              {rowRenewing ? '续签中...' : '确认续签'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
