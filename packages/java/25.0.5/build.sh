#!/usr/bin/env bash

# Eclipse Temurin 25.0.5+8 (LTS)
curl -L "https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25.0.5%2B8/OpenJDK25U-jdk_x64_linux_hotspot_25.0.5_8.tar.gz" -o java.tar.gz

tar xzf java.tar.gz --strip-components=1
rm java.tar.gz
