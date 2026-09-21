#!/bin/bash
# AWS retired the go1.x runtime, so build for provided.al2023: the handler binary
# must be named "bootstrap".
set -e
filename=forex-bot-$(date +%Y-%m-%d).zip
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bootstrap .
zip "$filename" bootstrap
