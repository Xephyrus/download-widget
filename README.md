# Pixiv Shelf（Go + HTMX 4 + ES Module）

本地下载与浏览 Pixiv 作品的小工具：Go 提供 HTTP / WebSocket 服务，前端用 **HTMX 4**（含官方 `hx-ws` 扩展）做列表与下载进度，页面逻辑为 **ES Module**（`web/js/`）。元数据用 [bbolt](https://github.com/etcd-io/bbolt) 索引；原图保存在 `DOWNLOAD_DIR`。

- 仓库：[github.com/Xephyrus/download-widget](https://github.com/Xephyrus/download-widget)
- 许可证：[MIT](./LICENSE)

## 功能概览

- **图库列表**：筛选、排序、分页；删除文件夹并同步索引
- **批量下载**：填写原图 URL / Cookie / 任务名；WebSocket 实时日志与进度条；可中断；支持「重新下载」（清空同任务目录后重下）
- **作品画廊**：缩略图 + Fancybox；支持从作品页跳转重新下载
- **磁盘同步**：启动时可扫描 `DOWNLOAD_DIR` 重建索引；空目录 / 错误路径不会误清空已有索引

## 环境

- Go 1.26+
- 访问 Pixiv 时默认走本地代理 `127.0.0.1:7897`（可用环境变量修改）

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
| `/` | 图库列表（筛选 / 排序 / 分页） |
| `/download` | 批量下载（表单 + `hx-ws` 日志） |
| `/artworks/:id` | 作品画廊（缩略图 + Fancybox） |

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
internal/
  config/               # 环境变量
  db/                   # bbolt 索引与 SyncFromDisk
  downloader/           # Pixiv 批量下载
  safepath/             # 路径 / 文件名校验
  server/               # HTTP、HTMX partial、WS、缩略图
web/
  *.html                # 页面
  app.css
  js/                   # ES Module（theme / scrollbar / 各页入口）
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
| `SYNC_ON_START` | `1` | 启动时同步磁盘→数据库，并压缩库文件；设 `0` 可跳过 |

说明：

- 创建时间 / 修改时间以磁盘为准（Windows 使用文件夹创建时间与修改时间）。
- 若 `DB_PATH` 仍是旧 SQLite 文件，会改名为 `*.sqlite.bak` 并强制同步重建。
- Cookie 仅通过请求头 `X-Pixiv-Cookie` 或表单字段传递，不支持 `?cookie=`。
- **务必设置正确的 `DOWNLOAD_DIR`**。同步会清理「磁盘上已不存在」的索引项；若扫描结果为空但库中仍有记录，会跳过清理，避免误清空索引。同步失败时不会执行 Compact。

## 前端

- 入口：`web/js/list.js`、`artworks.js`、`downloader.js`（`type="module"`）
- 公共：`boot.js` → 主题 + 自定义滚动条（移动端改用系统滚动条）
- 交互：
  - 列表 / 删除 / 画廊：HTMX partial
  - 下载页：HTMX 4 官方扩展 [`hx-ws`](https://four.htmx.org/extensions/hx-ws)（`hx-ws:connect="/ws"`）
  - 后台标签页不断开：通过官方配置 `ws.pauseOnBackground:false`（`<meta name="htmx-config">` / `hx-config`），不修改 `vendor/hx-ws.min.js`
  - 作品页：Fancybox（Hash 深链）

## 主要路由

**页面 / Partial**

- `GET /partials/list` — 列表行、统计、分页
- `DELETE /partials/folders/:id` — 删除后刷新列表
- `POST /partials/download` — 提交下载
- `POST /partials/download/cancel` — 中断下载
- `GET /partials/artworks/:id` — 画廊 HTML
- `GET /ws`（及 `/` 上的 WebSocket 升级）— 进度与状态推送

**其它**

- `GET /thumb/:id/:file?w=` — 缩略图
- `GET /download/:id/:file` — 原图文件
- `GET /static/...` — 静态资源（带缓存；前端资源用 `?v=` 版本号）

**JSON（调试 / 辅助）**

- `GET /api/list`
- `POST /api/download`
- `GET /api/artworks/:id`
- `GET /api/artwork-id/:id`（需 Cookie 头；下载页自动填充 URL / 页数）
- `DELETE /api/delete/:id`

## 下载页要点

- 表单 `POST /partials/download` 启动任务；进度条与日志经 WebSocket 推送 HTML / JSON `content`
- 任务状态带 `data-download-task` / `data-download-state`，完成时更新徽章与完成提示
- 从作品页带 `?taskName=&redownload=1` 进入时可自动填充并提交（需已保存 Cookie）
- 「重新下载」只取消并清空**同一任务目录**，不会误伤其它任务

## 许可证

本项目采用 [MIT License](./LICENSE) 发布。Copyright (c) 2026 Xephyrus。
