# syntax=docker/dockerfile:1
# Build context: repository root  (docker build -f docker/api/Dockerfile .)

FROM buildpack-deps:bookworm AS isolate
RUN apt-get update && \
    apt-get install -y --no-install-recommends git libcap-dev && \
    rm -rf /var/lib/apt/lists/* && \
    git clone https://github.com/envicutor/isolate.git /tmp/isolate/ && \
    cd /tmp/isolate && \
    git checkout af6db68042c3aa0ded80787fbb78bc0846ea2114 && \
    make -j"$(nproc)" install && \
    rm -rf /tmp/*

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download || true
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sendbox-api ./cmd/sendbox-api

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends libxml2 gnupg tar coreutils util-linux libc6-dev \
    binutils build-essential locales libpcre3-dev libevent-dev libgmp3-dev \
    libncurses6 libedit-dev libseccomp-dev rename procps python3 \
    libreadline-dev libblas-dev liblapack-dev libarpack2-dev \
    libfftw3-dev libglpk-dev libqhull-dev libqrupdate-dev libsuitesparse-dev \
    libsundials-dev libpcre2-dev libcap-dev ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*
RUN useradd -M cfm && mkdir -p /cfm/packages
COPY --from=isolate /usr/local/bin/isolate /usr/local/bin
COPY --from=isolate /usr/local/etc/isolate /usr/local/etc/isolate

RUN sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && locale-gen

WORKDIR /cfm_api
COPY --from=build /out/sendbox-api ./sendbox-api
COPY docker/api/docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./docker-entrypoint.sh ./sendbox-api

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD curl -fs http://127.0.0.1:2000/healthz || exit 1

CMD ["/cfm_api/docker-entrypoint.sh"]
EXPOSE 2000/tcp
