#!/usr/bin/env bash
set -euo pipefail

TO_DIR=""
FRONTEND_DIR=""
SKIP_BUILD=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --to) TO_DIR="${2:-}"; shift ;;
    --frontend) FRONTEND_DIR="${2:-}"; shift ;;
    --skip-build) SKIP_BUILD=true ;;
    --dry-run) DRY_RUN=true ;;
    -h|--help) awk '/^#/,/^$/' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
  shift
done

if [[ "$SKIP_BUILD" != true ]]; then
  go build -trimpath -ldflags="-s -w" -o /tmp/cpa-core ./cmd/server
fi

if [[ -n "$TO_DIR" ]]; then
  mkdir -p "$TO_DIR/static" "$TO_DIR/var" "$TO_DIR/logs" "$TO_DIR/auths"
  if [[ -f "$TO_DIR/cpa-core" ]]; then cp "$TO_DIR/cpa-core" "$TO_DIR/cpa-core.bak"; fi
  install -m 0755 /tmp/cpa-core "$TO_DIR/cpa-core"
  
  if [[ -n "$FRONTEND_DIR" && -f "$FRONTEND_DIR/dist/index.html" ]]; then
    install -m 0644 "$FRONTEND_DIR/dist/index.html" "$TO_DIR/static/web.html"
  fi
  
  if [[ ! -f "$TO_DIR/config.yaml" && -f ~/.config/cpa.yml ]]; then
    ln -sfn ~/.config/cpa.yml "$TO_DIR/config.yaml"
  fi
fi

echo "完成"
