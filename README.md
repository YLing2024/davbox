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

## 状态

阶段一：底盘（账号隔离 + 六动词 WebDAV）—— 已完成
阶段二：admin 页 + client 页 —— 已完成
阶段三（可选）：配额、用量趋势、分享链接
