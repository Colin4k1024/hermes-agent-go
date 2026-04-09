FROM golang:1.22-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /hermes ./cmd/hermes/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    curl \
    ripgrep \
    jq \
    python3 \
    python3-pip \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash hermes

COPY --from=builder /hermes /usr/local/bin/hermes

USER hermes
WORKDIR /home/hermes

RUN mkdir -p /home/hermes/.hermes/{sessions,logs,memories,skills,cron,cache}

ENTRYPOINT ["hermes"]
CMD ["gateway", "start"]
