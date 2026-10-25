http://140.210.12.162:8888/api/swagger/index.html

### 本地打包服务器

#### 打包

./deploy/build-prod.sh

#### 部署

解压正式发布包：
tar -xzvf tb-live-production-linux-amd64.tar.gz

进入解压后的目录：
cd tb-live-production-linux-amd64

启动或重启正式服务：
chmod +x tb-live-server restart-prod.sh
./restart-prod.sh

查看运行日志：
tail -f log/production/server-console.log


### 本地调试启动

cd server && GIN_MODE=debug go run . -c config.dev.yaml



ffmpeg \
-re \
-stream_loop -1 \
-i "./2.mp4" \
-c copy \
-f flv \
""