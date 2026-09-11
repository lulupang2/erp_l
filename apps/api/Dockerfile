FROM golang:1.26.7-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/erp-api ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/erp-migrate ./cmd/migrate

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home /nonexistent --shell /usr/sbin/nologin erp
WORKDIR /app
COPY --from=build /out/erp-api /app/erp-api
COPY --from=build /out/erp-migrate /app/erp-migrate
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=15s --retries=5 \
  CMD curl --fail --silent --show-error http://127.0.0.1:8080/api/v1/health >/dev/null || exit 1
ENTRYPOINT ["/app/erp-api"]

