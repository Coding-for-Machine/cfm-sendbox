#!/usr/bin/env bash

curl -L -o go.tar.gz https://go.dev/dl/go1.26.11.linux-amd64.tar.gz
tar -xzf go.tar.gz
rm go.tar.gz

# Since Go 1.20 the standard library is no longer shipped precompiled, so
# every cold build would recompile fmt, os, ... (several seconds). Pre-warm a
# build cache inside the package; the compile script copies it per job.
# The package is built at the path it is installed to, which keeps the cache
# keys valid. Settings here must match the "environment" file.
export PATH=$PWD/go/bin:$PATH
export GOROOT=$PWD/go GOPATH=$PWD/gopath GOTOOLCHAIN=local GOTELEMETRY=off
export CGO_ENABLED=0 GOFLAGS= GOCACHE=$PWD/warm-cache HOME=$PWD/warm-home
mkdir -p warm "$HOME"
cat > warm/main.go <<'GO'
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"container/heap"
	"container/list"
	"container/ring"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/big"
	"math/bits"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"
)

var _ = []any{bufio.NewReader, bytes.NewBuffer, cmp.Compare[int], heap.Init, list.New, ring.New, errors.New,
	io.EOF, maps.Keys[map[int]int], math.Abs, big.NewInt, bits.Len, rand.Intn, os.Stdin, regexp.MustCompile,
	slices.Sort[[]int], sort.Ints, strconv.Itoa, strings.Fields, sync.NewCond, tabwriter.NewWriter, time.Now,
	unicode.IsLetter, utf8.RuneLen}

func main() { fmt.Println("OK") }
GO
(cd warm && go build -o /dev/null main.go) || exit 1
rm -rf warm warm-home
go clean -modcache 2>/dev/null
