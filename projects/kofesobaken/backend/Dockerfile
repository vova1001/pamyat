# Сборка API. REGISTRY — откуда брать базовые образы (по умолчанию зеркало Docker Hub).
ARG REGISTRY=mirror.gcr.io/library
FROM ${REGISTRY}/golang:1.24-alpine AS build
ENV CGO_ENABLED=0
WORKDIR /src
# Необязательные доп. сертификаты (корпоративный прокси): положите *.crt в backend/certs/.
COPY certs/ /tmp/certs/
RUN cat /tmp/certs/*.crt >> /etc/ssl/certs/ca-certificates.crt 2>/dev/null || true
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM ${REGISTRY}/alpine:3.20
RUN adduser -D -u 10001 app && mkdir -p /data/uploads && chown app /data/uploads
COPY --from=build /out/server /usr/local/bin/server
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["server"]
