# 构造可执行文件
FROM --platform=$TARGETPLATFORM golang:1.17-alpine as build_bin
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.io
# 停用cgo
ENV CGO_ENABLED=0
WORKDIR /code
COPY go.mod /code/go.mod
COPY go.sum /code/go.sum
# 添加源文件
COPY main.go /code/thompsonsampling_orderrpc.go
RUN go build -ldflags "-s -w" -o thompsonsampling_orderrpc-go thompsonsampling_orderrpc.go

# 使用upx压缩可执行文件
FROM --platform=$TARGETPLATFORM alpine:3.11 as upx
WORKDIR /code
# 安装upx
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
RUN apk update && apk add --no-cache upx && rm -rf /var/cache/apk/*
COPY --from=build_bin /code/thompsonsampling_orderrpc-go .
RUN upx --best --lzma -o thompsonsampling_orderrpc thompsonsampling_orderrpc-go

# 编译获得grpc-health-probe
FROM golang:1.17.0-buster as build_grpc-health-probe
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.io
# 停用cgo
ENV CGO_ENABLED=0
# 安装grpc-health-probe
RUN go install github.com/grpc-ecosystem/grpc-health-probe@latest


# 使用压缩过的可执行文件构造镜像
FROM --platform=$TARGETPLATFORM alpine:3.14.1 as build_upximg
# 安装curl
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
RUN apk update && apk add --no-cache curl && rm -rf /var/cache/apk/*
# 打包镜像
COPY --from=build_grpc-health-probe /go/bin/grpc-health-probe .
RUN chmod +x /grpc-health-probe
COPY --from=upx /code/thompsonsampling_orderrpc .
RUN chmod +x /thompsonsampling_orderrpc
EXPOSE 5000
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 CMD [ "/grpc-health-probe","-addr=:5000" ]
ENTRYPOINT [ "/thompsonsampling_orderrpc"]