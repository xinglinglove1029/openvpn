export type NotifyProvider = 'dingtalk' | 'wecom';

export interface SettingsResponse {
  system: {
    base: {
      site_url: string;
      server_addr: string;
      web_port: string;
      admin_username: string;
      auto_update_ovpn_config: boolean;
      max_duplicate_login: number;
      history_max_days: number;
      renew_days: number;
      validate_client_config: boolean;
    };
    ldap: {
      ldap_auth: boolean;
      ldap_url: string;
      ldap_base_dn: string;
      ldap_user_attribute: string;
      ldap_user_group_filter: boolean;
      ldap_user_group_dn: string;
      ldap_user_attr_ipaddr_name: string;
      ldap_user_attr_config_name: string;
      ldap_bind_user_dn: string;
      ldap_bind_password: string;
    };
    email: {
      send_subject_prefix: string;
      send_from: string;
      host: string;
      port: number;
      username: string;
      password: string;
      security?: string | null;
    };
    notify: {
      enabled: boolean;
      provider: NotifyProvider | string;
      webhook: string;
      secret: string;
      mention_all: boolean;
    };
  };
  client: {
    client_url: {
      windows: string;
      linux: string;
      macos: string;
      ios: string;
      android: string;
    };
  };
  openvpn: {
    ovpn_port: number;
    ovpn_proto: string;
    ovpn_subnet: string;
    ovpn_max_clients: number;
    ovpn_gateway: boolean;
    ovpn_management: string;
    ovpn_ipv6: boolean;
    ovpn_subnet6: string;
    ovpn_push_dns1: string;
    ovpn_push_dns2: string;
  };
  ai?: {
    enabled: boolean;
    provider: string;       // ollama | openai | deepseek | customize
    base_url: string;
    api_key: string;        // 脱敏后的值
    model: string;
    system_prompt: string;
    max_tokens: number;
    temperature: number;
  };
}

export type AIProvider = 'ollama' | 'deepseek' | 'openai' | 'customize';

/** 单个 AI 供应商的非敏感配置。API Key 只通过 PUT 写入。 */
export interface AIProviderProfile {
  provider: AIProvider;
  base_url: string;
  model: string;
  system_prompt: string;
  max_tokens: number;
  temperature: number;
  has_api_key: boolean;
}

/** AI 配置详情（来自 /settings/ai 独立接口） */
export interface AISettingsResponse {
  // 保留 config/provider/model，兼容现有调用方。
  config: {
    enabled: boolean;
    provider: AIProvider;
    base_url: string;
    api_key: string;
    model: string;
    system_prompt: string;
    max_tokens: number;
    temperature: number;
  };
  profiles: AIProviderProfile[];
  active_provider: AIProvider;
  provider: string;
  model: string;
}

export interface AIProviderProfileSave extends Omit<AIProviderProfile, 'has_api_key'> {
  api_key?: string;
  clear_api_key?: boolean;
}

export interface AISettingsSaveRequest {
  enabled: boolean;
  active_provider: AIProvider;
  profiles: AIProviderProfileSave[];
}

export interface OnlineClient {
  id?: string | number;
  cid?: string | number;
  username?: string;
  commonName?: string;
  common_name?: string;
  vip?: string;
  vip6?: string;
  rip?: string;
  rip6?: string;
  connectedSince?: string;
  connected_since?: string;
  bytesReceived?: number;
  bytesSent?: number;
  bytes_received?: number;
  bytes_sent?: number;
  recvBytes?: number;
  sendBytes?: number;
  connDate?: string;
  onlineTime?: string;
  isNftBlacklist?: boolean;
  isNftBlackList?: boolean;
}

export interface OnlineResponse {
  server?: {
    Address?: string;
    Status?: string;
    BytesIn?: string;
    BytesOut?: string;
    RunDate?: string;
  };
  clients: OnlineClient[];
}

export interface UserRecord {
  id?: number;
  username: string;
  name?: string;
  email?: string;
  isEnable?: boolean;
  authUser?: boolean;
  expireDate?: string;
  ipAddr?: string;
  ipRegion?: string;
  gid?: number;
  password?: string;
  ovpnConfig?: string;
  mfaSecret?: string;
  mfaEnabled?: boolean;
  lastLoginAt?: string;
  createdAt?: string;
  updatedAt?: string;
  /** RBAC：角色 ID 列表（一个用户可绑定多个角色） */
  roleIds?: number[];
  /** RBAC：角色名称列表（仅用于展示，由后端 join 返回） */
  roleNames?: string[];
  /** 内置 admin 用户标记（运行时计算，前端用于隐藏删除按钮） */
  isBuiltin?: boolean;
}

export interface ClientRecord {
  name: string;
  file?: string;
  fullName?: string;
  date?: string;
}

export interface GroupRecord {
  id: number;
  name: string;
  parent_id?: number | null;
  config?: string;
  createdAt?: string;
  updatedAt?: string;
  /** 该组绑定的默认角色 ID（null 表示未绑定） */
  roleId?: number | null;
  /** 该组绑定的默认角色名（仅展示用） */
  roleName?: string;
}

export interface FirewallRecord {
  id: number;
  sip?: string;
  dip?: string;
  sg?: GroupRecord[];
  dg?: GroupRecord[];
  policy?: string;
  status?: boolean;
  comment?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface HistoryRecord {
  id?: number;
  username?: string;
  common_name?: string;
  commonName?: string;
  vip?: string;
  vip6?: string;
  rip?: string;
  rip6?: string;
  ripRegion?: string;
  rip6Region?: string;
  bytes_received?: number;
  bytes_sent?: number;
  bytesReceived?: number;
  bytesSent?: number;
  time_unix?: number;
  time_duration?: number | string;
}

export interface HistoryResponse {
  draw?: number;
  recordsTotal?: number;
  recordsFiltered?: number;
  data: HistoryRecord[];
}

export interface DashboardSummary {
  stats: {
    onlineClients: number;
    clientConfigs: number;
    totalUsers: number;
    enabledUsers: number;
    expiredUsers: number;
    expiringUsers: number;
    firewallRules: number;
    todayConnections: number;
    bytesReceived24h: string;
    bytesSent24h: string;
    serverStatus: string;
    managementOk: boolean;
  };
  trends: Array<{ hour: string; connections: number; received: number; sent: number }>;
  topUsers: Array<{ username: string; bytes: number; text: string }>;
  risks: Array<{ level: 'danger' | 'warning' | 'info' | string; title: string; message: string }>;
}

// 概览页 WebSocket 实时推送载荷（topic: dashboard:stats）
export interface DashboardStatsPayload {
  summary: DashboardSummary;
  online: OnlineClient[];
  server: {
    Address?: string;
    Status?: string;
    StatusDesc?: string;
    RunDate?: string;
    BytesIn?: string;
    BytesOut?: string;
    Nclients?: string;
    Mode?: string;
    Version?: string;
  };
  pushedAt: number;
}

export interface NotifyLogRecord {
  id: number;
  event: string;
  provider: string;     // 渠道类型：email / dingtalk / webhook ...
  channelName: string;  // 渠道名称：用户自定义的名称
  username: string;
  success: boolean;
  message: string;
  createdAt: string;
}

export interface AuditLogRecord {
  id: number;
  operator: string;
  module: string;
  action: string;
  target: string;
  success: boolean;
  message: string;
  ip: string;
  ipRegion?: string;
  createdAt: string;
}

export interface CertRecord {
  name?: string;
  type?: string;
  status?: string;
  notBefore?: string;
  notAfter?: string;
  expiresIn?: number | string;
}

// 通知渠道（多渠道维护）
export type ChannelType =
  | 'webhook'
  | 'email'
  | 'dingtalk'
  | 'feishu'
  | 'wecom'
  | 'discord'
  | 'slack'
  | 'telegram'
  | 'mattermost';

export interface ChannelTypeMeta {
  type: ChannelType;
  label: string;
  icon: string;
}

export interface NotificationChannel {
  id: number;
  name: string;
  type: ChannelType | string;
  enabled: boolean;
  // 后端以 json.RawMessage 存储；前端拿到时可能是对象或字符串
  config?: Record<string, unknown> | string | null;
  createdAt?: string;
  updatedAt?: string;
}

export interface AdminRuntime {
  page?: 'login' | 'client' | 'admin';
  version?: string;
  sysUser?: string;
  clientUrls?: SettingsResponse['client']['client_url'];
}

/** 系统监控：单次推送的快照（WebSocket topic: system:stats） */
export interface SystemStatsPayload {
  timestamp: number;
  intervalMs: number;
  host: SystemHostInfo;
  cpuPercent: number;
  cpuPerCore: number[];
  memory: SystemMemoryInfo;
  process: SystemProcessInfo;
  disks: SystemDiskInfo[];
  networks: SystemNetInfo[];
  netTotalRxBps: number;
  netTotalTxBps: number;
  /** 进程 Top 5：按 CPU 占用降序 */
  topCpuProcesses: SystemProcessTop[];
  /** 进程 Top 5：按 内存 RSS 降序 */
  topMemProcesses: SystemProcessTop[];
}

export interface SystemHostInfo {
  hostname: string;
  platform: string;
  platformVersion: string;
  kernelVersion: string;
  kernelArch: string;
  uptimeSeconds: number;
  bootTime: number;
  cpuModel: string;
  cpuCores: number;
  cpuMHz: number;
  loadAvg1: number;
  loadAvg5: number;
  loadAvg15: number;
}

export interface SystemMemoryInfo {
  totalBytes: number;
  usedBytes: number;
  availableBytes: number;
  usedPercent: number;
  swapTotalBytes: number;
  swapUsedBytes: number;
  swapPercent: number;
}

export interface SystemProcessInfo {
  pid: number;
  memoryRss: number;
  memoryVsz: number;
  cpuPercent: number;
  numThreads: number;
}

/** 进程 Top N 列表项 */
export interface SystemProcessTop {
  pid: number;
  name: string;
  username: string;
  cpuPercent: number;
  memoryRss: number;
  memoryVsz: number;
  status: string;
  nice: number;
  /** 进程启动时间（毫秒） */
  createTime: number;
}

export interface SystemDiskInfo {
  device: string;
  mountpoint: string;
  fsType: string;
  totalBytes: number;
  usedBytes: number;
  freeBytes: number;
  usedPercent: number;
}

export interface SystemNetInfo {
  name: string;
  rxBytes: number;
  txBytes: number;
  rxBps: number;
  txBps: number;
  rxPackets: number;
  txPackets: number;
  rxErrors: number;
  txErrors: number;
  rxDrops: number;
  txDrops: number;
  isPhysical: boolean;
}

export interface ClientPackage {
  id: number;
  platform: string;
  version: string;
  filename: string;
  storedName: string;
  fileSize: number;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
  downloadUrl: string;
}

export interface ClientUserInfo {
  id: number;
  username: string;
  name?: string;
  email?: string;
  ldapAuth?: boolean;
  isFirstLogin?: boolean;
  /** 自定义头像：data URL（base64）或预设样式标识 */
  avatar?: string;
  /** RBAC：角色 ID 列表（admin 用户为 []） */
  roleIds?: number[];
  /** RBAC：权限 code 列表（admin 为 ["*"]） */
  permissions?: string[];
  /** RBAC：是否为系统超管（绕过权限检查） */
  isAdmin?: boolean;
}

/** RBAC：角色 */
export interface Role {
  id: number;
  name: string;
  code: string;
  description?: string;
  isBuiltin: boolean;
  isEnable: boolean;
  sort: number;
  createdAt?: string;
  updatedAt?: string;
  /** 角色已分配的权限 code 列表 */
  permissions?: string[];
  /** 角色下用户数 */
  userCount?: number;
  /** 角色下用户组数 */
  groupCount?: number;
}

/** RBAC：权限定义 */
export interface Permission {
  id: number;
  parentId?: number;
  name: string;
  code: string;
  type: 'menu' | 'button';
  path?: string;
  icon?: string;
  sort: number;
  /** 内置权限（seed 维护）：code/type 不可改，不可删 */
  isBuiltin?: boolean;
}

/** RBAC：权限树节点（带 children） */
export interface PermissionTreeNode {
  id: number;
  parentId?: number;
  name: string;
  code: string;
  type: 'menu' | 'button';
  path?: string;
  icon?: string;
  sort: number;
  /** 内置权限（seed 维护）：code/type 不可改，不可删 */
  isBuiltin?: boolean;
  children?: PermissionTreeNode[];
}

export interface ClientMfaResponse {
  mfaEnable: boolean;
  user: ClientUserInfo & { mfaSecret: string };
}

declare global {
  interface Window {
    __OPENVPN_ADMIN__?: AdminRuntime;
  }
}
