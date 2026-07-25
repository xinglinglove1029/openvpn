import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { realtimeHub } from '@/lib/notificationHub';
import type { DashboardStatsPayload } from '@/types';

/** 顶部导航的精简状态：仅保留必要字段，避免大对象传递。 */
export interface SystemStatus {
  /** OpenVPN management 接口是否可用；false 时顶部需要呼吸灯提醒 */
  managementOk: boolean;
  /** 最近一次推送的服务器状态文本（UNKNOWN / up / ...） */
  serverStatus: string;
  /** 风险列表（同步自 dashboard:stats），供 TopBar Popover 复用 */
  risks: DashboardStatsPayload['summary']['risks'];
  /** 最近一次推送时间（毫秒）；0 表示从未收到 */
  pushedAt: number;
}

const defaultStatus: SystemStatus = {
  managementOk: true,
  serverStatus: '',
  risks: [],
  pushedAt: 0,
};

interface SystemStatusContextType {
  status: SystemStatus;
}

const SystemStatusContext = createContext<SystemStatusContextType | undefined>(undefined);

export function SystemStatusProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SystemStatus>(defaultStatus);

  useEffect(() => {
    const offSub = realtimeHub.subscribe<DashboardStatsPayload>('dashboard:stats', (payload) => {
      if (!payload?.summary) return;
      const summary = payload.summary;
      const stats = summary.stats;
      setStatus({
        // 后端字段为 managementOk；旧字段名 managementOK 兼容一下
        managementOk: Boolean((stats as { managementOk?: boolean }).managementOk ?? (stats as { managementOK?: boolean }).managementOK),
        serverStatus: stats.serverStatus || '',
        risks: summary.risks || [],
        pushedAt: payload.pushedAt || Date.now(),
      });
    });
    return offSub;
  }, []);

  return (
    <SystemStatusContext.Provider value={{ status }}>
      {children}
    </SystemStatusContext.Provider>
  );
}

export function useSystemStatus(): SystemStatusContextType {
  const context = useContext(SystemStatusContext);
  if (!context) {
    // 兜底：未挂载 Provider 时返回默认安全值，不影响页面渲染
    return { status: defaultStatus };
  }
  return context;
}
