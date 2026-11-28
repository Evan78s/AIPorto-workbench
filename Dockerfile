# 多阶段构建：编译产物很小，最终镜像不包含 Go 工具链和源码。
FROM golang:1.22-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 构建时注入 git commit 作为版本号；无 .git（如离线包）时回退 dev。
RUN GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
 && echo "building version ${GIT_COMMIT}" \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-X aiporto/internal/version.Commit=${GIT_COMMIT}" -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -trimpath -o /out/seed ./cmd/seed

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S app && adduser -S -G app app
COPY --from=build /out/gateway /usr/local/bin/gateway
COPY --from=build /out/seed /usr/local/bin/seed
USER app
WORKDIR /app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["gateway"]
