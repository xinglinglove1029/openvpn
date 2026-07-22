export type NotifyProvider = 'dingtalk' | 'wecom';

export interface SettingsResponse {
  system: {
    base: {
      site_url: string;
      web_port: string;
      admin_username: string;
      auto_update_ovpn_config: boolean;
      max_duplicate_login: number;
      history_max_days: number;
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
  gid?: number;
  password?: string;
  ovpnConfig?: string;
  mfaSecret?: string;
  lastLoginAt?: string;
  createdAt?: string;
  updatedAt?: string;
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

export interface NotifyLogRecord {
  id: number;
  event: string;
  provider: string;
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

export interface AdminRuntime {
  page?: 'login' | 'client' | 'admin';
  version?: string;
  sysUser?: string;
  clientUrls?: SettingsResponse['client']['client_url'];
}

export interface ClientUserInfo {
  id: number;
  username: string;
  name?: string;
  email?: string;
  ldapAuth?: boolean;
  isFirstLogin?: boolean;
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
