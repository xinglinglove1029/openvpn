import { useEffect, useMemo, useRef, useState } from 'react';
import QRCode from 'qrcode';
import { User, Lock, KeyRound, Mail, ShieldCheck, ChevronDown, ChevronLeft, ChevronRight, Circle, AlertTriangle, FolderTree } from 'lucide-react';
import { api } from './api';
import type { FormEvent } from 'react';
import type {
  AuditLogRecord,
  CertRecord,
  ClientRecord,
  ClientMfaResponse,
  ClientUserInfo,
  DashboardSummary,
  FirewallRecord,
  GroupRecord,
  HistoryResponse,
  NotifyLogRecord,
  OnlineClient,
  OnlineResponse,
  SettingsResponse,
  UserRecord,
} from './types';

type Section = 'overview' | 'users' | 'clients' | 'firewall' | 'history' | 'certs' | 'audit' | 'settings';
type ThemeKey = 'midnight' | 'aurora' | 'emerald' | 'daylight';
type AsyncState<T> = { loading: boolean; error?: string; data?: T };
type Toast = { type: 'success' | 'error' | 'info'; message: string };
type SelectOption = { value: string; label: string; description?: string };
type ConfirmState = { title: string; message: string; danger?: boolean; onConfirm: () => void | Promise<void> };
type FieldErrors = Record<string, string>;
type LoginResult = { message?: string; redirect?: string; user?: ClientUserInfo };
type ClientConfigResponse = { filename: string; content: string };
type ModalState =
  | { type: 'none' }
  | { type: 'server-config'; content: string }
  | { type: 'rate-limit'; client: OnlineClient; upload: string; uploadUnit: string; download: string; downloadUnit: string }
  | { type: 'user-form'; mode: 'add' | 'edit'; user?: UserRecord }
  | { type: 'reset-password'; user: UserRecord }
  | { type: 'client-form' }
  | { type: 'client-editor'; client: ClientRecord; editor: 'config' | 'ccd'; content: string }
  | { type: 'firewall-form'; mode: 'add' | 'edit'; firewall?: FirewallRecord }
  | { type: 'group-form'; mode: 'add'; parentGroup?: GroupRecord }
  | { type: 'group-form'; mode: 'edit'; group: GroupRecord }
  | { type: 'group-config'; group: GroupRecord; content: string }
  | { type: 'renew-cert' };

const navItems: Array<{ key: Section; label: string; eyebrow: string }> = [
  { key: 'overview', label: '态势总览', eyebrow: 'Command' },
  { key: 'users', label: '账号矩阵', eyebrow: 'Identity' },
  { key: 'clients', label: '客户端', eyebrow: 'Device' },
  { key: 'firewall', label: '防火墙', eyebrow: 'Policy' },
  { key: 'history', label: '连接历史', eyebrow: 'Telemetry' },
  { key: 'certs', label: '证书', eyebrow: 'Trust' },
  { key: 'audit', label: '操作审计', eyebrow: 'Audit' },
  { key: 'settings', label: '系统设置', eyebrow: 'Control' },
];

const runtime = window.__OPENVPN_ADMIN__ || {};
const themeOptions: Array<{ key: ThemeKey; label: string; description: string }> = [
  { key: 'midnight', label: '曜石蓝', description: '深色科技感' },
  { key: 'aurora', label: '极光紫', description: '高对比霓虹' },
  { key: 'emerald', label: '青峦绿', description: '冷静运维风' },
  { key: 'daylight', label: '晨雾白', description: '浅色办公风' },
];

const notifyProviderOptions: SelectOption[] = [
  { value: 'dingtalk', label: '钉钉机器人', description: 'Markdown 机器人 · 支持加签' },
  { value: 'wecom', label: '企业微信机器人', description: 'Markdown 机器人 · 群 Webhook' },
];

const userStatusOptions: SelectOption[] = [
  { value: 'all', label: '全部状态' },
  { value: 'enabled', label: '仅启用' },
  { value: 'disabled', label: '仅禁用' },
];

const mfaFilterOptions: SelectOption[] = [
  { value: 'all', label: '全部 MFA' },
  { value: 'enabled', label: 'MFA 已开' },
  { value: 'disabled', label: 'MFA 未开' },
];

const expireFilterOptions: SelectOption[] = [
  { value: 'all', label: '全部有效期' },
  { value: 'normal', label: '正常/长期' },
  { value: 'expiring', label: '即将过期' },
  { value: 'expired', label: '已过期' },
];

const pageSizeOptions: SelectOption[] = [
  { value: '10', label: '10 条/页' },
  { value: '20', label: '20 条/页' },
  { value: '50', label: '50 条/页' },
];

const auditModuleOptions: SelectOption[] = [
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

const auditActionOptions: SelectOption[] = [
  { value: '', label: '全部动作' },
  { value: 'login', label: '登录' },
  { value: 'create', label: '创建' },
  { value: 'update', label: '更新' },
  { value: 'delete', label: '删除' },
  { value: 'test', label: '测试' },
  { value: 'operate', label: '操作' },
  { value: 'disconnect', label: '断开' },
];

function getInitialTheme(): ThemeKey {
  const stored = window.localStorage.getItem('openvpn-admin-theme');
  return themeOptions.some((item) => item.key === stored) ? stored as ThemeKey : 'midnight';
}

function useAsync<T>(loader: () => Promise<T>, deps: unknown[] = []): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({ loading: true });

  useEffect(() => {
    let mounted = true;
    setState({ loading: true });
    loader()
      .then((data) => mounted && setState({ loading: false, data }))
      .catch((error) => mounted && setState({ loading: false, error: error instanceof Error ? error.message : String(error) }));

    return () => {
      mounted = false;
    };
  }, deps);

  return state;
}

function usePagination<T>(items: T[], resetKey = '', initialPageSize = 10) {
  const [page, setPage] = useState(1);
  const [pageSize, setPageSizeState] = useState(initialPageSize);
  const total = items.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, pageCount);
  const start = (currentPage - 1) * pageSize;
  const end = Math.min(start + pageSize, total);
  const pagedItems = items.slice(start, end);

  useEffect(() => {
    setPage(1);
  }, [resetKey, pageSize]);

  useEffect(() => {
    if (page !== currentPage) setPage(currentPage);
  }, [page, currentPage]);

  function setPageSize(next: number) {
    setPageSizeState(next);
  }

  return { page: currentPage, pageSize, setPageSize, total, pageCount, start, end, pagedItems, setPage };
}

function normalizeList<T>(value: unknown, candidates: string[] = []): T[] {
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    for (const candidate of candidates) {
      if (Array.isArray(record[candidate])) return record[candidate] as T[];
    }
  }
  return [];
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function trimText(value: unknown) {
  return String(value ?? '').trim();
}

function isValidEmail(value: string) {
  const text = trimText(value);
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(text);
}

function isValidPort(value: unknown) {
  const text = trimText(value);
  const port = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(port) && port >= 1 && port <= 65535;
}

function isNonNegativeInteger(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(numberValue) && numberValue >= 0;
}

function isPositiveInteger(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(numberValue) && numberValue > 0;
}

function isPositiveNumber(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+(\.\d+)?$/.test(text) && Number.isFinite(numberValue) && numberValue > 0;
}

function isValidUrl(value: unknown, protocols = ['http:', 'https:']) {
  const text = trimText(value);
  try {
    const url = new URL(text);
    return protocols.includes(url.protocol) && Boolean(url.hostname);
  } catch {
    return false;
  }
}

function isValidIpv4(value: string) {
  const parts = trimText(value).split('.');
  return parts.length === 4 && parts.every((part) => /^\d+$/.test(part) && Number(part) >= 0 && Number(part) <= 255);
}

function isValidIpv6(value: string) {
  const text = trimText(value);
  if (!text.includes(':') || !/^[0-9a-fA-F:.]+$/.test(text)) return false;
  if ((text.match(/::/g) || []).length > 1) return false;
  const parts = text.split(':');
  if (!text.includes('::') && parts.length !== 8) return false;
  if (parts.length > 8) return false;
  return parts.every((part) => part === '' || /^[0-9a-fA-F]{1,4}$/.test(part) || isValidIpv4(part));
}

function isValidIp(value: string) {
  return isValidIpv4(value) || isValidIpv6(value);
}

function isValidCidr(value: string, requiredVersion?: 4 | 6) {
  const [address, prefix, ...extraParts] = trimText(value).split('/');
  if (!address || !prefix || extraParts.length) return false;
  const prefixNumber = Number(prefix);
  if (!/^\d+$/.test(prefix) || !Number.isInteger(prefixNumber)) return false;
  if (requiredVersion === 4) return isValidIpv4(address) && prefixNumber >= 0 && prefixNumber <= 32;
  if (requiredVersion === 6) return isValidIpv6(address) && prefixNumber >= 0 && prefixNumber <= 128;
  return (isValidIpv4(address) && prefixNumber <= 32) || (isValidIpv6(address) && prefixNumber <= 128);
}

function isValidIpOrCidr(value: string) {
  const text = trimText(value);
  return text.includes('/') ? isValidCidr(text) : isValidIp(text);
}

function isValidIpOrCidrList(value: unknown) {
  const text = trimText(value);
  if (!text) return true;
  return text.split(',').map((part) => part.trim()).filter(Boolean).every(isValidIpOrCidr);
}

function isValidHost(value: unknown) {
  const text = trimText(value).replace(/^\[(.*)\]$/, '$1');
  if (!text) return false;
  if (isValidIp(text)) return true;
  return /^(localhost|[A-Za-z0-9](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9])?)*)$/.test(text);
}

function isValidHostPort(value: unknown) {
  const text = trimText(value);
  const bracketMatch = text.match(/^\[([^\]]+)\]:(\d+)$/);
  if (bracketMatch) return isValidHost(bracketMatch[1]) && isValidPort(bracketMatch[2]);
  const separatorIndex = text.lastIndexOf(':');
  if (separatorIndex <= 0) return false;
  return isValidHost(text.slice(0, separatorIndex)) && isValidPort(text.slice(separatorIndex + 1));
}

function isStrongPassword(value: unknown) {
  const text = trimText(value);
  return text.length >= 12 && /[a-z]/.test(text) && /[A-Z]/.test(text) && /\d/.test(text) && /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(text);
}

function isValidSafeName(value: unknown) {
  return /^[A-Za-z0-9._-]{2,64}$/.test(trimText(value));
}

function isValidAccount(value: unknown) {
  return /^\S{2,64}$/.test(trimText(value));
}

function formatBytes(value?: number | string | null) {
  if (value === null || value === undefined || value === '') return '0 B';
  const numValue = typeof value === 'number' ? value : Number(String(value).replace(/[^\d.eE+\-]/g, ''));
  if (!Number.isFinite(numValue) || numValue <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = numValue;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  const fixed = Number(size.toFixed(2));
  return `${fixed} ${units[unitIndex]}`;
}

function getClientName(client: OnlineClient) {
  return client.username && client.username !== 'UNDEF' ? client.username : client.commonName || client.common_name || '未知用户';
}

function getClientBytes(client: OnlineClient, direction: 'received' | 'sent') {
  const candidates = direction === 'received'
    ? [client.bytesReceived, client.bytes_received, client.recvBytes]
    : [client.bytesSent, client.bytes_sent, client.sendBytes];
  for (const value of candidates) {
    if (value == null) continue;
    const num = Number(value);
    if (Number.isFinite(num)) return num;
  }
  return 0;
}

function parseDateOnly(value?: string) {
  if (!value) return undefined;
  const date = new Date(`${value}T00:00:00`);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

function expiryStatus(user: UserRecord) {
  const date = parseDateOnly(user.expireDate);
  if (!date) return { label: '长期', className: 'neutral' };
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const diffDays = Math.ceil((date.getTime() - today.getTime()) / 86400000);
  if (diffDays < 0) return { label: '已过期', className: 'danger' };
  if (diffDays <= 7) return { label: '即将过期', className: 'warning' };
  return { label: '正常', className: 'success' };
}

function isUserExpired(user: UserRecord) {
  return expiryStatus(user).className === 'danger';
}

function isUserExpiring(user: UserRecord) {
  return expiryStatus(user).className === 'warning';
}

function formatDateLabel(value?: string) {
  if (!value) return '年 / 月 / 日';
  const [year, month, day] = value.split('-');
  return year && month && day ? `${year}/${month}/${day}` : value;
}

function addMonths(date: Date, months: number) {
  return new Date(date.getFullYear(), date.getMonth() + months, 1);
}

function sameDate(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function toDateInputValue(date: Date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function parseDateInputValue(value?: string) {
  if (!value) return undefined;
  const [year, month, day] = value.split('-').map(Number);
  if (!year || !month || !day) return undefined;
  const date = new Date(year, month - 1, day);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

function monthDays(viewDate: Date) {
  const first = new Date(viewDate.getFullYear(), viewDate.getMonth(), 1);
  const start = new Date(first);
  start.setDate(first.getDate() - first.getDay());
  return Array.from({ length: 42 }, (_, index) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + index));
}

function clientVips(client: OnlineClient) {
  return [client.vip, client.vip6].filter(Boolean).join(',');
}

function buildTree(groups: GroupRecord[], parentId: number | null = null, depth = 0): Array<GroupRecord & { depth: number }> {
  return groups
    .filter((item) => (item.parent_id === item.id ? null : item.parent_id ?? null) === parentId)
    .flatMap((item) => [{ ...item, depth }, ...buildTree(groups, item.id, depth + 1)]);
}

function getDescendantGroupIds(groups: GroupRecord[], groupId: number) {
  const ids = new Set<number>();
  const visit = (parentId: number) => {
    groups.filter((item) => item.parent_id === parentId).forEach((child) => {
      if (child.id === groupId || ids.has(child.id)) return;
      ids.add(child.id);
      visit(child.id);
    });
  };
  visit(groupId);
  return ids;
}

function StatCard({ label, value, trend }: { label: string; value: string | number; trend: string }) {
  return <div className="stat-card"><span>{label}</span><strong>{value}</strong><em>{trend}</em></div>;
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return <div className="empty-state"><div className="empty-orb" /><h3>{title}</h3><p>{description}</p></div>;
}

function HeroOrbitScene() {
  const mountRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    let cleanup: (() => void) | undefined;
    let disposed = false;

    import('three').then((THREE) => {
      if (disposed || !mountRef.current) return;

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 100);
    camera.position.set(0, 0.15, 5.6);

    const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    mount.appendChild(renderer.domElement);

    const rootStyles = getComputedStyle(document.documentElement);
    const accent = new THREE.Color(rootStyles.getPropertyValue('--accent').trim() || '#6df3ff');
    const accentTwo = new THREE.Color(rootStyles.getPropertyValue('--accent-2').trim() || '#8ea2ff');
    const accentThree = new THREE.Color(rootStyles.getPropertyValue('--accent-3').trim() || '#7658ff');

    const planet = new THREE.Mesh(
      new THREE.SphereGeometry(1, 72, 72),
      new THREE.MeshPhysicalMaterial({ color: accent, roughness: 0.38, metalness: 0.08, transmission: 0.18, thickness: 0.65, clearcoat: 0.6, clearcoatRoughness: 0.2 })
    );
    scene.add(planet);

    const atmosphere = new THREE.Mesh(
      new THREE.SphereGeometry(1.08, 72, 72),
      new THREE.MeshBasicMaterial({ color: accentTwo, transparent: true, opacity: 0.18, blending: THREE.AdditiveBlending })
    );
    scene.add(atmosphere);

    const orbitGroup = new THREE.Group();
    const orbitMaterial = new THREE.MeshBasicMaterial({ color: accentTwo, transparent: true, opacity: 0.42, side: THREE.DoubleSide });
    const innerOrbit = new THREE.Mesh(new THREE.TorusGeometry(1.9, 0.01, 12, 180), orbitMaterial);
    innerOrbit.rotation.x = Math.PI / 2.8;
    const outerOrbit = new THREE.Mesh(new THREE.TorusGeometry(2.75, 0.006, 12, 220), new THREE.MeshBasicMaterial({ color: accent, transparent: true, opacity: 0.24, side: THREE.DoubleSide }));
    outerOrbit.rotation.x = Math.PI / 2;
    orbitGroup.add(innerOrbit, outerOrbit);
    scene.add(orbitGroup);

    const particleCount = 120;
    const particlePositions = new Float32Array(particleCount * 3);
    for (let index = 0; index < particleCount; index += 1) {
      const radius = 2.5 + Math.random() * 1.45;
      const angle = Math.random() * Math.PI * 2;
      particlePositions[index * 3] = Math.cos(angle) * radius;
      particlePositions[index * 3 + 1] = (Math.random() - 0.5) * 1.9;
      particlePositions[index * 3 + 2] = Math.sin(angle) * radius * 0.72;
    }
    const particleGeometry = new THREE.BufferGeometry();
    particleGeometry.setAttribute('position', new THREE.BufferAttribute(particlePositions, 3));
    const particles = new THREE.Points(particleGeometry, new THREE.PointsMaterial({ color: accentThree, size: 0.025, transparent: true, opacity: 0.52, blending: THREE.AdditiveBlending }));
    scene.add(particles);

    scene.add(new THREE.AmbientLight(0xffffff, 1.1));
    const keyLight = new THREE.PointLight(0xffffff, 4.4, 12);
    keyLight.position.set(-1.4, 1.8, 2.8);
    scene.add(keyLight);
    const rimLight = new THREE.PointLight(accentTwo, 3.2, 10);
    rimLight.position.set(2.8, -1.8, 2.2);
    scene.add(rimLight);

    const pointer = { x: 0, y: 0 };
    const handlePointerMove = (event: PointerEvent) => {
      const rect = mount.getBoundingClientRect();
      pointer.x = ((event.clientX - rect.left) / rect.width - 0.5) * 0.6;
      pointer.y = ((event.clientY - rect.top) / rect.height - 0.5) * 0.4;
    };

    let animationFrame = 0;
    const resize = () => {
      const width = mount.clientWidth || 320;
      const height = mount.clientHeight || 280;
      renderer.setSize(width, height, false);
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
    };
    const animate = () => {
      animationFrame = window.requestAnimationFrame(animate);
      const elapsed = performance.now() * 0.001;
      planet.rotation.y = elapsed * 0.36;
      planet.rotation.x = Math.sin(elapsed * 0.38) * 0.08;
      atmosphere.rotation.y = -elapsed * 0.22;
      orbitGroup.rotation.z = elapsed * 0.16;
      particles.rotation.y = -elapsed * 0.05;
      scene.rotation.x += (pointer.y - scene.rotation.x) * 0.035;
      scene.rotation.y += (pointer.x - scene.rotation.y) * 0.035;
      renderer.render(scene, camera);
    };

    resize();
    animate();
    window.addEventListener('resize', resize);
    mount.addEventListener('pointermove', handlePointerMove);

      cleanup = () => {
      window.cancelAnimationFrame(animationFrame);
      window.removeEventListener('resize', resize);
      mount.removeEventListener('pointermove', handlePointerMove);
      if (renderer.domElement.parentElement === mount) mount.removeChild(renderer.domElement);
      planet.geometry.dispose();
      atmosphere.geometry.dispose();
      innerOrbit.geometry.dispose();
      outerOrbit.geometry.dispose();
      particleGeometry.dispose();
      if (Array.isArray(planet.material)) planet.material.forEach((material) => material.dispose()); else planet.material.dispose();
      if (Array.isArray(atmosphere.material)) atmosphere.material.forEach((material) => material.dispose()); else atmosphere.material.dispose();
      orbitMaterial.dispose();
      outerOrbit.material.dispose();
      particles.material.dispose();
      renderer.dispose();
    };
    });

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, []);

  return <div className="hero-orbit-scene" ref={mountRef} />;
}

function Toggle({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  return <button className={`toggle ${checked ? 'is-on' : ''}`} type="button" onClick={() => onChange(!checked)} aria-pressed={checked} />;
}

function TextField({ label, value, onSave, placeholder, type = 'text' }: { label: string; value: string | number | undefined | null; onSave: (value: string) => void; placeholder?: string; type?: string }) {
  const [draft, setDraft] = useState(String(value ?? ''));
  useEffect(() => setDraft(String(value ?? '')), [value]);
  return <label className="field-line"><span>{label}</span><input type={type} value={draft} placeholder={placeholder} onChange={(event) => setDraft(event.target.value)} onBlur={() => onSave(draft)} /></label>;
}

function Modal({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return <div className="modal-backdrop" role="dialog" aria-modal="true"><div className="modal-panel glass-panel"><div className="modal-heading"><h2>{title}</h2><button type="button" onClick={onClose}>关闭</button></div>{children}</div></div>;
}

function ConfirmDialog({ state, onClose, notify }: { state: ConfirmState; onClose: () => void; notify: (type: Toast['type'], message: string) => void }) {
  const [saving, setSaving] = useState(false);
  async function submit() {
    setSaving(true);
    try {
      await state.onConfirm();
      onClose();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return <div className="modal-backdrop" role="dialog" aria-modal="true"><div className="modal-panel glass-panel confirm-panel"><div className="confirm-icon" aria-label="warning"><AlertTriangle size={24} /></div><div><span className="confirm-eyebrow">Safety Check</span><h2>{state.title}</h2><p>{state.message}</p></div><div className="modal-actions"><button className="ghost-action" type="button" onClick={onClose} disabled={saving}>取消</button><button className={state.danger ? 'danger-action' : 'primary-action'} type="button" onClick={submit} disabled={saving}>{saving ? '处理中...' : '确认执行'}</button></div></div></div>;
}

function Toolbar({ children }: { children: React.ReactNode }) {
  return <div className="toolbar-actions">{children}</div>;
}

function ThemeSwitcher({ theme, onChange }: { theme: ThemeKey; onChange: (theme: ThemeKey) => void }) {
  const [open, setOpen] = useState(false);
  const selected = themeOptions.find((item) => item.key === theme) || themeOptions[0];
  return <div className={`theme-switcher ${open ? 'is-open' : ''}`} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false); }}><span className="theme-label">主题</span><button className="theme-trigger" type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen((value) => !value)}><span><strong>{selected.label}</strong><em>{selected.description}</em></span><ChevronDown size={16} className="trigger-chevron" aria-hidden="true" /></button>{open && <div className="theme-menu" role="listbox">{themeOptions.map((item) => <button key={item.key} className={`theme-option ${item.key === theme ? 'active' : ''}`} type="button" role="option" aria-selected={item.key === theme} onClick={() => { onChange(item.key); setOpen(false); }}><span className={`theme-dot theme-dot-${item.key}`} /><span><strong>{item.label}</strong><em>{item.description}</em></span></button>)}</div>}</div>;
}

function SettingSelect({ label, value, options, onChange }: { label: string; value: string; options: SelectOption[]; onChange: (value: string) => void }) {
  const [open, setOpen] = useState(false);
  const selected = options.find((option) => option.value === value) || options[0];

  return <div className={`field-line setting-select ${open ? 'is-open' : ''}`} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false); }}><span>{label}</span><button className="setting-select-trigger" type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen((current) => !current)}><span><strong>{selected?.label || '请选择'}</strong>{selected?.description && <em>{selected.description}</em>}</span><ChevronDown size={16} className="trigger-chevron" aria-hidden="true" /></button>{open && <div className="setting-select-menu" role="listbox">{options.map((option) => <button key={option.value} className={`setting-select-option ${option.value === value ? 'active' : ''}`} type="button" role="option" aria-selected={option.value === value} onClick={() => { onChange(option.value); setOpen(false); }}><strong>{option.label}</strong>{option.description && <em>{option.description}</em>}</button>)}</div>}</div>;
}

function CompactSelect({ value, options, onChange, placeholder }: { value: string; options: SelectOption[]; onChange: (value: string) => void; placeholder?: string }) {
  const [open, setOpen] = useState(false);
  const selected = options.find((option) => option.value === value);
  return <div className={`compact-select ${open ? 'is-open' : ''}`} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false); }}><button className="compact-trigger" type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen((current) => !current)}><span>{selected?.label || placeholder || '请选择'}</span><ChevronDown size={14} className="trigger-chevron" aria-hidden="true" /></button>{open && <div className="compact-menu" role="listbox">{options.map((option) => <button key={option.value} className={`compact-option ${option.value === value ? 'active' : ''}`} type="button" role="option" aria-selected={option.value === value} onClick={() => { onChange(option.value); setOpen(false); }}><strong>{option.label}</strong>{option.description && <em>{option.description}</em>}</button>)}</div>}</div>;
}

function PaginationBar({ page, pageSize, pageCount, total, start, end, onPageChange, onPageSizeChange }: { page: number; pageSize: number; pageCount: number; total: number; start: number; end: number; onPageChange: (page: number) => void; onPageSizeChange: (pageSize: number) => void }) {
  if (!total) return null;
  return <div className="pagination-bar"><div className="pagination-meta"><span>显示</span><strong>{start + 1}-{end}</strong><span>/ 共 {total} 条</span></div><CompactSelect value={String(pageSize)} options={pageSizeOptions} onChange={(value) => onPageSizeChange(Number(value))} /><div className="pagination-actions"><button type="button" disabled={page <= 1} onClick={() => onPageChange(1)}>首页</button><button type="button" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>上一页</button><strong>{page} / {pageCount}</strong><button type="button" disabled={page >= pageCount} onClick={() => onPageChange(page + 1)}>下一页</button><button type="button" disabled={page >= pageCount} onClick={() => onPageChange(pageCount)}>末页</button></div></div>;
}

function DatePicker({ value, onChange, placeholder = '年 / 月 / 日', allowClear = true }: { value: string; onChange: (value: string) => void; placeholder?: string; allowClear?: boolean }) {
  const selectedDate = parseDateInputValue(value);
  const [open, setOpen] = useState(false);
  const [viewDate, setViewDate] = useState(() => selectedDate || new Date());
  const today = new Date();
  const days = monthDays(viewDate);

  useEffect(() => {
    if (selectedDate) setViewDate(new Date(selectedDate.getFullYear(), selectedDate.getMonth(), 1));
  }, [value]);

  return <div className={`date-picker ${open ? 'is-open' : ''}`} onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false); }}><button className="compact-trigger date-trigger" type="button" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen((current) => !current)}><span className={value ? '' : 'is-placeholder'}>{value ? formatDateLabel(value) : placeholder}</span><ChevronDown size={14} className="trigger-chevron" aria-hidden="true" /></button>{open && <div className="date-popover"><div className="date-head"><button type="button" aria-label="上一月" onClick={() => setViewDate((date) => addMonths(date, -1))}><ChevronLeft size={16} /></button><strong>{viewDate.getFullYear()}年{String(viewDate.getMonth() + 1).padStart(2, '0')}月</strong><button type="button" aria-label="下一月" onClick={() => setViewDate((date) => addMonths(date, 1))}><ChevronRight size={16} /></button></div><div className="date-weekdays">{['日', '一', '二', '三', '四', '五', '六'].map((item) => <span key={item}>{item}</span>)}</div><div className="date-grid">{days.map((day) => { const muted = day.getMonth() !== viewDate.getMonth(); const active = Boolean(selectedDate && sameDate(day, selectedDate)); const current = sameDate(day, today); return <button key={day.toISOString()} className={`${muted ? 'muted' : ''} ${active ? 'active' : ''} ${current ? 'today' : ''}`} type="button" onClick={() => { onChange(toDateInputValue(day)); setOpen(false); }}>{day.getDate()}</button>; })}</div><div className="date-actions">{allowClear && <button type="button" onClick={() => { onChange(''); setOpen(false); }}>清空</button>}<button type="button" onClick={() => { onChange(toDateInputValue(new Date())); setOpen(false); }}>今天</button></div></div>}</div>;
}

const iconMap = { user: User, lock: Lock, key: KeyRound, mail: Mail, shield: ShieldCheck } as const;

function FieldIcon({ name, size = 16 }: { name: keyof typeof iconMap; size?: number }) {
  const Cmp = iconMap[name];
  return <Cmp size={size} className="field-icon-svg" aria-hidden="true" />;
}

function TextInput({ label, value, onChange, type = 'text', error, required, autoFocus, placeholder, icon }: { label: string; value: string; onChange: (value: string) => void; type?: string; error?: string; required?: boolean; autoFocus?: boolean; placeholder?: string; icon?: keyof typeof iconMap }) {
  if (type === 'date') return <div className={`field-line ${error ? 'has-error' : ''}`}><span>{label}{required && <b>*</b>}</span><DatePicker value={value} onChange={onChange} />{error && <small className="field-error">{error}</small>}</div>;
  return <label className={`field-line ${error ? 'has-error' : ''}`}><span className="field-label-text">{label}{required && <b>*</b>}</span><span className={`field-input-wrap ${icon ? 'has-icon' : ''}`}>{icon && <span className="field-icon-wrap"><FieldIcon name={icon} /></span>}<input type={type} value={value} required={required} autoFocus={autoFocus} placeholder={placeholder} aria-invalid={Boolean(error)} onChange={(event) => onChange(event.target.value)} /></span>{error && <small className="field-error">{error}</small>}</label>;
}

function TextAreaInput({ label, value, onChange, error }: { label: string; value: string; onChange: (value: string) => void; error?: string }) {
  return <label className={`field-line wide-field ${error ? 'has-error' : ''}`}><span>{label}</span><textarea value={value} aria-invalid={Boolean(error)} onChange={(event) => onChange(event.target.value)} />{error && <small className="field-error">{error}</small>}</label>;
}

function SelectInput({ label, value, options, onChange, error, required }: { label: string; value: string; options: Array<string | { value: string; label: string }>; onChange: (value: string) => void; error?: string; required?: boolean }) {
  const normalized = options.map((option) => typeof option === 'string' ? { value: option, label: option } : option);
  return <div className={`field-line ${error ? 'has-error' : ''}`}><span>{label}{required && <b>*</b>}</span><CompactSelect value={value} options={normalized} onChange={onChange} />{error && <small className="field-error">{error}</small>}</div>;
}

function CheckInput({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <div className="switch-line"><div><strong>{label}</strong></div><Toggle checked={checked} onChange={onChange} /></div>;
}

function MultiGroupInput({ label, value, groups, onChange, error }: { label: string; value: string; groups: Array<GroupRecord & { depth: number }>; onChange: (value: string) => void; error?: string }) {
  const selected = new Set(value.split(',').filter(Boolean));
  return <div className={`field-line group-picker wide-field ${error ? 'has-error' : ''}`}><span>{label}</span><div className="checkbox-grid">{groups.map((group) => { const checked = selected.has(String(group.id)); return <label key={group.id} className={`group-check ${checked ? 'is-checked' : ''}`} style={{ '--depth': group.depth } as React.CSSProperties}><input type="checkbox" checked={checked} onChange={(event) => { const next = new Set(selected); if (event.target.checked) next.add(String(group.id)); else next.delete(String(group.id)); onChange([...next].join(',')); }} /><i aria-hidden="true" /><span>{group.depth ? `${'— '.repeat(group.depth)}${group.name}` : group.name}</span></label>; })}</div>{error && <small className="field-error">{error}</small>}</div>;
}

function FormErrorSummary({ errors }: { errors: FieldErrors }) {
  const messages = [...new Set(Object.values(errors).filter(Boolean))];
  if (!messages.length) return null;
  return <div className="form-error-summary wide-field" role="alert"><strong>请先完善表单信息</strong><ul>{messages.map((message) => <li key={message}>{message}</li>)}</ul></div>;
}

function ImmersiveShell({ children, theme, setTheme, compact = false }: { children: React.ReactNode; theme: ThemeKey; setTheme: (theme: ThemeKey) => void; compact?: boolean }) {
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem('openvpn-admin-theme', theme);
  }, [theme]);

  return <main className={`immersive-shell ${compact ? 'is-compact' : ''}`}><div className="aurora aurora-a" /><div className="aurora aurora-b" /><div className="ambient-grid" aria-hidden="true" /><div className="ambient-scanline" aria-hidden="true" /><div className="ambient-particles" aria-hidden="true"><i /><i /><i /><i /><i /><i /></div><header className="portal-topbar glass-panel"><div className="brand"><div className="brand-mark">OV</div><div><strong>OpenVPN</strong><span>Secure Access</span></div></div><ThemeSwitcher theme={theme} onChange={setTheme} /></header>{children}</main>;
}

function PasswordStrength({ value }: { value: string }) {
  const checks = [
    { ok: value.length >= 12, label: '12 位以上' },
    { ok: /[a-z]/.test(value) && /[A-Z]/.test(value), label: '大小写字母' },
    { ok: /\d/.test(value), label: '数字' },
    { ok: /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(value), label: '特殊字符' },
  ];
  const score = checks.filter((item) => item.ok).length;
  return <div className="password-strength"><div><span style={{ width: `${score / checks.length * 100}%` }} /></div><ul>{checks.map((item) => <li key={item.label} className={item.ok ? 'active' : ''}>{item.label}</li>)}</ul></div>;
}

function LoginPage() {
  const [theme, setTheme] = useState<ThemeKey>(getInitialTheme);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [remember7d, setRemember7d] = useState(() => window.localStorage.getItem('openvpn-remember7d') === '1');
  const [passcode, setPasscode] = useState('');
  const [mode, setMode] = useState<'login' | 'mfa' | 'first-password'>('login');
  const [loginError, setLoginError] = useState('');
  const [errors, setErrors] = useState<FieldErrors>({});
  const [saving, setSaving] = useState(false);
  const [pendingUser, setPendingUser] = useState<ClientUserInfo>();
  const [currentPass, setCurrentPass] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newPasswordAgain, setNewPasswordAgain] = useState('');

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);

  useEffect(() => {
    function syncTheme(event: StorageEvent) {
      if (event.key !== 'openvpn-admin-theme') return;
      setTheme(getInitialTheme());
    }
    window.addEventListener('storage', syncTheme);
    return () => window.removeEventListener('storage', syncTheme);
  }, []);

  useEffect(() => {
    window.localStorage.setItem('openvpn-remember7d', remember7d ? '1' : '0');
  }, [remember7d]);

  function validateLogin() {
    const nextErrors: FieldErrors = {};
    if (!trimText(username)) nextErrors.username = '请输入 OpenVPN 账号';
    if (!password) nextErrors.password = '请输入登录密码';
    return nextErrors;
  }

  function buildLoginForm(extra: Record<string, unknown> = {}) {
    return { username: trimText(username), password, remember7d: remember7d ? 'on' : 'off', ...extra };
  }

  function handleLoginResult(result: LoginResult) {
    setLoginError('');
    if (result.user?.isFirstLogin) {
      setPendingUser(result.user);
      setMode('first-password');
      return;
    }
    if (result.redirect) {
      window.location.href = result.redirect;
      return;
    }
    if ((result.message || '').toUpperCase().includes('FA')) {
      setMode('mfa');
      return;
    }
    window.location.href = '/admin';
  }

  async function submitLogin(extra: Record<string, unknown> = {}) {
    const nextErrors = validateLogin();
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length) return;
    setSaving(true);
    setLoginError('');
    try {
      const result = await api.postForm<LoginResult>('/login', buildLoginForm(extra));
      handleLoginResult(result);
    } catch (error) {
      setLoginError(messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  function submitMfa(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!/^\d{6}$/.test(trimText(passcode))) {
      setErrors({ passcode: '请输入 6 位动态验证码' });
      return;
    }
    setErrors({});
    void submitLogin({ passcode: trimText(passcode) });
  }

  async function submitFirstPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextErrors: FieldErrors = {};
    if (!currentPass) nextErrors.currentPass = '请输入当前密码';
    if (!isStrongPassword(newPassword)) nextErrors.newPassword = '新密码需至少 12 位，包含大小写字母、数字和特殊字符';
    if (newPassword !== newPasswordAgain) nextErrors.newPasswordAgain = '两次输入的新密码不一致';
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length || !pendingUser) return;
    setSaving(true);
    setLoginError('');
    try {
      await api.postForm('/client/modifyPass', { id: pendingUser.id, currentPass, password: newPassword, isFirstLogin: false });
      window.location.href = '/';
    } catch (error) {
      setLoginError(messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return <main className="reference-login-shell">
    <div className="reference-login-mesh" aria-hidden="true" />
    <div className="reference-login-stars" aria-hidden="true"><i /><i /><i /><i /><i /><i /><i /><i /></div>
    <aside className="reference-login-brand">
      <div className="reference-login-gradient" />
      <div className="reference-login-glow glow-a" />
      <div className="reference-login-glow glow-b" />
      <div className="reference-login-glow glow-c" />
      <div className="reference-login-orbit" aria-hidden="true"><HeroOrbitScene /></div>
      <div className="reference-brand-content">
        <div className="reference-brand-logo"><div className="reference-brand-mark">OV</div><div><strong>OpenVPN</strong><span>Secure Console</span></div></div>
        <section className="reference-brand-copy">
          <span className="reference-chip">OpenVPN Secure Gateway</span>
          <h1>优雅地管理<br /><em>你的 VPN 访问网络</em></h1>
          <p>统一管理账号、客户端、分组、防火墙和消息告警，让安全接入更清晰、更可靠、更适合日常运维。</p>
          <div className="reference-stats">
            <div><strong>MFA</strong><span>动态口令认证</span></div>
            <div><strong>Notify</strong><span>上线下线通知</span></div>
            <div><strong>Audit</strong><span>操作留痕审计</span></div>
          </div>
        </section>
        <div className="reference-brand-badges"><span>实时在线监控</span><span>Webhook 告警</span><span>证书生命周期</span></div>
        <footer>OpenVPN Web Admin · Local Secure Operations</footer>
      </div>
    </aside>
    <section className="reference-login-form">
      <div className="reference-mobile-bg" />
      <div className="reference-login-card">
        <div className="reference-card-heading">
          <div className="reference-card-icon" aria-hidden="true"><Lock size={22} color="#fff" /></div>
          <div><strong>{mode === 'first-password' ? '首次登录' : mode === 'mfa' ? '安全验证' : '欢迎回来'}</strong><span>{saving ? '处理中...' : mode === 'mfa' ? '请完成 MFA 验证' : '请使用管理员账号登录'}</span></div>
        </div>
        {mode === 'login' && <form className="form-grid one-column reference-login-fields" noValidate onSubmit={(event) => { event.preventDefault(); void submitLogin(); }}>
          <TextInput label="账号" value={username} onChange={setUsername} error={errors.username} required autoFocus placeholder="请输入 OpenVPN 管理账号" icon="user" />
          <TextInput label="密码" value={password} type="password" onChange={setPassword} error={errors.password} required placeholder="请输入登录密码" icon="lock" />
          <CheckInput label="7 天内保持登录" checked={remember7d} onChange={setRemember7d} />
          {loginError && <div className="inline-error reference-login-error">{loginError}</div>}
          <div className="modal-actions wide-field reference-login-actions"><button type="submit" disabled={saving}>{saving ? '登录中...' : '登 录'}</button></div>
        </form>}
        {mode === 'mfa' && <form className="form-grid one-column reference-login-fields" noValidate onSubmit={submitMfa}>
          <TextInput label="MFA 动态验证码" value={passcode} onChange={setPasscode} error={errors.passcode} required autoFocus placeholder="请输入 6 位动态验证码" icon="key" />
          <p className="modal-hint">请输入认证器 App 中当前 6 位动态验证码。</p>
          {loginError && <div className="inline-error reference-login-error">{loginError}</div>}
          <div className="modal-actions wide-field reference-login-actions"><button type="button" className="ghost-action" onClick={() => setMode('login')}>返回登录</button><button type="submit" disabled={saving}>{saving ? '验证中...' : '完成验证'}</button></div>
        </form>}
        {mode === 'first-password' && <form className="form-grid one-column reference-login-fields" noValidate onSubmit={submitFirstPassword}>
          <TextInput label="当前密码" value={currentPass} type="password" onChange={setCurrentPass} error={errors.currentPass} required autoFocus placeholder="请输入当前密码" icon="lock" />
          <TextInput label="新密码" value={newPassword} type="password" onChange={setNewPassword} error={errors.newPassword} required placeholder="至少 12 位强密码" icon="shield" />
          <PasswordStrength value={newPassword} />
          <TextInput label="确认新密码" value={newPasswordAgain} type="password" onChange={setNewPasswordAgain} error={errors.newPasswordAgain} required placeholder="请再次输入新密码" icon="lock" />
          {loginError && <div className="inline-error reference-login-error">{loginError}</div>}
          <div className="modal-actions wide-field reference-login-actions"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存并进入门户'}</button></div>
        </form>}
      </div>
    </section>
  </main>;
}

function ClientPortalPage() {
  const [theme, setTheme] = useState<ThemeKey>(getInitialTheme);
  const [userInfo, setUserInfo] = useState<ClientUserInfo>();
  const [mfaState, setMfaState] = useState<ClientMfaResponse>();
  const [qrCode, setQrCode] = useState('');
  const [currentPass, setCurrentPass] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newPasswordAgain, setNewPasswordAgain] = useState('');
  const [passcode, setPasscode] = useState('');
  const [notify, setNotify] = useState('');
  const [errors, setErrors] = useState<FieldErrors>({});
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const info = await api.get<ClientUserInfo>('/client/userinfo');
        setUserInfo(info);
        const mfa = await api.get<ClientMfaResponse>('/client/mfa');
        setMfaState(mfa);
        if (!mfa.mfaEnable) {
          const otp = `otpauth://totp/openvpn-web:${encodeURIComponent(info.username)}?algorithm=SHA1&digits=6&period=30&secret=${encodeURIComponent(mfa.user.mfaSecret)}&issuer=openvpn-web`;
          setQrCode(await QRCode.toDataURL(otp));
        }
      } catch (error) {
        setNotify(messageOf(error));
      }
    })();
  }, []);

  async function downloadConfig() {
    try {
      const data = await api.get<ClientConfigResponse>('/client/userConfig');
      const blob = new Blob([data.content], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = data.filename || 'client.ovpn';
      anchor.click();
      URL.revokeObjectURL(url);
      setNotify('配置文件已开始下载');
    } catch (error) {
      setNotify(messageOf(error));
    }
  }

  async function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextErrors: FieldErrors = {};
    if (!currentPass) nextErrors.currentPass = '请输入当前密码';
    if (!isStrongPassword(newPassword)) nextErrors.newPassword = '新密码需至少 12 位，包含大小写字母、数字和特殊字符';
    if (newPassword !== newPasswordAgain) nextErrors.newPasswordAgain = '两次密码输入不一致';
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length || !userInfo) return;
    setSaving(true);
    try {
      await api.postForm('/client/modifyPass', { id: userInfo.id, currentPass, password: newPassword, isFirstLogin: false });
      setNotify('密码已修改');
      setCurrentPass('');
      setNewPassword('');
      setNewPasswordAgain('');
    } catch (error) {
      setNotify(messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  async function submitMfa(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextErrors: FieldErrors = {};
    if (!mfaState?.user.mfaSecret) nextErrors.passcode = '请先生成 MFA 密钥';
    if (!/^\d{6}$/.test(trimText(passcode))) nextErrors.passcode = '请输入 6 位动态验证码';
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length || !userInfo || !mfaState) return;
    setSaving(true);
    try {
      if (mfaState.mfaEnable) {
        await api.delete(`/client/mfa/${userInfo.id}`);
        setNotify('MFA 已关闭');
      } else {
        await api.postForm('/client/mfa', { id: userInfo.id, mfaSecret: mfaState.user.mfaSecret, passcode: trimText(passcode) });
        setNotify('MFA 已启用');
      }
    } catch (error) {
      setNotify(messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  return <ImmersiveShell theme={theme} setTheme={setTheme} compact><section className="client-portal"><div className="portal-hero glass-panel"><span className="chip">OpenVPN Client Portal</span><h1>{userInfo?.name || userInfo?.username || runtime.sysUser || '用户'}，欢迎回到专属空间</h1><p>这里可以下载配置、管理 MFA、修改密码，并快速进入各平台客户端。</p><div className="client-downloads"><button className="primary-action" type="button" onClick={downloadConfig}>下载配置文件</button><a className="ghost-action" href="/logout">退出登录</a></div><div className="portal-links"><a href={runtime.clientUrls?.windows || '#'} target="_blank" rel="noreferrer">Windows 客户端</a><a href={runtime.clientUrls?.macos || '#'} target="_blank" rel="noreferrer">macOS 客户端</a><a href={runtime.clientUrls?.linux || '#'} target="_blank" rel="noreferrer">Linux 客户端</a><a href={runtime.clientUrls?.ios || '#'} target="_blank" rel="noreferrer">iOS 客户端</a><a href={runtime.clientUrls?.android || '#'} target="_blank" rel="noreferrer">Android 客户端</a></div></div><div className="client-grid"><section className="glass-panel compact-card"><div className="section-heading"><span>安全设置</span><h2>MFA 与密码</h2></div>{notify && <div className="inline-error">{notify}</div>}<form className="form-grid one-column" onSubmit={submitPassword} noValidate><TextInput label="当前密码" value={currentPass} type="password" onChange={setCurrentPass} error={errors.currentPass} required /><TextInput label="新密码" value={newPassword} type="password" onChange={setNewPassword} error={errors.newPassword} required /><PasswordStrength value={newPassword} /><TextInput label="确认新密码" value={newPasswordAgain} type="password" onChange={setNewPasswordAgain} error={errors.newPasswordAgain} required /><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存密码'}</button></div></form><div className="mfa-panel">{mfaState?.mfaEnable ? <><strong>当前已启用 MFA</strong><p>如需停用请先完成一次验证。</p><form onSubmit={submitMfa} className="inline-form"><TextInput label="动态验证码" value={passcode} onChange={setPasscode} error={errors.passcode} required /><button type="submit" disabled={saving}>关闭 MFA</button></form></> : <><strong>当前未启用 MFA</strong><p>请扫描二维码完成绑定。</p>{qrCode && <div className="mfa-qr"><img src={qrCode} alt="MFA 二维码" /><code>{mfaState?.user.mfaSecret}</code></div>}<form onSubmit={submitMfa} className="inline-form"><TextInput label="动态验证码" value={passcode} onChange={setPasscode} error={errors.passcode} required /><button type="submit" disabled={saving}>启用 MFA</button></form></>}</div></section><section className="glass-panel compact-card"><div className="section-heading"><span>客户端下载</span><h2>平台入口</h2></div><div className="download-grid">{[ { label: 'Windows', href: runtime.clientUrls?.windows }, { label: 'macOS', href: runtime.clientUrls?.macos }, { label: 'Linux', href: runtime.clientUrls?.linux }, { label: 'iOS', href: runtime.clientUrls?.ios }, { label: 'Android', href: runtime.clientUrls?.android } ].map((item) => <a key={item.label} href={item.href || '#'} target="_blank" rel="noreferrer"><strong>{item.label}</strong><span>{item.href ? '立即下载 / 打开' : '未配置链接'}</span></a>)}</div></section></div></section></ImmersiveShell>;
}

function Overview({ onlineState, dashboardState, users, clients, settings, notify, openModal, reload, confirmAction }: { onlineState: AsyncState<OnlineResponse>; dashboardState: AsyncState<DashboardSummary>; users: UserRecord[]; clients: ClientRecord[]; settings?: SettingsResponse; notify: (type: Toast['type'], message: string) => void; openModal: (modal: ModalState) => void; reload: () => void; confirmAction: (state: ConfirmState) => void }) {
  const online = onlineState.data?.clients || [];
  const [onlineSearch, setOnlineSearch] = useState('');
  const activeUsers = users.filter((user) => user.isEnable !== false).length;
  const dashboard = dashboardState.data;
  const stats = dashboard?.stats;
  const risks = dashboard?.risks || [];
  const filteredOnline = online.filter((client) => {
    const keyword = onlineSearch.toLowerCase().trim();
    if (!keyword) return true;
    return [getClientName(client), client.vip, client.vip6, client.rip, client.rip6, client.commonName, client.common_name].filter(Boolean).some((value) => String(value).toLowerCase().includes(keyword));
  });
  const onlinePagination = usePagination(filteredOnline, onlineSearch);

  useEffect(() => {
    const timer = window.setInterval(reload, 10000);
    return () => window.clearInterval(timer);
  }, [reload]);

  async function serverAction(action: string) {
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', { action });
      notify('success', result.message || '操作成功');
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function editServerConfig() {
    try {
      const result = await api.postForm<{ content: string }>('/ovpn/server', { action: 'getConfig' });
      openModal({ type: 'server-config', content: result.content || '' });
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function killClient(client: OnlineClient) {
    confirmAction({ title: '断开在线连接', message: `确认断开 ${getClientName(client)} 吗？该用户需要重新连接 VPN。`, danger: true, onConfirm: async () => {
      await api.postForm('/ovpn/kill', { cid: client.id || client.cid });
      notify('success', '客户端已断开');
      reload();
    } });
  }

  async function setBlacklist(client: OnlineClient, action: 'add_blacklist' | 'remove_blacklist') {
    try {
      const result = await api.postForm<{ message: string }>(`/ovpn/firewall?a=${action}`, { vip: clientVips(client) });
      notify('success', result.message || '操作成功');
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function openRateLimit(client: OnlineClient) {
    try {
      const rate = await api.get<{ upQos?: { rate?: string; unit?: string }; downQos?: { rate?: string; unit?: string } }>(`/ovpn/firewall?a=get_rateLimit&vip=${encodeURIComponent(client.vip || client.vip6 || '')}`);
      openModal({ type: 'rate-limit', client, upload: rate.upQos?.rate || '', uploadUnit: rate.upQos?.unit || 'mbytes/second', download: rate.downQos?.rate || '', downloadUnit: rate.downQos?.unit || 'mbytes/second' });
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  return <div className="overview-grid">
    <div className="hero-card glass-panel"><div className="hero-copy"><span className="chip">OpenVPN Secure Console</span><h1>OpenVPN 统一运维控制台，守护你的网络边界。</h1><p>账号、客户端、防火墙、连接历史、证书与系统设置已统一接入，日常 VPN 管理都可以在这里完成。清爽一点，也稳一点。</p><div className="hero-actions"><button className="primary-action" type="button" onClick={() => serverAction('restartSrv')}>重启 OpenVPN</button><button className="ghost-action" type="button" onClick={editServerConfig}>编辑 server.conf</button></div></div><div className="hero-orbit" aria-hidden="true"><HeroOrbitScene /></div></div>
    <div className="stats-row"><StatCard label="在线连接" value={stats?.onlineClients ?? online.length} trend={onlineState.error ? '本地未连接 management' : '10 秒自动刷新'} /><StatCard label="账号总数" value={stats?.totalUsers ?? users.length} trend={`${stats?.enabledUsers ?? activeUsers} 个启用账号`} /><StatCard label="客户端配置" value={stats?.clientConfigs ?? clients.length} trend="证书与 CCD 管理" /><StatCard label="今日上线" value={stats?.todayConnections ?? 0} trend={`24h ${stats?.bytesReceived24h || '0 B'} / ${stats?.bytesSent24h || '0 B'}`} /></div>
    {risks.length > 0 && <section className="risk-grid">{risks.map((risk, index) => <div key={`${risk.title}-${index}`} className={`risk-card ${risk.level}`}><strong>{risk.title}</strong><span>{risk.message}</span></div>)}</section>}
    {dashboard && <section className="glass-panel compact-card"><div className="section-heading row-heading"><div><span>Ops Analytics</span><h2>近 24 小时趋势</h2></div><small>{dashboardState.error || '连接数、流量与 Top 用户聚合统计'}</small></div><div className="chart-grid"><div className="trend-bars">{dashboard.trends.map((point) => { const max = Math.max(1, ...dashboard.trends.map((item) => item.connections)); return <span key={point.hour} title={`${point.hour} · ${point.connections} 次`} style={{ height: `${Math.max(8, point.connections / max * 100)}%` }} />; })}</div><div className="top-users">{dashboard.topUsers.length ? dashboard.topUsers.map((user) => <div key={user.username}><span>{user.username}</span><strong>{user.text}</strong></div>) : <p>暂无 24 小时流量记录</p>}</div></div></section>}
    {onlineState.data?.server && <section className="glass-panel compact-card"><div className="section-heading row-heading"><div><span>Server Runtime</span><h2>服务状态</h2></div><button className="mini-button" type="button" onClick={reload}>刷新</button></div><div className="kv-grid"><div><span>地址</span><strong>{onlineState.data.server.Address || '-'}</strong></div><div><span>状态</span><strong>{onlineState.data.server.Status || '-'}</strong></div><div><span>入站</span><strong>{onlineState.data.server.BytesIn || '-'}</strong></div><div><span>出站</span><strong>{onlineState.data.server.BytesOut || '-'}</strong></div><div><span>运行时间</span><strong>{onlineState.data.server.RunDate || '-'}</strong></div></div></section>}
    <section className="glass-panel table-card"><div className="section-heading row-heading"><div><span>Live Tunnel</span><h2>在线连接</h2></div><Toolbar><input className="toolbar-input" value={onlineSearch} placeholder="搜索用户 / VPN IP / 来源 IP" onChange={(event) => setOnlineSearch(event.target.value)} /><button className="mini-button" type="button" onClick={reload}>手动刷新</button></Toolbar></div>{online.length ? filteredOnline.length ? <><div className="responsive-table"><table><thead><tr><th>用户/客户端</th><th>VPN IP</th><th>来源</th><th>接收</th><th>发送</th><th>上线时间</th><th>在线时长</th><th>操作</th></tr></thead><tbody>{onlinePagination.pagedItems.map((client, index) => <tr key={`${client.id || client.cid || onlinePagination.start + index}`}><td>{getClientName(client)}</td><td>{client.vip || client.vip6 || '-'}</td><td>{client.rip || client.rip6 || '-'}</td><td>{formatBytes(getClientBytes(client, 'received'))}</td><td>{formatBytes(getClientBytes(client, 'sent'))}</td><td>{client.connDate || client.connectedSince || client.connected_since || '-'}</td><td>{client.onlineTime || '-'}</td><td><div className="row-actions"><button type="button" onClick={() => killClient(client)}>断开</button><button type="button" onClick={() => openRateLimit(client)}>限速</button><button type="button" onClick={() => setBlacklist(client, client.isNftBlacklist || client.isNftBlackList ? 'remove_blacklist' : 'add_blacklist')}>{client.isNftBlacklist || client.isNftBlackList ? '解网' : '禁网'}</button></div></td></tr>)}</tbody></table></div><PaginationBar page={onlinePagination.page} pageSize={onlinePagination.pageSize} pageCount={onlinePagination.pageCount} total={onlinePagination.total} start={onlinePagination.start} end={onlinePagination.end} onPageChange={onlinePagination.setPage} onPageSizeChange={onlinePagination.setPageSize} /></> : <EmptyState title="没有匹配的在线连接" description="换个用户名、VPN IP 或来源 IP 再试试。" /> : <EmptyState title="暂无在线客户端" description="本地只启动 Web 服务时，这是正常现象；Docker 完整环境会显示真实连接。" />}</section>
  </div>;
}

function UsersPanel({ groups, selectedGroupId, setSelectedGroupId, usersState, clients, notify, reload, openModal, confirmAction }: { groups: GroupRecord[]; selectedGroupId: number; setSelectedGroupId: (id: number) => void; usersState: AsyncState<{ users: UserRecord[]; authUser?: boolean }>; clients: ClientRecord[]; notify: (type: Toast['type'], message: string) => void; reload: () => void; openModal: (modal: ModalState) => void; confirmAction: (state: ConfirmState) => void }) {
  const users = usersState.data?.users || [];
  const tree = buildTree(groups);
  const [selectedUserIds, setSelectedUserIds] = useState<number[]>([]);
  const [userSearch, setUserSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [mfaFilter, setMfaFilter] = useState('all');
  const [expireFilter, setExpireFilter] = useState('all');
  const filteredUsers = users.filter((user) => {
    const keyword = userSearch.toLowerCase().trim();
    const matchesKeyword = !keyword || [user.username, user.name, user.email, user.ipAddr, user.ovpnConfig].filter(Boolean).some((value) => String(value).toLowerCase().includes(keyword));
    const matchesStatus = statusFilter === 'all' || (statusFilter === 'enabled' ? user.isEnable !== false : user.isEnable === false);
    const matchesMfa = mfaFilter === 'all' || (mfaFilter === 'enabled' ? Boolean(user.mfaSecret) : !user.mfaSecret);
    const matchesExpire = expireFilter === 'all' || (expireFilter === 'expired' ? isUserExpired(user) : expireFilter === 'expiring' ? isUserExpiring(user) : !isUserExpired(user) && !isUserExpiring(user));
    return matchesKeyword && matchesStatus && matchesMfa && matchesExpire;
  });
  const userPagination = usePagination(filteredUsers, `${selectedGroupId}|${userSearch}|${statusFilter}|${mfaFilter}|${expireFilter}`);
  const visibleIds = userPagination.pagedItems.map((user) => user.id).filter((id): id is number => Boolean(id));
  const selectedUsers = users.filter((user) => user.id && selectedUserIds.includes(user.id));
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedUserIds.includes(id));

  useEffect(() => {
    setSelectedUserIds((ids) => ids.filter((id) => users.some((user) => user.id === id)));
  }, [users]);

  async function patchUser(user: UserRecord, form: Record<string, unknown>, success: string) {
    try {
      const result = await api.patchForm<{ message: string }>('/ovpn/user', { id: user.id, ...form });
      notify('success', result.message || success);
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function deleteUser(user: UserRecord) {
    confirmAction({ title: '删除 VPN 用户', message: `确认删除用户 ${user.username} 吗？该操作不可恢复。`, danger: true, onConfirm: async () => {
      const result = await api.delete<{ message: string }>(`/ovpn/user/${user.id}`);
      notify('success', result.message || '删除成功');
      reload();
    } });
  }

  async function resetMfa(user: UserRecord) {
    confirmAction({ title: '重置 MFA', message: `确认重置 ${user.username} 的 MFA 吗？用户下次需要重新绑定。`, danger: true, onConfirm: async () => {
      await api.delete(`/client/mfa/${user.id}`);
      notify('success', 'MFA 已重置');
      reload();
    } });
  }

  async function toggleAuthUser(checked: boolean) {
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/server', { action: 'settings', key: 'auth-user', value: checked });
      notify('success', result.message || '账号认证设置已更新');
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function deleteGroup(group: GroupRecord) {
    if (group.id === 1) return notify('error', '默认组不能删除');
    const childGroups = groups.filter((item) => item.parent_id === group.id);
    if (childGroups.length > 0) {
      notify('error', `分组「${group.name}」下还有 ${childGroups.length} 个子分组，请先迁移或删除子分组`);
      return;
    }
    if (usersState.loading) {
      notify('info', '正在读取分组用户，请稍后再删除');
      return;
    }
    if (users.length > 0) {
      notify('error', `分组「${group.name}」下还有 ${users.length} 个用户，请先迁移用户到其他分组`);
      return;
    }
    confirmAction({ title: '删除用户组', message: `确认删除分组 ${group.name} 吗？请先确认该分组下账号和策略已迁移。`, danger: true, onConfirm: async () => {
      const result = await api.delete<{ message: string }>(`/ovpn/group/${group.id}`);
      notify('success', result.message || '删除成功');
      setSelectedGroupId(1);
      reload();
    } });
  }

  function toggleSelectedUser(user: UserRecord, checked: boolean) {
    if (!user.id) return;
    setSelectedUserIds((ids) => checked ? [...new Set([...ids, user.id!])] : ids.filter((id) => id !== user.id));
  }

  function toggleVisibleUsers(checked: boolean) {
    setSelectedUserIds((ids) => checked ? [...new Set([...ids, ...visibleIds])] : ids.filter((id) => !visibleIds.includes(id)));
  }

  function batchAction(title: string, message: string, action: (user: UserRecord) => Promise<unknown>, success: string, danger = false) {
    if (!selectedUsers.length) {
      notify('error', '请先选择要操作的账号');
      return;
    }
    confirmAction({ title, message: `${message}（已选择 ${selectedUsers.length} 个账号）`, danger, onConfirm: async () => {
      for (const user of selectedUsers) {
        await action(user);
      }
      notify('success', success);
      setSelectedUserIds([]);
      reload();
    } });
  }

  async function importUsers(file?: File) {
    if (!file) return;
    try {
      const form = new FormData();
      form.set('file', file);
      form.set('gid', String(selectedGroupId));
      const result = await api.multipart<{ message: string }>('/ovpn/user', form);
      notify('success', result.message || '导入成功');
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  function exportSelectedUsers() {
    if (!selectedUsers.length) {
      notify('error', '请先选择要导出的账号');
      return;
    }
    const header = ['username', 'name', 'email', 'ipAddr', 'ovpnConfig', 'expireDate', 'isEnable', 'mfa'];
    const rows = selectedUsers.map((user) => [user.username, user.name || '', user.email || '', user.ipAddr || '', user.ovpnConfig || '', user.expireDate || '', user.isEnable === false ? 'disabled' : 'enabled', user.mfaSecret ? 'enabled' : 'disabled']);
    const csv = [header, ...rows].map((row) => row.map((value) => `"${String(value).replace(/"/g, '""')}"`).join(',')).join('\n');
    const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `openvpn-users-${Date.now()}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  }

  const selectedGroup = groups.find((item) => item.id === selectedGroupId);
  return <div className="split-layout"><aside className="glass-panel tree-panel"><div className="section-heading row-heading"><div><span>Group Tree</span><h2>用户组</h2></div><button className="mini-button" type="button" onClick={() => openModal({ type: 'group-form', mode: 'add', parentGroup: selectedGroup })}>新增</button></div><div className="tree-list">{tree.map((group) => <button key={group.id} className={selectedGroupId === group.id ? 'active' : ''} style={{ paddingLeft: 14 + group.depth * 18 }} type="button" onClick={() => setSelectedGroupId(group.id)}><span className="tree-glyph" aria-hidden="true"><Circle size={8} fill="currentColor" strokeWidth={0} /></span><span className="tree-name" title={group.name}>{group.name}</span></button>)}</div>{selectedGroup && <div className="panel-actions vertical-actions"><button type="button" onClick={() => openModal({ type: 'group-form', mode: 'edit', group: selectedGroup })}>编辑分组</button><button type="button" onClick={() => openModal({ type: 'group-config', group: selectedGroup, content: selectedGroup.config || '' })}>组配置</button><button type="button" onClick={() => deleteGroup(selectedGroup)}>删除分组</button></div>}</aside><section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Identity Matrix</span><h2>VPN 账号</h2></div><Toolbar><label className="upload-button">导入 CSV<input type="file" accept=".csv" onChange={(event) => importUsers(event.target.files?.[0])} /></label><a className="mini-button" href="/user/template">模板</a><a className="mini-button" href={`/ovpn/user/export?gid=${selectedGroupId}`}>导出分组</a><button type="button" onClick={() => openModal({ type: 'user-form', mode: 'add' })}>添加用户</button></Toolbar></div><div className="switch-line inline-switch"><div><strong>账号密码认证</strong><span>控制 auth-user-pass-verify 认证开关</span></div><Toggle checked={Boolean(usersState.data?.authUser)} onChange={toggleAuthUser} /></div><div className="filter-bar"><input className="toolbar-input" value={userSearch} placeholder="搜索账号 / 姓名 / 邮箱 / 固定 IP" onChange={(event) => setUserSearch(event.target.value)} /><CompactSelect value={statusFilter} options={userStatusOptions} onChange={setStatusFilter} /><CompactSelect value={mfaFilter} options={mfaFilterOptions} onChange={setMfaFilter} /><CompactSelect value={expireFilter} options={expireFilterOptions} onChange={setExpireFilter} /></div>{selectedUserIds.length > 0 && <div className="batch-bar"><strong>已选择 {selectedUserIds.length} 个账号</strong><button type="button" onClick={() => batchAction('批量启用账号', '确认启用这些账号吗？', (user) => api.patchForm('/ovpn/user', { id: user.id, isEnable: true }), '批量启用完成')}>启用</button><button type="button" onClick={() => batchAction('批量禁用账号', '确认禁用这些账号吗？', (user) => api.patchForm('/ovpn/user', { id: user.id, isEnable: false }), '批量禁用完成', true)}>禁用</button><button type="button" onClick={() => batchAction('批量重置 MFA', '确认重置这些账号的 MFA 吗？', (user) => api.delete(`/client/mfa/${user.id}`), '批量 MFA 重置完成', true)}>重置 MFA</button><button type="button" onClick={exportSelectedUsers}>导出选中</button><button type="button" onClick={() => batchAction('批量删除账号', '确认删除这些账号吗？该操作不可恢复。', (user) => api.delete(`/ovpn/user/${user.id}`), '批量删除完成', true)}>删除</button></div>}{users.length ? filteredUsers.length ? <><div className="responsive-table"><table><thead><tr><th><input type="checkbox" checked={allVisibleSelected} onChange={(event) => toggleVisibleUsers(event.target.checked)} /></th><th>账号</th><th>姓名</th><th>邮箱</th><th>固定 IP</th><th>配置文件</th><th>MFA</th><th>状态</th><th>有效期</th><th>操作</th></tr></thead><tbody>{userPagination.pagedItems.map((user) => { const expire = expiryStatus(user); return <tr key={user.id || user.username}><td><input type="checkbox" checked={Boolean(user.id && selectedUserIds.includes(user.id))} onChange={(event) => toggleSelectedUser(user, event.target.checked)} /></td><td>{user.username}</td><td>{user.name || '-'}</td><td>{user.email || '-'}</td><td>{user.ipAddr || '-'}</td><td>{user.ovpnConfig || '-'}</td><td>{user.mfaSecret ? '开启' : '-'}</td><td><span className={`status-pill ${user.isEnable === false ? 'danger' : 'success'}`}>{user.isEnable === false ? '禁用' : '启用'}</span></td><td><span className={`status-pill ${expire.className}`}>{user.expireDate || expire.label}</span></td><td><div className="row-actions"><button type="button" onClick={() => openModal({ type: 'user-form', mode: 'edit', user })}>编辑</button><button type="button" onClick={() => patchUser(user, { isEnable: user.isEnable === false }, '状态已更新')}>{user.isEnable === false ? '启用' : '禁用'}</button><button type="button" onClick={() => openModal({ type: 'reset-password', user })}>重置密码</button><button type="button" onClick={() => resetMfa(user)}>重置 MFA</button><button type="button" onClick={() => deleteUser(user)}>删除</button></div></td></tr>; })}</tbody></table></div><PaginationBar page={userPagination.page} pageSize={userPagination.pageSize} pageCount={userPagination.pageCount} total={userPagination.total} start={userPagination.start} end={userPagination.end} onPageChange={userPagination.setPage} onPageSizeChange={userPagination.setPageSize} /></> : <EmptyState title="没有匹配的 VPN 账号" description="调整关键词、状态、MFA 或有效期筛选后再试。" /> : <EmptyState title="当前分组暂无用户" description={clients.length ? '点击添加用户即可绑定客户端配置。' : '可以先添加客户端配置，再创建账号。'} />}</section></div>;
}

function ClientsPanel({ clients, settings, notify, reload, openModal, confirmAction }: { clients: ClientRecord[]; settings?: SettingsResponse; notify: (type: Toast['type'], message: string) => void; reload: () => void; openModal: (modal: ModalState) => void; confirmAction: (state: ConfirmState) => void }) {
  const [search, setSearch] = useState('');
  const filteredClients = clients.filter((client) => !search.trim() || [client.name, client.fullName, client.file].filter(Boolean).some((value) => String(value).toLowerCase().includes(search.toLowerCase().trim())));
  const clientPagination = usePagination(filteredClients, search);

  async function deleteClient(client: ClientRecord) {
    confirmAction({ title: '删除客户端配置', message: `确认删除客户端 ${client.name} 吗？这会吊销证书并删除配置文件。`, danger: true, onConfirm: async () => {
      const result = await api.delete<{ message: string }>(`/ovpn/client/${encodeURIComponent(client.name)}`);
      notify('success', result.message || '删除成功');
      reload();
    } });
  }

  async function openEditor(client: ClientRecord, editor: 'config' | 'ccd') {
    try {
      const result = await api.get<{ content: string }>(`/ovpn/client/${encodeURIComponent(client.name)}/${editor}`);
      openModal({ type: 'client-editor', client, editor, content: result.content || '' });
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function copyClientFile(client: ClientRecord) {
    if (!client.file) {
      notify('error', '当前客户端没有可复制的下载地址');
      return;
    }
    if (!navigator.clipboard) {
      notify('error', '当前浏览器不支持剪贴板复制');
      return;
    }
    try {
      await navigator.clipboard.writeText(client.file);
      notify('success', '下载地址已复制');
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  return <section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Device Fabric</span><h2>客户端配置</h2></div><Toolbar><input className="toolbar-input" value={search} placeholder="搜索客户端名称" onChange={(event) => setSearch(event.target.value)} /><button type="button" className="mini-button" onClick={() => openModal({ type: 'client-form' })}>添加客户端</button></Toolbar></div><div className="client-downloads">{settings && Object.entries(settings.client.client_url).map(([key, url]) => <a key={key} href={String(url)} target="_blank" rel="noreferrer">{key.toUpperCase()} 客户端</a>)}</div>{clients.length ? filteredClients.length ? <><div className="responsive-table"><table><thead><tr><th>名称</th><th>文件</th><th>更新时间</th><th>操作</th></tr></thead><tbody>{clientPagination.pagedItems.map((client) => <tr key={client.name}><td>{client.name}</td><td>{client.fullName || client.file || '-'}</td><td>{client.date || '-'}</td><td><div className="row-actions">{client.file && <a href={client.file} download={client.fullName}>下载</a>}<button type="button" onClick={() => copyClientFile(client)}>复制地址</button><button type="button" onClick={() => openEditor(client, 'config')}>配置</button><button type="button" onClick={() => openEditor(client, 'ccd')}>CCD</button><button type="button" onClick={() => deleteClient(client)}>删除</button></div></td></tr>)}</tbody></table></div><PaginationBar page={clientPagination.page} pageSize={clientPagination.pageSize} pageCount={clientPagination.pageCount} total={clientPagination.total} start={clientPagination.start} end={clientPagination.end} onPageChange={clientPagination.setPage} onPageSizeChange={clientPagination.setPageSize} /></> : <EmptyState title="没有匹配的客户端配置" description="换个客户端名称或配置文件关键词再试。" /> : <EmptyState title="还没有客户端" description="点击添加客户端生成 .ovpn 配置；本地非 Docker 环境可能缺少 easyrsa。" />}</section>;
}

function FirewallPanel({ firewalls, groups, notify, reload, openModal, confirmAction }: { firewalls: FirewallRecord[]; groups: GroupRecord[]; notify: (type: Toast['type'], message: string) => void; reload: () => void; openModal: (modal: ModalState) => void; confirmAction: (state: ConfirmState) => void }) {
  const groupNames = (items?: GroupRecord[]) => items?.map((item) => item.name).join(' / ') || '';
  const firewallPagination = usePagination(firewalls, String(firewalls.length));

  async function patchFirewall(firewall: FirewallRecord, status: boolean) {
    try {
      const result = await api.patchForm<{ message: string }>('/ovpn/firewall', { id: firewall.id, sip: firewall.sip || '', dip: firewall.dip || '', policy: firewall.policy, comment: firewall.comment || '', status });
      notify('success', result.message || '更新成功');
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    }
  }

  async function deleteFirewall(firewall: FirewallRecord) {
    confirmAction({ title: '删除防火墙规则', message: `确认删除防火墙规则 #${firewall.id} 吗？`, danger: true, onConfirm: async () => {
      const result = await api.delete<{ message: string }>(`/ovpn/firewall/${firewall.id}`);
      notify('success', result.message || '删除成功');
      reload();
    } });
  }

  return <section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Policy Mesh</span><h2>防火墙规则</h2></div><button type="button" className="mini-button" onClick={() => openModal({ type: 'firewall-form', mode: 'add' })}>添加规则</button></div>{firewalls.length ? <><div className="responsive-table"><table><thead><tr><th>ID</th><th>源</th><th>目的</th><th>策略</th><th>状态</th><th>备注</th><th>操作</th></tr></thead><tbody>{firewallPagination.pagedItems.map((firewall) => <tr key={firewall.id}><td>{firewall.id}</td><td>{[firewall.sip, groupNames(firewall.sg)].filter(Boolean).join(' / ') || '-'}</td><td>{[firewall.dip, groupNames(firewall.dg)].filter(Boolean).join(' / ') || '-'}</td><td>{firewall.policy || '-'}</td><td><span className={`status-pill ${firewall.status === false ? 'danger' : 'success'}`}>{firewall.status === false ? '禁用' : '启用'}</span></td><td>{firewall.comment || '-'}</td><td><div className="row-actions"><button type="button" onClick={() => openModal({ type: 'firewall-form', mode: 'edit', firewall })}>编辑</button><button type="button" onClick={() => patchFirewall(firewall, firewall.status === false)}>{firewall.status === false ? '启用' : '禁用'}</button><button type="button" onClick={() => deleteFirewall(firewall)}>删除</button></div></td></tr>)}</tbody></table></div><PaginationBar page={firewallPagination.page} pageSize={firewallPagination.pageSize} pageCount={firewallPagination.pageCount} total={firewallPagination.total} start={firewallPagination.start} end={firewallPagination.end} onPageChange={firewallPagination.setPage} onPageSizeChange={firewallPagination.setPageSize} /></> : <EmptyState title="暂无防火墙规则" description={groups.length ? '点击添加规则，可选择 IP、用户组、允许/拒绝策略。' : '先创建用户组，再配置访问策略。'} />}</section>;
}

function HistoryPanel({ initial }: { initial: HistoryResponse }) {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [range, setRange] = useState(() => {
    const end = new Date();
    const start = new Date();
    start.setMonth(start.getMonth() - 1);
    return { start: start.toISOString().slice(0, 10), end: end.toISOString().slice(0, 10) };
  });
  const qt = useMemo(() => {
    const startTime = new Date(`${range.start}T00:00:00`).getTime() / 1000;
    const endTime = new Date(`${range.end}T23:59:59`).getTime() / 1000;
    return `${startTime},${endTime}`;
  }, [range]);
  const state = useAsync(() => api.get<HistoryResponse>(`/ovpn/history?draw=1&offset=0&limit=50&orderColumn=time_unix&order=desc&search=${encodeURIComponent(search)}&qt=${qt}`), [reloadKey, qt]);
  const rows = state.data?.data || initial.data || [];
  const historyPagination = usePagination(rows, `${search}|${qt}|${reloadKey}`);

  return <section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Telemetry Log</span><h2>连接历史</h2></div><Toolbar><DatePicker value={range.start} allowClear={false} onChange={(next) => setRange((value) => ({ ...value, start: next }))} /><DatePicker value={range.end} allowClear={false} onChange={(next) => setRange((value) => ({ ...value, end: next }))} /><input className="toolbar-input" value={search} placeholder="搜索用户/IP" onChange={(event) => setSearch(event.target.value)} /><button type="button" onClick={() => setReloadKey((value) => value + 1)}>查询</button><a className="mini-button" href={`/ovpn/history/export?qt=${qt}`}>导出</a></Toolbar></div>{state.error && <p className="inline-error">{state.error}</p>}{rows.length ? <><div className="responsive-table"><table><thead><tr><th>用户</th><th>客户端</th><th>VPN IP</th><th>来源 IP</th><th>下载</th><th>上传</th><th>上线时间</th><th>在线时长</th></tr></thead><tbody>{historyPagination.pagedItems.map((item, index) => <tr key={item.id || historyPagination.start + index}><td>{item.username || '-'}</td><td>{item.common_name || item.commonName || '-'}</td><td>{item.vip || item.vip6 || '-'}</td><td>{item.rip || item.rip6 || '-'}</td><td>{formatBytes(getClientBytes(item as unknown as OnlineClient, 'received') || Number(item.bytes_received ?? item.bytesReceived ?? 0))}</td><td>{formatBytes(getClientBytes(item as unknown as OnlineClient, 'sent') || Number(item.bytes_sent ?? item.bytesSent ?? 0))}</td><td>{item.time_unix ? new Date(item.time_unix * 1000).toLocaleString() : '-'}</td><td>{item.time_duration || '-'}</td></tr>)}</tbody></table></div><PaginationBar page={historyPagination.page} pageSize={historyPagination.pageSize} pageCount={historyPagination.pageCount} total={historyPagination.total} start={historyPagination.start} end={historyPagination.end} onPageChange={historyPagination.setPage} onPageSizeChange={historyPagination.setPageSize} /></> : <EmptyState title="暂无历史记录" description="客户端下线后，OpenVPN hook 会写入这里并触发通知。" />}</section>;
}

function CertsPanel({ certs, openModal }: { certs: CertRecord[]; openModal: (modal: ModalState) => void }) {
  const classOf = (status?: string) => status === '已过期' ? 'danger' : status === '即将过期' ? 'warning' : 'success';
  const certPagination = usePagination(certs, String(certs.length));
  return <section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Trust Center</span><h2>证书管理</h2></div><button type="button" className="mini-button" onClick={() => openModal({ type: 'renew-cert' })}>更新证书</button></div>{certs.length ? <><div className="responsive-table"><table><thead><tr><th>名称</th><th>类型</th><th>状态</th><th>颁发时间</th><th>过期时间</th><th>剩余天数</th></tr></thead><tbody>{certPagination.pagedItems.map((cert, index) => <tr key={`${cert.name}-${certPagination.start + index}`}><td>{cert.name}</td><td>{cert.type}</td><td><span className={`status-pill ${classOf(cert.status)}`}>{cert.status || '-'}</span></td><td>{cert.notBefore || '-'}</td><td>{cert.notAfter || '-'}</td><td>{cert.expiresIn ?? '-'}</td></tr>)}</tbody></table></div><PaginationBar page={certPagination.page} pageSize={certPagination.pageSize} pageCount={certPagination.pageCount} total={certPagination.total} start={certPagination.start} end={certPagination.end} onPageChange={certPagination.setPage} onPageSizeChange={certPagination.setPageSize} /></> : <EmptyState title="暂无证书信息" description="Docker 完整环境会挂载 EasyRSA 数据并展示证书状态。" />}</section>;
}

function AuditPanel() {
  const [operator, setOperator] = useState('');
  const [module, setModule] = useState('');
  const [action, setAction] = useState('');
  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const query = useMemo(() => {
    const params = new URLSearchParams({ limit: '100' });
    if (operator.trim()) params.set('operator', operator.trim());
    if (module) params.set('module', module);
    if (action) params.set('action', action);
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    return params.toString();
  }, [operator, module, action, start, end]);
  const state = useAsync(() => api.get<unknown>(`/ovpn/audit/logs?${query}`), [query, reloadKey]);
  const rows = normalizeList<AuditLogRecord>(state.data, ['data', 'logs']);
  const total = state.data && typeof state.data === 'object' && 'total' in state.data ? Number((state.data as { total?: number }).total || rows.length) : rows.length;
  const exportUrl = `/ovpn/audit/export?${query}`;
  const auditPagination = usePagination(rows, `${query}|${reloadKey}`);

  return <section className="glass-panel table-card full-height"><div className="section-heading row-heading"><div><span>Audit Trail</span><h2>操作审计</h2></div><Toolbar><button type="button" className="mini-button" onClick={() => setReloadKey((value) => value + 1)}>刷新</button><a className="mini-button" href={exportUrl}>导出 CSV</a></Toolbar></div><div className="filter-bar audit-filters"><input className="toolbar-input" value={operator} placeholder="操作人" onChange={(event) => setOperator(event.target.value)} /><CompactSelect value={module} options={auditModuleOptions} onChange={setModule} /><CompactSelect value={action} options={auditActionOptions} onChange={setAction} /><DatePicker value={start} onChange={setStart} /><DatePicker value={end} onChange={setEnd} /></div>{state.error && <p className="inline-error">{state.error}</p>}<p className="audit-summary">共匹配 {total} 条记录，当前已加载 {rows.length} 条。</p>{rows.length ? <><div className="responsive-table"><table><thead><tr><th>时间</th><th>操作人</th><th>模块</th><th>动作</th><th>目标</th><th>结果</th><th>IP</th><th>说明</th></tr></thead><tbody>{auditPagination.pagedItems.map((item) => <tr key={item.id}><td>{item.createdAt ? new Date(item.createdAt).toLocaleString() : '-'}</td><td>{item.operator || '-'}</td><td>{item.module}</td><td>{item.action}</td><td>{item.target || '-'}</td><td><span className={`status-pill ${item.success ? 'success' : 'danger'}`}>{item.success ? '成功' : '失败'}</span></td><td>{item.ip || '-'}</td><td>{item.message || '-'}</td></tr>)}</tbody></table></div><PaginationBar page={auditPagination.page} pageSize={auditPagination.pageSize} pageCount={auditPagination.pageCount} total={auditPagination.total} start={auditPagination.start} end={auditPagination.end} onPageChange={auditPagination.setPage} onPageSizeChange={auditPagination.setPageSize} /></> : <EmptyState title="暂无审计记录" description="登录、创建/修改/删除账号、客户端、防火墙、系统设置和通知测试都会记录在这里。" />}</section>;
}

function SettingsPanel({ settings, refresh, notify }: { settings: SettingsResponse; refresh: () => void; notify: (type: Toast['type'], message: string) => void }) {
  const [savingKey, setSavingKey] = useState<string>();
  const [emailTo, setEmailTo] = useState('');
  const [notifyEvent, setNotifyEvent] = useState('connect');
  const [notifyUsername, setNotifyUsername] = useState('openvpn-test');
  const [notifyVip, setNotifyVip] = useState('10.8.0.100');
  const [notifyRip, setNotifyRip] = useState('127.0.0.1');
  const [notifyLogs, setNotifyLogs] = useState<NotifyLogRecord[]>([]);

  async function loadNotifyLogs() {
    try {
      const result = await api.get<unknown>('/ovpn/notify/logs?limit=20');
      setNotifyLogs(normalizeList<NotifyLogRecord>(result, ['data', 'logs']));
    } catch {
      setNotifyLogs([]);
    }
  }

  useEffect(() => {
    loadNotifyLogs();
  }, []);

  const notifyPreview = useMemo(() => {
    const title = notifyEvent === 'disconnect' ? 'OpenVPN 用户下线' : 'OpenVPN 用户上线';
    return [`### ${title}`, `- 用户：${trimText(notifyUsername) || '-'}`, `- 客户端：${trimText(notifyUsername) || 'openvpn-test'}-client`, `- VPN IP：${trimText(notifyVip) || '-'}`, `- 用户 IP：${trimText(notifyRip) || '-'}`, `- 通知时间：${new Date().toLocaleString()}`].join('\n');
  }, [notifyEvent, notifyUsername, notifyVip, notifyRip]);
  const notifyPagination = usePagination(notifyLogs, `${notifyLogs.length}|${notifyLogs[0]?.id ?? ''}`);

  function validateSetting(key: string, value: unknown) {
    const text = trimText(value);
    if (key === 'system.base.site_url' && !isValidUrl(text)) return '站点地址请输入完整的 http/https 地址';
    if (key === 'system.base.web_port' && !isValidPort(text)) return 'Web 端口必须是 1-65535 的整数';
    if (key === 'system.base.admin_username' && !isValidAccount(text)) return '管理员账号不能为空，长度需在 2-64 个字符内';
    if (key === 'system.base.admin_password' && !isStrongPassword(text)) return '管理员新密码需至少 12 位，包含大小写字母、数字和特殊字符';
    if (key === 'system.base.history_max_days' && !isNonNegativeInteger(text)) return '历史保留天数必须是非负整数';
    if (key === 'system.base.max_duplicate_login' && !isNonNegativeInteger(text)) return '重复登录数必须是非负整数';
    if (key === 'system.notify.provider' && !['dingtalk', 'wecom'].includes(text)) return '通知渠道只能选择钉钉或企业微信';
    if (key === 'system.notify.webhook' && text && !isValidUrl(text)) return 'Webhook 请输入完整的 http/https 地址';
    if (key === 'system.notify.enabled' && value === true && !trimText(settings.system.notify.webhook)) return '开启通知前，请先填写机器人 Webhook';
    if (key === 'system.ldap.ldap_url' && text && !isValidUrl(text, ['ldap:', 'ldaps:'])) return 'LDAP URL 请输入 ldap:// 或 ldaps:// 地址';
    if (key === 'system.email.send_from' && text && !isValidEmail(text)) return '发件人请输入有效邮箱地址';
    if (key === 'system.email.port' && !isValidPort(text)) return 'SMTP 端口必须是 1-65535 的整数';
    if (key.startsWith('client.client_url.') && text && !isValidUrl(text)) return '客户端下载地址请输入完整的 http/https 地址';
    if (key === 'openvpn.ovpn_port' && !isValidPort(text)) return 'OpenVPN 端口必须是 1-65535 的整数';
    if (key === 'openvpn.ovpn_subnet' && !isValidCidr(text, 4)) return 'IPv4 网段请输入 CIDR 格式，例如 10.8.0.0/24';
    if (key === 'openvpn.ovpn_subnet6' && text && !isValidCidr(text, 6)) return 'IPv6 网段请输入 CIDR 格式，例如 fd00::/64';
    if (key === 'openvpn.ovpn_max_clients' && !isPositiveInteger(text)) return '最大客户端数量必须是正整数';
    if (key === 'openvpn.ovpn_management' && !isValidHostPort(text)) return 'Management 请输入 host:port，例如 127.0.0.1:7505';
    if ((key === 'openvpn.ovpn_push_dns1' || key === 'openvpn.ovpn_push_dns2') && text && !isValidIp(text)) return 'DNS 地址请输入合法 IPv4 或 IPv6';
    return undefined;
  }

  async function save(key: string, value: unknown) {
    const normalizedValue = typeof value === 'string' ? trimText(value) : value;
    const error = validateSetting(key, normalizedValue);
    if (error) {
      notify('error', error);
      return;
    }
    setSavingKey(key);
    try {
      const result = await api.postForm<{ message: string }>('/settings', { [key]: normalizedValue });
      notify('success', result.message || '设置已保存');
      refresh();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSavingKey(undefined);
    }
  }

  async function sendTestEmail() {
    const email = trimText(emailTo);
    if (!isValidEmail(email)) {
      notify('error', '请输入有效的测试收件邮箱');
      return;
    }
    try {
      const result = await api.postForm<{ message: string }>('/email/send', { email, subject: 'Test', content: '测试邮件！' });
      notify('success', result.message || '测试邮件已发送');
    } catch (error) {
      notify('error', messageOf(error));
    }
  }
  async function sendTestNotify() {
    if (!settings.system.notify.enabled) {
      notify('error', '请先开启上线/下线通知');
      return;
    }
    if (!isValidUrl(settings.system.notify.webhook)) {
      notify('error', '请先填写有效的机器人 Webhook');
      return;
    }
    if (!trimText(notifyUsername)) {
      notify('error', '请输入测试用户名');
      return;
    }
    if (!isValidIp(notifyVip)) {
      notify('error', '测试 VPN IP 请输入合法 IP');
      return;
    }
    if (!isValidIp(notifyRip)) {
      notify('error', '测试来源 IP 请输入合法 IP');
      return;
    }
    try {
      const result = await api.postForm<{ message: string }>('/ovpn/notify', {
        event: notifyEvent,
        username: trimText(notifyUsername),
        common_name: `${trimText(notifyUsername)}-client`,
        vip: trimText(notifyVip),
        rip: trimText(notifyRip),
        time_unix: Math.floor(Date.now() / 1000),
        bytes_received: 1048576,
        bytes_sent: 524288,
        time_duration: 60,
      });
      notify('success', result.message || '测试通知已发送，请检查机器人群消息');
      loadNotifyLogs();
    } catch (error) {
      notify('error', messageOf(error));
      loadNotifyLogs();
    }
  }
  return <div className="settings-grid"><section className="glass-panel settings-card"><div className="section-heading"><span>Base Control</span><h2>基础控制</h2></div><TextField label="站点地址" value={settings.system.base.site_url} onSave={(value) => save('system.base.site_url', value)} /><TextField label="Web 端口" value={settings.system.base.web_port} onSave={(value) => save('system.base.web_port', value)} /><TextField label="管理员账号" value={settings.system.base.admin_username} onSave={(value) => save('system.base.admin_username', value)} /><TextField label="管理员新密码" value="" type="password" placeholder="留空不改；建议 12 位强密码" onSave={(value) => value && save('system.base.admin_password', value)} /><TextField label="历史保留天数" value={settings.system.base.history_max_days} onSave={(value) => save('system.base.history_max_days', value)} /><TextField label="重复登录数" value={settings.system.base.max_duplicate_login} onSave={(value) => save('system.base.max_duplicate_login', value || 0)} /><div className="switch-line"><div><strong>自动更新 OpenVPN 配置</strong><span>保存设置后自动同步 server.conf</span></div><Toggle checked={settings.system.base.auto_update_ovpn_config} onChange={(checked) => save('system.base.auto_update_ovpn_config', checked)} /></div><div className="switch-line"><div><strong>校验客户端配置</strong><span>登录时校验用户绑定的配置文件</span></div><Toggle checked={settings.system.base.validate_client_config} onChange={(checked) => save('system.base.validate_client_config', checked)} /></div></section><section className="glass-panel settings-card highlight-card notify-card"><div className="section-heading"><span>Message Robot</span><h2>消息通知</h2></div><div className="switch-line"><div><strong>上线/下线通知</strong><span>支持钉钉与企业微信机器人</span></div><Toggle checked={settings.system.notify.enabled} onChange={(checked) => save('system.notify.enabled', checked)} /></div><SettingSelect label="通知渠道" value={settings.system.notify.provider || 'dingtalk'} options={notifyProviderOptions} onChange={(value) => save('system.notify.provider', value)} /><TextField label="Webhook" value={settings.system.notify.webhook} onSave={(value) => save('system.notify.webhook', value)} /><TextField label="加签 Secret" value={settings.system.notify.secret} type="password" onSave={(value) => save('system.notify.secret', value)} /><div className="switch-line"><div><strong>@所有人</strong><span>重大网络变更时更醒目</span></div><Toggle checked={settings.system.notify.mention_all} onChange={(checked) => save('system.notify.mention_all', checked)} /></div><div className="notify-tester"><SettingSelect label="测试事件" value={notifyEvent} options={[{ value: 'connect', label: '模拟上线', description: '发送用户上线 Markdown' }, { value: 'disconnect', label: '模拟下线', description: '发送用户下线 Markdown' }]} onChange={setNotifyEvent} /><TextInput label="测试用户" value={notifyUsername} onChange={setNotifyUsername} /><TextInput label="VPN IP" value={notifyVip} onChange={setNotifyVip} /><TextInput label="来源 IP" value={notifyRip} onChange={setNotifyRip} /><pre className="markdown-preview">{notifyPreview}</pre></div><div className="setting-actions"><button className="primary-action notify-test-button" type="button" onClick={sendTestNotify}>发送测试通知</button><span>会使用上方模拟参数推送，并在下方保留最近 20 条结果。</span></div><div className="notify-log-list"><div className="section-heading row-heading"><div><span>Delivery Log</span><h2>最近通知结果</h2></div><button className="mini-button" type="button" onClick={loadNotifyLogs}>刷新</button></div>{notifyLogs.length ? <><div className="notify-log-page">{notifyPagination.pagedItems.map((item) => <div key={item.id} className="notify-log-item"><span className={`status-pill ${item.success ? 'success' : 'danger'}`}>{item.success ? '成功' : '失败'}</span><strong>{item.event} · {item.username}</strong><em>{item.provider} · {item.createdAt}</em><p>{item.message}</p></div>)}</div><PaginationBar page={notifyPagination.page} pageSize={notifyPagination.pageSize} pageCount={notifyPagination.pageCount} total={notifyPagination.total} start={notifyPagination.start} end={notifyPagination.end} onPageChange={notifyPagination.setPage} onPageSizeChange={notifyPagination.setPageSize} /></> : <EmptyState title="暂无通知记录" description="发送测试通知后，这里会显示最近成功或失败原因。" />}</div></section><section className="glass-panel settings-card"><div className="section-heading"><span>LDAP Bridge</span><h2>LDAP 认证</h2></div><div className="switch-line"><div><strong>启用 LDAP</strong><span>启用后本地 VPN 账号不参与认证</span></div><Toggle checked={settings.system.ldap.ldap_auth} onChange={(checked) => save('system.ldap.ldap_auth', checked)} /></div><TextField label="LDAP URL" value={settings.system.ldap.ldap_url} onSave={(value) => save('system.ldap.ldap_url', value)} /><TextField label="Base DN" value={settings.system.ldap.ldap_base_dn} onSave={(value) => save('system.ldap.ldap_base_dn', value)} /><TextField label="用户属性" value={settings.system.ldap.ldap_user_attribute} onSave={(value) => save('system.ldap.ldap_user_attribute', value)} /><TextField label="绑定 DN" value={settings.system.ldap.ldap_bind_user_dn} onSave={(value) => save('system.ldap.ldap_bind_user_dn', value)} /><TextField label="绑定密码" value={settings.system.ldap.ldap_bind_password} type="password" onSave={(value) => save('system.ldap.ldap_bind_password', value)} /><TextField label="IP 属性名" value={settings.system.ldap.ldap_user_attr_ipaddr_name} onSave={(value) => save('system.ldap.ldap_user_attr_ipaddr_name', value)} /><TextField label="配置属性名" value={settings.system.ldap.ldap_user_attr_config_name} onSave={(value) => save('system.ldap.ldap_user_attr_config_name', value)} /><div className="switch-line"><div><strong>用户组过滤</strong><span>只允许指定 memberOf 登录</span></div><Toggle checked={settings.system.ldap.ldap_user_group_filter} onChange={(checked) => save('system.ldap.ldap_user_group_filter', checked)} /></div><TextField label="用户组 DN" value={settings.system.ldap.ldap_user_group_dn} onSave={(value) => save('system.ldap.ldap_user_group_dn', value)} /></section><section className="glass-panel settings-card"><div className="section-heading"><span>Mail Bridge</span><h2>邮件通道</h2></div><TextField label="主题前缀" value={settings.system.email.send_subject_prefix} onSave={(value) => save('system.email.send_subject_prefix', value)} /><TextField label="发件人" value={settings.system.email.send_from} onSave={(value) => save('system.email.send_from', value)} /><TextField label="SMTP 主机" value={settings.system.email.host} onSave={(value) => save('system.email.host', value)} /><TextField label="SMTP 端口" value={settings.system.email.port} onSave={(value) => save('system.email.port', value)} /><TextField label="发送账号" value={settings.system.email.username} onSave={(value) => save('system.email.username', value)} /><TextField label="发送密码" value={settings.system.email.password} type="password" onSave={(value) => save('system.email.password', value)} /><SettingSelect label="安全协议" value={settings.system.email.security || ''} options={[{ value: '', label: '无', description: '不启用加密传输' }, { value: 'tls', label: 'TLS', description: 'STARTTLS 安全连接' }, { value: 'ssl', label: 'SSL', description: 'SSL 加密连接' }]} onChange={(value) => save('system.email.security', value)} /><div className="inline-form"><input value={emailTo} placeholder="测试收件邮箱" onChange={(event) => setEmailTo(event.target.value)} /><button type="button" onClick={sendTestEmail}>发送测试</button></div></section><section className="glass-panel settings-card"><div className="section-heading"><span>Client Portal</span><h2>客户端下载</h2></div>{Object.entries(settings.client.client_url).map(([key, value]) => <TextField key={key} label={`${key.toUpperCase()} 下载地址`} value={String(value)} onSave={(next) => save(`client.client_url.${key}`, next)} />)}</section><section className="glass-panel settings-card"><div className="section-heading"><span>OpenVPN Core</span><h2>OpenVPN 参数</h2></div><TextField label="端口" value={settings.openvpn.ovpn_port} onSave={(value) => save('openvpn.ovpn_port', value)} /><SettingSelect label="协议" value={settings.openvpn.ovpn_proto} options={[{ value: 'udp', label: 'UDP', description: '低延迟，常用推荐' }, { value: 'tcp', label: 'TCP', description: '穿透性更好，稳定优先' }]} onChange={(value) => save('openvpn.ovpn_proto', value)} /><TextField label="IPv4 网段" value={settings.openvpn.ovpn_subnet} onSave={(value) => save('openvpn.ovpn_subnet', value)} /><TextField label="最大客户端" value={settings.openvpn.ovpn_max_clients} onSave={(value) => save('openvpn.ovpn_max_clients', value)} /><TextField label="Management" value={settings.openvpn.ovpn_management} onSave={(value) => save('openvpn.ovpn_management', value)} /><TextField label="DNS1" value={settings.openvpn.ovpn_push_dns1} onSave={(value) => save('openvpn.ovpn_push_dns1', value)} /><TextField label="DNS2" value={settings.openvpn.ovpn_push_dns2} onSave={(value) => save('openvpn.ovpn_push_dns2', value)} /><div className="switch-line"><div><strong>全局网关</strong><span>推送默认路由</span></div><Toggle checked={settings.openvpn.ovpn_gateway} onChange={(checked) => save('openvpn.ovpn_gateway', checked)} /></div><div className="switch-line"><div><strong>IPv6</strong><span>启用 IPv6 地址池</span></div><Toggle checked={settings.openvpn.ovpn_ipv6} onChange={(checked) => save('openvpn.ovpn_ipv6', checked)} /></div><TextField label="IPv6 网段" value={settings.openvpn.ovpn_subnet6} onSave={(value) => save('openvpn.ovpn_subnet6', value)} /></section>{savingKey && <div className="save-toast">正在保存 {savingKey}</div>}</div>;
}

function AdminModal({ modal, groups, clients, selectedGroupId, notify, reload, close }: { modal: ModalState; groups: GroupRecord[]; clients: ClientRecord[]; selectedGroupId: number; notify: (type: Toast['type'], message: string) => void; reload: () => void; close: () => void }) {
  const tree = buildTree(groups);
  const groupNameInitial = modal.type === 'group-form' && modal.mode === 'edit' ? modal.group.name || '' : '';
  const groupParentInitial = modal.type === 'group-form'
    ? String(modal.mode === 'edit' ? (modal.group.parent_id ?? 0) : (modal.parentGroup?.id ?? selectedGroupId ?? 0))
    : String(selectedGroupId ?? 0);
  const [saving, setSaving] = useState(false);
  const [content, setContent] = useState('content' in modal ? modal.content : '');
  const [name, setName] = useState(modal.type === 'user-form' ? modal.user?.name || '' : groupNameInitial);
  const [username, setUsername] = useState(modal.type === 'user-form' ? modal.user?.username || '' : modal.type === 'client-form' ? '' : '');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState(modal.type === 'user-form' ? modal.user?.email || '' : '');
  const [ipAddr, setIpAddr] = useState(modal.type === 'user-form' ? modal.user?.ipAddr || '' : '');
  const [ovpnConfig, setOvpnConfig] = useState(modal.type === 'user-form' ? modal.user?.ovpnConfig || '' : clients[0]?.name || '');
  const [expireDate, setExpireDate] = useState(modal.type === 'user-form' ? modal.user?.expireDate || '' : '');
  const [sendNotifyEmail, setSendNotifyEmail] = useState(false);
  const [isFirstLogin, setIsFirstLogin] = useState(false);
  const [parentId, setParentId] = useState(groupParentInitial);
  const [sip, setSip] = useState(modal.type === 'firewall-form' ? modal.firewall?.sip || '' : '');
  const [dip, setDip] = useState(modal.type === 'firewall-form' ? modal.firewall?.dip || '' : '');
  const [sg, setSg] = useState(modal.type === 'firewall-form' ? modal.firewall?.sg?.map((item) => item.id).join(',') || '' : '');
  const [dg, setDg] = useState(modal.type === 'firewall-form' ? modal.firewall?.dg?.map((item) => item.id).join(',') || '' : '');
  const [policy, setPolicy] = useState(modal.type === 'firewall-form' ? modal.firewall?.policy || 'accept' : 'accept');
  const [status, setStatus] = useState(modal.type === 'firewall-form' ? modal.firewall?.status !== false : true);
  const [comment, setComment] = useState(modal.type === 'firewall-form' ? modal.firewall?.comment || '' : '');
  const [upload, setUpload] = useState(modal.type === 'rate-limit' ? modal.upload : '');
  const [uploadUnit, setUploadUnit] = useState(modal.type === 'rate-limit' ? modal.uploadUnit : 'mbytes/second');
  const [download, setDownload] = useState(modal.type === 'rate-limit' ? modal.download : '');
  const [downloadUnit, setDownloadUnit] = useState(modal.type === 'rate-limit' ? modal.downloadUnit : 'mbytes/second');
  const [serverAddr, setServerAddr] = useState('');
  const [serverPort, setServerPort] = useState('');
  const [clientConfig, setClientConfig] = useState('');
  const [ccdConfig, setCcdConfig] = useState('');
  const [mfa, setMfa] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});

  function setValue(setter: (value: string) => void, key?: string) {
    return (value: string) => {
      setter(value);
      if (key) setErrors((current) => {
        if (!current[key]) return current;
        const next = { ...current };
        delete next[key];
        return next;
      });
    };
  }

  function validateRateLimit() {
    const nextErrors: FieldErrors = {};
    const nextUpload = trimText(upload);
    const nextDownload = trimText(download);
    if (!trimText(modal.type === 'rate-limit' ? modal.client.vip || modal.client.vip6 : '')) nextErrors.vip = '当前客户端缺少 VPN IP，不能设置限速';
    if (!nextUpload && !nextDownload) nextErrors.upload = '请至少填写上传或下载速率';
    if (nextUpload && !isPositiveNumber(nextUpload)) nextErrors.upload = '上传速率必须是大于 0 的数字';
    if (nextDownload && !isPositiveNumber(nextDownload)) nextErrors.download = '下载速率必须是大于 0 的数字';
    return nextErrors;
  }

  function validateUserForm() {
    const nextErrors: FieldErrors = {};
    if (!isValidAccount(username)) nextErrors.username = '账号不能为空，长度需在 2-64 个字符内';
    if (modal.type === 'user-form' && modal.mode === 'add' && !isStrongPassword(password)) nextErrors.password = '初始密码需至少 12 位，包含大小写字母、数字和特殊字符';
    if (trimText(email) && !isValidEmail(email)) nextErrors.email = '邮箱格式不正确';
    if (trimText(ipAddr) && !isValidIp(ipAddr)) nextErrors.ipAddr = '固定 IP 请输入合法 IPv4 或 IPv6';
    return nextErrors;
  }

  function validateResetPassword() {
    const nextErrors: FieldErrors = {};
    if (!isStrongPassword(password)) nextErrors.password = '新密码需至少 12 位，包含大小写字母、数字和特殊字符';
    return nextErrors;
  }

  function validateClientForm() {
    const nextErrors: FieldErrors = {};
    if (!isValidSafeName(username)) nextErrors.username = '客户端名称只能包含字母、数字、点、下划线、中划线，长度 2-64 位';
    if (!isValidHost(serverAddr)) nextErrors.serverAddr = 'VPN 服务器地址不能为空，请填写域名或 IP';
    if (!isValidPort(serverPort)) nextErrors.serverPort = 'VPN 端口必须是 1-65535 的整数';
    return nextErrors;
  }

  function validateFirewallForm() {
    const nextErrors: FieldErrors = {};
    if (!trimText(sip) && !trimText(sg)) {
      nextErrors.sip = '请至少填写源 IP/CIDR 或选择源用户组';
      nextErrors.sg = '请至少填写源 IP/CIDR 或选择源用户组';
    }
    if (!trimText(dip) && !trimText(dg)) {
      nextErrors.dip = '请至少填写目的 IP/CIDR 或选择目的用户组';
      nextErrors.dg = '请至少填写目的 IP/CIDR 或选择目的用户组';
    }
    if (!isValidIpOrCidrList(sip)) nextErrors.sip = '源 IP/CIDR 支持逗号分隔，例如 10.8.0.0/24,10.8.0.10';
    if (!isValidIpOrCidrList(dip)) nextErrors.dip = '目的 IP/CIDR 支持逗号分隔，例如 192.168.1.0/24';
    if (!['accept', 'drop'].includes(policy)) nextErrors.policy = '策略只能选择允许或拒绝';
    return nextErrors;
  }

  function validateGroupForm() {
    const nextErrors: FieldErrors = {};
    if (!trimText(name)) nextErrors.name = '分组名称不能为空';
    const normalizedParent = Number(parentId);
    if (!Number.isInteger(normalizedParent) || normalizedParent < 0) nextErrors.parentId = '请选择有效的上级分组';
    if (modal.type === 'group-form' && modal.mode === 'edit' && String(modal.group.id) === parentId) nextErrors.parentId = '上级分组不能选择自己';
    if (modal.type === 'group-form' && parentId === '0' && !(modal.mode === 'edit' && modal.group.id === 1)) nextErrors.parentId = '只有默认分组可以设置为无上级分组';
    return nextErrors;
  }

  function submitForm(event: FormEvent<HTMLFormElement>, validate: () => FieldErrors, action: () => Promise<{ message?: string } | unknown>, success: string) {
    event.preventDefault();
    const nextErrors = validate();
    setErrors(nextErrors);
    const firstError = Object.values(nextErrors)[0];
    if (firstError) {
      notify('error', firstError);
      return;
    }
    void submit(action, success);
  }

  async function submit(action: () => Promise<{ message?: string } | unknown>, success: string) {
    setSaving(true);
    try {
      const result = await action();
      notify('success', typeof result === 'object' && result && 'message' in result ? String(result.message) : success);
      close();
      reload();
    } catch (error) {
      notify('error', messageOf(error));
    } finally {
      setSaving(false);
    }
  }

  if (modal.type === 'none') return null;

  if (modal.type === 'server-config') {
    return <Modal title="编辑 server.conf" onClose={close}><div className="modal-body"><textarea className="code-editor" value={content} onChange={(event) => setContent(event.target.value)} /><div className="modal-actions"><button type="button" onClick={() => submit(() => api.postForm('/ovpn/server', { action: 'updateConfig', content }), '服务端配置已保存')} disabled={saving}>保存配置</button></div></div></Modal>;
  }

  if (modal.type === 'rate-limit') {
    return <Modal title={`限速：${getClientName(modal.client)}`} onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateRateLimit, () => api.postForm('/ovpn/firewall?a=set_rateLimit', { vip: modal.client.vip || modal.client.vip6 || '', upload: trimText(upload), uploadUnit, download: trimText(download), downloadUnit }), '限速规则已更新')}><FormErrorSummary errors={errors} /><TextInput label="上传速率" value={upload} onChange={setValue(setUpload, 'upload')} error={errors.upload || errors.vip} /><SelectInput label="上传单位" value={uploadUnit} options={['bytes/second', 'kbytes/second', 'mbytes/second']} onChange={setUploadUnit} /><TextInput label="下载速率" value={download} onChange={setValue(setDownload, 'download')} error={errors.download} /><SelectInput label="下载单位" value={downloadUnit} options={['bytes/second', 'kbytes/second', 'mbytes/second']} onChange={setDownloadUnit} /><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存限速'}</button></div></form></Modal>;
  }

  if (modal.type === 'user-form') {
    const title = modal.mode === 'add' ? '添加 VPN 用户' : `编辑用户：${modal.user?.username}`;
    const saveUser = () => modal.mode === 'add'
      ? api.postForm('/ovpn/user', { username: trimText(username), name: trimText(name), password: trimText(password), email: trimText(email), ipAddr: trimText(ipAddr), gid: selectedGroupId, ovpnConfig, expireDate, sendNotifyEmail, isFirstLogin })
      : api.patchForm('/ovpn/user', { id: modal.user?.id, username: trimText(username), name: trimText(name), email: trimText(email), ipAddr: trimText(ipAddr), gid: modal.user?.gid || selectedGroupId, ovpnConfig, expireDate });
    return <Modal title={title} onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateUserForm, saveUser, modal.mode === 'add' ? '用户已创建' : '用户已保存')}><FormErrorSummary errors={errors} /><TextInput label="账号" value={username} onChange={setValue(setUsername, 'username')} error={errors.username} required autoFocus /><TextInput label="姓名" value={name} onChange={setName} /><TextInput label="邮箱" value={email} onChange={setValue(setEmail, 'email')} error={errors.email} /><TextInput label="固定 IP" value={ipAddr} onChange={setValue(setIpAddr, 'ipAddr')} error={errors.ipAddr} /><SelectInput label="客户端配置" value={ovpnConfig} options={[{ value: '', label: '不绑定' }, ...clients.map((item) => ({ value: item.name, label: item.name }))]} onChange={setOvpnConfig} />{modal.mode === 'add' && <TextInput label="初始密码" value={password} type="password" onChange={setValue(setPassword, 'password')} error={errors.password} required />}<TextInput label="过期日期" value={expireDate} type="date" onChange={setExpireDate} />{modal.mode === 'add' && <CheckInput label="发送通知邮件" checked={sendNotifyEmail} onChange={setSendNotifyEmail} />}{modal.mode === 'add' && <CheckInput label="首次登录修改密码" checked={isFirstLogin} onChange={setIsFirstLogin} />}<div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存用户'}</button></div></form></Modal>;
  }

  if (modal.type === 'reset-password') {
    return <Modal title={`重置密码：${modal.user.username}`} onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateResetPassword, () => api.patchForm('/ovpn/user', { id: modal.user.id, password: trimText(password), sendNotifyEmail }), '密码已重置')}><FormErrorSummary errors={errors} /><TextInput label="新密码" value={password} type="password" onChange={setValue(setPassword, 'password')} error={errors.password} required autoFocus /><CheckInput label="发送通知邮件" checked={sendNotifyEmail} onChange={setSendNotifyEmail} /><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '处理中...' : '确认重置'}</button></div></form></Modal>;
  }

  if (modal.type === 'client-form') {
    return <Modal title="添加客户端配置" onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateClientForm, () => api.postForm('/ovpn/client', { name: trimText(username), serverAddr: trimText(serverAddr), serverPort: trimText(serverPort), ccdConfig, config: clientConfig, mfa }), '客户端已创建')}><FormErrorSummary errors={errors} /><TextInput label="客户端名称" value={username} onChange={setValue(setUsername, 'username')} error={errors.username} required autoFocus /><TextInput label="VPN 服务器地址" value={serverAddr} onChange={setValue(setServerAddr, 'serverAddr')} error={errors.serverAddr} required /><TextInput label="VPN 端口" value={serverPort} onChange={setValue(setServerPort, 'serverPort')} error={errors.serverPort} required /><CheckInput label="启用 MFA" checked={mfa} onChange={setMfa} /><TextAreaInput label="CCD 配置" value={ccdConfig} onChange={setCcdConfig} /><TextAreaInput label="自定义配置" value={clientConfig} onChange={setClientConfig} /><p className="modal-hint wide-field">名称会用于生成证书与 .ovpn 文件；地址、端口、CCD、自定义配置与 MFA 会写入当前客户端配置。</p><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '生成中...' : '生成客户端'}</button></div></form></Modal>;
  }

  if (modal.type === 'client-editor') {
    return <Modal title={`${modal.editor === 'config' ? '客户端配置' : 'CCD 配置'}：${modal.client.name}`} onClose={close}><div className="modal-body"><textarea className="code-editor" value={content} onChange={(event) => setContent(event.target.value)} /><div className="modal-actions"><button type="button" onClick={() => submit(() => api.putForm(`/ovpn/client/${encodeURIComponent(modal.client.name)}/${modal.editor}`, { content }), '配置已保存')} disabled={saving}>保存</button></div></div></Modal>;
  }

  if (modal.type === 'firewall-form') {
    const title = modal.mode === 'add' ? '添加防火墙规则' : `编辑防火墙规则 #${modal.firewall?.id}`;
    const form = { id: modal.firewall?.id, sip: trimText(sip), dip: trimText(dip), sg, dg, policy, status, comment: trimText(comment) };
    return <Modal title={title} onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateFirewallForm, () => modal.mode === 'add' ? api.postForm('/ovpn/firewall', form) : api.patchForm('/ovpn/firewall', form), '防火墙规则已保存')}><FormErrorSummary errors={errors} /><TextInput label="源 IP/CIDR" value={sip} onChange={setValue(setSip, 'sip')} error={errors.sip} /><TextInput label="目的 IP/CIDR" value={dip} onChange={setValue(setDip, 'dip')} error={errors.dip} /><MultiGroupInput label="源用户组" value={sg} groups={tree} onChange={setValue(setSg, 'sg')} error={errors.sg} /><MultiGroupInput label="目的用户组" value={dg} groups={tree} onChange={setValue(setDg, 'dg')} error={errors.dg} /><SelectInput label="策略" value={policy} options={[{ value: 'accept', label: '允许 accept' }, { value: 'drop', label: '拒绝 drop' }]} onChange={setValue(setPolicy, 'policy')} error={errors.policy} required /><CheckInput label="启用规则" checked={status} onChange={setStatus} /><TextAreaInput label="备注" value={comment} onChange={setComment} /><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存规则'}</button></div></form></Modal>;
  }

  if (modal.type === 'group-form') {
    const title = modal.mode === 'add' ? '新增用户组' : `编辑用户组：${modal.group?.name}`;
    const blockedParentIds = modal.mode === 'edit' ? new Set([modal.group.id, ...getDescendantGroupIds(groups, modal.group.id)]) : new Set<number>();
    const isDefaultGroup = modal.mode === 'edit' && modal.group.id === 1;
    const parentOptions = [...(isDefaultGroup ? [{ value: '0', label: '— 无上级分组 —' }] : []), ...tree.filter((item) => !blockedParentIds.has(item.id)).map((item) => ({ value: String(item.id), label: `${'— '.repeat(item.depth)}${item.name}` }))];
    const form = modal.mode === 'add'
      ? { name: trimText(name), parent_id: parentId === '0' || parentId === 'null' ? null : parentId }
      : { id: modal.group.id, name: trimText(name), parent_id: parentId === '0' || parentId === 'null' ? null : parentId };
    return <Modal title={title} onClose={close}><form className="modal-body form-grid" noValidate onSubmit={(event) => submitForm(event, validateGroupForm, () => modal.mode === 'add' ? api.postForm('/ovpn/group', form) : api.patchForm('/ovpn/group', form), '用户组已保存')}><FormErrorSummary errors={errors} /><TextInput label="分组名称" value={name} onChange={setValue(setName, 'name')} error={errors.name} required autoFocus /><SelectInput label="上级分组" value={parentId} options={parentOptions} onChange={setValue(setParentId, 'parentId')} error={errors.parentId} required /><div className="modal-actions wide-field"><button type="submit" disabled={saving}>{saving ? '保存中...' : '保存分组'}</button></div></form></Modal>;
  }

  if (modal.type === 'group-config') {
    const form = { id: modal.group.id, name: modal.group.name, config: content };
    return <Modal title={`组配置：${modal.group.name}`} onClose={close}><div className="modal-body"><textarea className="code-editor" value={content} onChange={(event) => setContent(event.target.value)} /><div className="modal-actions"><button type="button" onClick={() => submit(() => api.patchForm('/ovpn/group', form), '组配置已保存')} disabled={saving}>保存组配置</button></div></div></Modal>;
  }

  return <Modal title="更新证书" onClose={close}><div className="modal-body"><p className="modal-hint">会调用服务端 renewCert 动作，请确认 EasyRSA 数据已挂载且当前环境允许执行证书脚本。</p><div className="modal-actions"><button type="button" onClick={() => submit(() => api.postForm('/ovpn/server', { action: 'renewCert' }), '证书更新任务已触发')} disabled={saving}>开始更新</button></div></div></Modal>;
}

function AdminApp() {
  const [active, setActive] = useState<Section>('overview');
  const [theme, setTheme] = useState<ThemeKey>(getInitialTheme);
  const [selectedGroupId, setSelectedGroupId] = useState(1);
  const [reloadKey, setReloadKey] = useState(0);
  const [modal, setModal] = useState<ModalState>({ type: 'none' });
  const [confirmState, setConfirmState] = useState<ConfirmState>();
  const [toast, setToast] = useState<Toast>();
  const refresh = () => setReloadKey((value) => value + 1);
  const notify = (type: Toast['type'], message: string) => {
    setToast({ type, message });
    window.setTimeout(() => setToast(undefined), 3600);
  };

  const settingsState = useAsync(() => api.get<SettingsResponse>('/settings'), [reloadKey]);
  const dashboardState = useAsync(() => api.get<DashboardSummary>('/ovpn/dashboard/summary'), [reloadKey]);
  const onlineState = useAsync(() => api.get<OnlineResponse>('/ovpn/online-client'), [reloadKey]);
  const groupsState = useAsync(() => api.get<unknown>('/ovpn/group').then((value) => normalizeList<GroupRecord>(value, ['groups', 'data'])), [reloadKey]);
  const clientsState = useAsync(() => api.get<unknown>('/ovpn/client').then((value) => normalizeList<ClientRecord>(value, ['clients', 'data'])), [reloadKey]);
  const firewallsState = useAsync(() => api.get<unknown>('/ovpn/firewall').then((value) => normalizeList<FirewallRecord>(value, ['firewalls', 'data'])), [reloadKey]);
  const certsState = useAsync(() => api.get<unknown>('/ovpn/certs').then((value) => normalizeList<CertRecord>(value, ['certs', 'data'])), [reloadKey]);
  const historyState = useAsync(() => api.get<HistoryResponse>('/ovpn/history?draw=1&offset=0&limit=20&orderColumn=time_unix&order=desc'), [reloadKey]);
  const usersState = useAsync(() => api.get<{ users?: UserRecord[]; authUser?: boolean }>(`/ovpn/group/${selectedGroupId}/users`).then((value) => ({ users: normalizeList<UserRecord>(value, ['users', 'data']), authUser: value.authUser })), [selectedGroupId, reloadKey]);

  const groups = groupsState.data || [];
  const clients = clientsState.data || [];
  const users = usersState.data?.users || [];
  const firewalls = firewallsState.data || [];
  const certs = certsState.data || [];
  const settings = settingsState.data;
  const title = navItems.find((item) => item.key === active)?.label || '管理台';

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem('openvpn-admin-theme', theme);
  }, [theme]);

  useEffect(() => {
    if (!groups.length) return;
    if (!groups.some((item) => item.id === selectedGroupId)) setSelectedGroupId(groups[0].id);
  }, [groups, selectedGroupId]);

  function renderPanel() {
    if (active === 'overview') return <Overview onlineState={onlineState} dashboardState={dashboardState} users={users} clients={clients} settings={settings} notify={notify} openModal={setModal} reload={refresh} confirmAction={setConfirmState} />;
    if (active === 'users') return <UsersPanel groups={groups} selectedGroupId={selectedGroupId} setSelectedGroupId={setSelectedGroupId} usersState={usersState} clients={clients} notify={notify} reload={refresh} openModal={setModal} confirmAction={setConfirmState} />;
    if (active === 'clients') return <ClientsPanel clients={clients} settings={settings} notify={notify} reload={refresh} openModal={setModal} confirmAction={setConfirmState} />;
    if (active === 'firewall') return <FirewallPanel firewalls={firewalls} groups={groups} notify={notify} reload={refresh} openModal={setModal} confirmAction={setConfirmState} />;
    if (active === 'history') return <HistoryPanel initial={historyState.data || { data: [] }} />;
    if (active === 'certs') return <CertsPanel certs={certs} openModal={setModal} />;
    if (active === 'audit') return <AuditPanel />;
    if (settings) return <SettingsPanel settings={settings} refresh={refresh} notify={notify} />;
    return <section className="glass-panel table-card"><EmptyState title="设置加载中" description={settingsState.error || '正在读取系统设置接口。'} /></section>;
  }

  function modalKey() {
    if (modal.type === 'client-editor') return `${modal.type}-${modal.client.name}-${modal.editor}`;
    if (modal.type === 'rate-limit') return `${modal.type}-${modal.client.id || modal.client.cid || getClientName(modal.client)}`;
    if (modal.type === 'group-form') return modal.mode === 'edit' ? `${modal.type}-${modal.group.id}` : `${modal.type}-new-${modal.parentGroup?.id || selectedGroupId}`;
    if (modal.type === 'group-config') return `${modal.type}-${modal.group.id}`;
    if (modal.type === 'user-form' || modal.type === 'reset-password') return `${modal.type}-${modal.user?.id || 'new'}`;
    if (modal.type === 'firewall-form') return `${modal.type}-${modal.firewall?.id || 'new'}`;
    return modal.type;
  }

  return <main className="app-shell"><div className="aurora aurora-a" /><div className="aurora aurora-b" /><div className="ambient-grid" aria-hidden="true" /><div className="ambient-scanline" aria-hidden="true" /><div className="ambient-particles" aria-hidden="true"><i /><i /><i /><i /><i /><i /></div><aside className="sidebar glass-panel"><div className="brand"><div className="brand-mark">OV</div><div><strong>OpenVPN</strong><span>Secure Console</span></div></div><nav>{navItems.map((item) => <button key={item.key} className={active === item.key ? 'active' : ''} type="button" onClick={() => setActive(item.key)}><span>{item.eyebrow}</span>{item.label}</button>)}</nav><div className="sidebar-card"><span>Current Operator</span><strong>{runtime.sysUser || 'admin'}</strong><em>{runtime.version || 'local-dev'}</em></div></aside><section className="content-shell"><header className="topbar glass-panel"><div><span>OpenVPN Console</span><h1>{title}</h1></div><div className="topbar-actions"><ThemeSwitcher theme={theme} onChange={setTheme} /><button type="button" className="mini-button" onClick={refresh}>刷新</button><a href="/logout">退出</a></div></header>{(settingsState.loading || groupsState.loading || clientsState.loading || dashboardState.loading) && <div className="loading-line">正在同步系统接口数据...</div>}<div key={active} className="panel-transition">{renderPanel()}</div></section>{modal.type !== 'none' && <AdminModal key={modalKey()} modal={modal} groups={groups} clients={clients} selectedGroupId={selectedGroupId} notify={notify} reload={refresh} close={() => setModal({ type: 'none' })} />}{confirmState && <ConfirmDialog state={confirmState} onClose={() => setConfirmState(undefined)} notify={notify} />}{toast && <div className={`toast ${toast.type}`}>{toast.message}</div>}</main>;
}

export function App() {
  const page = runtime.page || 'admin';
  if (page === 'login') return <LoginPage />;
  if (page === 'client') return <ClientPortalPage />;
  return <AdminApp />;
}



