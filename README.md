# davbox

一个**概念只有两个**的自建 WebDAV 服务：**admin 管账号，client 登账号管文件**。
主要给 App 用（思源笔记、Joplin 等通过 WebDAV 同步），一个应用一个账号一个目录，互不可见。

![License](https://img.shields.io/badge/license-MIT-blue)

## 为什么自己做

市面上的开源 WebDAV 服务要么是给运维用的平台（SFTPGo：十几个概念、管理端全英文），
要么只有启动参数没有管理界面（dufs），要么隔离实测不可信（hacdias/webdav）。
需要的缺口只有薄薄一层：**极简的账号管理 + 可信的目录隔离**。协议层不重写，用 Go 官方实现。

## 特性

- **一个应用一个账号一个目录**：每个账号只看得到自己的根目录，跨账号访问一律拒绝，路径逃逸（`../`、`%2e%2e`、双重编码）一律拒绝
- **admin 页**：新增 / 停用 / 删除账号、换口令、查看每个账号的目录与用量，一键「复制连接信息」（地址 + 用户名 + 口令）
- **client 页**：账号登录后浏览 / 上传（含拖拽）/ 下载 / 重命名 / 新建文件夹 / 删除
- **只读账号**：写动词一律 `403`，读正常
- **协议用官方实现**：`golang.org/x/net/webdav`（含 `LOCK` / `UNLOCK` / `COPY` / `MOVE` / 死属性持久化），不自研协议
- **单二进制交付**：前端 `//go:embed` 打进二进制，无运行时、无数据库、无中间件依赖
- **认证可切换**：默认自带管理员口令（开箱即用），也可关掉、改接你自己的 SSO 网关（见下）

## 快速开始

```bash
git clone https://github.com/YLing2024/davbox.git
cd davbox
make build        # 先构建前端，再编译出单二进制 ./davbox
./davbox          # 默认监听 127.0.0.1:18900，数据落在 ./data
```

首次启动会生成 `admin.json`（管理员口令的 bcrypt 哈希）、`admin-password.txt`、`secret.key`，
并在 stdout **打印一次**管理员初始口令 —— 请立刻保存。

- 管理页 `/admin`，文件页 `/`
- App 的 WebDAV 地址填 `http(s)://<你的站点>/<应用名>`

启动参数：

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-addr` | `127.0.0.1:18900` | 监听地址。**建议只监听回环**，由 nginx 反代提供 TLS 与公网入口 |
| `-data` | `./data` | 数据目录（账号表、管理员口令、会话密钥、各账号目录） |

## 目录结构

```
cmd/davbox/     程序入口
internal/       账号、隔离、admin/client API
web/            前端源码（Vite + React + TS）
docs/           需求、选型与验收记录
```

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

## 部署（nginx 反代）

```nginx
server {
    listen 443 ssl;
    server_name dav.example.com;

    location / {
        proxy_pass http://127.0.0.1:18900;
        proxy_request_buffering off;   # WebDAV 请求体流式直传，大文件不落盘
        client_max_body_size 0;        # 不限体积
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

接 `sso` 模式时再额外做两件事：把 `/_auth/`、`/admin`、`/api/admin/` 指向网关（`/_auth/*` 的 location 必须排在受保护 location 之前），
WebDAV 数据面与 `/assets/*` 直连 davbox —— 协议端点必须保持自带 Basic 认证，否则 App 无法同步。

## 状态

- 阶段一：底盘（账号隔离 + 六动词 WebDAV）—— 已完成
- 阶段二：admin 页 + client 页 —— 已完成
- 阶段三（可选）：配额、用量趋势、分享链接

## License

[MIT](LICENSE)
