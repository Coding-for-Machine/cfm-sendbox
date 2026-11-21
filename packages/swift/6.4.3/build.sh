#!/usr/bin/env bash

# Binary install (Debian 12)
curl -L "https://download.swift.org/swift-6.4.3-release/debian12/swift-6.4.3-RELEASE/swift-6.4.3-RELEASE-debian12.tar.gz" -o swift.tar.gz
tar xzf swift.tar.gz --strip-components=2
rm swift.tar.gz
