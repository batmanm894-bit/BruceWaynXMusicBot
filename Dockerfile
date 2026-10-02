FROM golang:1.26.4-bookworm AS builder

WORKDIR /build

# hadolint ignore=DL3015
RUN apt-get update && \
    apt-get install -y \
        unzip \
        curl \
        zlib1g-dev && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
COPY install.sh ./
COPY . .

# Crash debugging: symbols are kept (no "-w -s") so a crash address like
# 0x201d51a can be resolved. Set build arg CGOCHECK2=1 to also enable the
# strict cgo pointer checker (slower; turn it off again once the crash is
# found).
ARG CGOCHECK2=0
RUN go mod tidy && \
    chmod +x install.sh && \
    ./install.sh -n && \
    if [ "$CGOCHECK2" = "1" ]; then export GOEXPERIMENT=cgocheck2; fi && \
    CGO_ENABLED=1 go build -v -trimpath -o app ./cmd/app/


FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        curl \
        unzip && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /etc/ssl/certs /etc/ssl/certs

RUN curl -fL \
      https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
      -o /usr/local/bin/yt-dlp && \
    chmod 0755 /usr/local/bin/yt-dlp && \
    curl -fsSL https://deno.land/install.sh -o /tmp/deno-install.sh && \
    sh /tmp/deno-install.sh && \
    rm -f /tmp/deno-install.sh

# Full goroutine dump + abort on fatal errors, easier to debug native crashes.
ENV GOTRACEBACK=crash

ENV DENO_INSTALL=/root/.deno
ENV PATH=$DENO_INSTALL/bin:$PATH

RUN useradd -r -u 10001 appuser && \
    mkdir -p /app && \
    chown -R appuser:appuser /app

WORKDIR /app

COPY --from=builder /build/app /app/app
COPY --from=builder /build/internal/modules/*.jpg /app/internal/modules/
RUN chown -R appuser:appuser /app

USER appuser

ENTRYPOINT ["/app/app"]
