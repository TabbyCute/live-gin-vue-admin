# 公开主页本地隔离回归

从仓库根目录运行 `bash server/tests/anchorhome/run.sh`，需要本地 Docker 和仓库声明的 Go 工具链。脚本不读取生产配置，不连接 SSH，不调用支付服务。

仅创建 `anchor-home-tests` Compose 项目，MySQL 8.0.46 只绑定 `127.0.0.1:13339`，数据在 tmpfs 中。测试会重建固定的 `anchor_home_test_*` 库；每个测试在连接前校验地址、端口、库名。端口已被其他服务占用时应停止并检查，不能改成远程地址。

包含公开权限/净贡献/守护/性能与查询计划、相册净化/并发/幂等、真实 HTTP/媒体授权、限流、显式迁移及真实 SQL 备份还原、日志脱敏、5 秒 JPEG EXIF fuzz。浏览器 harness 必须另行显式启动，普通脚本不会开启常驻 HTTP 服务。

测试后容器保留供排查。清理命令仅针对本项目：

```sh
docker compose -f server/tests/anchorhome/compose.yaml down
```

既有资金业务完整回归见 `server/tests/backend/run.sh`；它使用另外六个专用本地项目。当前交付证据、显式跳过项目、H5/后台验证与生产迁移状态见 `docs/anchor-home/README.md`。
