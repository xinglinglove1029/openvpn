import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatBytes(value?: number | string | null): string {
  if (value === null || value === undefined || value === '') return '0 B';
  const size = typeof value === 'string' ? Number(value.replace(/[^0-9.]/g, '')) : Number(value);
  if (isNaN(size) || size < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(size) / Math.log(1024));
  return `${size / Math.pow(1024, i) <= 0 ? '0' : (size / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
}

export function trimText(value: unknown): string {
  return String(value ?? '').trim();
}

export function isValidEmail(value: string): boolean {
  const text = trimText(value);
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(text);
}

export function isValidPort(value: unknown): boolean {
  const text = trimText(value);
  const port = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(port) && port >= 1 && port <= 65535;
}

export function isNonNegativeInteger(value: unknown): boolean {
  const text = trimText(value);
  const numberValue = Number(text);
  return /^\d+$/.test(text) && Number.isInteger(numberValue) && numberValue >= 0;
}

export function formatDuration(seconds: number | string): string {
  const sec = typeof seconds === 'string' ? Number(seconds) : seconds;
  if (isNaN(sec) || sec < 0) return '0秒';

  const hours = Math.floor(sec / 3600);
  const minutes = Math.floor((sec % 3600) / 60);
  const secs = Math.floor(sec % 60);

  if (hours > 0) {
    return `${hours}小时${minutes}分${secs}秒`;
  }
  if (minutes > 0) {
    return `${minutes}分${secs}秒`;
  }
  return `${secs}秒`;
}

export function formatDateTime(date: string | number | Date | undefined | null): string {
  if (!date) return '-';

  try {
    const d = typeof date === 'string' || typeof date === 'number' ? new Date(date) : date;
    if (isNaN(d.getTime())) return '-';

    return d.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    });
  } catch {
    return '-';
  }
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}