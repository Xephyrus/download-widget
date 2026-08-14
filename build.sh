#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

if [ -n "${GOROOT:-}" ]; then
  PATH="$GOROOT/bin:$PATH"
  export PATH
fi

OUT=pixiv-downloader
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW*|MSYS*|CYGWIN*) OUT=pixiv-downloader.exe ;;
esac

go mod tidy
go build -trimpath -ldflags="-s -w" -o "$OUT" .

SIZE=$(wc -c < "$OUT" | tr -d ' ')
SIZE_MB=$(awk "BEGIN { printf \"%.2f\", $SIZE / (1024*1024) }")
echo "Built $OUT (${SIZE_MB} MB)"
