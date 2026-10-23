# ARXML Studio — Go + Vue

本地 ARXML 服务浏览、语义对比、vsomeip + CommonAPI 配置导出工具。后端 Go，前端 Vue 3 + TypeScript + Vite。前端资源、Vue 运行库和演示 ARXML 通过 `go:embed` 编入一个可执行文件，运行时无需 Python、Node、Go 或外部静态文件。

## Docker / Linux 后端服务

支持无前端的独立 HTTP API 服务，提供 Dockerfile、Compose、Bearer Token 鉴权、CORS 配置和健康检查。完整部署及接口示例见 [后端服务文档](docs/backend.md)。

```sh
./scripts/build-server.sh
./dist/arxml-server --listen 127.0.0.1:8080
```

该构建仅需 Go；使用 `-tags headless` 运行测试和编译时也无需前端资源。Docker 部署请先按文档设置 `.env` 中的随机访问令牌。

## 直接运行

预编译安装包见 [GitHub Releases](https://github.com/bkcarlos/ArxmlStudio/releases)。根据操作系统和 CPU 架构下载并解压，可使用同一 Release 中的 `SHA256SUMS.txt` 校验安装包。

当前构建产物为 **macOS Apple Silicon / arm64**：

```sh
./dist/arxml-studio
```

打开 http://127.0.0.1:8765 。使用 `--port 8766` 切换端口，`--port 0` 自动选择空闲端口。终端会打印访问地址，Ctrl+C 退出。程序仅监听本机，不自动打开浏览器。

可把可执行文件复制到任意目录单独运行。其他系统需要分别编译；一个 macOS 文件不能直接在 Windows/Linux 上运行。

## 从源码构建

构建环境：Go 1.25+、Node 22.12+、npm。仅构建时需要前端依赖，Go 后端只使用标准库。

```sh
./scripts/build.sh
```

脚本按锁文件安装依赖、执行 TypeScript 检查、构建 Vue，再将前端产物嵌入 Go 程序，输出 `dist/arxml-studio`。

手动执行与校验：

```sh
npm --prefix frontend ci
npm --prefix frontend run build
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/arxml-studio .
```

**必须先构建前端再执行 Go 构建或测试**，因为 `go:embed` 需要 `frontend/dist`。不要直接修改该生成目录。

前端构建完成后可交叉编译，例如：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/arxml-studio-linux-amd64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/arxml-studio-windows-amd64.exe .
```

开发时先启动 `go run .`，另一个终端执行 `npm --prefix frontend run dev`。Vite 将 `/api` 代理至 `127.0.0.1:8765`；修改 Go 代码后需重启后端。

## 使用流程

1. 加载文件 A，查看服务列表、methods、events、fields、参数方向、类型引用和 SOME/IP 部署，支持搜索。
2. 可选加载 B；以 A → B 比较新增、删除和修改，支持导出差异 JSON。
3. 「当前文件」选择 A/B，选择服务，点击「导出配置」。
4. 表单选择 Server / Stub 或 Client / Proxy，填写 IP、实例和通信 ID，下载 ZIP。

可点击「加载演示文件」体验。文件 A 的导出参数见 `examples/export-settings.json`：Application ID `0x1001`、IP `127.0.0.1`、端口 `30509`、Instance ID `1`、两个事件组均为 `1`、字段 getter ID `2`、notifier ID `0x8002`。这些是演示值，并非实际网络配置。

支持 Adaptive `SERVICE-INTERFACE`、Classic `CLIENT-SERVER-INTERFACE` 和 `SENDER-RECEIVER-INTERFACE` 浏览。比较按完整 ARXML 路径识别对象，保留参数顺序，忽略排版、命名空间前缀、UUID 及方法/事件集合顺序，显示具体属性路径与前后值。比较范围为已识别接口、关联部署和数据类型，**不是整个 AUTOSAR 模型的完整比较**。

文件只发送至本机，不持久保存上传内容，不使用 CDN。单文件限制 20 MiB，支持 UTF-8 与带 BOM 的 UTF-16，拒绝 DTD/实体声明和过深 XML。

## 导出规则和边界

每次导出一个选定服务，ZIP 包含 `.fidl`、`.fdepl`、`vsomeip-server.json` 或 `vsomeip-client.json`、`export-settings.json` 和使用说明。

双方共用接口和部署，服务端实现 Stub，客户端使用 Proxy。客户端通过服务发现查找服务，不声明本地提供的服务。双方 Service ID、Instance ID、接口版本、成员 ID 必须一致；各自设置应用名、应用 ID 和本机地址。

自动提取唯一关联部署的 Service ID、版本、method/event ID。实例、网络地址、字段 ID、事件组需在表单补充，缺失或非法值会阻止导出；多个部署时手动填写目标部署参数。当前统一选择 TCP 或 UDP，不自动转换混合传输。服务发现默认 `224.224.224.245:30490`，可在生成 JSON 中调整；同机多个进程需配置共享 routing manager。

基础类型根据 SW-BASE-TYPE 位宽/编码及别名引用映射。高级类型映射可显式指定 `{"/Types/Speed":"UInt16"}`，仅在确认等价时使用。复杂结构、数组、枚举、应用错误、INOUT、无 getter 字段尚不支持可靠转换，会拒绝导出；字符串可显式映射，但编码/长度仍需核对。

不做 AUTOSAR XSD 验证，不支持跨文件合并引用（A/B 是两个独立版本）。实例拓扑、E2E、完整序列化约束和厂商扩展尚未自动映射。生成文件仍需目标 CommonAPI 生成器、运行库和业务实现；**尚未通过实际 CommonAPI 工具链编译或 ECU 通信验证**。

## CLI

```sh
./dist/arxml-studio --version
./dist/arxml-studio --export examples/vehicle-v1.arxml --output parsed.json
./dist/arxml-studio --compare examples/vehicle-v1.arxml --with examples/vehicle-v2.arxml --output diff.json
./dist/arxml-studio --bundle examples/vehicle-v1.arxml --service /Vehicle/VehicleService --settings examples/export-settings.json --output vehicle-server.zip
```

客户端导出将参数文件的 `role` 改为 `client`。CLI 同样不依赖 Python。

## 结构与验证

- `main.go`：内嵌静态资源、HTTP API、CLI、服务生命周期。
- `internal/arxml/`：Go 解析、统一模型、语义对比、导出与测试。
- `frontend/src/`：Vue 组件、TypeScript API 类型、表单与界面。
- `examples/`：回归输入和导出参数；`frontend/public/examples/` 为内嵌演示输入。
- `scripts/build.sh`：单文件构建入口。

旧 Python 与原生 JS 实现已从构建和测试入口移除，不参与交付。

测试覆盖解析/UTF-16、格式噪声、参数顺序、类型与部署差异、非法 XML、缺失引用、双方导出和传输协议、ID 校验、ZIP 内容、HTTP API 与嵌入资源。另已验证演示导出与迁移前一致。

参考：[Go embed](https://pkg.go.dev/embed)、[Vue 文档](https://vuejs.org/guide/quick-start.html)、[COVESA vsomeip 配置](https://github.com/COVESA/vsomeip/blob/master/documentation/vsomeipConfiguration.md)、[CommonAPI SOME/IP 部署规范](https://github.com/COVESA/capicxx-someip-tools/blob/master/org.genivi.commonapi.someip/deployment/CommonAPI-SOMEIP_deployment_spec.fdepl)。

## 生成 Release 安装包

```sh
./scripts/release.sh v0.3.0
```

分别构建完整程序 `arxml-studio` 和纯后端 `arxml-server` 的 macOS arm64/amd64、Linux arm64/amd64、Windows amd64 安装包（共 10 个），附带使用说明、示例和 Vue 第三方许可，输出至 `dist/releases/v0.3.0/`。安装包的 SHA-256 校验值汇总在 `SHA256SUMS.txt`。脚本只构建和打包，不自动发布。

## GitHub Actions 自动发布

`main` 分支和 Pull Request 自动运行前端构建、Go 测试、竞态检测及静态检查。推送 `v*.*.*` 标签后，Actions 构建两种程序的全部安装包、校验清单并自动发布 GitHub Release。无需配置个人访问令牌，发布 job 使用仓库自带的 `GITHUB_TOKEN`，仅该 job 拥有 `contents: write`。

后续版本请先更新 `main.go`、前端 package/package-lock 版本、页面版本显示，以及 `docs/releases/vX.Y.Z.md`，提交后推送相同版本的标签。发布脚本会检查前端版本与标签一致；已正式发布的版本不会被自动覆盖。工作流可从仓库 Actions 页面查看，失败的草稿发布可重跑。
