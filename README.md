# Pixiv Shelf（Go + HTMX + ES Module）

本地下载与浏览 Pixiv 作品的小工具：Go 提供 HTTP/WebSocket 服务，前端用 **HTMX** 做列表与进度交互，页面脚本为 **ES Module**（`web/js/`）。元数据用 [bbolt](https://github.com/etcd-io/bbolt) 索引；图片文件本身在 `DOWNLOAD_DIR`。

- 仓库：[github.com/Xephyrus/download-widget](https://github.com/Xephyrus/download-widget)
- 许可证：[MIT](./LICENSE)

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

| 路径              | 说明                      |
| --------------- | ----------------------- |
| `/`             | 图库列表（筛选 / 排序 / 分页）      |
| `/download`     | 批量下载（表单 + WebSocket 日志） |
| `/artworks/:id` | 作品画廊（缩略图 + Fancybox）    |

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
  safepath/             # 路径/文件名校验
  server/               # HTTP、HTMX partial、WS、缩略图
web/
  *.html                # 页面
  app.css
  js/                   # ES Module（theme / scrollbar / 各页入口）
  vendor/               # htmx、fancybox、hx-ws
```

## 配置（环境变量）

| 变量                     | 默认                | 说明                        |
| ---------------------- | ----------------- | ------------------------- |
| `HOST`                 | `0.0.0.0`         | 监听地址；仅本机可设 `127.0.0.1`    |
| `PORT`                 | `7892`            | 端口                        |
| `PROXY_HOST`           | `127.0.0.1`       | 下载代理主机                    |
| `PROXY_PORT`           | `7897`            | 下载代理端口                    |
| `DOWNLOAD_DIR`         | `./downloads`     | 图片目录（缩略图缓存在其下 `.thumbs/`） |
| `DB_PATH`              | `./data/pixiv.db` | bbolt 库文件                 |
| `WEB_DIR`              | `./web`           | 静态前端目录                    |
| `DOWNLOAD_DELAY_MS`    | `1000`            | 下载间隔                      |
| `DOWNLOAD_TIMEOUT_SEC` | `30`              | 单请求超时                     |
| `SYNC_ON_START`        | `1`               | 启动时同步磁盘→数据库，并压缩库文件        |

说明：

- 创建时间 / 修改时间以磁盘为准（Windows 使用文件夹创建时间与修改时间）。
- 若 `DB_PATH` 仍是旧 SQLite 文件，会改名为 `*.sqlite.bak` 并强制同步重建。
- Cookie 仅通过请求头 `X-Pixiv-Cookie` 或表单字段传递，不支持 `?cookie=`。

## 前端

- 入口：`web/js/list.js`、`artworks.js`、`downloader.js`（`type="module"`）
- 公共：`boot.js` → 主题 + 自定义滚动条（移动端改用系统滚动条）
- 交互：HTMX partial；下载页 WebSocket（`hx-ws`）；作品页 Fancybox

## 主要路由

**页面 / Partial**

- `GET /partials/list` — 列表行、统计、分页
- `DELETE /partials/folders/:id` — 删除后刷新列表
- `POST /partials/download` — 提交下载
- `POST /partials/download/cancel` — 中断下载
- `GET /partials/artworks/:id` — 画廊 HTML
- `GET /ws` — 进度推送

**其它**

- `GET /thumb/:id/:file?w=` — 缩略图
- `GET /download/:id/:file` — 原图文件
- `GET /static/...` — 静态资源

**JSON（调试）**

- `GET /api/list`
- `POST /api/download`
- `GET /api/artworks/:id`
- `GET /api/artwork-id/:id`（需 Cookie 头）
- `DELETE /api/delete/:id`

## 许可证

本项目采用 [MIT License](./LICENSE) 发布。Copyright (c) 2026 Xephyrus。
