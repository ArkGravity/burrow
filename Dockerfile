FROM oven/bun:1.4.2 AS frontend
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.27.1-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY configs/ configs/
COPY internal/ internal/
COPY web/*.go web/
COPY --from=frontend /src/web/dist web/dist
RUN CGO_ENABLED=1 go build -tags embedweb -trimpath -ldflags="-s -w" -o /burrow ./cmd/burrow

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /burrow /usr/local/bin/burrow
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=20s --timeout=5s --start-period=20s CMD ["burrow", "healthcheck"]
ENTRYPOINT ["burrow"]
CMD ["serve"]
