# davbox · 需求文档

## 0. 定位

用户需要一个**概念只有两个**的自建 WebDAV 服务：

- **admin**：管理账号（新增 / 删除 / 改密码 / 看目录与用量）
- **client**：账号登录后管理自己的文件
- **主要消费者是 App**：思源笔记、Joplin 等通过 WebDAV 同步

设计分工（关键）：**WebDAV 协议不自研**（底盘用 Go 官方 `golang.org/x/net/webdav`），
**产品层 100% 自研**（账号模型、目录隔离、admin 页、client 页）。

## 1. 阶段一：底盘（本次委托范围）

**做**：一个 Go 二进制，读账号配置，按账号隔离地提供完整 WebDAV 服务。
**不做**（留给阶段二/三）：admin 页、client 页、配额、HTTPS、SSO、日志文件、数据库。

阶段一唯一目的：**证明底盘与隔离是可信的**。

### 硬性边界（违反任何一条即视为任务失败）

- **绝不** 执行 `systemctl` / `service` 任何操作
- **绝不** 执行 `pkill` / `killall` / `kill` 任何**已存在**的进程
- **绝不** 触碰任何已在运行的端口（80 / 443 / 3100 / 3200 / 4000 / 5241-5243 / 5300 / 6806 / 7897 / 8000 / 3010 等）
- **绝不** 修改任何现有服务、容器、nginx 配置、cron
- **绝不** 改动 `` 下**其他**项目的任何文件
- **不新增第三方依赖**：只允许 `golang.org/x/net/webdav`
- 只准修改 `./` 内的文件
- 服务只准监听 `127.0.0.1:18900`
- 测试进程自己起自己收，按 PID 收尾

### 交付物

```
./
├── cmd/davbox/main.go        # 入口
├── internal/account/         # 账号加载 + Basic 认证 + 路由
├── internal/fs/              # 按账号 chroot 的文件系统封装
├── go.mod                    # module davbox
├── accounts.json             # 示例：app1 / app2
└── davbox                    # go build 产物
```

### 功能要求

1. **配置**：`accounts.json` 为数组，每项
   `{ "user": "app1", "pass": "...", "root": "/abs/path", "readonly": false }`
2. **认证**：HTTP Basic，口令用 `crypto/subtle` 常量时间比较；
   未认证/失败 → `401`，**必须带 `WWW-Authenticate: Basic realm="davbox"`**（客户端靠它重试）
3. **URL 空间**：`/<user>/...` → 剥离前缀映射到该账号 `root` 下的相对路径
   例：账号 `app1`（root=`/srv/dav/app1`）收 `PUT /app1/notes/a.txt` → 落盘 `/srv/dav/app1/notes/a.txt`
4. **越权必须失败**：访问他人前缀、`../`、`%2e%2e`、双重编码、绝对路径逃逸 → 401/403/404 之一，绝不允许成功
5. **只读账号**：PUT / DELETE / MKCOL / MOVE / COPY / PROPPATCH → 403；GET / PROPFIND 正常
6. **六个动词可用**：GET / PUT / DELETE / MKCOL / MOVE / PROPFIND（PROPFIND 正确处理 `Depth: 0` 与 `Depth: 1` 并返回合法 `207 Multi-Status`）
7. **启动参数**：`-addr 127.0.0.1:18900 -c accounts.json`；日志到 stdout，不写文件
8. `root` 不存在时自动 `MkdirAll`
9. Basic 认证通过后的请求**不得**把口令写进日志

### 验收标准（Hermes 亲自实测，不看自述）

| # | 项目 | 通过条件 |
|---|---|---|
| 1 | 六动词 | GET / PUT / DELETE / MKCOL / MOVE / PROPFIND 全部返回正确状态码 |
| 2 | 跨账号读 | app1 读 app2 目录 → 401/403/404 |
| 3 | 跨账号写 | app1 写 app2 目录 → 401/403/404 |
| 4 | `../` 逃逸 | 错误码，且磁盘不产生越狱文件 |
| 5 | `%2e%2e` 逃逸 | 同上 |
| 6 | 落盘位置 | app1 写的内容只在 app1 的 root 里 |
| 7 | 未认证 | 401 且响应头含 `WWW-Authenticate` |
| 8 | 只读账号 | 写 403 / 读 200-207 |
| 9 | 空闲内存 | RSS < 30 MB |
| 10 | 收尾 | 只监听 127.0.0.1:18900；按 PID 可干净停掉 |
| 11 | 合规（加分项） | 若能跑通 litmus 基础集更好（`internal/litmus_test_server.go` 有现成入口） |

## 2. 阶段二：admin 页 + client 页（下一轮委托）

- **admin 页**（登录：独立管理员账号）
  - 列表：应用名 / 目录 / 已用空间 / 只读开关 / 最近活动
  - 操作：新增应用（自动生成强口令 + 建目录）、改密码、改只读、停用、删除（目录保留）
  - 每行一个「复制连接信息」按钮：地址 + 用户名 + 口令，一次复制走
- **client 页**（登录：应用账号）
  - 浏览 / 上传 / 下载 / 删除 / 新建文件夹 / 重命名；面包屑导航；移动端可用（不隐藏导航）
- 视觉：Swiss 极简，与个人站同风格；文案中文、克制，不放技术说明

## 3. 阶段三（可选，等你点名）

配额限制、用量趋势、分享链接（走 admin 现有 `/s/<token>` 那套口径）、审计日志页
