# 独立本地 SRS 媒体烟测

2026-11-12 本机已实际验证 RTMP 推流 → SRS 实时快照 → HTTP-FLV 拉流 → 后端 Client 踢流 → 发布者消失 → 重复停止幂等。

## 隔离边界

- 使用已缓存的 `ossrs/srs:7.0.165`（本机镜像短 ID `829863163c19`），脚本不会自动拉取镜像。
- 每次新建独立 `tb-live-srs-smoke-<PID>` 容器与专用 bridge 网络，只将随机 host 端口映射到 `127.0.0.1`。
- 只挂载新建测试配置 `server/tests/srs/local.conf`，不挂载、不读取现有运行容器的配置；`http_hooks.enabled off`，无任何回调目标。没有 SSH、远程接口请求或远程媒体测试。
- 已有 `tb-live-srs` 及其他数据库/Redis/媒体容器均不在脚本控制范围。EXIT trap 只删除本次创建的专名容器/网络。
- 测试仅接受无凭证、无 query/fragment/path 的 `http/rtmp://127.0.0.1:<port>` 基地址；先确认 SRS 没有已有发布者，再生成独一测试流名。
- 本机 Docker 的 `--internal` 网络不提供 published host port，因此最终使用专用普通 bridge；依然没有外网回调或远程测试请求。这里不承诺容器网络层出站阻断。

## 实际步骤与结果

`server/internal/srs/client_media_integration_test.go` 的 `TestLocalMediaSmoke`：

1. 通过真实 `internal/srs.Client.CurrentRuntime` 等待新 SRS 就绪，验证初始发布列表为空。
2. 运行系统 ffmpeg，lavfi 生成 320×180/15fps 测试视频和 44100Hz 音频，编码为 H264/AAC，通过真实 RTMP 推入独立 SRS。
3. 通过 `ListPublishersWithRuntime` 读取实际流 ID、vhost、尺寸与活动状态。
4. 用系统 ffprobe 从真实 HTTP-FLV 地址读取音视频流信息，验证 H264 320×180 与 AAC。
5. 用 `Client.StopPublisher` 通过 SRS HTTP API 断开该发布客户端，轮询确认发布者消失，并确认 ffmpeg 的 RTMP 连接失败退出。
6. 对同一 StreamRef 再次调用 StopPublisher，得到幂等成功。测试结束自动停止 ffmpeg 并清理临时 Docker 资源。

本机执行结果：

```text
=== RUN   TestLocalMediaSmoke
actual RTMP publish -> SRS 320x180 H264/AAC snapshot -> HTTP-FLV probe -> client kick -> absent publisher -> idempotent repeat passed
--- PASS: TestLocalMediaSmoke (1.21s)
PASS
ok  tb_live_module/internal/srs  1.886s
```

测试配置的 HTTP-FLV mount 使用 `/[app]/[stream].flv`。首次省略前导斜杠时真实拉流返回 404，已在独立测试配置修正；没有修改现有 SRS 配置。

## 复现

需要本机 Docker、Go、ffmpeg、ffprobe 与上述已缓存镜像。在仓库根执行：

```sh
bash server/tests/srs/run-local.sh
```

正常 `go test ./internal/srs` 不会启动 Docker 或媒体进程：该测试在缺少 `SRS_LOCAL_SMOKE=1` 时跳过。推荐仅经上述脚本执行，不手工指向已有 SRS。

现有同包的 httptest 单测另覆盖 HTTP API 分页、运行时代际变化、模糊/冲突发布者身份、SRS 7 JSON 不存在码、幂等断开和超时等控制逻辑；本次烟测补上真实媒体链路证据。它不覆盖业务鉴权 hooks、H5、WebRTC/HLS/SRT、真实主播网络或生产容量，不代表远程部署已验收。
