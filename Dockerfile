FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o qianwen-creator2api main.go

FROM chromedp/headless-shell:latest

RUN set -eux; \
    if [ -f /etc/apt/sources.list ]; then \
      sed -i \
        -e 's|http://deb.debian.org/debian|http://mirrors.tencentyun.com/debian|g' \
        -e 's|http://security.debian.org/debian-security|http://mirrors.tencentyun.com/debian-security|g' \
        /etc/apt/sources.list; \
    fi; \
    if [ -d /etc/apt/sources.list.d ]; then \
      sed -i \
        -e 's|http://deb.debian.org/debian|http://mirrors.tencentyun.com/debian|g' \
        -e 's|http://security.debian.org/debian-security|http://mirrors.tencentyun.com/debian-security|g' \
        /etc/apt/sources.list.d/*.sources /etc/apt/sources.list.d/*.list 2>/dev/null || true; \
    fi; \
    apt-get update && \
    apt-get install -y ca-certificates dumb-init && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r appuser && useradd -r -g appuser appuser

WORKDIR /app

COPY --from=builder /app/qianwen-creator2api .

RUN mkdir -p /app/data && chown -R appuser:appuser /app

USER appuser

ENV HOST=0.0.0.0 \
    PORT=8000 \
    DATA_DIR=/app/data \
    DATABASE_PATH=/app/data/qianwen-creator-01.sqlite

EXPOSE 8000

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["./qianwen-creator2api"]
