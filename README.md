[简体中文](README.md) ｜ [English](README.en.md)

# davbox

一个概念只有两个的自建 WebDAV 服务：**admin 管账号，client 登账号管文件**。
面向需要 WebDAV 同步的 App（思源笔记、Joplin 等），一个应用一个账号一个目录，互不可见。

![License](https://img.shields.io/badge/license-MIT-blue)

## 为什么自己做

市面上的开源 WebDAV 服务要么是给运维用的平台（SFTPGo：十几个概念、管理端全英文），
要么只有启动参数没有管理界面（dufs），要么隔离实测不可信（hacdias/webdav）。
需要的缺口只有薄薄一层：**极简的账号管理 + 可信的目录隔离**。协议层不重写，用 Go 官方实现。

## 特性

- **一个应用一个账号一个目录**：每个账号只看得到自己的根目录，跨账号访问一律拒绝；路径逃逸（`../`、编码后的 `%2e`、反斜杠、空字节）一律 `400`
- **admin 页**：新增 / 停用 / 删除账号、换口令、查看每个账号的目录与用量，一键「复制连接信息」（地址 + 用户名 + 口令）
- **client 页**：账号登录后浏览 / 上传（含拖拽）/ 下载 / 重命名 / 新建文件夹 / 删除
- **只读账号**：写动词一律 `403`，读正常
- **协议用官方实现**：`golang.org/x/net/webdav`（含 `LOCK` / `UNLOCK` / `COPY` / `MOVE` / 死属性持久化），不自研协议
- **单二进制交付**：前端 `//go:embed` 打进二进制，无外部运行时、无数据库
- **管理端认证可切换**：默认自带管理员口令，也可以关闭自带口令、改由网关注入身份

## 快速开始

```bash
git clone https://github.com/YLing2024/davbox.git
cd davbox
make build        # 先构建前端，再编译出单二进制 ./davbox
./davbox          # 默认监听 127.0.0.1:18900，数据落在 ./data
```

首次启动（`builtin` 模式）会在数据目录生成：

- `accounts.json` —— 账号表
- `admin.json` —— 管理员口令的 bcrypt 哈希
- `admin-password.txt` —— 管理员初始口令明文
- `secret.key` —— 会话 cookie 的签名密钥

管理员初始口令只在首次生成时于 stdout 打印一次，请立刻保存。

- 管理页 `/admin`，文件页 `/`
- App 的 WebDAV 地址填 `http(s)://<你的站点>/<应用名>`

启动参数：

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-addr` | `127.0.0.1:18900` | 监听地址。建议只监听回环，由 nginx 反代提供 TLS 与入口 |
| `-data` | `./data` | 数据目录（账号表、管理员凭据、会话密钥、各账号目录） |

Makefile 其他目标：`make frontend` 只构建前端，`make run` 编译后直接启动，`make vet` 静态检查，`make test` 跑测试，`make clean` 清理产物。

## 目录结构

```
cmd/davbox/     程序入口
internal/       账号、认证与会话，以及 WebDAV 和 admin/client 路由
web/            Vite + React + TS 前端（构建产物由 go:embed 嵌入）
docs/           需求、选型与验收记录
```

## 配置

| 名称 | 默认值 | 说明 |
|---|---|---|
| `AUTH_MODE` | `builtin` | 管理端认证模式，取值 `builtin` 或 `sso` |
| `CORS_ORIGINS` | （空，关闭） | 浏览器跨域直连的来源白名单，逗号分隔，精确匹配 `scheme://host[:port]`；留空即完全关闭 |

- `builtin`：自带管理员口令，`/admin` 输入口令登录，使用签名 cookie 会话（12 小时）。
- `sso`：不使用自带口令登录，管理端身份取自网关注入的 `X-Auth-User`；缺失或为空返回 `401 JSON`，不会回退到 cookie。仅当 davbox 只监听回环、且该请求头由网关注入并对外剥离时才可使用。

```bash
AUTH_MODE=builtin ./davbox -addr 127.0.0.1:18900 -data ./data   # 默认，可省略
AUTH_MODE=sso     ./davbox -addr 127.0.0.1:18900 -data ./data
```

两种模式都不受影响的部分：

- WebDAV 数据面 `/<账号名>/...`：始终用应用账号 + HTTP Basic
- 客户端接口 `/api/client/*` 与静态资源 `/assets/*`
- 应用账号的增删改查在两种模式下都需要管理端身份

## 跨域直连（浏览器端使用）

默认关闭。浏览器用 `fetch` 直连 WebDAV 时，`PROPFIND` / `PUT` 等方法会先发 CORS 预检；不开 CORS 时预检收到 `401`，浏览器只会报 `TypeError: Failed to fetch`。

开启后只有白名单里的来源能跨域访问，服务仍用 Basic 认证，绝不回 `*`：

```bash
CORS_ORIGINS=https://app.example.com,https://notes.example.com ./davbox -addr 127.0.0.1:18900 -data ./data
```

- 逗号分隔，精确匹配 `scheme://host[:port]`；忽略来源末尾的 `/`，`scheme`/`host` 大小写不敏感，端口精确（`https://app.example.com` 与 `https://app.example.com:443` 视为不同来源）。
- 命中的预检直接 `204` 且不要求认证；随后的真实请求仍走账号 + Basic，权限与目录隔离完全不变。
- 只填自己信任的前端域名，不要填 `*`，也不要填别人的站点。跨域响应允许携带凭据（`Access-Control-Allow-Credentials: true`），来源写错就等于把 WebDAV 交给了该来源。

**替代方案**：若不想开 CORS（或上游 WebDAV 服务端根本不能开），让前端与 WebDAV 同域，用 nginx 反向代理把 WebDAV 挂到同域子路径，前端请求同源路径即可，完全不需要 CORS：

```nginx
server {
    listen 443 ssl;
    server_name app.example.com;

    # 前端静态资源 / 单页应用
    location / {
        root /var/www/app;
        try_files $uri /index.html;
    }

    # 同域子路径直通 davbox 的 WebDAV 数据面
    location /dav/ {
        proxy_pass http://127.0.0.1:18900/;
        proxy_request_buffering off;
        client_max_body_size 0;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

前端把 WebDAV 地址写成同域的 `/dav/<应用名>` 即可（例：`/dav/myapp`）。第三方 WebDAV 服务器常常不能开 CORS，这时同域反代是唯一可行解。

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

接 `sso` 时把 `/admin`、`/api/admin/` 交给你的认证入口，WebDAV 数据面与 `/assets/*` 直连 davbox ——
协议端点保持自带 Basic 认证，否则 App 无法同步。

## 已完成

- 账号隔离与 WebDAV 六动词：`GET` / `PUT` / `DELETE` / `MKCOL` / `MOVE` / `PROPFIND`（`Depth 0/1` 均返回合法 `207`）
- admin 页与 client 页
- 单二进制 + nginx 反代部署

## License

[MIT](LICENSE)
