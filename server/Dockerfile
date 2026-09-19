FROM golang:alpine as builder

WORKDIR /go/src/tb_live_module
COPY . .

RUN go env -w GO111MODULE=on \
    && go env -w GOPROXY=https://goproxy.cn,direct \
    && go env -w CGO_ENABLED=0 \
    && go env \
    && go mod tidy \
    && go build -o server .

FROM alpine:latest

LABEL MAINTAINER="SliverHorn@sliver_horn@qq.com"
# 设置时区
ENV TZ=Asia/Shanghai
RUN apk update && apk add --no-cache tzdata openntpd \
    && ln -sf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

WORKDIR /go/src/tb_live_module

COPY --from=0 /go/src/tb_live_module/server ./
COPY --from=0 /go/src/tb_live_module/resource ./resource/
COPY --from=0 /go/src/tb_live_module/config.docker.yaml ./

# 挂载目录：如果使用了sqlite数据库，容器命令示例：docker run -d -v /宿主机路径/gva.db:/go/src/tb_live_module/gva.db -p 8888:8888 --name gva-server-v1 gva-server:1.0
# VOLUME ["/go/src/tb_live_module"]

EXPOSE 8888
ENTRYPOINT ./server -c config.docker.yaml
