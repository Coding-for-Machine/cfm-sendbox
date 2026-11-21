#!/usr/bin/env bash

# Official prebuilt binaries of Julia 1.10.13 (LTS)
curl -L "https://julialang-s3.julialang.org/bin/linux/x64/1.10/julia-1.10.13-linux-x86_64.tar.gz" -o julia.tar.gz
tar xzf julia.tar.gz --strip-components=1
rm julia.tar.gz
