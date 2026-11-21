#!/usr/bin/env bash
# Management script for cfm-sendbox. Everything here is plain bash + Go + Docker.

EXECUTION_PATH="$PWD"
CFM_PATH="$(dirname "$(realpath "$0")")"

cd "$CFM_PATH" || exit 1
CFM_ENV=$(cat .cfm_env 2> /dev/null || echo dev)
CLI="$CFM_PATH/bin/cfm"

docker_compose(){
    if docker compose version > /dev/null 2>&1; then DC="docker compose"; else DC="docker-compose"; fi
    if [ -f "docker-compose.$CFM_ENV.yaml" ]; then
        $DC -f "docker-compose.$CFM_ENV.yaml" "$@"
    else
        $DC "$@"
    fi
}

# Build the Go CLI (bin/cfm) when missing or older than its sources.
ensure_cli(){
    if [ -x "$CLI" ] && [ -z "$(find cmd/sendbox internal/cli go.mod -newer "$CLI" -type f 2>/dev/null | head -1)" ]; then
        return 0
    fi
    mkdir -p bin
    if command -v go > /dev/null 2>&1; then
        CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$CLI" ./cmd/sendbox || exit 1
    else
        docker run --rm -v "$PWD:/src" -w /src -e CGO_ENABLED=0 -u "$(id -u):$(id -g)" \
            -e GOCACHE=/tmp/gocache -e GOPATH=/tmp/gopath golang:1.26-bookworm \
            go build -trimpath -ldflags="-s -w" -o bin/cfm ./cmd/sendbox || exit 1
    fi
}

case $1 in
    help)
        echo "=== cfm-sendbox / cfm Management ==="
        echo "Current Environment: $CFM_ENV"
        echo
        echo "Commands:"
        echo " select <environment>           Select the environment"
        echo " docker_compose <args...>       Interact directly with the docker-compose for the selected environment"
        echo " logs                           Show docker-compose logs"
        echo
        echo " start                          Starts the sandbox API"
        echo " stop                           Stops the sandbox API"
        echo " restart                        Restarts the sandbox API"
        echo " bash                           Opens a bash shell for the api container"
        echo
        echo " update                         Fetches and applies latest updates"
        echo " cli-build                      (Re)builds the Go CLI into bin/cfm"
        echo
        echo " <args..>                       Passthrough to the Go CLI (run, ppman, runtimes, ...)"
        echo
        echo "Development Commands:"

        if [ "$CFM_ENV" == dev ]; then

        echo " clean-pkgs                              Clean any package build artifacts on disk"
        echo " clean-repo                              Remove all packages from local repo"
        echo " list-pkgs                               Lists all packages that can be built"
        echo " build-pkg <package> <version> [builder] Build a package [with desired builder image]"
        echo " rebuild                                 Build and restart the docker container"
        echo " lint                                    gofmt + go vet"
        echo " test                                    go test -race ./..."

        else

        echo " Switch to developement environment for more info"
        echo " > cfm select dev"

        fi
        ;;


    select) echo "$2" > .cfm_env ;;
    docker_compose) shift; docker_compose "$@";;
    logs) docker_compose logs -f ;;

    restart) docker_compose restart ;;
    start)
        docker_compose up -d
        ;;
    stop) docker_compose down ;;
    bash) docker_compose exec api /bin/bash ;;

    rebuild)
        docker_compose build && docker_compose up -d
        ;;

    update)
        git pull
        rm -f "$CLI"; ensure_cli
        docker_compose pull
        docker_compose up -d
        ;;

    cli-build) rm -f "$CLI"; ensure_cli ;;

    clean-pkgs) git clean -fqXd packages ;;
    clean-repo) git clean -fqXd repo ;;

    list-pkgs) find packages -maxdepth 2 | awk -F/ '$2 && $3{ print $2 "-" $3 }' | column ;;

    build-pkg)
        PKGSLUG="$2-$3"
        BUILDER="${4:-sendbox-repo-builder}"
        echo "Building $PKGSLUG"
        echo "Ensuring latest builder image"
        docker build -f repo/Dockerfile -t "$BUILDER" .
        docker run --rm -v "$PWD:/cfm" "$BUILDER" --no-server "$PKGSLUG"
        ;;

    lint)
        go vet ./... && test -z "$(gofmt -l cmd internal)" || { gofmt -l cmd internal; exit 1; }
        ;;
    test) go test -race ./... ;;
    *)
        ensure_cli
        cd "$EXECUTION_PATH" || exit 1
        exec "$CLI" "$@"
        ;;
esac
