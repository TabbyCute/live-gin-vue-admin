# 本地 SRS

此目录保存 SRS 配置和 Docker Compose 文件，镜像固定为 `ossrs/srs:7.0.165`。

运行时生成的 HLS 文件统一映射到：

```text
release/srs/data/
```

SRS 使用控制台日志，Docker 日志采用带大小限制的 `local` 驱动；执行 `docker compose down` 删除容器时，对应容器日志也会清理。

## 启动

先确保本机 Go 后端正在监听 `8888`，然后执行：

```bash
cd /path/to/live-gin-vue-admin/srs
docker compose pull
SRS_HOOK_TOKEN='请替换为高熵随机值' docker compose up -d
```

检查状态：

```bash
docker compose ps
docker compose logs --tail=100
curl --fail http://127.0.0.1:12985/api/v1/versions
```

SRS 容器使用以下地址回调 macOS 主机上的 Go 后端：

```text
http://host.docker.internal:8888/api/v1/app/live/hook/publish
http://host.docker.internal:8888/api/v1/app/live/hook/unpublish
```

回调URL会附带 `hook_token`。Go 后端优先读取同名 `SRS_HOOK_TOKEN` 环境变量；两端必须一致。
本地未设置时 Compose 仅使用便于联调的默认值，不能用于生产。

后端还会读取 SRS HTTP API 每个响应中的 `server` 标识。该标识变化代表 SRS 已重启，
此时旧连接身份全部失效，合法主播可在新实例第一次 `on_publish` 时立即受控接管，无需等待三轮缺失对账。

容器内不能使用 `127.0.0.1:8888` 回调主机，因为容器内的 `127.0.0.1` 指向 SRS 容器自身。

## 端口

| 本机地址 | 用途 |
|---|---|
| `127.0.0.1:12935` | RTMP 推流和播放，映射到容器 `1935` |
| `127.0.0.1:12985` | SRS HTTP API，映射到容器 `1985` |
| `127.0.0.1:18080` | HTTP-FLV 和 HLS |
| `127.0.0.1:8000/udp` | WebRTC 媒体传输 |

本地 Go 后端的 SRS 配置应使用宿主机映射后的端口：

```yaml
live:
  srs:
    app: "TB_LIVE"
    http_apis: "http://127.0.0.1:12985"
    push-base-url: "rtmp://127.0.0.1:12935"
    play-base-url: "rtmp://127.0.0.1:12935"
```

默认 `CANDIDATE` 为 `127.0.0.1`，适合在当前 Mac 上联调。如果需要从局域网其他设备使用 WebRTC，应设置 Mac 的局域网 IP，并相应调整 Compose 的端口绑定：

```bash
CANDIDATE=192.168.1.10 docker compose up -d
```

## 停止与清理

```bash
docker compose down --remove-orphans
```

停止后可以直接删除 `release/srs/` 中的运行数据。不再使用镜像时执行：

```bash
docker image rm ossrs/srs:7.0.165
```
