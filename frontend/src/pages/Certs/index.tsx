import { useEffect, useMemo, useState } from 'react';
import { RefreshCw, Search, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { messageOf, normalizeList } from '@/lib/format';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { DataTable, type Column } from '@/components/DataTable';
import { HasPermission } from '@/components/HasPermission';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
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
import type { CertRecord } from '../../types';

function certStatus(status?: string): 'success' | 'warning' | 'danger' | 'neutral' {
  if (status === '已过期') return 'danger';
  if (status === '即将过期') return 'warning';
  if (status === '正常' || status === 'Valid') return 'success';
  return 'neutral';
}

export default function CertsPage() {
  const [reloadKey, setReloadKey] = useState(0);
  const [renewOpen, setRenewOpen] = useState(false);
  const [renewing, setRenewing] = useState(false);
  // 搜索与筛选
  const [searchText, setSearchText] = useState('');
  const [filterStatus, setFilterStatus] = useState<'all' | 'normal' | 'expiring' | 'expired'>('all');

  const state = useAsync(
    () => api.get<unknown>('/ovpn/certs').then((v) => normalizeList<CertRecord>(v, ['certs', 'data'])),
    [reloadKey],
  );

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

  async function handleRenew() {
    setRenewing(true);
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', { action: 'renewCert' });
      toast.success(result.message || '证书更新任务已触发');
      setReloadKey((v) => v + 1);
    } catch (error) {
      toast.error(`更新失败：${messageOf(error)}`);
    } finally {
      setRenewing(false);
      setRenewOpen(false);
    }
  }

  const columns: Column<CertRecord>[] = useMemo(
    () => [
      {
        key: 'name',
        header: '名称',
        sortable: true,
        sortAccessor: (cert) => cert.name ?? '',
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
        sortAccessor: (cert) => (cert.expiresIn == null ? -Infinity : Number(cert.expiresIn)),
        render: (cert) =>
          cert.expiresIn != null ? `${cert.expiresIn} 天` : '-',
      },
    ],
    [],
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
            className="pl-9 h-8"
          />
        </div>
        <Select value={filterStatus} onValueChange={(v) => setFilterStatus(v as 'all' | 'normal' | 'expiring' | 'expired')}>
          <SelectTrigger className="w-full sm:w-[110px] h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="normal">正常</SelectItem>
            <SelectItem value="expiring">即将过期</SelectItem>
            <SelectItem value="expired">已过期</SelectItem>
          </SelectContent>
        </Select>
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 sm:ml-auto">
          <HasPermission code="cert:view">
            <Button size="sm" variant="outline" onClick={() => setReloadKey((v) => v + 1)} className="w-full sm:w-auto">
              <RefreshCw className="h-3.5 w-3.5 mr-1" />
              刷新
            </Button>
          </HasPermission>
          <HasPermission code="cert:renew">
            <Button size="sm" onClick={() => setRenewOpen(true)} className="w-full sm:w-auto">
              <ShieldCheck className="h-3.5 w-3.5 mr-1" />
              更新证书
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

      {/* 更新证书确认弹窗 */}
      <AlertDialog open={renewOpen} onOpenChange={setRenewOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>更新证书</AlertDialogTitle>
            <AlertDialogDescription>
              会调用服务端 renewCert 动作，请确认 EasyRSA 数据已挂载且当前环境允许执行证书脚本。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={renewing}>取消</AlertDialogCancel>
            <AlertDialogAction onClick={handleRenew} disabled={renewing}>
              {renewing ? '更新中...' : '开始更新'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
