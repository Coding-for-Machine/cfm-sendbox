#!/bin/bash
curl "https://nodejs.org/dist/v22.23.5/node-v22.23.5-linux-x64.tar.xz" -o node.tar.xz
tar xf node.tar.xz --strip-components=1
rm node.tar.xz