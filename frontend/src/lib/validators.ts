/** 通用验证函数 - 从旧 App.tsx 提取 */

export function trimText(value: unknown) {
  return String(value ?? '').trim();
}

export function isValidEmail(value: string) {
  const text = trimText(value);
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(text);
}

export function isValidPort(value: unknown) {
  const text = trimText(value);
  const port = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(port) && port >= 1 && port <= 65535;
}

export function isNonNegativeInteger(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(numberValue) && numberValue >= 0;
}

export function isPositiveInteger(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(numberValue) && numberValue > 0;
}

export function isPositiveNumber(value: unknown) {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+(\.\d+)?$/.test(text) && Number.isFinite(numberValue) && numberValue > 0;
}

export function isValidUrl(value: unknown, protocols = ['http:', 'https:']) {
  const text = trimText(value);
  try {
    const url = new URL(text);
    return protocols.includes(url.protocol) && Boolean(url.hostname);
  } catch {
    return false;
  }
}

export function isValidIpv4(value: string) {
  const parts = trimText(value).split('.');
  return parts.length === 4 && parts.every((part) => /^\d+$/.test(part) && Number(part) >= 0 && Number(part) <= 255);
}

export function isValidIpv6(value: string) {
  const text = trimText(value);
  if (!text.includes(':') || !/^[0-9a-fA-F:.]+$/.test(text)) return false;
  if ((text.match(/::/g) || []).length > 1) return false;
  const parts = text.split(':');
  if (!text.includes('::') && parts.length !== 8) return false;
  if (parts.length > 8) return false;
  return parts.every((part) => part === '' || /^[0-9a-fA-F]{1,4}$/.test(part) || isValidIpv4(part));
}

export function isValidIp(value: string) {
  return isValidIpv4(value) || isValidIpv6(value);
}

export function isValidCidr(value: string, requiredVersion?: 4 | 6) {
  const [address, prefix, ...extraParts] = trimText(value).split('/');
  if (!address || !prefix || extraParts.length) return false;
  const prefixNumber = Number(prefix);
  if (!/^\d+$/.test(prefix) || !Number.isInteger(prefixNumber)) return false;
  if (requiredVersion === 4) return isValidIpv4(address) && prefixNumber >= 0 && prefixNumber <= 32;
  if (requiredVersion === 6) return isValidIpv6(address) && prefixNumber >= 0 && prefixNumber <= 128;
  return (isValidIpv4(address) && prefixNumber <= 32) || (isValidIpv6(address) && prefixNumber <= 128);
}

export function isValidIpOrCidr(value: string) {
  const text = trimText(value);
  return text.includes('/') ? isValidCidr(text) : isValidIp(text);
}

export function isValidIpOrCidrList(value: unknown) {
  const text = trimText(value);
  if (!text) return true;
  return text.split(',').map((part) => part.trim()).filter(Boolean).every(isValidIpOrCidr);
}

export function isValidHost(value: unknown) {
  const text = trimText(value).replace(/^\[(.*)\]$/, '$1');
  if (!text) return false;
  if (isValidIp(text)) return true;
  return /^(localhost|[A-Za-z0-9](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9_-]{0,61}[A-Za-z0-9])?)*)$/.test(text);
}

export function isValidHostPort(value: unknown) {
  const text = trimText(value);
  const bracketMatch = text.match(/^\[([^\]]+)\]:(\d+)$/);
  if (bracketMatch) return isValidHost(bracketMatch[1]) && isValidPort(bracketMatch[2]);
  const separatorIndex = text.lastIndexOf(':');
  if (separatorIndex <= 0) return false;
  return isValidHost(text.slice(0, separatorIndex)) && isValidPort(text.slice(separatorIndex + 1));
}

export function isStrongPassword(value: unknown) {
  const text = trimText(value);
  return text.length >= 12 && /[a-z]/.test(text) && /[A-Z]/.test(text) && /\d/.test(text) && /[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(text);
}

export function isValidSafeName(value: unknown) {
  return /^[A-Za-z0-9._-]{2,64}$/.test(trimText(value));
}

export function isValidAccount(value: unknown) {
  return /^\S{2,64}$/.test(trimText(value));
}
