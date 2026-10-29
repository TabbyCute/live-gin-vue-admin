安装docker https://cloud.tencent.com/document/product/213/46000
设置代理 https://cloud.tencent.com/document/product/457/51237

固定openim v3.8.3-patch.1 版本

/data/openim/openim-docker创建目录

sudo chown -R ubuntu:ubuntu /data/openim/openim-docker

到/data/openim下面执行：

git clone https://github.com/openimsdk/openim-docker.git && \
cd openim-docker && \
git fetch --tags && \
git checkout --detach v3.8.3-patch.1 && \
echo "Using OpenIM stable release: $(git describe --tags --exact-match)"


完了后会出现：

HEAD is now at xxxxxxx ...
Using OpenIM stable release: v3.8.3-patch.1



查看数据目录配置
grep '^DATA_DIR=' .env
如果 DATA_DIR=./，建议继续保留这个相对路径，方便以后整体迁移。

检查 Docker Compose 配置
docker compose config --quiet

查看实际使用的镜像版本
docker compose config --images



修改配置：

# 数据持久化（保持原值）
DATA_DIR=./

# MongoDB
MONGO_USERNAME=openIM
MONGO_PASSWORD=Tabby404ABC.

# Redis
REDIS_PASSWORD=Tabby404ABC.

# MinIO
MINIO_EXTERNAL_ADDRESS="https://im-file.example.com"
MINIO_INTERNAL_ADDRESS="minio:9000"
MINIO_ACCESS_KEY_ID=替换为MinIO账号
MINIO_SECRET_ACCESS_KEY=替换为MinIO强密码

# OpenIM
OPENIM_SECRET=TabbyLive404ABC
OPENIM_API_PORT=10002
OPENIM_MSG_GATEWAY_PORT=10001

# Chat
CHAT_API_PORT=10008
ADMIN_API_PORT=10009

# 日志
LOG_IS_STDOUT=true
LOG_LEVEL=3




其他启动啥的查看下面的：
https://docs.openim.io/zh/docs/guides/deployment/docker-compose



你目前是单机 Docker Compose 部署，Kafka 和 etcd 暂时可以不启用认证，但必须做好网络隔离。无所谓
WARN[0000] The "KAFKA_USERNAME" variable is not set. Defaulting to a blank string.
WARN[0000] The "KAFKA_PASSWORD" variable is not set. Defaulting to a blank string.
WARN[0000] The "ETCD_USERNAME" variable is not set. Defaulting to a blank string.
WARN[0000] The "ETCD_PASSWORD" variable is not set. Defaulting to a blank string.
WARN[0000] The "ETCD_USERNAME" variable is not set. Defaulting to a blank string.
WARN[0000] The "ETCD_PASSWORD" variable is not set. Defaulting to a blank string.

http://139.155.158.139/

curl --request POST "http://139.155.158.139:10002/auth/get_admin_token" \
--header "Content-Type: application/json; charset=utf-8" \
--header "operationID: a333444dawda222" \
--data-raw '{
"secret": "TabbyLive404ABC",
"userID": "imAdmin"
}'