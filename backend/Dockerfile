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
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agp-server ./cmd/server

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /out/agp-server /app/agp-server
COPY backend/migrations /app/migrations

ENV AGP_ADDR=:8080
ENV AGP_MIGRATIONS_DIR=/app/migrations

EXPOSE 8080
CMD ["/app/agp-server"]
