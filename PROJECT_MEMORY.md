# davbox · 项目记忆

## 一句话

极简 WebDAV 服务：admin 管账号、client 管文件、App 用来同步。**一个应用一个账号一个目录，互不可见。**

## 立项原因（2026-09-28）

用户需要一个像坚果云"创建应用"那样简单的 WebDAV，主要给 App 用。
调研过的市面方案与实测结论：

| 方案 | 实测结论 |
|---|---|
| SFTPGo（12,588★） | 功能对 ✓ 隔离实测通过 ✓ 但十几概念 + 管理端全英文 ✗ "功能复杂度"远超需求 |
| dufs（10,802★，3.02 MiB） | 隔离实测通过 ✓（越权 403/403、`../` 400）但只有启动参数、无 admin ✗ |
| hacdias/webdav（5,882★，7.09 MiB） | **两种配置写法隔离都没锁住** ✗（可见兄弟目录 / 暴露容器根）|
| rclone / copyparty / OpenList / Alist / Cloudreve | 均无"每账号独立根 + 管理页"这套 ✓ 或过重 ✗ |
| filebrowser（35,936★） | **根本没有 WebDAV** ✗ |
| 结论 | 缺口只有薄薄一层 → 自研产品层，协议用官方库 ✓ |

## 位置与环境

- 代码：`./`（git 仓库 ✓）
- 端口规划：开发/测试 `127.0.0.1:18900`
- 数据目录规划：`./data/`（账号 JSON + 各应用目录 + 审计 JSONL）
- 编译：Go 1.24.4（`Go 安装目录`）✓ 模块缓存 `~/go/pkg/mod`（x/net v0.59.0 已预热 ✓）
- 前端构建：Node v26 / Bun 1.4.2 ✓

## 技术选型

见 `docs/DECISIONS.md`（含全部被否方案与当时的实测证据）。
要点：Go + `golang.org/x/net/webdav`（官方，7543 行实现 / 4275 行测试）+ React/Vite/TS 内嵌 + JSON 账号 + 单静态二进制。

## 部署规划（未执行）

- systemd unit 常驻（干活留 systemd；检查归 Hermes 巡检 ✓）
- nginx 反代 + TLS：新子域**成对做**（主域块 + 镜像域块 + 逐名证书，`certbot certonly` 绝不改配置 ✓）
- 认证：自带账号体系 → **不叠 SSO** ✓（与 memos / SiYuan 同一约定）
- 与 SFTPGo 的关系：davbox 上线验证通过后再决定是否拆掉 SFTPGo（目前 SFTPGo 无真实业务在用）

## 协作约定

- 编码任务一律委托 opencode ✓（需求文档在 `docs/REQUIREMENTS.md`；Hermes 负责诊断/验收/部署）
- 委托命令必须：`set -a; . 外部环境变量; set +a` 且只能用 opencode 自己的 key ✓
- 需求文档必须写明"不许 systemctl / pkill / 动生产端口" ✓
- 不硬编码任何域名与 IP ✓（一律环境变量或配置注入）
- 文案：中文、唯美克制、禁 emoji / 鸡汤 / AI 腔 ✓

## 进度

- [x] 技术选型 + 命名 + 建项目（2026-09-28）
- [ ] 阶段一：底盘（账号隔离 + 六动词 WebDAV）→ 委托 opencode
- [ ] 阶段二：admin 页 + client 页
- [ ] 阶段三（可选）：配额 / 用量 / 分享
