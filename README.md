<p align="center">
  <img src="frontend/static/manifest/app.png" width="112" height="112" alt="Leave app icon" />
</p>

<h1 align="center">Leave</h1>

<p align="center">
  A leave-slip simulation and learning project with an Android app, a web admin panel and a Go backend.
</p>

<p align="center">
  <strong>English</strong> · <a href="README_zh_CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/avery-my/Leave/releases/latest"><img src="https://img.shields.io/github/v/release/avery-my/Leave?style=flat-square&amp;label=release" alt="Release" /></a>
  <a href="https://github.com/avery-my/Leave/releases"><img src="https://img.shields.io/github/downloads/avery-my/Leave/total?style=flat-square&amp;logo=github" alt="Downloads" /></a>
</p>

This project explores leave-slip generation and related account and data management. It does not implement a school approval workflow or provide official leave authorization.

## Features

- Account registration, login, profile editing and password changes.
- Leave applications, leave-slip details and leave history.
- Light and dark themes, appearance settings, feedback and app updates.
- Admin tools for student accounts, leave records, classes, dormitories, leave types, feedback, version records and overview statistics.
- Server-side login sessions and request rate limiting backed by Redis.

Development is ongoing. Some features still use mock or placeholder data, and integration progress varies by module.

## Download

Get the latest Android package from [GitHub Releases](https://github.com/avery-my/Leave/releases/latest), or browse [previous releases](https://github.com/avery-my/Leave/releases).

| Platform | Package | Download |
| --- | --- | --- |
| Android | APK | [Latest Android APK](https://github.com/avery-my/Leave/releases/latest) |

The web admin and Go backend require your own deployment. See [Run from source](#run-from-source) below and the [deployment guide](deploy/README.md).

## Tech stack

| Module | Technologies |
| --- | --- |
| Android app | uni-app x, UTS, uvue, HBuilderX |
| Web admin | Vue 3, TypeScript, Vite, Naive UI, UnoCSS, Pinia |
| Backend | Go, Gin, GORM, Zap |
| Storage | MySQL for business data; Redis for sessions and rate limiting |

The admin panel is based on Soybean Admin. Android is the app's primary target platform.

## Run from source

You need Go, MySQL, Redis, Node.js, pnpm and HBuilderX. See [go.mod](backend/go.mod) and [package.json](admin/package.json) for toolchain requirements. Running the app also requires an Android device or a configured Android runtime.

Run each module in its own terminal. Start the commands below from the repository root.

### Backend

On macOS, copy the templates if the local files are missing.

```sh
[ -e .env ] || cp .env.example .env
[ -e backend/config/config.local.yaml ] || cp backend/config/config.example.yaml backend/config/config.local.yaml
```

On Windows, use PowerShell’s `Copy-Item` for the same templates.

```powershell
Copy-Item .env.example .env
Copy-Item backend/config/config.example.yaml backend/config/config.local.yaml
```

Configure the following before starting:

- Set `MYSQL_PASSWORD` in the root `.env`. Set `REDIS_PASSWORD` if Redis authentication is enabled; otherwise leave it empty.
- Set the MySQL and Redis connection details in `backend/config/config.local.yaml`. Both services must be available.
- For direct access to an existing remote development environment, use the remote MySQL and Redis hosts and ports on a trusted private network. The backend currently connects without TLS. If the connection crosses an untrusted network, use an encrypted VPN or SSH forwarding.
- Use an environment-specific `app.namespace`. The session TTLs can use the template values.
- Set `app.package_dir` to your APK directory, and configure `storage.root_dir` and `storage.base_url` for uploads. Serve uploaded files through a static server such as Nginx, using the matching directory and URL prefix.

If you need to initialize or upgrade a separate development database, follow the [database guide](backend/database/README.md). New databases use the current table definitions and required seed data; existing databases use incremental migrations. No default admin account or password is provided.

```sh
cd backend
go mod download
go run ./cmd/server
```

For hot reload, run `air -c .air.toml` from `backend/`. Both commands use the same backend connection settings.

The template uses port `8080` and the `/api/v1` prefix. Check the service from another terminal:

```sh
curl -fsS http://127.0.0.1:8080/api/v1/health
```

In Windows PowerShell, use `Invoke-RestMethod http://127.0.0.1:8080/api/v1/health`.

App requests also check the client version. Prepare published version data matching `frontend/manifest.json` before app integration; see the [release guide](deploy/README.md) for version data and APK preparation.

To enable school photo import, set `SCHOOL_URL` and `SCHOOL_PASSWORD` in the root `.env`. Use an HTTPS root URL for `SCHOOL_URL`. When a student opens their leave record in the app and their profile has no photo, the backend uses their student ID and OCR to fetch the photo from School. The detail request makes up to five attempts, stopping at the first success. The file is saved under `photo/` inside the YAML-configured `storage.root_dir`, and its relative path is stored in `profiles.photo_path`. If import fails, the app still receives the leave record details and can retry by opening them again.

#### Internal OCR

Construct a shared instance with `ocr.NewService(logger)` from `backend/internal/service/common/ocr` and inject it into the consuming service. Call `ocrService.RecognizeBase64(ctx, imageBase64)` and close the instance at shutdown. It accepts a PNG or JPEG as Base64 or a data URI and returns text and an error. For local development, set the working directory to `backend/` or the repository root. Models are in `backend/assets/ocr/model`. Put the platform’s ONNX Runtime library in `backend/assets/ocr/lib` or set `OCR_ONNX_RUNTIME` to its absolute path.

### Web admin

macOS terminal

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

Set `VITE_SERVICE_BASE_URL` in `admin/.env` to your backend API. The template uses `http://localhost:8080/api/v1`. Open the development URL printed in the terminal.

See the [admin guide](admin/README.md) for environment configuration and additional commands.

### Android app

1. Open `frontend/` in HBuilderX. If requested by the compiler, install the [Dart Sass plugin](https://ext.dcloud.net.cn/plugin?name=compile-dart-sass).
2. Check the development API and avatar URLs in `frontend/utils/api.uts`. Running uses the development configuration; release packaging uses the production configuration.
3. Connect and authorize an Android device, then run the app from HBuilderX.

For USB debugging, the development API defaults to `http://127.0.0.1:8080/api/v1`. With ADB available and the device authorized, forward the backend port:

```sh
adb reverse tcp:8080 tcp:8080
```

The default development avatar URL uses a static server on port `8081`. If you use it, start that server and also run `adb reverse tcp:8081 tcp:8081`. The backend's `storage.base_url` must be reachable from the device. A reachable LAN address is another option; update the development URLs accordingly.

The app uses HBuilderX's uni-app x toolchain and has no npm/pnpm build command.

### Optional development helpers

On macOS, install Air and ADB with `brew install go-air` and `brew install --cask android-platform-tools`.

From the repository root, `make phone` checks the device and local services, then sets up USB forwarding. For multiple devices, pass `DEVICE_SERIAL=<serial>`. macOS also accepts `ADB_PATH=/path/to/adb`. Windows requires PowerShell.

Windows developers who need SSH tunnels can run `make air` from the repository root. See the [SSH tunnel setup](deploy/README.md#windows-本地开发).

## Checks

In `backend/`:

```sh
go test ./...
go build ./...
```

In `admin/`:

```sh
pnpm typecheck
pnpm exec oxlint
pnpm exec eslint .
pnpm exec oxfmt --check src packages build tests
node --test tests/*.test.mjs
pnpm build
```

These lint and formatting commands only check files. `pnpm lint` and `pnpm fmt` modify files. `pnpm build` uses production mode; use `pnpm build:test` for test mode, with the corresponding `.env.prod.example` or `.env.test.example` template.

Database and Redis integration tests require isolated local test instances; see the [backend conventions](backend/AGENTS.md). A skipped integration test is not a passed test. The Node tests cover some app logic, but app compilation and device interaction checks still require HBuilderX and Android.

## Documentation

- [Authentication and Redis sessions](docs/authentication.md)
- [Admin development](admin/README.md)
- [Database initialization and migrations](backend/database/README.md)
- [Deployment and app releases](deploy/README.md)
- [Development conventions](AGENTS.md)

- [Feature implementation status (Chinese)](docs/feature-status.md)
- [Contributing (Chinese)](CONTRIBUTING.md)
- [Security reporting (Chinese)](SECURITY.md)
- [Android release checklist (Chinese)](docs/release-checklist.md)
- [Maintainer checklist (Chinese)](docs/maintainer-checklist.md)

The detailed guides are currently written in Chinese.

## License

Leave original code is licensed under [GNU GPL v3.0 only](LICENSE) (`GPL-3.0-only`). Copyright (C) 2026 avery-my. Third-party code and assets retain their own licenses, including the Soybean Admin [MIT notice](admin/LICENSE). See [licensing and distribution notes (Chinese)](docs/licensing.md).
