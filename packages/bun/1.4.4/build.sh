#!/usr/bin/env bash

curl -L "https://github.com/oven-sh/bun/releases/download/bun-v1.4.4/bun-linux-x64.zip" -o bun.zip
unzip -o bun.zip
mv bun-linux-x64/bun .
rm -rf bun.zip bun-linux-x64
chmod +x bun
