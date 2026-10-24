#!/bin/sh
# usage: ./build.sh 1.4.0
# optional: HOST=play.example.com CLIENT_ID=<azure app id> ./build.sh 1.4.0
set -e
V=${1:-dev}; OUT=dist; mkdir -p $OUT
LD="-s -w -X main.LauncherVersion=$V -X main.defaultHost=$HOST -X main.defaultClientID=$CLIENT_ID"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD -H=windowsgui" -o $OUT/KoreaWarLauncher-windows-x64.exe .
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD -H=windowsgui" -o $OUT/KoreaWarLauncher-windows-arm64.exe .
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD" -o $OUT/KoreaWarLauncher-linux-x64 .
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD" -o $OUT/KoreaWarLauncher-linux-arm64 .
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD" -o $OUT/KoreaWarLauncher-mac-arm64 .
GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD" -o $OUT/KoreaWarLauncher-mac-x64 .
