# Pixiv Shelf（Go + HTMX 4 + ES Module）

本地下载与浏览 Pixiv 作品的小工具：Go 提供 HTTP / WebSocket 服务，前端用 **HTMX 4**（含官方 `hx-ws` 扩展）做列表与下载进度，页面逻辑为 **ES Module**（`web/js/`）。元数据用 [bbolt](https://github.com/etcd-io/bbolt) 索引；原图保存在 `DOWNLOAD_DIR`。

- 仓库：[github.com/Xephyrus/download-widget](https://github.com/Xephyrus/download-widget)
- 许可证：[MIT](./LICENSE)

## 功能概览

- **图库列表**：搜索 / 标签 / 排序 / 分页；筛选条件写入地址栏，刷新后保持；列表中标签仅展示并可点选筛选；删除文件夹并同步索引
- **作品标签**：作品页芯片编辑；回车添加、点 × 移除（可短时撤销）；变更自动保存到数据库；支持多标签
- **批量下载**：填写原图 URL / Cookie / 任务名；WebSocket 实时日志与进度条；可中断；支持「重新下载」（清空同任务目录后重下）；无固定下载数量上限
- **作品画廊**：缩略图 + Fancybox（Hash 深链）；可从作品页跳转重新下载
- **磁盘同步**：启动时可扫描 `DOWNLOAD_DIR` 重建索引（保留自定义名称与标签）；空目录 / 错误路径不会误清空已有索引

## 环境

- Go 1.27+（见 `go.mod`）
- 访问 Pixiv 时默认走本地代理 `127.0.0.1:7897`（可用环境变量或下载页端口选择修改）

## 快速开始

**Windows（PowerShell）**

```powershell
cd <repo>
./build.ps1
./pixiv-downloader.exe

# 常用环境（按需）
$env:DOWNLOAD_DIR = "./downloads"
$env:DB_PATH = "./data/pixiv.db"
$env:SYNC_ON_START = "1"
```

**Linux / macOS / Git Bash**

```sh
cd <repo>
chmod +x ./build.sh
./build.sh
./pixiv-downloader

# 常用环境（按需）
export DOWNLOAD_DIR="./downloads"
export DB_PATH="./data/pixiv.db"
export SYNC_ON_START=1
```

也可用 `go run .` 直接启动（不经 release 瘦身参数）。

浏览器打开：

- 本机：http://127.0.0.1:7892
- 局域网：启动日志会打印可达的私网地址（默认监听 `0.0.0.0`）

| 路径 | 说明 |
| --- | --- |
| `/` | 图库列表（筛选 / 排序 / 分页；查询参数可分享、可刷新保持） |
| `/download` | 批量下载（表单 + `hx-ws` 日志） |
| `/artworks/:id` | 作品画廊 + 标签编辑 |

常用列表 URL 示例：`/?search=关键词`、`/?tag=标签名`、`/?tag=标签名&page=2`。

## 持久化运行

推荐把环境变量写进仓库根目录的 `config.env`（从 `config.env.example` 复制），再交给脚本后台启动；登录 Windows 后自动拉起。

### 1. 准备配置

```powershell
cd <repo>
copy config.env.example config.env
# 编辑 config.env：至少确认 DOWNLOAD_DIR、DB_PATH、WEB_DIR、PORT
./build.ps1
```

`config.env` 已加入 `.gitignore`，不会进仓库。日志目录为 `logs/`（同样忽略）。

### 2. Windows：后台启动 / 停止

```powershell
./scripts/start.ps1    # 按 config.env 后台运行，输出到 logs/
./scripts/stop.ps1     # 结束 pixiv-downloader 进程
```

若手动删改了下载目录中的图片，而 `SYNC_ON_START=0`，可临时设 `SYNC_ON_START=1` 后重启一次以重建文件索引（标签等自定义元数据会保留）。

### 3. Windows：开机（登录）自启

用当前用户的**计划任务**注册（无需改写成 Windows Service）：

```powershell
./scripts/install-autostart.ps1 -StartNow   # 注册任务并立即启动
# 卸载自启（可选同时停进程）：
./scripts/uninstall-autostart.ps1 -Stop
```

- 任务名默认 `PixivShelf`，触发器为**当前用户登录**
- 失败后最多自动重试 3 次（间隔 1 分钟）
- 查看任务：`taskschd.msc` → 任务计划程序库 → `PixivShelf`

### 4. Linux：systemd

```sh
sudo cp scripts/pixiv-shelf.service /etc/systemd/system/
# 按实际安装路径修改 WorkingDirectory / EnvironmentFile / ExecStart
sudo systemctl daemon-reload
sudo systemctl enable --now pixiv-shelf
sudo systemctl status pixiv-shelf
```

## 获取 Pixiv Cookie

下载原图需要已登录账号的 Cookie（至少包含 `PHPSESSID`）。按浏览器开发者工具复制即可：

1. 用浏览器打开 [https://www.pixiv.net](https://www.pixiv.net) 并登录。
2. 按 `F12`（或右键 → 检查）打开开发者工具，切到 **Network（网络）**。
3. 刷新页面，在请求列表里点选任意发往 `www.pixiv.net` 的文档/XHR 请求。
4. 在右侧 **Headers（标头）** 里找到 **Request Headers → Cookie**，整段复制。
5. 粘贴到本应用「下载」页的 **Pixiv Cookie** 输入框（会写入本地 `localStorage`，仅保存在当前浏览器）。

也可在 **Application / 存储 → Cookies → https://www.pixiv.net** 中查看；本应用需要的是请求头里那种 `name=value; name2=value2` 拼接字符串，而不是单个 Cookie 行。

注意：

- Cookie 等同登录凭证，不要发给他人或提交到公开仓库。
- 退出登录、改密或过期后需重新复制；失效时下载会 403/失败。
- 本服务只通过表单字段或请求头 `X-Pixiv-Cookie` 接收 Cookie，不会把它写进 URL。

## 项目结构

```
main.go                 # 入口：同步、compact、监听
build.ps1 / build.sh    # Release 构建（Windows / Unix）
config.env.example      # 环境变量模板（复制为 config.env）
scripts/                # 后台启动 / 计划任务自启 / systemd 单元
internal/
  config/               # 环境变量
  db/                   # bbolt 索引、标签、SyncFromDisk
  downloader/           # Pixiv 批量下载
  safepath/             # 路径 / 文件名校验
  server/               # HTTP、HTMX partial、WS、缩略图、标签 API
web/
  *.html                # 页面
  app.css
  js/                   # ES Module（boot / theme / scrollbar / list / artworks / tags-editor / downloader）
  vendor/               # htmx、hx-ws、fancybox（官方原版，不改源码）
```

## 配置（环境变量）

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `HOST` | `0.0.0.0` | 监听地址；仅本机可设 `127.0.0.1` |
| `PORT` | `7892` | 端口 |
| `PROXY_HOST` | `127.0.0.1` | 下载代理主机 |
| `PROXY_PORT` | `7897` | 下载代理端口 |
| `DOWNLOAD_DIR` | `./downloads` | 图片目录（缩略图缓存在其下 `.thumbs/`） |
| `DB_PATH` | `./data/pixiv.db` | bbolt 库文件 |
| `WEB_DIR` | `./web` | 静态前端目录 |
| `DOWNLOAD_DELAY_MS` | `1000` | 下载间隔 |
| `DOWNLOAD_TIMEOUT_SEC` | `30` | 单请求超时 |
| `SYNC_ON_START` | `1`（示例配置常为 `0`） | 启动时同步磁盘→数据库，并压缩库文件；设 `0` 可跳过 |

说明：

- 创建时间 / 修改时间以磁盘为准（Windows 使用文件夹创建时间与修改时间）。
- 若 `DB_PATH` 仍是旧 SQLite 文件，会改名为 `*.sqlite.bak` 并强制同步重建。
- Cookie 仅通过请求头 `X-Pixiv-Cookie` 或表单字段传递，不支持 `?cookie=`。
- **务必设置正确的 `DOWNLOAD_DIR`**。同步会清理「磁盘上已不存在」的索引项；若扫描结果为空但库中仍有记录，会跳过清理，避免误清空索引。同步失败时不会执行 Compact。
- 下载页可选择代理端口 `7897` / `7890`，或「自动检测」；无需改 `PROXY_PORT` 重启服务。
- 标签最多 20 个，单标签最长 32 字符（去重、去首尾空白）。

## 前端

- 入口：`web/js/list.js`、`artworks.js`、`downloader.js`（`type="module"`）
- 公共：`boot.js` → 主题 + 自定义滚动条（移动端改用系统滚动条）
- 标签：`tags-editor.js`（作品页自动保存；仅 × 可移除，支持短时撤销）
- 交互：
  - 列表 / 删除 / 画廊：HTMX partial；列表筛选用 `history.replaceState` 同步到 `/?…`
  - 下载页：HTMX 4 官方扩展 [`hx-ws`](https://four.htmx.org/extensions/hx-ws)（`hx-ws:connect="/ws"`）
  - 后台标签页不断开：通过官方配置 `ws.pauseOnBackground:false`（`<meta name="htmx-config">` / `hx-config`），不修改 `vendor/hx-ws.min.js`
  - 作品页：Fancybox（Hash 深链）+ 标签编辑器
- 缓存：`/static/vendor/` 长缓存；应用 `js` / `css` 为 `no-cache`，入口脚本另带 `?v=` 版本号

## 主要路由

**页面 / Partial**

- `GET /partials/list` — 列表行、统计、分页
- `DELETE /partials/folders/:id` — 删除后刷新列表
- `GET /partials/folders/:id/tags` — 作品标签面板 HTML
- `POST /partials/folders/:id/tags` — 保存标签（表单；也可返回列表 partial）
- `POST /partials/download` — 提交下载
- `POST /partials/download/cancel` — 中断下载
- `GET /partials/artworks/:id` — 画廊 HTML
- `GET /ws`（及 `/` 上的 WebSocket 升级）— 进度与状态推送

**其它**

- `GET /thumb/:id/:file?w=` — 缩略图
- `GET /download/:id/:file` — 原图文件
- `GET /static/...` — 静态资源

**JSON**

- `GET /api/list`
- `GET /api/tags` — 全部标签（去重）
- `GET|POST|PUT /api/folders/:id/tags` — 读写作品标签（JSON 或表单）
- `POST /api/download`
- `GET /api/artworks/:id`
- `GET /api/artwork-id/:id`（需 Cookie 头；下载页自动填充 URL / 页数）
- `DELETE /api/delete/:id`

## 下载页要点

- 表单 `POST /partials/download` 启动任务；进度条与日志经 WebSocket 推送 HTML / JSON `content`
- 任务状态带 `data-download-task` / `data-download-state`，完成时更新徽章与完成提示
- 从作品页带 `?taskName=&redownload=1` 进入时可自动填充并提交（需已保存 Cookie）
- 「重新下载」只取消并清空**同一任务目录**，不会误伤其它任务

## 作品标签要点

- 作品页 `/artworks/:id`：回车或逗号添加，点 × 移除；保存走 `POST /api/folders/:id/tags`
- 列表页标签为只读芯片，点击跳转 `/?tag=…` 筛选
- 磁盘同步会保留已有标签；删图后需同步才会更新文件数 / 画廊文件列表

## 许可证

本项目采用 [MIT License](./LICENSE) 发布。Copyright (c) 2026 Xephyrus。
