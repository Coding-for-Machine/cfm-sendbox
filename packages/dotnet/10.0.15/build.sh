#!/usr/bin/env bash

# .NET 10 (LTS): install the latest SDK of the 10.0 channel
curl -sSL https://dot.net/v1/dotnet-install.sh -o dotnet-install.sh
chmod +x dotnet-install.sh
./dotnet-install.sh --channel 10.0 --install-dir "$PWD"
rm dotnet-install.sh

# Cache nuget packages
export DOTNET_CLI_HOME=$PWD
./dotnet new console -o cache_application
./dotnet new console -lang F# -o fs_cache_application
./dotnet new console -lang VB -o vb_cache_application
# This calls a restore on the global-packages index ($DOTNET_CLI_HOME/.nuget/packages)
# If we want to allow more packages, we could add them to this cache_application

rm -rf cache_application fs_cache_application vb_cache_application
# Get rid of it, we don't actually need the application - just the restore
