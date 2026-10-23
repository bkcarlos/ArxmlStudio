# 独立后端服务部署

后端是无状态 HTTP 服务：请求携带 ARXML，响应返回解析结果、差异或配置包，不保存用户文件。当前不包含数据库、用户账号或历史记录。原有完整 Go + Vue 程序仍可使用；`headless` 构建不包含前端，构建时也不需要 Node。

## Docker / Linux

在源码目录执行：

```sh
cp .env.example .env
# 生成随机令牌，将输出填入 .env 的 ARXML_API_TOKEN
openssl rand -hex 32
docker compose up -d --build
docker compose ps
docker compose logs -f api
```

Compose 默认将服务绑定到服务器的 `127.0.0.1:8080`，适合由同机 Nginx/Caddy 提供 HTTPS 反向代理。需要局域网直接访问时，将 `.env` 的 `ARXML_BIND_IP` 改为服务器 IP 或 `0.0.0.0`。访问 `/api/*` 必须携带 `Authorization: Bearer <ARXML_API_TOKEN>`；`GET /health` 无需令牌。

Docker 镜像是非 root 的纯 Go 服务，最终镜像不包含 shell、编译器或 Node；Compose 启用只读文件系统、资源限制和自动重启。接收 SIGTERM 后会等待现有 HTTP 请求完成，最多等待 5 秒再关闭连接。

`.env` 不进入 Git 或 Docker 构建上下文。远程监听强制要求至少 32 字符的令牌。该令牌是服务级共享密钥，不是用户登录；面向多用户时建议由业务网关鉴权并转发请求，避免把服务密钥写进公开前端构建产物。跨网络部署时由反向代理终止 HTTPS；服务自身只提供 HTTP。

## Docker Hub 不可达时

可以使用本机 Go 编译 Linux 二进制，再构建不依赖任何远端基础镜像的 scratch 镜像：

```sh
./scripts/build-image.sh             # 使用本机 CPU 架构
# 或 ./scripts/build-image.sh amd64   # 面向 Linux x86-64 服务器
# 仍需按前面的步骤准备 .env
docker compose up -d --no-build
```

离线镜像与标准多阶段镜像使用相同的运行配置。默认 tag 是 `arxml-studio-api:local`。执行 Docker build 不需访问 Docker Hub，但需要本机安装与 go.mod 兼容的 Go 工具链。

## 裸程序直接运行

从 GitHub Releases 下载对应平台的 `arxml-server_*` 安装包，解压后执行：

```sh
# Linux / macOS；必要时先 chmod +x arxml-server
./arxml-server --listen 127.0.0.1:8080
# Windows PowerShell
# .\arxml-server.exe --listen 127.0.0.1:8080
```

无需 Docker、Go、Node 或 Python；本机监听可以不设置令牌。对外监听时设置 `ARXML_API_TOKEN`，并使用 `--listen 0.0.0.0:8080`。这里的 server 包不提供页面；需要内置 Vue 页面请下载 `arxml-studio_*` 包。

## Linux 原生构建

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags headless -trimpath -o dist/arxml-server .
# 也可在目标机器执行 ./scripts/build-server.sh
export ARXML_API_TOKEN='替换为随机生成的至少32字符令牌'
./dist/arxml-server --listen 0.0.0.0:8080 --api-only
```

| 配置 | 默认值 / 说明 |
| --- | --- |
| `--listen` / `ARXML_LISTEN` | 默认为 `127.0.0.1:8765`；显式 `--listen` 优先于环境变量及 `--port` |
| `--api-only` | headless 构建默认为 true，完整构建默认为 false |
| `ARXML_API_TOKEN` | API Bearer token；非本机监听必须设置至少 32 字符 |
| `--cors-origins` / `ARXML_CORS_ORIGINS` | 允许的浏览器 Origin，逗号分隔，如 `https://studio.example`，不带尾部斜杠；默认只接受同源请求 |
| `--max-concurrent` | 默认 4，同时处理的 API 请求数，范围 1–64；超过返回 429 |
| `--healthcheck URL` | 探测 `/health` 后退出，用于容器健康检查 |

反向代理后的 HTTPS 前端即使使用同一公网域名，也应将公开前端 Origin 加入允许列表。服务不信任客户端提供的 `X-Forwarded-*`。已配置的 CORS origin 只决定浏览器可访问范围，不会跳过 Bearer token 校验。健康检查只代表进程可服务，当前没有数据库等依赖。

## API 协议

请求为 `application/json`。文件使用 `{ "name": "input.arxml", "data": "原始文件字节的Base64" }`。每份 ARXML 上限 20 MiB，整个 HTTP 请求体上限 60 MiB。

| 方法 / 路径 | 请求 | 响应 |
| --- | --- | --- |
| `GET /health` | 无 | `{ "status":"ok", "version":"..." }` |
| `POST /api/parse` | 文件对象 | `format`、`source`、`services`、`types`、`warnings`、`summary` |
| `POST /api/compare` | `{ "before":文件对象, "after":文件对象 }` | `changes`、`summary` 及来源文件名 |
| `POST /api/settings` | `{ "file":文件对象, "path":"/包/服务" }` | 从 ARXML 推导的导出参数；缺失信息需调用方填写 |
| `POST /api/export` | `{ "file":文件对象, "path":"/包/服务", "settings":导出参数 }` | `filename`、Base64 ZIP `data`、文本文件字典 `files` |
| `POST /api/export/zip` | 同上 | 直接返回 `application/zip`，含下载文件名 |

导出参数参见 `examples/export-settings.json`，`role` 可为 `server` 或 `client`。解析与导出规则沿用 [项目说明](../README.md#导出规则和边界)。错误响应统一为 `{ "error":"说明" }`；400 为输入/配置错误、401 为鉴权失败、403 为 Origin/Host 被拒绝、404 为未知路径、405 为不支持的方法、413 为请求体过大、429 为并发已满。

### curl 示例

以下命令使用 `jq` 将 ARXML 文件转换为请求，不需要 Python。先在当前终端设置真实的 `ARXML_API_TOKEN`：

```sh
export API_URL=http://127.0.0.1:8080
curl -fsS "$API_URL/health"

jq -n --rawfile xml examples/vehicle-v1.arxml \
  '{name:"vehicle-v1.arxml",data:($xml|@base64)}' > /tmp/arxml-request.json
curl -fsS "$API_URL/api/parse" \
  -H "Authorization: Bearer $ARXML_API_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @/tmp/arxml-request.json

jq -n --slurpfile file /tmp/arxml-request.json \
  --slurpfile settings examples/export-settings.json \
  '{file:$file[0],path:"/Vehicle/VehicleService",settings:$settings[0]}' > /tmp/arxml-export.json
curl -fsS "$API_URL/api/export/zip" \
  -H "Authorization: Bearer $ARXML_API_TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @/tmp/arxml-export.json -o vehicle-server.zip
```

以上 `jq --rawfile` 示例针对 UTF-8 文件；UTF-16 文件应对原始字节进行 Base64 编码。独立前端调用时在请求中设置 `Authorization`；现有打包的 Vue 页面仍默认调用同源 `/api`，没有服务密钥输入功能。可由前端的业务后端代理注入鉴权，或自行接入这些 API。

## 检查

```sh
go test -tags headless -race ./...
go vet -tags headless ./...
./scripts/build-server.sh
# 容器环境可用时验证构建与启动
docker compose up -d --build
```
