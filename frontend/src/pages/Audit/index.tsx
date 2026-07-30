import { useMemo, useState } from 'react';
import { Download, RefreshCw } from 'lucide-react';
import { api } from '../../api';
import { useAsync } from '@/hooks/useAsync';
import { usePagination } from '@/hooks/usePagination';
import { normalizeList } from '@/lib/format';
import { formatDateTime } from '@/lib/utils';
import { PageHeader } from '@/components/PageHeader';
import { TimeRangePicker } from '@/components/TimeRangePicker';
import { StatusBadge } from '@/components/StatusBadge';
import { DataTable, type Column } from '@/components/DataTable';
import { Button } from '@/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/ui/select';
import type { AuditLogRecord, UserRecord } from '../../types';

const moduleOptions = [
  { value: '', label: '全部模块' },
  { value: 'auth', label: '登录' },
  { value: 'user', label: '账号' },
  { value: 'group', label: '用户组' },
  { value: 'client', label: '客户端' },
  { value: 'firewall', label: '防火墙' },
  { value: 'settings', label: '系统设置' },
  { value: 'notify', label: '通知' },
  { value: 'server', label: '服务' },
  { value: 'email', label: '邮件' },
];

const actionOptions = [
  { value: '', label: '全部动作' },
  { value: 'login', label: '登录' },
  { value: 'create', label: '创建' },
  { value: 'update', label: '更新' },
  { value: 'delete', label: '删除' },
  { value: 'test', label: '测试' },
  { value: 'operate', label: '操作' },
  { value: 'disconnect', label: '断开' },
];

export default function AuditPage() {
  const [operator, setOperator] = useState('');
  const [module, setModule] = useState('');
  const [action, setAction] = useState('');
  const [dateRange, setDateRange] = useState({ from: '', to: '' });
  const [reloadKey, setReloadKey] = useState(0);

  const usersState = useAsync<UserRecord[]>(
    () =>
      api
        .get<{ data?: UserRecord[] } | UserRecord[]>('/ovpn/audit/user-options')
        .then((v) => normalizeList<UserRecord>(v, ['data'])),
    [],
  );
  const users = usersState.data || [];

  const query = useMemo(() => {
    const params = new URLSearchParams({ limit: '100' });
    if (operator.trim()) params.set('operator', operator.trim());
    if (module) params.set('module', module);
    if (action) params.set('action', action);
    if (dateRange.from) params.set('start', dateRange.from);
    if (dateRange.to) params.set('end', dateRange.to);
    return params.toString();
  }, [operator, module, action, dateRange]);

  const state = useAsync(
    () => api.get<unknown>(`/ovpn/audit/logs?${query}`),
    [query, reloadKey],
  );

  const rows = normalizeList<AuditLogRecord>(state.data, ['data', 'logs']);
  const total =
    state.data && typeof state.data === 'object' && 'total' in state.data
      ? Number((state.data as { total?: number }).total || rows.length)
      : rows.length;
  const exportUrl = `/ovpn/audit/export?${query}`;
  const pagination = usePagination(rows, `${query}|${reloadKey}`);

  const columns: Column<AuditLogRecord>[] = useMemo(
    () => [
      {
        key: 'createdAt',
        header: '时间',
        sortable: true,
        sortAccessor: (item) => (item.createdAt ? new Date(item.createdAt).getTime() : 0),
        render: (item) =>
          item.createdAt ? formatDateTime(item.createdAt) : '-',
      },
      {
        key: 'operator',
        header: '操作人',
        sortable: true,
        sortAccessor: (item) => item.operator ?? '',
        render: (item) => item.operator || '-',
      },
      {
        key: 'module',
        header: '模块',
        sortable: true,
        sortAccessor: (item) => item.module ?? '',
        render: (item) => item.module || '-',
      },
      {
        key: 'action',
        header: '动作',
        sortable: true,
        sortAccessor: (item) => item.action ?? '',
        render: (item) => item.action || '-',
      },
      {
        key: 'target',
        header: '目标',
        sortable: true,
        sortAccessor: (item) => item.target ?? '',
        render: (item) => item.target || '-',
      },
      {
        key: 'success',
        header: '结果',
        sortable: true,
        sortAccessor: (item) => (item.success ? 1 : 0),
        render: (item) => (
          <StatusBadge status={item.success ? 'success' : 'danger'}>
            {item.success ? '成功' : '失败'}
          </StatusBadge>
        ),
      },
      {
        key: 'ip',
        header: 'IP',
        sortable: true,
        sortAccessor: (item) => item.ip ?? '',
        render: (item) => item.ip || '-',
      },
      {
        key: 'message',
        header: '说明',
        sortable: true,
        sortAccessor: (item) => item.message ?? '',
        render: (item) => item.message || '-',
      },
    ],
    [],
  );

  return (
    <div className="space-y-4">
      <PageHeader eyebrow="Audit" title="操作审计" description="查看系统操作日志与审计记录" />

      {/* 操作工具栏：筛选条件 在左，刷新、导出 在右 */}
      <div className="flex flex-wrap items-center gap-3">
        <Select value={operator} onValueChange={setOperator}>
          <SelectTrigger className="w-[140px] h-8">
            <SelectValue placeholder="全部操作人" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">全部操作人</SelectItem>
            {users.map((u) => (
              <SelectItem key={u.id} value={u.username}>
                {u.username}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={module} onValueChange={setModule}>
          <SelectTrigger className="w-[120px] h-8">
            <SelectValue placeholder="全部模块" />
          </SelectTrigger>
          <SelectContent>
            {moduleOptions.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={action} onValueChange={setAction}>
          <SelectTrigger className="w-[120px] h-8">
            <SelectValue placeholder="全部动作" />
          </SelectTrigger>
          <SelectContent>
            {actionOptions.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <TimeRangePicker
          value={dateRange}
          onChange={setDateRange}
          placeholder="选择时间范围"
        />
        <div className="ml-auto flex items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => setReloadKey((v) => v + 1)}>
            <RefreshCw className="h-3.5 w-3.5 mr-1" />
            刷新
          </Button>
          <Button size="sm" variant="outline" asChild>
            <a href={exportUrl}>
              <Download className="h-3.5 w-3.5 mr-1" />
              导出 CSV
            </a>
          </Button>
        </div>
      </div>

      {/* 摘要 */}
      {!state.loading && rows.length > 0 && (
        <p className="text-xs text-muted-foreground">
          共匹配 {total} 条记录，当前已加载 {rows.length} 条。
        </p>
      )}

      {/* 加载中 */}
      {state.loading && (
        <p className="text-sm text-muted-foreground text-center py-8">
          加载中...
        </p>
      )}

      {/* 表格 */}
      {!state.loading && rows.length > 0 && (
        <DataTable
          columns={columns}
          data={pagination.pagedItems}
          fullData={rows}
          page={pagination.page}
          pageSize={pagination.pageSize}
          pageCount={pagination.pageCount}
          total={pagination.total}
          start={pagination.start}
          end={pagination.end}
          onPageChange={pagination.setPage}
          onPageSizeChange={pagination.setPageSize}
          keyFn={(item) => item.id}
        />
      )}

      {/* 空状态 */}
      {!state.loading && rows.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground font-medium">暂无审计记录</p>
          <p className="text-sm text-muted-foreground mt-1">
            登录、创建/修改/删除账号、客户端、防火墙、系统设置和通知测试都会记录在这里。
          </p>
        </div>
      )}
    </div>
  );
}
