import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { adminApi, ApiError, type AccountView, type ConnInfo } from '../api'
import { formatSize } from '../format'
import {
  IconCopy,
  IconKey,
  IconLogout,
  IconPlus,
  IconPower,
  IconTrash,
} from '../icons'

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    const area = document.createElement('textarea')
    area.value = text
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    let ok = false
    try {
      ok = document.execCommand('copy')
    } catch {
      ok = false
    }
    document.body.removeChild(area)
    return ok
  }
}

function connText(conn: ConnInfo): string {
  return `地址：${conn.url}\n用户名：${conn.user}\n口令：${conn.pass}`
}

function Switch({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean
  onChange: () => void
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      className={checked ? 'switch on' : 'switch'}
      onClick={onChange}
      disabled={disabled}
    >
      <span className="knob" />
    </button>
  )
}

function ConnModal({ conn, onClose }: { conn: ConnInfo; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{'连接信息'}</h2>
        <dl className="conn">
          <dt>地址</dt>
          <dd>{conn.url}</dd>
          <dt>用户名</dt>
          <dd>{conn.user}</dd>
          <dt>口令</dt>
          <dd>{conn.pass}</dd>
        </dl>
        <p className="hint">把地址填进 App 的 WebDAV 设置。</p>
        <div className="modal-actions">
          <button
            type="button"
            className="btn primary"
            onClick={async () => {
              if (await copyText(connText(conn))) {
                setCopied(true)
                window.setTimeout(() => setCopied(false), 1600)
              }
            }}
          >
            <IconCopy />
            {copied ? '已复制' : '复制连接信息'}
          </button>
          <button type="button" className="btn" onClick={onClose}>
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}

export function AdminPage() {
  const [ready, setReady] = useState(false)
  const [authed, setAuthed] = useState(false)
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [accounts, setAccounts] = useState<AccountView[]>([])
  const [busy, setBusy] = useState(false)
  const [conn, setConn] = useState<ConnInfo | null>(null)
  const [showNew, setShowNew] = useState(false)
  const [newUser, setNewUser] = useState('')
  const [newReadonly, setNewReadonly] = useState(false)

  const load = useCallback(async () => {
    try {
      const list = await adminApi.list()
      setAccounts(list)
      setAuthed(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAuthed(false)
      } else {
        setError(err instanceof Error ? err.message : '加载失败')
      }
    } finally {
      setReady(true)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const run = async (fn: () => Promise<void>) => {
    setError('')
    setBusy(true)
    try {
      await fn()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAuthed(false)
      } else {
        setError(err instanceof Error ? err.message : '操作失败')
      }
    } finally {
      setBusy(false)
    }
  }

  const login = (e: FormEvent) => {
    e.preventDefault()
    void run(async () => {
      await adminApi.login(password)
      setPassword('')
      await load()
    })
  }

  const logout = () => {
    void run(async () => {
      await adminApi.logout()
      setAuthed(false)
      setAccounts([])
    })
  }

  const createAccount = (e: FormEvent) => {
    e.preventDefault()
    const user = newUser.trim()
    if (!user) return
    void run(async () => {
      const created = await adminApi.create(user, newReadonly, '')
      setNewUser('')
      setNewReadonly(false)
      setShowNew(false)
      setConn({ url: `${window.location.origin}/${created.user}`, user: created.user, pass: created.pass })
      await load()
    })
  }

  const showConn = (user: string) => {
    void run(async () => {
      const info = await adminApi.conn(user)
      setConn({ url: `${window.location.origin}/${info.user}`, user: info.user, pass: info.pass })
    })
  }

  const rotate = (user: string) => {
    if (!window.confirm(`重新生成「${user}」的口令？旧口令会立即失效。`)) return
    void run(async () => {
      const info = await adminApi.rotate(user)
      setConn({ url: `${window.location.origin}/${info.user}`, user: info.user, pass: info.pass })
    })
  }

  const patch = (user: string, body: { readonly?: boolean; disabled?: boolean }) => {
    void run(async () => {
      await adminApi.patch(user, body)
      await load()
    })
  }

  const remove = (user: string) => {
    if (!window.confirm(`删除账号「${user}」？磁盘上的目录会保留。`)) return
    void run(async () => {
      await adminApi.remove(user)
      await load()
    })
  }

  if (!ready) {
    return (
      <div className="page">
        <div className="center-note">加载中</div>
      </div>
    )
  }

  if (!authed) {
    return (
      <div className="page narrow">
        <header className="masthead">
          <span className="wordmark">davbox</span>
          <span className="tag">管理</span>
        </header>
        <form className="login" onSubmit={login}>
          <label htmlFor="admin-pass">管理员口令</label>
          <input
            id="admin-pass"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            autoFocus
          />
          {error ? <p className="error">{error}</p> : null}
          <button type="submit" className="btn primary wide" disabled={busy}>
            进入
          </button>
        </form>
      </div>
    )
  }

  return (
    <div className="page">
      <header className="masthead">
        <span className="wordmark">davbox</span>
        <span className="tag">管理</span>
        <span className="spacer" />
        <button type="button" className="btn ghost" onClick={logout}>
          <IconLogout />
          退出
        </button>
      </header>

      <div className="toolbar">
        <button type="button" className="btn primary" onClick={() => setShowNew((v) => !v)}>
          <IconPlus />
          新建应用
        </button>
      </div>

      {showNew ? (
        <form className="panel new-app" onSubmit={createAccount}>
          <div className="field">
            <label htmlFor="new-user">应用名</label>
            <input
              id="new-user"
              value={newUser}
              onChange={(e) => setNewUser(e.target.value)}
              placeholder="小写字母、数字、下划线或连字符"
              autoFocus
            />
          </div>
          <label className="check">
            <input
              type="checkbox"
              checked={newReadonly}
              onChange={(e) => setNewReadonly(e.target.checked)}
            />
            只读
          </label>
          <button type="submit" className="btn primary" disabled={busy || !newUser.trim()}>
            创建
          </button>
        </form>
      ) : null}

      {error ? <p className="error">{error}</p> : null}

      <div className="list account-list">
        <div className="row head">
          <div>应用</div>
          <div className="col-root">目录</div>
          <div>已用</div>
          <div>只读</div>
          <div>停用</div>
          <div className="col-ops">操作</div>
        </div>
        {accounts.length === 0 ? <div className="empty">还没有应用账号</div> : null}
        {accounts.map((a) => (
          <div className={a.disabled ? 'row muted' : 'row'} key={a.user}>
            <div className="name">
              <span className="mono">{a.user}</span>
              {a.note ? <span className="note">{a.note}</span> : null}
            </div>
            <div className="col-root mono ellipsis">{a.root}</div>
            <div className="mono">{formatSize(a.usedBytes)}</div>
            <div>
              <Switch checked={a.readonly} onChange={() => patch(a.user, { readonly: !a.readonly })} />
            </div>
            <div>
              <Switch checked={a.disabled} onChange={() => patch(a.user, { disabled: !a.disabled })} />
            </div>
            <div className="ops">
              <button
                type="button"
                className="icon-btn"
                title="复制连接信息"
                onClick={() => showConn(a.user)}
                disabled={busy}
              >
                <IconCopy />
              </button>
              <button
                type="button"
                className="icon-btn"
                title="换口令"
                onClick={() => rotate(a.user)}
                disabled={busy}
              >
                <IconKey />
              </button>
              <button
                type="button"
                className="icon-btn"
                title={a.disabled ? '启用' : '停用'}
                onClick={() => patch(a.user, { disabled: !a.disabled })}
                disabled={busy}
              >
                <IconPower />
              </button>
              <button
                type="button"
                className="icon-btn danger"
                title="删除"
                onClick={() => remove(a.user)}
                disabled={busy}
              >
                <IconTrash />
              </button>
            </div>
          </div>
        ))}
      </div>

      {conn ? <ConnModal conn={conn} onClose={() => setConn(null)} /> : null}
    </div>
  )
}
