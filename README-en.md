# 后端本地打包与多环境部署

本文只介绍后端部署：在本地编译一次 Linux 二进制文件，上传到服务器后，通过不同的外部 YAML 配置文件启动正式服、测试服或其他环境。

同一操作系统和 CPU 架构下，所有环境共用同一个二进制文件，不需要为正式服、测试服分别编译。

```text
同一个 tb-live-server
├── -c /etc/tb-live/config.production.yaml
├── -c /etc/tb-live/config.testing.yaml
└── -c /etc/tb-live/config.staging.yaml
```

## 1. 确认服务器架构

先在本地查询目标服务器的 CPU 架构：

```bash
ssh deploy@your-server 'uname -m'
```

对应关系：

| 服务器返回值 | Go 编译参数 |
|---|---|
| `x86_64` | `GOARCH=amd64` |
| `aarch64` 或 `arm64` | `GOARCH=arm64` |

不同 CPU 架构不能共用同一个二进制文件。如果所有服务器架构一致，只需构建一次。

## 2. 在本地构建后端

项目当前使用 Go `1.24.x`。在项目根目录执行：

```bash
cd /path/to/live-gin-vue-admin

export TARGET_ARCH=amd64
export RELEASE_ID="$(date +%Y%m%d%H%M%S)"

mkdir -p release

cd server
go mod download

CGO_ENABLED=0 GOOS=linux GOARCH="$TARGET_ARCH" \
  go build -trimpath \
  -o "../release/tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH" .
```

ARM64 服务器请将 `TARGET_ARCH` 改为 `arm64`。

该命令只生成一个后端二进制文件，环境配置不会编译进二进制。

这里保留 `-trimpath`，用于移除构建机器上的本地绝对路径；不使用 `-ldflags="-s -w"`，从而保留更完整的符号表和 DWARF 调试信息，方便通过 Delve、GDB 或 core dump 排查生产问题。该构建仍会使用 Go 默认的编译优化，不是关闭优化的调试构建。

在 macOS 上可以检查文件类型和生成校验值：

```bash
cd /path/to/live-gin-vue-admin/release

file "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH"
shasum -a 256 "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH" \
  > "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH.sha256"
```

不要尝试在 macOS 或 Windows 上直接运行 Linux 二进制文件。

## 3. 准备各环境的配置文件

可以以 `server/config.prod.yaml` 或 `server/config.dev.yaml` 为模板，在本地分别准备：

```text
config.production.yaml
config.testing.yaml
config.staging.yaml
```

配置文件不应打进二进制，也不要提交包含真实密码和密钥的生产配置到 Git。

如果多个实例同时运行在同一台服务器上，每份配置至少需要检查以下内容：

- `system.addr`：每个实例必须使用不同端口，例如正式服 `8888`、测试服 `8889`。
- `system.router-prefix`：根据实际网关路由设置，例如 `/api`。
- `mysql`：使用对应环境的数据库地址、账号、密码和数据库名。
- `redis`：使用对应环境的 Redis；共用 Redis 时至少隔离数据库编号和业务数据。
- `jwt.signing-key`、`jwt-app.signing-key`：使用安全密钥，并确保两者不同。
- `zap.director`：使用不同日志目录，例如 `log/production` 和 `log/testing`。
- `local.path`、`local.store-path`：使用不同上传目录，例如 `uploads/production` 和 `uploads/testing`。
- `live.publish-token-key` 和 `live.srs`：填写对应环境的直播密钥及 SRS 地址。
- `live.log-srs-hook-raw-body`：只在排查 SRS 回调时临时开启；原始 publish 参数可能包含 `pt` 凭据。
- `system.disable-auto-migrate`：正式环境通常设置为 `true`，数据库变更应经过审核后单独执行。

示例目录和端口：

| 环境 | 配置文件 | 后端端口 | 日志目录 | 上传目录 |
|---|---|---:|---|---|
| 正式服 | `config.production.yaml` | `8888` | `log/production` | `uploads/production` |
| 测试服 | `config.testing.yaml` | `8889` | `log/testing` | `uploads/testing` |
| 预发布 | `config.staging.yaml` | `8890` | `log/staging` | `uploads/staging` |

## 4. 上传二进制和配置文件

在本地执行：

```bash
cd /path/to/live-gin-vue-admin/release

scp "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH" \
  "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH.sha256" \
  deploy@your-server:/tmp/

scp /path/to/config.production.yaml \
  /path/to/config.testing.yaml \
  /path/to/config.staging.yaml \
  deploy@your-server:/tmp/
```

如果没有预发布环境，可以从命令中去掉 `config.staging.yaml`。

## 5. 在服务器上安装

登录服务器：

```bash
ssh deploy@your-server
```

设置本次发布参数，并校验二进制：

```bash
export RELEASE_ID=<release-id>
export TARGET_ARCH=<amd64-or-arm64>

cd /tmp
sha256sum -c "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH.sha256"
file "tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH"
```

创建服务用户和目录：

```bash
sudo useradd --system --home /opt/tb-live --shell /usr/sbin/nologin tb-live 2>/dev/null || true

sudo install -d -m 0755 /opt/tb-live/bin
sudo install -d -m 0755 /opt/tb-live/server
sudo install -d -m 0750 -o tb-live -g tb-live /opt/tb-live/server/log
sudo install -d -m 0750 -o tb-live -g tb-live /opt/tb-live/server/uploads
sudo install -d -m 0750 -o root -g tb-live /etc/tb-live
```

先限制上传到临时目录中的配置文件权限：

```bash
chmod 600 /tmp/config.production.yaml
chmod 600 /tmp/config.testing.yaml
chmod 600 /tmp/config.staging.yaml
```

安装这一个公共二进制文件：

```bash
sudo install -m 0755 -o root -g root \
  "/tmp/tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH" \
  /opt/tb-live/bin/tb-live-server
```

安装各环境配置：

```bash
sudo install -m 0640 -o root -g tb-live \
  /tmp/config.production.yaml \
  /etc/tb-live/config.production.yaml

sudo install -m 0640 -o root -g tb-live \
  /tmp/config.testing.yaml \
  /etc/tb-live/config.testing.yaml

sudo install -m 0640 -o root -g tb-live \
  /tmp/config.staging.yaml \
  /etc/tb-live/config.staging.yaml
```

如果需要使用后台的代码生成或模板相关功能，还应将仓库中的 `server/resource` 目录部署为 `/opt/tb-live/server/resource`。普通环境仍然共用同一个二进制文件。

## 6. 手动验证指定配置文件

后端通过 `-c` 参数选择配置文件，且该参数的优先级最高。

例如，用测试服配置启动同一个二进制：

```bash
cd /opt/tb-live/server
sudo -u tb-live /opt/tb-live/bin/tb-live-server \
  -c /etc/tb-live/config.testing.yaml
```

启动日志中应出现实际加载的配置路径。完成手动验证后按 `Ctrl+C` 停止，再配置 systemd 常驻运行。

## 7. 使用 systemd 启动多个环境

创建模板服务 `/etc/systemd/system/tb-live@.service`：

```ini
[Unit]
Description=TB Live backend (%i)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=tb-live
Group=tb-live
WorkingDirectory=/opt/tb-live/server
Environment=GIN_MODE=release
ExecStart=/opt/tb-live/bin/tb-live-server -c /etc/tb-live/config.%i.yaml
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

这里的 `%i` 是服务实例名：

| systemd 服务 | 实际加载的配置文件 |
|---|---|
| `tb-live@production` | `/etc/tb-live/config.production.yaml` |
| `tb-live@testing` | `/etc/tb-live/config.testing.yaml` |
| `tb-live@staging` | `/etc/tb-live/config.staging.yaml` |

加载 systemd 配置：

```bash
sudo systemctl daemon-reload
```

按需启动并设置开机自启：

```bash
sudo systemctl enable --now tb-live@production
sudo systemctl enable --now tb-live@testing
sudo systemctl enable --now tb-live@staging
```

不需要哪个环境，就不要启动对应实例。

查看状态和日志：

```bash
sudo systemctl status tb-live@production --no-pager
sudo journalctl -u tb-live@production -n 100 --no-pager

sudo systemctl status tb-live@testing --no-pager
sudo journalctl -u tb-live@testing -n 100 --no-pager
```

虽然测试服使用测试配置，但 `GIN_MODE=release` 仍然是合适的服务器运行模式；具体环境参数由 `-c` 指定的 YAML 文件决定。

## 8. 验证不同环境

以下端口仅对应前文示例：

```bash
curl --fail http://127.0.0.1:8888/api/health
curl --fail http://127.0.0.1:8889/api/health
curl --fail http://127.0.0.1:8890/api/health
```

如果配置中的 `system.router-prefix` 为空，健康检查地址应改为 `/health`。

还应确认：

- 每个 systemd 实例都加载了正确的配置文件。
- 每个实例监听不同端口，没有端口冲突。
- 数据库、Redis、SRS、日志和上传目录均指向对应环境。
- 后端端口只允许本机网关或受信任网络访问。

## 9. 更新公共二进制

发布新版本时，本地仍然只构建一个二进制。上传并校验后，先写入临时文件，再原子替换公共文件：

```bash
sudo install -m 0755 -o root -g root \
  "/tmp/tb-live-server-$RELEASE_ID-linux-$TARGET_ARCH" \
  /opt/tb-live/bin/tb-live-server.new

sudo mv -f \
  /opt/tb-live/bin/tb-live-server.new \
  /opt/tb-live/bin/tb-live-server
```

然后重启需要切换到新版本的实例：

```bash
sudo systemctl restart tb-live@production
sudo systemctl restart tb-live@testing
sudo systemctl restart tb-live@staging
```

Linux 进程启动后会继续使用内存中已加载的旧程序，因此替换文件后，必须重启对应实例才能使用新版本。

配置文件发生变化后也建议重启对应服务：

```bash
sudo systemctl restart tb-live@production
```

## 10. 常用命令

```bash
# 正式服
sudo systemctl start tb-live@production
sudo systemctl stop tb-live@production
sudo systemctl restart tb-live@production
sudo systemctl status tb-live@production --no-pager

# 测试服
sudo systemctl restart tb-live@testing
sudo journalctl -u tb-live@testing -f

# 确认所有实例都使用同一个二进制路径
systemctl show tb-live@production -p ExecStart
systemctl show tb-live@testing -p ExecStart
```

最终只需要维护：

- 一个公共后端二进制：`/opt/tb-live/bin/tb-live-server`
- 多个外部配置文件：`/etc/tb-live/config.<环境>.yaml`
- 一个 systemd 模板服务：`tb-live@.service`
