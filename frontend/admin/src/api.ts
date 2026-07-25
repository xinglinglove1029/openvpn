type RequestOptions = RequestInit & { form?: Record<string, unknown>; json?: unknown };

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
    throw new Error('登录状态已过期');
  }

  const contentType = response.headers.get('content-type') || '';
  const payload = contentType.includes('application/json') ? await response.json() : await response.text();

  if (!response.ok) {
    const message = typeof payload === 'object' && payload && 'message' in payload ? String(payload.message) : response.statusText;
    throw new Error(message || '请求失败');
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
};
