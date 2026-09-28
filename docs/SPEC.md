# davbox · 实现规格（交付给 opencode 执行）

> 本文件是**权威施工图**。与 `docs/REQUIREMENTS.md` 冲突时以本文件为准。
> 目标：一次交付「阶段一（WebDAV 底盘 + 账号隔离）」+「阶段二（admin 页 + client 页）」。

---

## 0. 硬性边界（违反任何一条 = 任务失败，直接回滚）

- **绝不** `systemctl` / `service` 任何操作
- **绝不** `pkill` / `killall` / 杀任何**已存在**的进程（自己起的测试进程按 PID 清理允许）
- **绝不** 触碰已在运行的端口：80 / 443 / 3100 / 3200 / 4000 / 5241-5243 / 5300 / 6806 / 7897 / 8000 / 3010 / 17890
- **绝不** 修改 `仓库之外的系统配置`、任何 nginx 配置、任何容器、任何 cron
- **绝不** 改动 `` 下**其他**项目
- **只在 `./` 内新建/修改文件**
- 运行端口：只准 `127.0.0.1:18900`
- 运行数据目录：只准 `./data/`（可创建）
- **Go 依赖只准加 `golang.org/x/net`**。不许加 sqlite/gin/echo/任何 web 框架、不许加 ORM、不许加日志库
- **npm 依赖只准新增**：`react`、`react-dom`、`vite`、`@vitejs/plugin-react`、`typescript`、`@types/react`、`@types/react-dom`。不许加 UI 组件库、不许加状态管理库、不许加路由库、不许加 CSS 框架、不许加图标库（图标用内联 SVG 手写）

- 提交前自查：源码里**不得出现**真实域名、私有 IP、私有路径、任何口令与密钥

## 1. 交付物

```
./
├── go.mod                      module github.com/YLing2024/davbox
├── Makefile                    build / run / test / clean
├── cmd/davbox/main.go          入口：参数解析、装配、启动
├── internal/account/           账号模型、加载/保存 accounts.json、认证
├── internal/auth/              Basic 认证 + 会话 cookie 签名
├── internal/server/            路由装配：WebDAV 挂载 + admin API + client API + 静态资源
├── internal/usage/             目录用量统计
├── web/                        Vite + React + TS 前端（admin 页 + client 页）
├── web/dist/                   构建产物（被 go:embed 嵌入）
├── accounts.example.json       示例配置（不含真实口令）
├── docs/                       已有文档，保持
└── davbox                      go build 产物（已在 .gitignore）
```

单二进制：`make build` 先构建前端到 `web/dist`，再用 `//go:embed all:web/dist` 打进 `davbox`。

## 2. 数据与配置

运行目录 `./data/`：

| 文件 | 作用 | 权限 |
|---|---|---|
| `accounts.json` | 账号表（明文口令，见 §3 说明） | 0600 |
| `admin.json` | 管理员口令的 **bcrypt 哈希** | 0600 |
| `admin-password.txt` | 首次启动生成的管理员明文口令（仅首次写入，供用户自行保存） | 0600 |
| `secret.key` | 会话 cookie 签名密钥（首次启动生成 32 字节随机） | 0600 |
| `data/<user>/` | 各账号默认根目录 | 0755 |

`accounts.json` 结构：

```json
[
  { "user": "siyuan", "pass": "...", "root": "./data/data/siyuan", "readonly": false, "disabled": false, "note": "思源笔记同步" }
]
```

- `root` 为绝对路径；缺省 = `./data/data/<user>`
- 写入用「临时文件 + `os.Rename`」原子替换，并保持 0600
- 首次启动若 `accounts.json` 不存在 → 创建空数组 `[]`
- 首次启动若 `admin.json` 不存在 → 生成 24 位随机口令 → 写 bcrypt 哈希 + 写 `admin-password.txt` + **在 stdout 打印一次**

**§3 为什么账号口令是明文**：admin 页需要提供「复制连接信息」（把地址+用户名+口令一次给用户）→ 必须能取回原文。代价与对策：文件 0600、只监听回环、admin 页有独立登录。管理员口令反过来用 bcrypt（不需要回显）→ 不存明文。

## 3. WebDAV 接口（阶段一）

- 挂载点：`/<user>/…`，例：`https://host/siyuan/notes/a.txt` → 磁盘 `<root>/notes/a.txt`
- 认证：HTTP Basic，口令用 `crypto/subtle.ConstantTimeCompare`
  - 未认证/失败 → `401` + **必须**响应头 `WWW-Authenticate: Basic realm="davbox"`
  - 账号 `disabled: true` → 同 401（不要泄漏账号是否存在）
- 请求经过 `golang.org/x/net/webdav` 的 `Handler`：
  - `Prefix` 设为 `/<user>/`（前缀剥离由它负责）
  - `FileSystem` = 该账号 root 的 `webdav.Dir`
  - `LockSystem` = `webdav.NewMemLS()`
  - `Logger` 只记错误，**绝不**记录 `Authorization` 头或口令
- `readonly: true` 的账号：`PUT` / `DELETE` / `MKCOL` / `MOVE` / `COPY` / `PROPPATCH` → `403`；`GET` / `HEAD` / `PROPFIND` / `OPTIONS` 正常
- 六动词必须真能用：`GET` `PUT` `DELETE` `MKCOL` `MOVE` `PROPFIND`（`Depth: 0` 与 `Depth: 1` 都要正确返回 `207`）
- 路径安全：除库自身防护外，处理器入口再显式校验一次请求路径，拒绝含 `..`、`%2e`（大小写）、`\`、空字节的路径 → `400`

## 4. admin API（阶段二）

前缀 `/api/admin`，除 `login` 外都要会话 cookie。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/admin/login` | 体 `{"password":"..."}` → 成功设 cookie，失败 401（比较用 bcrypt） |
| POST | `/api/admin/logout` | 清 cookie |
| GET | `/api/admin/accounts` | 列表：`user, root, readonly, disabled, note, usedBytes, usedFiles` |
| POST | `/api/admin/accounts` | 体 `{"user":"...","readonly":false,"note":"..."}` → 建账号（自动生成 20 位口令、建目录）→ 返回 `{user, pass, url}` |
| GET | `/api/admin/accounts/{user}/conn` | 返回 `{url, user, pass}`（供「复制连接信息」） |
| POST | `/api/admin/accounts/{user}/rotate` | 换口令 → 返回新口令 |
| PATCH | `/api/admin/accounts/{user}` | 体 `{"readonly":true,"disabled":false,"note":"..."}` 局部更新 |
| DELETE | `/api/admin/accounts/{user}` | 停用并删除条目；**磁盘目录保留**（响应里说明） |

- 账号名校验：`^[a-z0-9][a-z0-9_-]{0,31}$`；重名 → 409
- 用量统计：按需 `filepath.WalkDir` 统计，结果缓存 30 秒
- 所有写操作返回后立刻落盘（原子替换）

## 5. client API（阶段二）

前缀 `/api/client`，除 `login` 外都要会话 cookie。会话里绑定账号名，只能操作该账号 root 内。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/client/login` | 体 `{"user":"...","pass":"..."}` → 设 cookie |
| POST | `/api/client/logout` | 清 cookie |
| GET | `/api/client/list?path=/a/b` | 目录列表：`[{name,isDir,size,mtime}]`，按「目录优先 + 名称」排序 |
| GET | `/api/client/raw/{path}` | 下载（支持 `Range`）|
| PUT | `/api/client/raw/{path}` | 上传（流式；父目录不存在 → 400） |
| POST | `/api/client/mkdir` | 体 `{"path":"/a/b"}` |
| POST | `/api/client/rename` | 体 `{"from":"/a","to":"/b"}` |
| DELETE | `/api/client/entry?path=/a` | 删除文件/空目录（非空目录 → 409） |

- 路径一律以 `/` 开头、相对账号 root 解析；任何逃逸 → 400
- 只读账号：所有写接口 → 403

## 6. 前端（阶段二）

`web/`：Vite + React + TS，两个入口页面由后端按路径给同一个 SPA，路由用 `window.location.pathname` 手写（**不加路由库**）。

**页面 1 `/admin`**
- 未登录 → 输入管理员口令 → 进入
- 列表：应用名 / 目录 / 已用空间（人类可读）/ 只读开关 / 停用开关 / 最近可有可无
- 每行按钮：`复制连接信息`（地址+用户名+口令，一次复制）、`换口令`、`停用`、`删除`
- 顶部：`+ 新建应用`（输入应用名 + 是否只读 → 建完弹出「连接信息」，含复制按钮 + 一段说明：把地址填进 App 的 WebDAV 设置）
- 连接信息里的地址格式：`<当前站点 origin>/<user>`（**不要**硬编码任何域名，用 `window.location.origin`）

**页面 2 `/`（client）**
- 未登录 → 输入应用账号 + 口令 → 进入
- 文件列表：面包屑导航 / 名称 / 大小 / 修改时间
- 操作：上传（点击选择 + 拖拽）、下载、新建文件夹、重命名、删除（删除要二次确认）
- 窄屏（≤640px）**不许隐藏导航**：面包屑与操作按钮保持可见，列表改为紧凑行

**视觉与文案**（硬要求）
- Swiss 极简：大量留白、细边框、单一强调色、系统字体栈；**不引任何 UI 库**
- 浅色为主，`prefers-color-scheme: dark` 时给一套深色变量
- 文案：中文、克制、**禁 emoji / 禁鸡汤 / 禁"AI 腔"**、不写技术说明文案（如"这是基于 WebDAV 的…"一律不要）
- 图标用内联 SVG（线性、1.5px 描边）

## 7. 质量要求

- `go vet ./...` 与 `go test ./...` 必须通过
- 单元测试至少覆盖：口令校验（对/错）、只读拒绝、账号禁用、路径逃逸拒绝、accounts.json 读写
- 前端 `npm run build` 必须零错误、零 TS 报错
- 代码里**不得**有 `TODO`、占位实现、示例硬编码口令
- 所有错误返回统一 JSON：`{"error":"..."}`（WebDAV 路径除外，它必须返回标准状态码）

## 8. 必须自测并贴出证据（不可只声明）

在 `./` 下启动 `./davbox -addr 127.0.0.1:18900 -data /tmp/davbox-dev`，用 `curl` 逐项跑，并把**原始输出**写进你的完成报告：

1. 六动词：`PROPFIND Depth:0` → 207、`PROPFIND Depth:1` → 207、`PUT` → 201、`GET`（内容一致）→ 200、`MOVE` → 201、`DELETE` → 204
2. 未认证 `PROPFIND` → 401，且 `curl -D-` 输出里能看到 `WWW-Authenticate: Basic realm="davbox"`
3. 错口令 → 401
4. 跨账号：账号 A 读/写账号 B 的路径 → 401/403/404
5. `--path-as-is` 的 `/../`、`/%2e%2e/`、`/..%2f` → 拒绝，且磁盘上不产生越狱文件
6. 只读账号：`PUT` → 403、`GET` → 200
7. `ls` 证明确实落在各自 root 目录
8. admin API：建账号 → 列表出现 → 换口令 → 旧口令失效、新口令可用 → 删除后列表消失但目录仍在
9. client API：登录 → 上传 → 列表 → 下载内容一致 → 重命名 → 删除
10. 空闲内存：`ps -o rss=` 或 `docker stats` 等实测值（目标 < 30 MB）
11. 收尾：杀掉自己起的进程，确认 18900 已释放，不留任何后台进程

## 9. 完成报告要求

- 逐条对应 §8 贴原始命令输出（截取关键行即可）
- 列出实际新增的 Go / npm 依赖（必须与 §0 允许清单一致）
- 说明任何你**没做到**的点（诚实优先，不许含糊）
- 不要执行 `git commit` / `git push`（由验收方统一处理）
