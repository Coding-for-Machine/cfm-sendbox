#!/usr/bin/env bash

# Download and extract JDK 21 (Temurin)
curl -L "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jdk_x64_linux_hotspot_21.0.13_9.tar.gz" -o jdk.tar.gz
tar xzf jdk.tar.gz --strip-components=1
rm jdk.tar.gz

# Download and extract Kotlin
curl -L "https://github.com/JetBrains/kotlin/releases/download/v2.4.21/kotlin-compiler-2.4.21.zip" -o kotlin.zip
unzip kotlin.zip
rm kotlin.zip
cp -r kotlinc/* .
rm -rf kotlinc
