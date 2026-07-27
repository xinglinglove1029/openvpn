import { toast } from 'sonner';

type RequestOptions = RequestInit & { form?: Record<string, unknown>; json?: unknown };

// ApiError 标识已由 api 层处理过提示的错误，调用方捕获后可跳过自己的 toast
export class ApiError extends Error {
  status: number;
  handled: boolean;
  constructor(message: string, status: number, handled = false) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.handled = handled;
  }
}

function toFormBody(data: Record<string, unknown>) {
  const body = new URLSearchParams();

  Object.entries(data).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      body.set(key, String(value));
    }
  });

  return body;
}

async function request<T>(url: string, options: RequestOptions = {}): Promise<T> {
  const init: RequestInit = {
    credentials: 'same-origin',
    headers: {
      Accept: 'application/json',
      ...(options.headers || {}),
    },
    ...options,
  };

  if (options.json !== undefined) {
    init.method = init.method || 'POST';
    init.body = JSON.stringify(options.json);
    init.headers = {
      ...init.headers,
      'Content-Type': 'application/json; charset=UTF-8',
    };
  } else if (options.form) {
    init.method = init.method || 'POST';
    init.body = toFormBody(options.form);
    init.headers = {
      ...init.headers,
      'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
    };
  }

  const response = await fetch(url, init);

  if (response.redirected && response.url.includes('/login')) {
    window.location.href = '/login';
    throw new ApiError('登录状态已过期', 401, true);
  }

  const contentType = response.headers.get('content-type') || '';
  const payload = contentType.includes('application/json') ? await response.json() : await response.text();

  if (!response.ok) {
    // RBAC：403 无权限统一 toast 提示，不跳转
    // 标记 handled=true，调用方捕获到 ApiError 时检查 handled 字段跳过自己的 toast
    if (response.status === 403) {
      const forbiddenMessage =
        typeof payload === 'object' && payload && 'message' in payload
          ? String(payload.message)
          : '无权限执行此操作';
      toast.error(forbiddenMessage);
      throw new ApiError(forbiddenMessage, 403, true);
    }
    const message = typeof payload === 'object' && payload && 'message' in payload ? String(payload.message) : response.statusText;
    throw new ApiError(message || '请求失败', response.status, false);
  }

  return payload as T;
}

export const api = {
  get: <T>(url: string) => request<T>(url),
  postForm: <T>(url: string, form: Record<string, unknown>) => request<T>(url, { method: 'POST', form }),
  patchForm: <T>(url: string, form: Record<string, unknown>) => request<T>(url, { method: 'PATCH', form }),
  putForm: <T>(url: string, form: Record<string, unknown>) => request<T>(url, { method: 'PUT', form }),
  postJson: <T>(url: string, body: unknown) => request<T>(url, { method: 'POST', json: body }),
  putJson: <T>(url: string, body: unknown) => request<T>(url, { method: 'PUT', json: body }),
  delete: <T>(url: string) => request<T>(url, { method: 'DELETE' }),
  multipart: <T>(url: string, body: FormData) => request<T>(url, { method: 'POST', body }),

  clientPackages: {
    list: () => request<any[]>('/ovpn/client-packages'),
    upload: (formData: FormData) => request<any>('/ovpn/client-packages', { method: 'POST', body: formData }),
    remove: (id: number) => request<any>(`/ovpn/client-packages/${id}`, { method: 'DELETE' }),
    enable: (id: number) => request<any>(`/ovpn/client-packages/${id}/enable`, { method: 'POST' }),
  },
};
