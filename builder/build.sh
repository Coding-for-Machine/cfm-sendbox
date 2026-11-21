#!/usr/bin/env bash

# Build a container using the spec file provided

START_DIR=$PWD
cd "$(dirname "$BASH_SOURCE[0]}")"

help_msg(){
    echo "Usage: $0 [specfile] [tag]"
    echo
    echo "$1"

    exit 1
}

cleanup(){
    echo "Exiting..."
    docker stop builder_cfm_instance && docker rm builder_cfm_instance
}

fetch_packages(){
    local port=$((5535 + $RANDOM % 60000))
    mkdir build
    # Start a cfm container
    docker run \
        --privileged \
        -v "$PWD/build":'/cfm/packages' \
        -dit \
        -p $port:2000 \
        --name builder_cfm_instance \
        ghcr.io/coding-for-machine/cfm-sendbox:latest

    # Ensure the Go CLI is built
    (cd .. && CGO_ENABLED=0 go build -trimpath -o bin/cfm ./cmd/sendbox) || exit 1

    # Evalulate the specfile
    ../bin/cfm -u "http://127.0.0.1:$port" ppman spec $1
}

build_container(){
    docker build -t $1 -f "Dockerfile" "$START_DIR/build"
}


SPEC_FILE=$START_DIR/$1
TAG=$2

[ -z "$1" ] && help_msg "specfile is required"
[ -z "$TAG" ] && help_msg "tag is required"

[ -f "$SPEC_FILE" ] || help_msg "specfile does not exist"

which go || help_msg "go is required"
which docker || help_msg "docker is required"

trap cleanup EXIT

fetch_packages $SPEC_FILE
build_container $TAG

echo "Start your custom cfm container with"
echo "$ docker run --privileged -dit -p 2000:2000 $TAG"
