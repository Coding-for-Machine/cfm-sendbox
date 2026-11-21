#!/usr/bin/env bash

# Eclipse Temurin 17.0.21+9 (LTS)
curl -L "https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.21%2B9/OpenJDK17U-jdk_x64_linux_hotspot_17.0.21_9.tar.gz" -o java.tar.gz

tar xzf java.tar.gz --strip-components=1
rm java.tar.gz
