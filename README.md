# davbox

一个**概念只有两个**的自建 WebDAV 服务：**admin 管账号，client 登账号管文件**。
主要给 App 用（思源笔记、Joplin 等通过 WebDAV 同步），一个应用一个账号一个目录，互不可见。

## 为什么自己做

市面上的开源 WebDAV 服务要么是给运维用的平台（SFTPGo：十几个概念、管理端全英文），
要么只有启动参数没有管理界面（dufs），要么隔离实测不可信（hacdias/webdav）。
需要的缺口只有薄薄一层：**极简的账号管理 + 可信的目录隔离**。协议层不重写，用 Go 官方实现。

## 技术栈

- 后端：Go 1.24 + `golang.org/x/net/webdav`（官方协议实现，不自研协议）
- 前端：React + Vite + TypeScript，构建产物 `//go:embed` 进二进制
- 账号：JSON 文件（+ JSONL 审计日志），不上数据库
- 交付：单个静态二进制

选型依据与全部被否方案见 [`docs/DECISIONS.md`](docs/DECISIONS.md)。

## 形态

```
admin 页  ── 新增/删除应用账号、改密码、看每个账号的目录与用量
client 页 ── 账号登录后浏览/上传/下载/删除/新建文件夹
WebDAV    ── https://<域名>/<应用名>/  ← 思源、Joplin 等 App 直接填这个
```

## 目录结构

```
cmd/davbox/     程序入口
internal/       账号、隔离、admin/client API
web/            前端源码（Vite + React + TS）
docs/           需求与选型文档
```

## 运行

```
make build            # 先构建前端，再编译出单二进制 ./davbox
./davbox -addr 127.0.0.1:18900 -data ./data
```

首次启动会在数据目录生成 `admin.json`、`admin-password.txt`、`secret.key`，并在 stdout 打印一次管理员初始口令。
管理页 `/admin`，文件页 `/`；App 的 WebDAV 地址填 `http(s)://<站点>/<应用名>`。

## 认证模式

管理端支持两种认证模式，由环境变量 `AUTH_MODE` 选择：

| 值 | 行为 |
|---|---|
| `builtin`（**默认**） | davbox 自带管理员口令：`/admin` 输入口令登录，使用内置 cookie 会话。开箱即用 |
| `sso` | 关闭自带口令，管理接口只认网关注入的 `X-Auth-User`；缺失或为空 → `401 JSON` |

```
AUTH_MODE=builtin ./davbox -addr 127.0.0.1:18900 -data ./data   # 默认，可省略
AUTH_MODE=sso     ./davbox -addr 127.0.0.1:18900 -data ./data
```

`builtin` 是默认值，行为与自带账号版本完全一致；`sso` 模式下启动日志会打印一行提示。

### sso 模式的安全前提

davbox **不解析也不校验** `X-Auth-User`（不验签、不看 JWT 或 cookie），它把该请求头当作「已经过网关认证」的事实。这个信任成立的前提是两条同时满足：

1. davbox 只监听回环地址（如 `127.0.0.1`），外部无法绕过网关直达；
2. 所有外部流量先经过网关，由网关完成登录，并**剥离客户端伪造的同名请求头**后再注入真实的 `X-Auth-User`。

只满足其一都不够：若 davbox 直接对公网暴露，任何人都能伪造 `X-Auth-User` 绕过认证。

### 接线示意（nginx + 网关）

```
浏览器 ──► nginx(example.com) ──► auth-gateway ──► davbox(127.0.0.1:18900)
             │  /_auth/*        登录与退出由网关处理
             │  其余请求        剥离外部 X-Auth-User → 注入认证后的 X-Auth-User → 反代到 davbox
             └─ 只反代回环，davbox 不对公网直接暴露
```

- 登录：`/_auth/login?next=<当前路径>`
- 退出：`/_auth/logout`
- 管理接口在 `sso` 下未带 `X-Auth-User` 时统一返回 `401 JSON`（不重定向、不下发 cookie）

### 两种模式都不受影响的部分

- WebDAV 数据面 `/<账号名>/...`：始终用应用账号 + HTTP Basic，与 `AUTH_MODE` 无关
- 客户端接口 `/api/client/*` 与静态资源 `/assets/*`
- 应用账号的增删改查在两种模式下都需要管理端身份：`builtin` 用管理员口令，`sso` 用 `X-Auth-User`

## 状态

阶段一：底盘（账号隔离 + 六动词 WebDAV）—— 已完成
阶段二：admin 页 + client 页 —— 已完成
阶段三（可选）：配额、用量趋势、分享链接
