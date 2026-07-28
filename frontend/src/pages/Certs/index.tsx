import { useEffect, useMemo, useState } from 'react';
import { RefreshCw, ShieldCheck } from 'lucide-react';
import { toast } from 'sonner';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { messageOf, normalizeList } from '@/lib/format';
import { PageHeader } from '@/components/PageHeader';
import { StatusBadge } from '@/components/StatusBadge';
import { DataTable, type Column } from '@/components/DataTable';
import { HasPermission } from '@/components/HasPermission';
import { Card, CardContent } from '@/ui/card';
import { Button } from '@/ui/button';
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

  const state = useAsync(
    () => api.get<unknown>('/ovpn/certs').then((v) => normalizeList<CertRecord>(v, ['certs', 'data'])),
    [reloadKey],
  );

  useEffect(() => {
    if (state.error) {
      toast.error(`加载证书失败：${messageOf(state.error)}`);
    }
  }, [state.error]);

  const certs = state.data || [];
  const pagination = usePagination(certs, String(certs.length));

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
    <div className="space-y-6">
      <PageHeader eyebrow="Trust" title="证书管理" description="管理 CA 证书与客户端证书生命周期">
        <HasPermission code="cert:renew">
          <Button size="sm" onClick={() => setRenewOpen(true)}>
            <ShieldCheck className="h-4 w-4 mr-1" />
            更新证书
          </Button>
        </HasPermission>
      </PageHeader>

      <Card>
        <CardContent className="p-6">
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
                Docker 完整环境会挂载 EasyRSA 数据并展示证书状态。
              </p>
            </div>
          )}
        </CardContent>
      </Card>

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
