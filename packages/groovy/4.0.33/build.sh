#!/bin/bash

# Groovy depends on JDK 11+ (Temurin 21)
mkdir -p java
cd java
curl -L "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.13%2B9/OpenJDK21U-jdk_x64_linux_hotspot_21.0.13_9.tar.gz" -o java.tar.gz
tar xzf java.tar.gz --strip-components=1
rm java.tar.gz
cd ..

# Download Groovy binaries
curl -L "https://groovy.jfrog.io/artifactory/dist-release-local/groovy-zips/apache-groovy-binary-4.0.33.zip" -o groovy.zip
unzip -q groovy.zip
rm groovy.zip
