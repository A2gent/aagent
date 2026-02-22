# syntax=docker/dockerfile:1.7

FROM golang:1.24-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/a2 ./cmd/aagent

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    git \
    ripgrep \
    tini \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --system --gid 10001 aagent \
    && useradd --system --uid 10001 --gid aagent --create-home --home-dir /home/aagent aagent \
    && mkdir -p /workspace /data /data/home/.config/aagent \
    && chown -R aagent:aagent /workspace /data /home/aagent

COPY --from=builder /out/a2 /usr/local/bin/a2

ENV HOME=/data/home \
    AAGENT_DATA_PATH=/data

WORKDIR /workspace
USER aagent

EXPOSE 8080

ENTRYPOINT ["/usr/bin/tini", "--", "a2"]
CMD []
