#!/usr/bin/env bash

# Eclipse Temurin 21.0.13+9 (LTS)
curl -L "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jdk_x64_linux_hotspot_21.0.13_9.tar.gz" -o java.tar.gz

tar xzf java.tar.gz --strip-components=1
rm java.tar.gz
