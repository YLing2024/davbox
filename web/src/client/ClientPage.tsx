import { useCallback, useEffect, useRef, useState, type DragEvent, type ReactNode } from 'react'
import {
  Alert,
  App as AntApp,
  Breadcrumb,
  Button,
  Card,
  Form,
  Input,
  Layout,
  Modal,
  Space,
  Table,
  Typography,
  type TableProps,
} from 'antd'
import { clientApi, ApiError, type Entry } from '../api'
import { AppHeader } from '../appChrome'
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

const { Content } = Layout

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

function ClientShell({ who, actions, children }: { who?: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <AppHeader title="davbox" tag="文件" who={who} actions={actions} />
      <Content style={{ padding: '24px 16px 64px' }}>
        <div style={{ maxWidth: 1040, margin: '0 auto' }}>{children}</div>
      </Content>
    </Layout>
  )
}

export function ClientPage() {
  const { modal } = AntApp.useApp()
  const [ready, setReady] = useState(false)
  const [authed, setAuthed] = useState(false)
  const [user, setUser] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [path, setPath] = useState('/')
  const [entries, setEntries] = useState<Entry[]>([])
  const [busy, setBusy] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [prompt, setPrompt] = useState<{ title: string; initial: string; mode: 'new' | 'rename' } | null>(null)
  const [promptValue, setPromptValue] = useState('')
  const fileInput = useRef<HTMLInputElement>(null)

  const load = useCallback(async (target: string, silent = false) => {
    if (!silent) setError('')
    try {
      const list = await clientApi.list(target)
      setEntries(list)
      setPath(target)
      setAuthed(true)
      try {
        const me = await clientApi.me()
        setUser(me.user)
      } catch {
        /* 保持已有账号信息 */
      }
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

  const login = () => {
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
    setPromptValue('')
    setPrompt({ title: '新建文件夹名称', initial: '', mode: 'new' })
  }

  const rename = (entry: Entry) => {
    setPromptValue(entry.name)
    setPrompt({ title: '重命名', initial: entry.name, mode: 'rename' })
  }

  const submitPrompt = () => {
    if (!prompt) return
    const current = prompt
    const name = promptValue
    setPrompt(null)
    if (!name) return
    if (current.mode === 'rename' && name === current.initial) return
    if (name.includes('/')) {
      setError('名称不能包含斜杠')
      return
    }
    void run(async () => {
      if (current.mode === 'new') {
        await clientApi.mkdir(joinPath(path, name))
      } else {
        await clientApi.rename(joinPath(path, current.initial), joinPath(path, name))
      }
      await load(path)
    })
  }

  const remove = (entry: Entry) => {
    modal.confirm({
      title: `删除「${entry.name}」？此操作不可撤销。`,
      onOk: () =>
        run(async () => {
          await clientApi.remove(joinPath(path, entry.name))
          await load(path)
        }),
    })
  }

  const download = (entry: Entry) => {
    const a = document.createElement('a')
    a.href = clientApi.rawURL(joinPath(path, entry.name))
    a.download = entry.name
    document.body.appendChild(a)
    a.click()
    a.remove()
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    setDragging(false)
    if (e.dataTransfer.files.length > 0) uploadFiles(e.dataTransfer.files)
  }

  if (!ready) {
    return (
      <ClientShell>
        <Typography.Text type="secondary">加载中</Typography.Text>
      </ClientShell>
    )
  }

  if (!authed) {
    return (
      <ClientShell>
        <div style={{ display: 'flex', justifyContent: 'center' }}>
          <Card style={{ width: 380, maxWidth: '100%' }}>
            <Form layout="vertical" onFinish={login}>
              <Form.Item label="应用账号" htmlFor="client-user" style={{ marginBottom: 16 }}>
                <Input
                  id="client-user"
                  value={user}
                  onChange={(e) => setUser(e.target.value)}
                  autoComplete="username"
                  autoFocus
                />
              </Form.Item>
              <Form.Item label="口令" htmlFor="client-pass" style={{ marginBottom: 16 }}>
                <Input.Password
                  id="client-pass"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                />
              </Form.Item>
              {error ? (
                <Form.Item style={{ marginBottom: 16 }}>
                  <Alert type="error" message={error} showIcon />
                </Form.Item>
              ) : null}
              <Form.Item style={{ marginBottom: 0 }}>
                <Button type="primary" htmlType="submit" block loading={busy}>
                  进入
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </div>
      </ClientShell>
    )
  }

  const trail = crumbs(path)

  const columns: TableProps<Entry>['columns'] = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (_, e) => (
        <Space size={8}>
          <Typography.Text type="secondary">
            {e.isDir ? <IconFolder /> : <IconFile />}
          </Typography.Text>
          {e.isDir ? (
            <Typography.Link onClick={() => load(joinPath(path, e.name))}>{e.name}</Typography.Link>
          ) : (
            <Typography.Text
              style={{
                display: 'inline-block',
                maxWidth: 360,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                verticalAlign: 'bottom',
              }}
            >
              {e.name}
            </Typography.Text>
          )}
        </Space>
      ),
    },
    {
      title: '大小',
      dataIndex: 'size',
      key: 'size',
      width: 120,
      render: (_, e) => (
        <Typography.Text type="secondary">{e.isDir ? '—' : formatSize(e.size)}</Typography.Text>
      ),
    },
    {
      title: '修改时间',
      dataIndex: 'mtime',
      key: 'mtime',
      width: 180,
      render: (_, e) => (
        <Typography.Text type="secondary">{formatTime(e.mtime)}</Typography.Text>
      ),
    },
    {
      title: '操作',
      key: 'ops',
      width: 150,
      align: 'right',
      render: (_, e) => (
        <Space size={0}>
          {e.isDir ? null : (
            <Button
              type="text"
              title="下载"
              icon={<IconDownload />}
              disabled={busy}
              onClick={() => download(e)}
            />
          )}
          <Button type="text" title="重命名" icon={<IconPencil />} disabled={busy} onClick={() => rename(e)} />
          <Button
            type="text"
            danger
            title="删除"
            icon={<IconTrash />}
            disabled={busy}
            onClick={() => remove(e)}
          />
        </Space>
      ),
    },
  ]

  return (
    <ClientShell
      who={user}
      actions={
        <Button icon={<IconLogout />} onClick={logout}>
          退出
        </Button>
      }
    >
      <Breadcrumb
        aria-label="路径"
        style={{ marginBottom: 16 }}
        separator={<IconChevron size={14} />}
        items={trail.map((c, i) => ({
          key: c.path,
          title:
            i === trail.length - 1 ? (
              <Typography.Text>{c.label}</Typography.Text>
            ) : (
              <Typography.Link onClick={() => load(c.path)}>{c.label}</Typography.Link>
            ),
        }))}
      />

      <Space style={{ marginBottom: 16 }}>
        <Button
          type="primary"
          icon={<IconUpload />}
          onClick={() => fileInput.current?.click()}
          disabled={busy}
        >
          上传
        </Button>
        <Button icon={<IconFolderPlus />} onClick={newFolder} disabled={busy}>
          新建文件夹
        </Button>
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
      </Space>

      {error ? <Alert type="error" message={error} showIcon style={{ marginBottom: 16 }} /> : null}

      <div
        className={dragging ? 'davbox-drop davbox-drop-active' : 'davbox-drop'}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
      >
        <Table<Entry>
          rowKey="name"
          columns={columns}
          dataSource={entries}
          pagination={false}
          size="middle"
          scroll={{ x: 'max-content' }}
          locale={{ emptyText: '这里还没有文件' }}
        />
      </div>

      <Modal
        open={prompt !== null}
        title={prompt?.title}
        onCancel={() => setPrompt(null)}
        onOk={submitPrompt}
      >
        <Input
          value={promptValue}
          onChange={(e) => setPromptValue(e.target.value)}
          autoFocus
          onPressEnter={submitPrompt}
        />
      </Modal>
    </ClientShell>
  )
}
