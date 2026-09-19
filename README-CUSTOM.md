# TB Live Module 环境初始化与启动手册

本项目将正式环境和测试环境的后端配置完全分开。Makefile 启动命令会通过 `-c` 明确指定配置文件；直接启动后端时，程序也会根据 Gin 模式自动选择对应文件。

## 1. 环境映射

| 环境 | 后端配置 | MySQL 数据库 | Redis DB | Gin 模式 | 日志目录 | 本地上传目录 |
| --- | --- | --- | ---: | --- | --- | --- |
| 测试 | `server/config.dev.yaml` | `live_dev` | `1` | `debug` | `server/log/test` | `server/uploads/test` |
| 正式 | `server/config.prod.yaml` | `live_pro` | `0` | `release` | `server/log/production` | `server/uploads/production` |

两个环境的 JWT `signing-key` 必须不同，不能共用数据库、Redis DB、日志目录或上传目录。

`config.dev.yaml` 与 `config.prod.yaml` 使用同一份配置模板，字段、顺序和注释保持一致，只允许环境相关的配置值不同。以后新增或删除配置项时，必须同时修改这两个文件。

当前两套后端都默认监听 `8888`，适用于分别部署或不同时启动。如果要在同一台机器同时运行，必须给其中一套修改 `system.addr`，并同步修改前端代理或 Nginx 上游端口。

后端的自动选择规则如下：

- `GIN_MODE=debug` 或 `GIN_MODE=test`：加载 `config.dev.yaml`。
- `GIN_MODE=release`：加载 `config.prod.yaml`。
- 未设置 `GIN_MODE` 时 Gin 默认为 `debug`，因此加载 `config.dev.yaml`。
- `-c` 参数或 `GVA_CONFIG` 环境变量的优先级高于以上自动规则。

日常使用仍建议执行本文提供的 Makefile 命令，因为命令同时写明了 Gin 模式和配置文件，不容易误启环境。

## 2. 环境要求

- Go `1.24+`
- Node.js `20`
- pnpm
- MySQL 5.7+ / MySQL 8
- Redis（当 `system.use-redis: true` 时必须可连接）

安装前端依赖：

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin/web
pnpm install --frozen-lockfile
```

安装后端依赖：

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin/server
go mod download
```

## 3. 首次初始化的重要原则

仅启动后端会执行 AutoMigrate 建表，不会插入 `admin`、角色、菜单和 API 等基础数据。必须让 `/init/checkdb` 返回 `needInit: true`，然后通过网页初始化向导完成初始化。

初始化前需要把对应配置文件中的 MySQL `db-name` 临时设为空字符串：

```yaml
mysql:
  db-name: ""
```

初始化成功后，程序会自动将页面填写的数据库名和新的 JWT 密钥写回本次启动的配置文件。

## 4. 初始化测试环境

### 4.1 准备数据库

测试环境数据库固定为 `live_dev`。MySQL 账号需要有连接、建库、建表和写数据权限。

如果需要重新初始化，先备份后再删除旧的 `live_dev`；不要对有价值的数据库直接重置。

### 4.2 进入初始化模式

停止后端，将 `server/config.dev.yaml` 中的 `mysql.db-name` 临时设为 `""`，然后在项目根目录启动：

```bash
make run-server-test
```

另开一个终端验证：

```bash
curl -X POST http://127.0.0.1:8888/init/checkdb
```

必须返回：

```json
{"code":0,"data":{"needInit":true},"msg":"前往初始化数据库"}
```

### 4.3 启动前端并提交初始化

```bash
make run-web-test
```

打开 `http://localhost:8080`，在初始化页面填写：

- 数据库类型：`mysql`
- 数据库地址、端口、账号、密码：测试环境 MySQL 信息
- 数据库名：`live_dev`
- 管理员密码：自行设置至少 6 位的密码

后端终端必须出现类似：

```text
[mysql] --> sys_users 初始数据成功!
[mysql] --> 初始数据成功!
```

完成后，`server/config.dev.yaml` 中的 `db-name` 应自动变回 `live_dev`。

## 5. 初始化正式环境

正式环境数据库固定为 `live_pro`。初始化时不要同时运行测试后端，因为两者默认都使用 `8888` 端口。

1. 停止后端并备份相关数据。
2. 将 `server/config.prod.yaml` 中的 `mysql.db-name` 临时设为 `""`。
3. 启动正式后端：

```bash
make run-server-production
```

4. 验证必须返回 `needInit: true`：

```bash
curl -X POST http://127.0.0.1:8888/init/checkdb
```

5. 首次初始化期间，可以在受信任的内网或本机临时启动前端：

```bash
make run-web-test
```

6. 打开 `http://localhost:8080`，填写正式 MySQL 信息，数据库名必须填 `live_pro`，并设置强管理员密码。
7. 初始化成功后立即停止临时前端和后端，确认 `server/config.prod.yaml` 中已写回 `db-name: live_pro`。
8. 重新按正式环境方式启动。

正式配置保持 `disable-auto-migrate: true`。首次向导会自行建表和初始化数据；后续发版不会在启动时自动修改正式表结构，表结构变更应使用可审计的迁移脚本。

## 6. 日常启动测试环境

终端 1：

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin
make run-server-test
```

终端 2：

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin
make run-web-test
```

访问：

- 前端：`http://localhost:8080`
- 后端健康检查：`http://localhost:8888/health`
- Swagger：`http://localhost:8888/swagger/index.html`

## 7. 日常启动正式环境

### 7.1 直接运行（本机验证）

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin
make run-server-production
```

### 7.2 构建后端二进制

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin/server
mkdir -p bin
go build -o bin/tb-live-server .
GIN_MODE=release ./bin/tb-live-server -c config.prod.yaml
```

在 systemd、Supervisor 或容器中启动时，工作目录必须是 `server` 目录，或给 `-c` 传入绝对路径。

### 7.3 构建正式前端

首先在 `web/.env.production` 中将 `VITE_BASE_PATH` 替换为正式域名，然后执行：

```bash
cd /Users/tabby/codes/go/live-gin-vue-admin
make build-web-production
```

构建结果在 `web/dist`，建议由 Nginx 托管。基本配置示例：

```nginx
server {
    listen 80;
    server_name your-domain.example.com;
    root /path/to/live-gin-vue-admin/web/dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        rewrite ^/api/(.*)$ /$1 break;
        proxy_pass http://127.0.0.1:8888;
    }
}
```

真实上线时还应在 Nginx 或负载均衡层配置 HTTPS。

## 8. 启动后验证

健康检查：

```bash
curl http://127.0.0.1:8888/health
```

已完成初始化的环境应返回 `needInit: false`：

```bash
curl -X POST http://127.0.0.1:8888/init/checkdb
```

数据库验证：

```sql
SELECT id, username, authority_id, enable
FROM sys_users;

SELECT *
FROM sys_user_authority;
```

正常情况下，`sys_users` 至少包含 `admin` 和初始化示例用户。`admin` 的密码是初始化页面中填写的管理员密码，不应假设始终是 `123456`。

## 9. 重新初始化

1. 停止对应环境后端。
2. 备份数据库。
3. 删除并重建对应的 `live_dev` 或 `live_pro`。
4. 将对应配置文件的 `mysql.db-name` 设为 `""`。
5. 按本文档的测试或正式环境初始化流程重新执行。

仅删库后重启不等于完整初始化：如果 `db-name` 仍然非空，系统只会建表，不会插入默认用户和权限数据。

## 10. 常见问题

### 前端反复显示初始化页面

检查启动命令是否传入了正确的 `-c` 配置文件，并检查对应配置中的 `mysql.db-name`。

### 数据库只有表，`sys_users` 没有数据

说明只执行了 AutoMigrate，没有成功调用 `/init/initdb`。清空对应配置的 `db-name`，确认 `needInit: true` 后重新走初始化向导。

### `created_at` 出现 `[]uint8` 无法转换到 `time.Time`

确认 MySQL 配置包含：

```yaml
config: "charset=utf8mb4&parseTime=True&loc=Local"
```

### Redis 连接失败导致后端不能启动

当 `system.use-redis: true` 时，后端启动会立即 Ping Redis。确认对应环境的 Redis 地址和密码正确；如果不需要 Redis，将 `use-redis` 设为 `false`。

## 11. 安全与发布

- 不要把真实数据库、Redis、邮件或 OSS 密钥提交到公开仓库。
- 如果密钥已经提交或分享，应立即轮换，仅从 Git 删除是不够的。
- 正式环境建议通过 `GVA_CONFIG=/absolute/path/config.prod.yaml` 指向仓库外部的私密配置文件。
- 正式环境使用 `GIN_MODE=release`，并保持 `disable-auto-migrate: true`。
- 正式数据库和上传目录必须配置定期备份。

`server/Dockerfile` 默认仍使用 `config.docker.yaml`，不属于本文档的两套本地/二进制启动流程。如果改用 Docker 部署，应将对应环境配置作为容器 Secret 或只读文件挂载，并用 `-c` 指向挂载路径。
