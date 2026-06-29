#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <vet|test> <go-files...>" >&2
  exit 1
fi

mode="$1"
shift

if [ "$mode" != "vet" ] && [ "$mode" != "test" ]; then
  echo "lefthook: 不支持的模式: $mode" >&2
  exit 1
fi

packages=()
seen=""

for file in "$@"; do
  case "$file" in
    *.go) ;;
    *) continue ;;
  esac

  dir="$(dirname "$file")"
  pkg="."
  if [ "$dir" != "." ]; then
    pkg="./$dir"
  fi

  case " $seen " in
    *" $pkg "*) continue ;;
  esac

  seen="$seen $pkg"
  packages+=("$pkg")
done

if [ "${#packages[@]}" -eq 0 ]; then
  echo "lefthook: 未检测到受影响 Go package，跳过 go $mode。"
  exit 0
fi

for pkg in "${packages[@]}"; do
  if [ "$mode" = "vet" ]; then
    echo "lefthook: go vet $pkg"
    go vet "$pkg"
  else
    echo "lefthook: go test $pkg"
    go test "$pkg"
  fi
done
