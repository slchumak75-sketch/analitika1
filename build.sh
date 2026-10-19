#!/usr/bin/env bash
# Кросс-компиляция Windows .exe с Linux/macOS
set -e
cd "$(dirname "$0")"

go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H windowsgui" -o RetailBreakEven.exe .

echo "Готово: RetailBreakEven.exe"
