# davbox · 完成报告（阶段一 + 阶段二）

日期：2026-09-28 · 任务书：`docs/SPEC.md`

## 1. 交付概览

单个 Go 静态二进制，内嵌 React + Vite + TS 前端，实现：

- **WebDAV 底盘**：`/<账号>/…`，每账号独立 root（chroot），HTTP Basic，六动词可用，PROPFIND Depth 0/1 返回 207，路径逃逸拒绝，只读账号写操作 403。
- **admin 页 / admin API**：管理员独立口令（bcrypt）登录；账号列表（目录 + 用量）；新建（自动口令 + 建目录）；复制连接信息；换口令；只读/停用开关；删除（保留目录）。
- **client 页 / client API**：应用账号登录；浏览 / 上传 / 下载 / 新建文件夹 / 重命名 / 删除；面包屑；窄屏不隐藏导航。
- **视觉**：Swiss 极简、中文文案、无 emoji、无技术说明文案、图标全部内联 SVG。

产物清单：

```
go.mod  go.sum  Makefile
cmd/davbox/main.go
internal/account/  internal/auth/  internal/randstr/  internal/server/  internal/usage/
web/（Vite+React+TS 源码，web/dist 构建产物被 //go:embed all:web/dist 嵌入）
accounts.example.json
```

## 2. 实际新增依赖

### Go（`go.mod`）

| 依赖 | 版本 | 说明 |
|---|---|---|
| `golang.org/x/net` | v0.59.0 | 官方 WebDAV 底盘（SPEC 允许） |
| `golang.org/x/crypto` | v0.57.0 | **仅**用于 `bcrypt`（SPEC §2/§4 明确要求管理员口令用 bcrypt） |

**偏差说明（诚实）**：SPEC §0 写「Go 依赖只准加 `golang.org/x/net`」，但 SPEC §2 与 §4 又明确要求管理员口令用 **bcrypt 哈希**；Go 标准库没有 bcrypt，唯一实现是 `golang.org/x/crypto/bcrypt`。另外 `golang.org/x/net@v0.59.0` 自身的 `go.mod` 已经把 `golang.org/x/crypto` 列为 require。因此这里引入了 `x/crypto`，除此之外没有引入任何 web 框架 / ORM / sqlite / 日志库。

**go 指令**：`go.mod` 为 `go 1.26.0`。原因：`x/net@v0.59.0` 与 `x/crypto@v0.57.0` 的 `go.mod` 都声明 `go >= 1.26.0`，Go 工具链自动切换到 go1.26.0 并把主模块 go 指令提升到 1.26.0（环境自带 Go 1.24.4，`GOTOOLCHAIN=auto`）。

### npm（`web/package.json`，全部在白名单内）

- dependencies：`react` `react-dom`
- devDependencies：`vite` `@vitejs/plugin-react` `typescript` `@types/react` `@types/react-dom`

未引入任何 UI 组件库 / 路由库 / 状态管理库 / CSS 框架 / 图标库。

## 3. §8 自测原始证据

启动命令：`./davbox -addr 127.0.0.1:18900 -data /tmp/davbox-dev`

### 3.1 管理页登录 / admin API 全流程

```
===== 1. admin 登录（错误口令） =====
$ curl -sS -D- -o /tmp/b -X POST /api/admin/login -d {"password":"wrong"}
HTTP/1.1 401 Unauthorized
Content-Type: application/json; charset=utf-8
{"error":"口令错误"}

===== 2. admin 登录（正确口令） =====
HTTP/1.1 200 OK
Set-Cookie: davbox_admin=<会话 cookie>; Path=/; Max-Age=43200; HttpOnly; SameSite=Lax
{"ok":true}

===== 3-5. 新建账号 app1 / ro(只读) / app2 =====
{"pass":"<自动生成>","url":"http://127.0.0.1:18900/app1","user":"app1"}
{"pass":"<自动生成>","url":"http://127.0.0.1:18900/ro","user":"ro"}
{"pass":"<自动生成>","url":"http://127.0.0.1:18900/app2","user":"app2"}

===== 6. admin 列表（含用量） =====
[{"user":"app1","root":"/tmp/davbox-dev/data/app1","readonly":false,"disabled":false,"note":"同步测试","usedBytes":0,"usedFiles":0},{"user":"app2",...},{"user":"ro","root":"/tmp/davbox-dev/data/ro","readonly":true,...}]

===== 22. 换口令（app2） =====
{"pass":"<自动生成>",...}
===== 23. 旧口令失效 / 新口令可用 =====
app2:<旧> -> HTTP 401
app2:<新> -> HTTP 207

===== 24. 复制连接信息（app1） =====
{"pass":"<自动生成>","url":"http://127.0.0.1:18900/app1","user":"app1"}

===== 25. 删除 app2 -> 列表消失、目录保留 =====
{"message":"账号已删除，磁盘目录保留","ok":true}
[{"user":"app1",...},{"user":"ro",...}]
drwxr-xr-x 2 root root 4096 ... /tmp/davbox-dev/data/app2

===== 边界：重名 -> 409；非法名 -> 400；未登录 -> 401 =====
{"error":"账号已存在"} / {"error":"账号名只能用小写字母、数字、下划线或连字符，且不超过 32 位"} / {"error":"未登录"}
```

### 3.2 未认证 / 错口令（401 且必须带 WWW-Authenticate）

```
===== 7. 未认证 PROPFIND =====
HTTP/1.1 401 Unauthorized
Www-Authenticate: Basic realm="davbox"
{"error":"未认证"}

===== 8. 错误口令 PROPFIND =====
HTTP/1.1 401 Unauthorized
Www-Authenticate: Basic realm="davbox"
{"error":"未认证"}
```

### 3.3 六动词

```
MKCOL    /app1/notes        -> HTTP 201
PROPFIND /app1/  Depth:0    -> HTTP 207
PROPFIND /app1/notes/ Depth:1 -> HTTP 207
PUT      /app1/notes/a.txt  -> HTTP 201
GET      /app1/notes/a.txt  -> HTTP 200   body: hello-davbox
MOVE     /app1/notes/a.txt -> /app1/notes/b.txt  -> HTTP 201
DELETE   /app1/notes/b.txt  -> HTTP 204
```

PROPFIND Depth:0 响应体（截断）证明是合法 207 Multi-Status：

```
<?xml version="1.0" encoding="UTF-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>/app1/</D:href>
<D:propstat><D:prop><D:resourcetype><D:collection xmlns:D="DAV:"/></D:resourcetype>...
```

### 3.4 落盘位置（各账号独立 root）

```
$ ls -R /tmp/davbox-dev/data/app1
/tmp/davbox-dev/data/app1:
notes
/tmp/davbox-dev/data/app1/notes:
b.txt
```

### 3.5 跨账号越权

```
$ curl -u app1:*** -X PROPFIND -H 'Depth: 0' /ro/
HTTP/1.1 401 Unauthorized
```

### 3.6 路径逃逸（`--path-as-is`，磁盘不产生越狱文件）

```
/app1/../etc      -> HTTP 400  {"error":"请求路径不合法"}
/app1/%2e%2e/etc  -> HTTP 400  {"error":"请求路径不合法"}
/app1/..%2fetc    -> HTTP 400  {"error":"请求路径不合法"}

$ ls -la /tmp/davbox-dev/data/app1     # 只有 notes，无越狱文件
$ ls /tmp/davbox-dev                   # accounts.json admin.json admin-password.txt secret.key data
```

### 3.7 只读账号

```
PUT /ro/x.txt        -> HTTP 403  {"error":"只读账号"}
GET /ro/readme.txt   -> HTTP 200  body: readonly-ok
client 上传 (ro)      -> HTTP 403  {"error":"只读账号"}
$ ls /tmp/davbox-dev/data/ro           # 只有 readme.txt
```

### 3.8 停用账号

```
PATCH 停用 -> HTTP 200
PROPFIND app1 -> HTTP 401
PATCH 恢复 -> HTTP 200
```

### 3.9 client API 全流程

```
登录(错口令)          -> HTTP 401
登录(正确)            -> HTTP 200  Set-Cookie: davbox_client=...; HttpOnly; SameSite=Lax
PUT  /api/client/raw/hello.txt  -> HTTP 201
GET  /api/client/list?path=/    -> [{"name":"notes","isDir":true,...},{"name":"hello.txt","isDir":false,"size":11,...}]
GET  /api/client/raw/hello.txt  -> HTTP 200  body: client-body
POST /api/client/mkdir  {"path":"/sub"}       -> HTTP 201
POST /api/client/rename {"from":"/hello.txt","to":"/renamed.txt"} -> HTTP 200
DELETE /api/client/entry?path=/renamed.txt    -> HTTP 200
GET  /api/client/list?path=/../               -> HTTP 400  （逃逸）
```

### 3.10 静态页面

```
GET /       -> HTTP 200 text/html
GET /admin  -> HTTP 200 text/html
GET /assets/index-Bpeh7WbW.js -> HTTP 200 text/javascript bytes=236073
GET /nope/x -> HTTP 404 {"error":"未找到"}
```

### 3.11 空闲内存

```
$ ps -o pid=,rss=,vsz= -p 1935474
1935474 14440 1636680
RSS 14440 KB = 14.10 MB        （目标 < 30 MB）
```

### 3.12 收尾

```
$ ss -ltnp | grep 18900
LISTEN ... 127.0.0.1:18900 users:(("davbox",pid=1935474,fd=3))

$ kill 1935474
$ ss -ltnp | grep 18900     # 无输出，端口已释放
process gone                 # 无残留 davbox 进程
```

### 3.13 质量门禁

```
go vet ./...  -> 通过
go test ./... -> ok github.com/YLing2024/davbox/internal/account
                 ok github.com/YLing2024/davbox/internal/server
npm run build -> tsc --noEmit 零错误 + vite build 成功
```

单元测试覆盖：口令校验（对/错）、账号禁用、只读拒绝、路径逃逸拒绝、accounts.json 读写与 0600 权限、admin/client API 全流程。

## 4. 未做到 / 偏差（诚实清单）

1. **新增了 `golang.org/x/crypto`**，超出「只准 x/net」的字面白名单；原因是 SPEC 明确要求 bcrypt，而标准库无 bcrypt。
2. **`go.mod` 的 go 指令是 1.26.0 而非 1.24**：依赖的 `go.mod` 要求 ≥1.26，工具链自动切换所致。
3. **未运行 litmus 合规套件**：SPEC §8 未列入必测项（REQUIREMENTS 列为加分项）。
4. **只读账号的 `LOCK/UNLOCK` 未阻断**：SPEC 只列出 PUT/DELETE/MKCOL/MOVE/COPY/PROPPATCH 为写操作，故按此实现；只读账号即使加锁，写操作仍 403。
5. **client 页刷新后不显示当前账号名**：登录信息在 HttpOnly cookie 内，前端读不到，也未额外加接口（避免超出 SPEC 接口表）。
6. **新建/重命名/删除确认使用浏览器原生 `prompt`/`confirm`**，未自绘对话框。
7. **移动端仅做响应式 CSS**（面包屑与操作按钮不隐藏），未做真机/多浏览器截图验证。
8. 自测数据留在 `/tmp/davbox-dev`（未删除），测试进程已停止，18900 已释放。

## 5. 合规自查

- 未执行 `systemctl` / `service` / `pkill` / `killall`；测试进程按 PID 停止。
- 只监听 `127.0.0.1:18900`；未触碰任何生产端口。
- 未改动 `仓库之外的系统配置`、nginx、容器、cron、其他项目。
- 源码中无私有域名、公网 IP、真实口令；前端地址取自 `window.location.origin`。
- 未执行 `git push`。
