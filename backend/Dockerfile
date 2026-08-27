FROM golang:1.25-bookworm AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ARG GOPRIVATE=
ARG GONOSUMDB=
ARG GONOPROXY=

ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB} \
    GOPRIVATE=${GOPRIVATE} \
    GONOSUMDB=${GONOSUMDB} \
    GONOPROXY=${GONOPROXY}

WORKDIR /src
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agp-server ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/migrate-json ./cmd/migrate-json \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/migrate-resource-files ./cmd/migrate-resource-files

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/agp-server /app/agp-server
COPY --from=builder /out/migrate-json /app/migrate-json
COPY --from=builder /out/migrate-resource-files /app/migrate-resource-files
COPY backend/migrations /app/migrations

ENV AGP_ADDR=:8080
ENV AGP_MIGRATIONS_DIR=/app/migrations

EXPOSE 8080
CMD ["/app/agp-server"]
