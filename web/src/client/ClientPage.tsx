import { useCallback, useEffect, useRef, useState, type DragEvent, type FormEvent } from 'react'
import { clientApi, ApiError, type Entry } from '../api'
import { formatSize, formatTime, joinPath } from '../format'
import {
  IconChevron,
  IconDownload,
  IconFile,
  IconFolder,
  IconFolderPlus,
  IconLogout,
  IconPencil,
  IconTrash,
  IconUpload,
} from '../icons'

function crumbs(path: string): { label: string; path: string }[] {
  const segments = path.split('/').filter(Boolean)
  const out = [{ label: '根目录', path: '/' }]
  let acc = ''
  for (const seg of segments) {
    acc += `/${seg}`
    out.push({ label: seg, path: acc })
  }
  return out
}

export function ClientPage() {
  const [ready, setReady] = useState(false)
  const [authed, setAuthed] = useState(false)
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [path, setPath] = useState('/')
  const [entries, setEntries] = useState<Entry[]>([])
  const [busy, setBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)

  const load = useCallback(async (target: string, silent = false) => {
    if (!silent) setError('')
    try {
      const list = await clientApi.list(target)
      setEntries(list)
      setPath(target)
      setAuthed(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setAuthed(false)
      } else if (!silent) {
        setError(err instanceof Error ? err.message : '加载失败')
      }
    } finally {
      setReady(true)
    }
  }, [])

  useEffect(() => {
    void load('/', true)
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
      const res = await clientApi.login(user.trim(), password)
      setUser(res.user)
      setPassword('')
      await load('/')
    })
  }

  const logout = () => {
    void run(async () => {
      await clientApi.logout()
      setAuthed(false)
      setEntries([])
      setUser('')
      setPath('/')
    })
  }

  const uploadFiles = (files: FileList | File[]) => {
    const list = Array.from(files)
    if (list.length === 0) return
    const dir = path
    void run(async () => {
      for (const file of list) {
        await clientApi.upload(joinPath(dir, file.name), file)
      }
      await load(dir)
    })
  }

  const newFolder = () => {
    const name = window.prompt('新建文件夹名称')
    if (!name) return
    if (name.includes('/')) {
      setError('名称不能包含斜杠')
      return
    }
    void run(async () => {
      await clientApi.mkdir(joinPath(path, name))
      await load(path)
    })
  }

  const rename = (entry: Entry) => {
    const name = window.prompt('重命名', entry.name)
    if (!name || name === entry.name) return
    if (name.includes('/')) {
      setError('名称不能包含斜杠')
      return
    }
    void run(async () => {
      await clientApi.rename(joinPath(path, entry.name), joinPath(path, name))
      await load(path)
    })
  }

  const remove = (entry: Entry) => {
    if (!window.confirm(`删除「${entry.name}」？此操作不可撤销。`)) return
    void run(async () => {
      await clientApi.remove(joinPath(path, entry.name))
      await load(path)
    })
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    if (e.dataTransfer.files.length > 0) uploadFiles(e.dataTransfer.files)
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
          <span className="tag">文件</span>
        </header>
        <form className="login" onSubmit={login}>
          <label htmlFor="client-user">应用账号</label>
          <input
            id="client-user"
            value={user}
            onChange={(e) => setUser(e.target.value)}
            autoComplete="username"
            autoFocus
          />
          <label htmlFor="client-pass">口令</label>
          <input
            id="client-pass"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
          {error ? <p className="error">{error}</p> : null}
          <button type="submit" className="btn primary wide" disabled={busy}>
            进入
          </button>
        </form>
      </div>
    )
  }

  const trail = crumbs(path)

  return (
    <div className="page">
      <header className="masthead">
        <span className="wordmark">davbox</span>
        <span className="tag">文件</span>
        <span className="spacer" />
        <button type="button" className="btn ghost" onClick={logout}>
          <IconLogout />
          退出
        </button>
      </header>

      <nav className="crumbs" aria-label="路径">
        {trail.map((c, i) => (
          <span className="crumb" key={c.path}>
            {i > 0 ? <IconChevron size={14} /> : null}
            <button
              type="button"
              className={i === trail.length - 1 ? 'crumb-link current' : 'crumb-link'}
              onClick={() => load(c.path)}
              disabled={i === trail.length - 1}
            >
              {c.label}
            </button>
          </span>
        ))}
      </nav>

      <div className="toolbar">
        <button type="button" className="btn primary" onClick={() => fileInput.current?.click()} disabled={busy}>
          <IconUpload />
          上传
        </button>
        <button type="button" className="btn" onClick={newFolder} disabled={busy}>
          <IconFolderPlus />
          新建文件夹
        </button>
        <input
          ref={fileInput}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            if (e.target.files) uploadFiles(e.target.files)
            e.target.value = ''
          }}
        />
      </div>

      {error ? <p className="error">{error}</p> : null}

      <div
        className={dragging ? 'list drop active' : 'list drop'}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
      >
        <div className="row head">
          <div>名称</div>
          <div>大小</div>
          <div>修改时间</div>
          <div className="col-ops">操作</div>
        </div>
        {entries.length === 0 ? <div className="empty">这里还没有文件</div> : null}
        {entries.map((e) => {
          const full = joinPath(path, e.name)
          return (
            <div className="row" key={e.name}>
              <div className="name">
                <span className="file-icon">{e.isDir ? <IconFolder /> : <IconFile />}</span>
                {e.isDir ? (
                  <button type="button" className="linkish" onClick={() => load(full)}>
                    {e.name}
                  </button>
                ) : (
                  <span className="ellipsis">{e.name}</span>
                )}
              </div>
              <div className="mono">{e.isDir ? '—' : formatSize(e.size)}</div>
              <div className="mono">{formatTime(e.mtime)}</div>
              <div className="ops">
                {e.isDir ? null : (
                  <a className="icon-btn" href={clientApi.rawURL(full)} download={e.name} title="下载">
                    <IconDownload />
                  </a>
                )}
                <button
                  type="button"
                  className="icon-btn"
                  title="重命名"
                  onClick={() => rename(e)}
                  disabled={busy}
                >
                  <IconPencil />
                </button>
                <button
                  type="button"
                  className="icon-btn danger"
                  title="删除"
                  onClick={() => remove(e)}
                  disabled={busy}
                >
                  <IconTrash />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
