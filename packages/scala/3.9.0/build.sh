#!/usr/bin/env bash

# Download and extract JDK 21 (Temurin)
curl -L "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jdk_x64_linux_hotspot_21.0.13_9.tar.gz" -o jdk.tar.gz
tar xzf jdk.tar.gz --strip-components=1
rm jdk.tar.gz

# Download and extract Scala 3 (3.9.0, LTS)
curl -L "https://github.com/scala/scala3/releases/download/3.9.0/scala3-3.9.0.tar.gz" -o scala.tar.gz
tar -xzf scala.tar.gz --strip-components=1
rm scala.tar.gz
