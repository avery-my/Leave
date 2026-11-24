<p align="center">
  <img src="frontend/static/manifest/app.png" width="112" height="112" alt="Leave app icon" />
</p>

<h1 align="center">Leave</h1>

<p align="center">
  一个假条模拟与学习项目，包含 Android 客户端、Web 管理端和 Go 后端。
</p>

<p align="center">
  <a href="README.md">English</a> · <strong>简体中文</strong>
</p>

<p align="center">
  <a href="https://github.com/avery-my/Leave/releases/latest"><img src="https://img.shields.io/github/v/release/avery-my/Leave?style=flat-square&amp;label=release" alt="Release" /></a>
  <a href="https://github.com/avery-my/Leave/releases"><img src="https://img.shields.io/github/downloads/avery-my/Leave/total?style=flat-square&amp;logo=github" alt="Downloads" /></a>
</p>

项目用于学习假条生成及相关账号、数据管理，不包含学校审批流程，也不提供正式请假授权。

## 功能

- 账号注册、登录、个人资料维护和密码修改。
- 请假申请、假条详情和请假记录。
- 明暗主题、外观设置、意见反馈和客户端更新。
- 管理端提供学生账号、请假记录、班级、宿舍、请假类型、反馈、版本记录及数据概览等功能。
- 基于 Redis 的服务端登录会话和请求限流。

项目持续开发中，部分功能仍使用 Mock 或占位数据，各模块的接口联调进度不同。

## 下载

从 [GitHub Releases](https://github.com/avery-my/Leave/releases/latest) 获取最新 Android 安装包，或查看[历史版本](https://github.com/avery-my/Leave/releases)。

| 平台 | 安装包 | 下载 |
| --- | --- | --- |
| Android | APK | [最新 Android APK](https://github.com/avery-my/Leave/releases/latest) |

Web 管理端和 Go 后端需自行部署，参见下方的[从源码运行](#从源码运行)和[部署说明](deploy/README.md)。

## 技术栈

| 模块 | 技术 |
| --- | --- |
| Android 客户端 | uni-app x、UTS、uvue、HBuilderX |
| Web 管理端 | Vue 3、TypeScript、Vite、Naive UI、UnoCSS、Pinia |
| 后端服务 | Go、Gin、GORM、Zap |
| 数据存储 | MySQL（业务数据）、Redis（登录会话与请求限流） |

管理端基于 Soybean Admin 开发，客户端目前以 Android 为主要运行平台。

## 从源码运行

需要 Go、MySQL、Redis、Node.js、pnpm 和 HBuilderX。工具链版本要求见 [go.mod](backend/go.mod) 和 [package.json](admin/package.json)。客户端运行还需要 Android 真机或已配置的 Android 运行环境。

各模块使用独立终端，以下命令均以项目根目录为起点。

### 后端

macOS 上复制配置模板。已有本地配置时跳过对应命令。

```sh
[ -e .env ] || cp .env.example .env
[ -e backend/config/config.local.yaml ] || cp backend/config/config.example.yaml backend/config/config.local.yaml
```

Windows 上可用 PowerShell 的 `Copy-Item` 复制同一组模板。

```powershell
Copy-Item .env.example .env
Copy-Item backend/config/config.example.yaml backend/config/config.local.yaml
```

启动前完成以下配置：

- 在根目录 `.env` 中填写 `MYSQL_PASSWORD`；Redis 启用认证时填写 `REDIS_PASSWORD`，否则留空。
- 在 `backend/config/config.local.yaml` 中设置 MySQL 和 Redis 连接信息，确保两个服务均可连接。
- 在可信私有网络内直连已有远端开发环境时，填写远端 MySQL 和 Redis 的主机与端口。后端当前未启用数据库连接 TLS，跨不可信网络时应使用加密 VPN 或 SSH 转发。
- 设置当前环境独立的 `app.namespace`，会话有效期可沿用模板值。
- 将 `app.package_dir` 设为本地 APK 目录，配置上传文件的 `storage.root_dir` 和 `storage.base_url`。上传文件通过 Nginx 等静态服务访问，目录与 URL 前缀需相互对应。

若需要新建或升级独立开发库，按 [数据库说明](backend/database/README.md) 初始化独立的开发数据库。新数据库使用当前完整表结构和所需的 seed 数据，已有数据库使用增量迁移。初始化数据不提供默认管理员账号或固定密码。

```sh
cd backend
go mod download
go run ./cmd/server
```

需要热重载时，在 `backend/` 内执行 `air -c .air.toml`。两种启动方式使用相同的后端连接配置。

模板使用 `8080` 端口和 `/api/v1` 前缀。可在另一个终端检查服务：

```sh
curl -fsS http://127.0.0.1:8080/api/v1/health
```

Windows PowerShell 可使用 `Invoke-RestMethod http://127.0.0.1:8080/api/v1/health`。

App 请求还会校验客户端版本。联调前需准备与 `frontend/manifest.json` 对应的已发布版本数据，版本数据与 APK 的准备方式见 [发布说明](deploy/README.md)。

如需在后端获取学校照片，在根目录 `.env` 中设置 `SCHOOL_URL` 和 `SCHOOL_PASSWORD`。`SCHOOL_URL` 必须是 HTTPS 根地址。调用方传入用户名，后端获取验证码并通过 OCR 识别。照片保存在 YAML 配置的 `storage.root_dir` 下的 `photo/` 目录。

#### 内部 OCR

通过 `backend/internal/service/common/ocr` 的 `ocr.NewService(logger)` 创建共享实例，再注入使用 OCR 的业务 Service。调用 `ocrService.RecognizeBase64(imageBase64)`，并在应用退出时关闭实例。输入是 PNG 或 JPEG 的 Base64 或 Data URI，返回识别文本和错误。本地开发时，将工作目录设为 `backend/` 或仓库根目录。模型位于 `backend/assets/ocr/model`。将对应平台的 ONNX Runtime 动态库放入 `backend/assets/ocr/lib`，或通过 `OCR_ONNX_RUNTIME` 指定动态库的绝对路径。

### Web 管理端

macOS 终端

```sh
cd admin
[ -e .env ] || cp .env.example .env
pnpm install --frozen-lockfile
pnpm dev
```

Windows PowerShell

```powershell
cd admin
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
pnpm install --frozen-lockfile
pnpm dev
```

通过 `admin/.env` 中的 `VITE_SERVICE_BASE_URL` 设置后端 API 地址，模板默认使用 `http://localhost:8080/api/v1`。启动后访问终端显示的开发地址。

环境配置和其他命令见 [管理端说明](admin/README.md)。

### Android 客户端

1. 使用 HBuilderX 打开 `frontend/`。如果编译器提示缺少 Dart Sass，安装 [Dart Sass 插件](https://ext.dcloud.net.cn/plugin?name=compile-dart-sass)。
2. 检查 `frontend/utils/api.uts` 中的开发接口和头像地址。运行使用 development 配置，发行打包使用 production 配置。
3. 连接并授权 Android 设备，通过 HBuilderX 运行客户端。

USB 调试时，开发接口默认使用 `http://127.0.0.1:8080/api/v1`。ADB 可用且设备已授权后，配置后端端口反向转发：

```sh
adb reverse tcp:8080 tcp:8080
```

默认开发头像地址使用 `8081` 端口的静态服务。使用时先启动该服务，再执行 `adb reverse tcp:8081 tcp:8081`。后端的 `storage.base_url` 也需能被设备访问。也可以使用设备可访问的电脑局域网地址，并同步调整开发地址。

客户端使用 HBuilderX 的 uni-app x 工具链，没有 npm/pnpm 构建命令。

### 可选开发脚本

在 macOS 上，可用 Homebrew 安装 Air（`brew install go-air`）和 ADB（`brew install --cask android-platform-tools`）。

在项目根目录执行 `make phone`，可检查设备和本地服务并配置 USB 反向转发。多设备时传入 `DEVICE_SERIAL=<设备序列号>`。macOS 可传入 `ADB_PATH=/path/to/adb`，Windows 需要 PowerShell。

Windows 开发需要 SSH 隧道时，可在项目根目录运行 `make air`。配置详见 [SSH 隧道流程](deploy/README.md#windows-本地开发)。

## 检查

在 `backend/` 中执行：

```sh
go test ./...
go build ./...
```

在 `admin/` 中执行：

```sh
pnpm typecheck
pnpm exec oxlint
pnpm exec eslint .
pnpm exec oxfmt --check src packages build tests
node --test tests/*.test.mjs
pnpm build
```

以上 Lint 和格式命令仅检查文件，`pnpm lint`、`pnpm fmt` 会修改文件。`pnpm build` 使用生产模式，测试模式使用 `pnpm build:test`，对应环境模板为 `.env.prod.example` 和 `.env.test.example`。

数据库和 Redis 集成测试需要独立的本机测试实例，详见 [后端开发约定](backend/AGENTS.md)。跳过不代表通过。Node 测试覆盖部分客户端逻辑，客户端编译和设备交互仍需通过 HBuilderX 与 Android 验证。

## 相关文档

- [认证与 Redis 会话](docs/authentication.md)
- [管理端开发](admin/README.md)
- [数据库初始化与迁移](backend/database/README.md)
- [部署与客户端发布](deploy/README.md)
- [项目开发约定](AGENTS.md)
- [功能实现状态](docs/feature-status.md)
- [参与贡献](CONTRIBUTING.md)
- [安全问题报告](SECURITY.md)
- [Android 发布验收清单](docs/release-checklist.md)
- [维护者待办](docs/maintainer-checklist.md)

## 许可证

Leave 自有代码采用 [GNU GPL v3.0 only](LICENSE)（`GPL-3.0-only`），Copyright (C) 2026 avery-my。第三方代码和素材沿用各自许可，包括 Soybean Admin 的 [MIT 声明](admin/LICENSE)。详见 [许可证与分发说明](docs/licensing.md)。
