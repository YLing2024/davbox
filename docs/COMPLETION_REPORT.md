# davbox · 完成报告（阶段一 + 阶段二）

日期：2026-09-28 · 任务书：`docs/SPEC.md`

## 0. 第二轮修复（验收反馈的 4 项）

> 说明：验收方指定的自测端口 `127.0.0.1:18901` 被 **opencode 服务本体**占用

> 按红线不得杀已有进程，故第二轮自测改用 `127.0.0.1:18902` + 临时数据目录
> `/tmp/davbox-accept`。验收实例 `127.0.0.1:18900` 全程未触碰。

### 0.1 死属性 / PROPPATCH 持久化

**实现**：新增 `internal/server/deadprops.go`，用 `deadPropFS` 包装 `webdav.Dir`，
使其返回的 `File` 实现 `webdav.DeadPropsHolder`（`DeadProps` + `Patch`）。
sidecar 存于 **账号 root 之外**：`<数据目录>/davprops/<user>/<资源相对路径…>/__props__.json`。

**为什么放在 root 之外**：root 内任何隐藏目录仍处于 PROPFIND `Depth:1` 与 client
`/api/client/list` 可见的命名空间里，必须在每一处遍历（Readdir、client list、
用量统计）都过滤，漏一处就污染用户目录；放到 root 外则从结构上杜绝。
`PROPFIND Depth:1` 与 client 列表天然看不到它，用量统计也不会计入。

COPY/MOVE/DELETE 跟随：
- COPY 文件：`x/net/webdav` 内置 `copyProps(dst,src)` 会调用双方的 `Patch`；
- MOVE：`deadPropFS.Rename` 同步搬移 sidecar 子树；
- DELETE / 覆盖写：`deadPropFS.RemoveAll` 清理对应 sidecar 子树；
- `PROPPATCH remove` 清空属性时删除 sidecar 并剪掉空目录。

修复前（PROPPATCH 一律 403）：

```
$ curl -sS -u litmus:litmus-pass -X PROPPATCH --data-binary @pp.xml .../litmus/p.txt -D-
HTTP/1.1 207 Multi-Status
<D:propstat><D:prop><author xmlns="urn:test:"></author></D:prop>
<D:status>HTTP/1.1 403 Forbidden</D:status></D:propstat>
```

修复后：

```
$ curl -sS -u litmus:litmus-pass -X PROPPATCH --data-binary @pp.xml .../litmus/p.txt -D-
HTTP/1.1 207 Multi-Status
<D:propstat><D:prop><author xmlns="urn:test:"></author></D:prop>
<D:status>HTTP/1.1 200 OK</D:status></D:propstat>

$ curl -sS -u litmus:litmus-pass -X PROPFIND -H 'Depth: 0' --data-binary @pf.xml .../litmus/p.txt -D-
HTTP/1.1 207 Multi-Status
<D:prop><author xmlns="urn:test:">alice</author></D:prop><D:status>HTTP/1.1 200 OK</D:status>

# 客户端目录列表看不到 sidecar
$ curl -sS -b ck.txt '.../api/client/list?path=/'
[{"name":"p.txt","isDir":false,"size":2,"mtime":1790588327477}]

# 账号 root 内没有 davprops；sidecar 在数据目录
$ ls -a data-after/data/litmus
.  ..  p.txt
$ find data-after/davprops -type f
data-after/davprops/litmus/p.txt/__props__.json
```

### 0.2 只读账号 LOCK/UNLOCK → 403

`isWriteMethod` 增加 `LOCK`/`UNLOCK`。

修复前：

```
$ curl -sS -u ro:ro-pass -X LOCK ... -D-
HTTP/1.1 201 Created
Lock-Token: <1790588046>
```

修复后：

```
$ curl -sS -u ro:ro-pass -X LOCK ... -D-
HTTP/1.1 403 Forbidden
{"error":"只读账号"}

$ curl -sS -u ro:ro-pass -X UNLOCK -H 'Lock-Token: <...>' ... -D-
HTTP/1.1 403 Forbidden
{"error":"只读账号"}
```

### 0.3 不存在的账号 → 401（原来 404）

路由 default 分支：首段账号不在账号表时，与未认证一致返回
`401 + WWW-Authenticate: Basic realm="davbox"`，不再 404，避免泄漏账号是否存在。

修复前：

```
$ curl -sS -u ghost:whatever -X PROPFIND -H 'Depth: 0' .../ghost/ -D-
HTTP/1.1 404 Not Found
{"error":"未找到"}
```

修复后：

```
$ curl -sS -u ghost:whatever -X PROPFIND -H 'Depth: 0' .../ghost/ -D-
HTTP/1.1 401 Unauthorized
Www-Authenticate: Basic realm="davbox"
{"error":"未认证"}

# 不带凭据同样 401
HTTP/1.1 401 Unauthorized
Www-Authenticate: Basic realm="davbox"
```

### 0.4 client 刷新后显示当前账号名

新增 `GET /api/client/me`（会话内返回 `{user, readonly}`，不含口令）；
client 页每次加载列表后拉取 me，顶栏显示账号名，刷新不丢失。

修复前：

```
$ curl -sS -b ck.txt .../api/client/me -D-
HTTP/1.1 404 Not Found
{"error":"未找到"}
```

修复后：

```
$ curl -sS -b ck.txt .../api/client/me -D-
HTTP/1.1 200 OK
{"readonly":false,"user":"litmus"}
```

### 0.5 litmus 结果（`litmus -k http://127.0.0.1:18902/litmus/ litmus litmus-pass`）

修复前：

```
<- summary for `basic':    of 16 tests run: 16 passed, 0 failed. 100.0%
<- summary for `copymove': of 13 tests run: 13 passed, 0 failed. 100.0%
<- summary for `props':    of 14 tests run: 10 passed, 4 failed. 71.4%   (16 skipped)
<- summary for `locks':    of 34 tests run: 30 passed, 4 failed. 88.2%
<- summary for `http':     of 4 tests run: 4 passed, 0 failed. 100.0%
```

修复后：

```
<- summary for `basic':    of 16 tests run: 16 passed, 0 failed. 100.0%
<- summary for `copymove': of 13 tests run: 13 passed, 0 failed. 100.0%
<- summary for `props':    of 30 tests run: 29 passed, 1 failed. 96.7%
<- summary for `locks':    of 34 tests run: 32 passed, 2 failed. 94.1%
<- summary for `http':     of 4 tests run: 4 passed, 0 failed. 100.0%
```

因 PROPPATCH 失败的目标 5 项全部转绿：`props: propset / propmanyns / propget`
以及 `locks: owner_modify`（两处，文件与集合成员）。

### 0.6 仍未通过的 litmus 项（逐条定性）

| 项 | 现象 | 定性 |
|---|---|---|
| `props: propfind_invalid2` | 带非法命名空间声明的 PROPFIND 返回 207 而非 400 | **上游库限制**：`x/net/webdav` 的 XML 解码器宽松，不校验命名空间声明。非本轮范围。 |
| `locks: fail_complex_cond_put` | 复杂伪造 If 条件仍 201 | **上游库限制**：`x/net/webdav` 的 If 头解析宽松。非本轮范围。 |
| `locks: lock_shared` | 共享锁 LOCK 返回 501 | **RFC 允许**：RFC 4918 的共享锁为可选能力，未实现返回 501 合法。`x/net/webdav` 的 MemLS 只支持排他锁。后续共享锁相关 7 项被 litmus 自动跳过。 |
| `locks: cond_put_corrupt_token` | 412（litmus WARNING 期望 423） | **RFC 允许**：RFC 4918 §10.4.1 要求 If 求值全失败返回 412；`x/net/webdav` 源码注释明确选择 412。计入 pass with 1 warning。 |

### 0.7 质量门禁（第二轮）

```
$ go vet ./...        -> 通过
$ go test ./...       -> ok internal/account, ok internal/server
$ gofmt -l ./internal ./cmd -> 无输出
$ cd web && npm run build  -> tsc --noEmit 零错误 + vite build 成功
```

新增单元测试：`deadprops_test.go`（死属性读写/COPY/MOVE/DELETE 清理/目录不可见）、
`lock_test.go`（只读 LOCK/UNLOCK 403）、`account_test.go`（未知账号 401）、
`client_test.go`（`/api/client/me` 返回账号名且不含口令）。

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
3. **litmus 已在第二轮运行**（结果见 §0.5）：basic 16/16、copymove 13/13、props 29/30、locks 32/34、http 4/4；剩余 3 项为上游库限制或 RFC 允许，见 §0.6。
4. **只读账号的 `LOCK/UNLOCK` 已在第二轮阻断为 403**（见 §0.2），SPEC §3 已同步。
5. **client 页刷新后显示当前账号名已在第二轮完成**：新增 `GET /api/client/me`（见 §0.4），SPEC §5 已同步。
6. **新建/重命名/删除确认使用浏览器原生 `prompt`/`confirm`**，未自绘对话框。
7. **移动端仅做响应式 CSS**（面包屑与操作按钮不隐藏），未做真机/多浏览器截图验证。
8. **已知偏差（第二轮明确）**：`propfind_invalid2` 属上游 `x/net/webdav` 的 XML 宽松；`fail_complex_cond_put` 属上游 If 头宽松；共享锁（`lock_shared`）未实现、返回 501 为 RFC 4918 允许；`cond_put_corrupt_token` 返回 412 亦符 RFC 4918 §10.4.1。详见 §0.6。
9. 第一轮自测数据留在 `/tmp/davbox-dev`（未删除）；第二轮自测数据在 `/tmp/davbox-accept`，测试进程按 PID 停止，18902 已释放。

## 5. 合规自查

- 未执行 `systemctl` / `service` / `pkill` / `killall`；测试进程按 PID 停止。
- 验收实例 `127.0.0.1:18900` 全程未触碰；第二轮只监听 `127.0.0.1:18902`（18901 被 opencode 本体占用，见 §0 说明）；未触碰任何生产端口。
- 未改动 `仓库之外的系统配置`（除任务指定的临时目录 `/tmp/davbox-accept`）、nginx、容器、cron、其他项目。
- 源码中无私有域名、公网 IP、真实口令；前端地址取自 `window.location.origin`。
- 未执行 `git push`。
