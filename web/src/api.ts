export interface AccountView {
  user: string
  root: string
  readonly: boolean
  disabled: boolean
  note: string
  usedBytes: number
  usedFiles: number
}

export interface ConnInfo {
  url: string
  user: string
  pass: string
}

export type AuthMode = 'builtin' | 'sso'

export interface Entry {
  name: string
  isDir: boolean
  size: number
  mtime: number
}

class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'same-origin' }
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(body)
  }
  const res = await fetch(url, init)
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }
  if (!res.ok) {
    const message =
      data && typeof data === 'object' && 'error' in data
        ? String((data as { error: unknown }).error)
        : '请求失败'
    throw new ApiError(res.status, message)
  }
  return data as T
}

export { ApiError }

export const adminApi = {
  mode: () => request<{ mode: AuthMode }>('GET', '/api/admin/mode'),
  login: (password: string) => request<{ ok: boolean }>('POST', '/api/admin/login', { password }),
  logout: () => request<{ ok: boolean }>('POST', '/api/admin/logout'),
  list: () => request<AccountView[]>('GET', '/api/admin/accounts'),
  create: (user: string, readonly: boolean, note: string) =>
    request<{ user: string; pass: string; url: string }>('POST', '/api/admin/accounts', {
      user,
      readonly,
      note,
    }),
  conn: (user: string) => request<ConnInfo>('GET', `/api/admin/accounts/${encodeURIComponent(user)}/conn`),
  rotate: (user: string) =>
    request<{ user: string; pass: string; url: string }>(
      'POST',
      `/api/admin/accounts/${encodeURIComponent(user)}/rotate`,
    ),
  patch: (user: string, patch: { readonly?: boolean; disabled?: boolean; note?: string }) =>
    request<AccountView>('PATCH', `/api/admin/accounts/${encodeURIComponent(user)}`, patch),
  remove: (user: string) =>
    request<{ ok: boolean; message: string }>('DELETE', `/api/admin/accounts/${encodeURIComponent(user)}`),
}

function encodePath(p: string): string {
  return p
    .split('/')
    .map((seg) => encodeURIComponent(seg))
    .join('/')
}

export const clientApi = {
  login: (user: string, pass: string) =>
    request<{ ok: boolean; user: string }>('POST', '/api/client/login', { user, pass }),
  logout: () => request<{ ok: boolean }>('POST', '/api/client/logout'),
  me: () => request<{ user: string; readonly: boolean }>('GET', '/api/client/me'),
  list: (path: string) => request<Entry[]>('GET', `/api/client/list?path=${encodeURIComponent(path)}`),
  mkdir: (path: string) => request<{ ok: boolean }>('POST', '/api/client/mkdir', { path }),
  rename: (from: string, to: string) => request<{ ok: boolean }>('POST', '/api/client/rename', { from, to }),
  remove: (path: string) =>
    request<{ ok: boolean }>('DELETE', `/api/client/entry?path=${encodeURIComponent(path)}`),
  upload: async (target: string, file: File): Promise<void> => {
    const res = await fetch(`/api/client/raw${encodePath(target)}`, {
      method: 'PUT',
      credentials: 'same-origin',
      body: file,
    })
    if (!res.ok) {
      let message = '上传失败'
      try {
        const data = (await res.json()) as { error?: string }
        if (data.error) message = data.error
      } catch {
        /* 保持默认文案 */
      }
      throw new ApiError(res.status, message)
    }
  },
  rawURL: (path: string) => `/api/client/raw${encodePath(path)}`,
}
