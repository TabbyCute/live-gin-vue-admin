http://140.210.12.162:8888/api/swagger/index.html

http://140.210.12.162:8888/app/v1/assets/admin

http://140.210.12.162:8888/app/v1/assets/h5/

https://www.itouxiang.com/touxiang/89727144.html

### 本地打包服务器

#### 打包

./deploy/build-prod.sh

正式包会一并构建后台和 H5 客户端，包含 `assets/admin` 和 `assets/h5`。
H5 源码默认位于 `$HOME/Desktop/IM/H5Player`，换电脑时可指定：

```bash
H5_SOURCE_DIR=/你的路径/H5Player ./deploy/build-prod.sh
```

只打包 H5 到 `server/assets/h5`：

```bash
bash ./deploy/build-h5.sh
```

H5 使用生产构建，部署前缀为 `/app/v1/assets/h5/`，API 为同源 `/api`。
支持首页及 history 子路由直接访问、刷新；访问入口和部署前缀不受服务器目录变化影响。

#### 部署

打包完成后，`release` 目录会生成同名的发布包和部署脚本，例如：

```text
tb-live-production-linux-2026102601.tar
tb-live-production-linux-2026102601.sh
```

将这两个文件上传到服务器的同一个目录。无需固定服务器目录，也无需手动解压，直接执行：

```bash
chmod +x ./tb-live-production-linux-2026102601.sh && ./tb-live-production-linux-2026102601.sh
```

部署脚本会先校验并解压同名发布包，然后停止全部旧的 `tb-live-server` 进程并启动新版本。每天的打包序号从 `01` 开始递增。

查看运行日志：

```bash
tail -f ./tb-live-production-linux-2026102601/log/production/server-console.log
```


### 本地调试启动

cd server && GIN_MODE=debug go run . -c config.dev.yaml



ffmpeg \
-re \
-stream_loop -1 \
-i "./2.mp4" \
-c copy \
-f flv \
""
