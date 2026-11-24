# syntax=docker/dockerfile:1
# Package builder + index server. Build context: repository root
#   docker build -f repo/Dockerfile -t sendbox-repo-builder .

FROM golang:1.26-bookworm AS serve
WORKDIR /src
COPY go.mod go.sum* ./
COPY cmd/sendbox-repo ./cmd/sendbox-repo
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sendbox-repo ./cmd/sendbox-repo

FROM debian:bookworm-slim
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends unzip autoconf build-essential libssl-dev \
        pkg-config zlib1g-dev libargon2-dev libsodium-dev libcurl4-openssl-dev \
        sqlite3 libsqlite3-dev libonig-dev libxml2 libxml2-dev bc curl git ca-certificates \
        perl xz-utils python3 python3-pip gnupg jq \
        cmake extra-cmake-modules gcc binutils bash coreutils \
        util-linux pciutils usbutils findutils grep libncurses-dev \
        libgmp-dev libmpfr-dev libffi-dev gfortran make \
        libreadline-dev libblas-dev liblapack-dev libpcre2-dev libpcre3-dev libarpack2-dev libfftw3-dev \
        libglpk-dev libqhull-dev libqrupdate-dev libsuitesparse-dev libsundials-dev \
        libbz2-dev liblzma-dev gperf bison flex g++ gawk zip && \
        ln -sf /bin/bash /bin/sh && \
        rm -rf /var/lib/apt/lists/* && \
        update-alternatives --install /usr/bin/python python /usr/bin/python3 2

COPY repo/entrypoint.sh repo/mkindex.sh /
COPY --from=serve /out/sendbox-repo /sendbox-repo

ENTRYPOINT ["bash","/entrypoint.sh"]
CMD ["--no-build"]
