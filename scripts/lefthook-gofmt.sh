#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "lefthook: 未检测到暂存区 Go 文件，跳过 gofmt。"
  exit 0
fi

gofmt -w "$@"
