# ============================================================
# mc-starter-server Dockerfile
# 多阶段构建：编译阶段 + 运行阶段
# ============================================================

# ---- 编译阶段 ----
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false \
    -ldflags="-s -w" \
    -o /mc-starter-server \
    ./cmd/mc-starter-server/

# ---- 运行阶段 ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# 运行时非 root 用户
RUN addgroup -S app && adduser -S -G app app

WORKDIR /app

# 只复制二进制
COPY --from=builder /mc-starter-server .
# 复制示例配置文件
COPY --from=builder /src/configs/server.example.yml ./configs/

# 运行时数据卷
VOLUME ["/app/data", "/app/packs"]

EXPOSE 8443

USER app

ENTRYPOINT ["/app/mc-starter-server"]
CMD ["start", "--config", "/app/server.yml"]
