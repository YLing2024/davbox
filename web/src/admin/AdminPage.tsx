import { useCallback, useEffect, useReducer, useRef, useState, type ReactNode } from 'react'
import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Checkbox,
  Descriptions,
  Form,
  Input,
  Layout,
  Modal,
  Space,
  Switch,
  Table,
  Typography,
  type TableProps,
} from 'antd'
import { adminApi, ApiError, type AccountView, type AuthMode, type ConnInfo } from '../api'
import { AppHeader } from '../appChrome'
import { parseCorsOrigins } from '../corsOrigins'
import { formatSize } from '../format'
import { initialSettingsModalState, settingsModalReducer } from '../settingsModal'
import {
  IconCheck,
  IconCopy,
  IconKey,
  IconLogout,
  IconPlus,
  IconPower,
  IconSettings,
  IconTrash,
} from '../icons'

const { Content } = Layout

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

// SSO 模式下 401 统一交给网关登录页处理。
function redirectToGatewayLogin() {
  const next = window.location.pathname + window.location.search
  window.location.href = `/_auth/login?next=${encodeURIComponent(next)}`
}

function AdminShell({
  actions,
  centered,
  children,
}: {
  actions?: ReactNode
  centered?: boolean
  children: ReactNode
}) {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <AppHeader title="davbox" tag="管理" actions={actions} />
      {centered ? (
        <Content
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '64px 16px',
          }}
        >
          {children}
        </Content>
      ) : (
        <Content style={{ padding: '24px 16px 64px' }}>
          <div style={{ maxWidth: 1040, margin: '0 auto' }}>{children}</div>
        </Content>
      )}
    </Layout>
  )
}

function ConnModal({ conn, onClose }: { conn: ConnInfo; onClose: () => void }) {
  const [copied, setCopied] = useState<Record<string, boolean>>({})

  const copyItem = async (key: string, value: string) => {
    if (await copyText(value)) {
      setCopied((state) => ({ ...state, [key]: true }))
      window.setTimeout(() => setCopied((state) => ({ ...state, [key]: false })), 1600)
    }
  }

  const fields: { key: string; label: string; value: string }[] = [
    { key: 'url', label: '地址', value: conn.url },
    { key: 'user', label: '用户名', value: conn.user },
    { key: 'pass', label: '口令', value: conn.pass },
  ]

  return (
    <Modal
      open
      title="连接信息"
      onCancel={onClose}
      footer={[
        <Button
          key="copy"
          type="primary"
          icon={<IconCopy />}
          onClick={async () => {
            if (await copyText(connText(conn))) {
              setCopied((state) => ({ ...state, all: true }))
              window.setTimeout(() => setCopied((state) => ({ ...state, all: false })), 1600)
            }
          }}
        >
          {copied.all ? '已复制' : '复制连接信息'}
        </Button>,
        <Button key="close" onClick={onClose}>
          关闭
        </Button>,
      ]}
    >
      <Descriptions column={1} size="small" bordered>
        {fields.map((field) => (
          <Descriptions.Item key={field.key} label={field.label}>
            <Space style={{ width: '100%', justifyContent: 'space-between' }} align="start">
              <Typography.Text code style={{ wordBreak: 'break-all' }}>
                {field.value}
              </Typography.Text>
              <Button
                type="text"
                size="small"
                aria-label={copied[field.key] ? '已复制' : `复制${field.label}`}
                title={copied[field.key] ? '已复制' : `复制${field.label}`}
                icon={copied[field.key] ? <IconCheck /> : <IconCopy />}
                onClick={() => copyItem(field.key, field.value)}
              />
            </Space>
          </Descriptions.Item>
        ))}
      </Descriptions>
      <Typography.Paragraph type="secondary" style={{ margin: '16px 0 0' }}>
        把地址填进 App 的 WebDAV 设置。
      </Typography.Paragraph>
    </Modal>
  )
}

function SettingsModal({
  text,
  error,
  saved,
  saving,
  onChange,
  onSave,
  onClose,
}: {
  text: string
  error: string
  saved: boolean
  saving: boolean
  onChange: (text: string) => void
  onSave: () => void
  onClose: () => void
}) {
  return (
    <Modal
      open
      title="设置"
      onCancel={onClose}
      footer={[
        <Button key="save" type="primary" loading={saving} onClick={onSave}>
          保存
        </Button>,
        <Button key="close" onClick={onClose}>
          关闭
        </Button>,
      ]}
    >
      <label htmlFor="cors-origins" style={{ display: 'block' }}>
        <Typography.Text>跨域白名单（CORS）</Typography.Text>
      </label>
      <Typography.Paragraph type="secondary" style={{ margin: '4px 0 8px' }}>
        允许这些来源的网页在浏览器里直接访问本 WebDAV。留空表示关闭跨域。请只填你自己的前端域名。
      </Typography.Paragraph>
      <Input.TextArea
        id="cors-origins"
        rows={4}
        spellCheck={false}
        value={text}
        onChange={(e) => onChange(e.target.value)}
        placeholder="https://app.example.com"
      />
      {error ? <Alert type="error" message={error} showIcon style={{ marginTop: 12 }} /> : null}
      {saved ? (
        <Typography.Text type="success" style={{ display: 'block', marginTop: 12 }}>
          已保存并即时生效
        </Typography.Text>
      ) : null}
    </Modal>
  )
}

/**
 * 管理端页面：builtin 模式下渲染口令登录与账号列表，sso 模式下未认证时交回网关登录。
 *
 * @remarks 账号根目录与用量来自 `/api/admin/accounts`；侧边设置弹窗维护跨域白名单。
 */
export function AdminPage() {
  const { modal } = AntApp.useApp()
  const [mode, setMode] = useState<AuthMode | null>(null)
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
  const [corsOrigins, setCorsOrigins] = useState<string[]>([])
  const [settings, dispatchSettings] = useReducer(
    settingsModalReducer,
    undefined,
    initialSettingsModalState,
  )
  const modeRef = useRef<AuthMode>('builtin')
  const corsLoadedRef = useRef(false)

  const handleUnauthorized = useCallback(() => {
    if (modeRef.current === 'sso') {
      redirectToGatewayLogin()
      return
    }
    setAuthed(false)
  }, [])

  const load = useCallback(async () => {
    try {
      const list = await adminApi.list()
      setAccounts(list)
      setAuthed(true)
      if (!corsLoadedRef.current) {
        const st = await adminApi.getSettings()
        setCorsOrigins(st.corsOrigins)
        corsLoadedRef.current = true
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        handleUnauthorized()
      } else {
        setError(err instanceof Error ? err.message : '加载失败')
      }
    } finally {
      setReady(true)
    }
  }, [handleUnauthorized])

  useEffect(() => {
    let alive = true
    void (async () => {
      let detected: AuthMode = 'builtin'
      try {
        const info = await adminApi.mode()
        if (info.mode === 'sso') detected = 'sso'
      } catch {
        /* 取不到模式时按 builtin 处理，保持默认行为 */
      }
      if (!alive) return
      modeRef.current = detected
      setMode(detected)
      await load()
    })()
    return () => {
      alive = false
    }
  }, [load])

  const run = async (fn: () => Promise<void>) => {
    setError('')
    setBusy(true)
    try {
      await fn()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        handleUnauthorized()
      } else {
        setError(err instanceof Error ? err.message : '操作失败')
      }
    } finally {
      setBusy(false)
    }
  }

  const login = () => {
    void run(async () => {
      await adminApi.login(password)
      setPassword('')
      await load()
    })
  }

  const logout = () => {
    if (mode === 'sso') {
      window.location.href = '/_auth/logout'
      return
    }
    void run(async () => {
      await adminApi.logout()
      setAuthed(false)
      setAccounts([])
      corsLoadedRef.current = false
      setCorsOrigins([])
      dispatchSettings({ type: 'close' })
    })
  }

  const createAccount = () => {
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
    modal.confirm({
      title: `重新生成「${user}」的口令？旧口令会立即失效。`,
      onOk: () =>
        run(async () => {
          const info = await adminApi.rotate(user)
          setConn({ url: `${window.location.origin}/${info.user}`, user: info.user, pass: info.pass })
        }),
    })
  }

  const patch = (user: string, body: { readonly?: boolean; disabled?: boolean }) => {
    void run(async () => {
      await adminApi.patch(user, body)
      await load()
    })
  }

  const remove = (user: string) => {
    modal.confirm({
      title: `删除账号「${user}」？磁盘上的目录会保留。`,
      onOk: () =>
        run(async () => {
          await adminApi.remove(user)
          await load()
        }),
    })
  }

  const openSettings = () => {
    dispatchSettings({ type: 'open', origins: corsOrigins })
  }

  const saveSettings = () => {
    const parsed = parseCorsOrigins(settings.text)
    if (parsed.error) {
      dispatchSettings({ type: 'save-error', message: parsed.error })
      return
    }
    dispatchSettings({ type: 'save-start' })
    void (async () => {
      setBusy(true)
      try {
        const saved = await adminApi.saveSettings(parsed.origins)
        setCorsOrigins(saved.corsOrigins)
        dispatchSettings({ type: 'save-ok', origins: saved.corsOrigins })
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          handleUnauthorized()
        } else {
          dispatchSettings({
            type: 'save-error',
            message: err instanceof Error ? err.message : '保存失败',
          })
        }
      } finally {
        setBusy(false)
      }
    })()
  }

  if (!ready || mode === null) {
    return (
      <AdminShell centered>
        <Typography.Text type="secondary">加载中</Typography.Text>
      </AdminShell>
    )
  }

  if (!authed) {
    // SSO 模式不显示口令输入框；未认证时已跳转网关登录页。
    if (mode === 'sso') {
      return (
        <AdminShell centered>
          <Typography.Text type="secondary">正在跳转登录</Typography.Text>
        </AdminShell>
      )
    }
    return (
      <AdminShell centered>
        <Card style={{ width: 380, maxWidth: '100%' }}>
          <Form layout="vertical" onFinish={login}>
            <Form.Item label="管理员口令" htmlFor="admin-pass" style={{ marginBottom: 16 }}>
              <Input.Password
                id="admin-pass"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                autoFocus
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
      </AdminShell>
    )
  }

  const columns: TableProps<AccountView>['columns'] = [
    {
      title: '应用',
      dataIndex: 'user',
      key: 'user',
      render: (_, a) => (
        <Space size={8}>
          <Typography.Text code className="davbox-ellipsis" title={a.user}>
            {a.user}
          </Typography.Text>
          {a.note ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {a.note}
            </Typography.Text>
          ) : null}
        </Space>
      ),
    },
    {
      title: '目录',
      dataIndex: 'root',
      key: 'root',
      render: (_, a) => (
        <Typography.Text
          code
          title={a.root}
          style={{
            display: 'inline-block',
            maxWidth: 260,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            verticalAlign: 'bottom',
          }}
        >
          {a.root}
        </Typography.Text>
      ),
    },
    {
      title: '已用',
      dataIndex: 'usedBytes',
      key: 'used',
      render: (_, a) => <Typography.Text type="secondary">{formatSize(a.usedBytes)}</Typography.Text>,
    },
    {
      title: '只读',
      key: 'readonly',
      width: 72,
      render: (_, a) => (
        <Switch size="small" checked={a.readonly} onChange={() => patch(a.user, { readonly: !a.readonly })} />
      ),
    },
    {
      title: '停用',
      key: 'disabled',
      width: 72,
      render: (_, a) => (
        <Switch size="small" checked={a.disabled} onChange={() => patch(a.user, { disabled: !a.disabled })} />
      ),
    },
    {
      title: '操作',
      key: 'ops',
      width: 180,
      align: 'right',
      render: (_, a) => (
        <Space size={0} className="davbox-actions">
          <Button
            type="text"
            title="复制连接信息"
            icon={<IconCopy />}
            disabled={busy}
            onClick={() => showConn(a.user)}
          />
          <Button type="text" title="换口令" icon={<IconKey />} disabled={busy} onClick={() => rotate(a.user)} />
          <Button
            type="text"
            title={a.disabled ? '启用' : '停用'}
            icon={<IconPower />}
            disabled={busy}
            onClick={() => patch(a.user, { disabled: !a.disabled })}
          />
          <Button
            type="text"
            danger
            title="删除"
            icon={<IconTrash />}
            disabled={busy}
            onClick={() => remove(a.user)}
          />
        </Space>
      ),
    },
  ]

  const headerActions = (
    <>
      <Button icon={<IconSettings />} onClick={openSettings}>
        设置
      </Button>
      <Button icon={<IconLogout />} onClick={logout}>
        退出
      </Button>
    </>
  )

  return (
    <AdminShell actions={headerActions}>
      <Space className="davbox-toolbar" style={{ marginBottom: 16 }}>
        <Button type="primary" icon={<IconPlus />} onClick={() => setShowNew((v) => !v)}>
          新建应用
        </Button>
      </Space>

      {showNew ? (
        <Card size="small" style={{ marginBottom: 16 }}>
          <Form layout="inline" onFinish={createAccount}>
            <Form.Item label="应用名" htmlFor="new-user" style={{ flex: 1, minWidth: 220 }}>
              <Input
                id="new-user"
                value={newUser}
                onChange={(e) => setNewUser(e.target.value)}
                placeholder="小写字母、数字、下划线或连字符"
                autoFocus
              />
            </Form.Item>
            <Form.Item>
              <Checkbox checked={newReadonly} onChange={(e) => setNewReadonly(e.target.checked)}>
                只读
              </Checkbox>
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" disabled={busy || !newUser.trim()}>
                创建
              </Button>
            </Form.Item>
          </Form>
        </Card>
      ) : null}

      {error ? <Alert type="error" message={error} showIcon style={{ marginBottom: 16 }} /> : null}

      <Table<AccountView>
        rowKey="user"
        columns={columns}
        dataSource={accounts}
        pagination={false}
        size="middle"
        scroll={{ x: 'max-content' }}
        locale={{ emptyText: '还没有应用账号' }}
        rowClassName={(a) => (a.disabled ? 'davbox-row-disabled' : '')}
      />

      {conn ? <ConnModal conn={conn} onClose={() => setConn(null)} /> : null}

      {settings.open ? (
        <SettingsModal
          text={settings.text}
          error={settings.error}
          saved={settings.saved}
          saving={settings.saving}
          onChange={(text) => dispatchSettings({ type: 'edit', text })}
          onSave={saveSettings}
          onClose={() => dispatchSettings({ type: 'close' })}
        />
      ) : null}
    </AdminShell>
  )
}
