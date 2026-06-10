#!/usr/bin/env bash

# cpa-core 构建与部署脚本
#
# 编译 Go 二进制并部署到指定目录。同时可选部署前端 web.html。
#
# 用法:
#   ./deploy.sh                                       # 编译到 /tmp/cpa-core
#   ./deploy.sh --to ~/cpa-core                        # 编译并部署到目标目录
#   ./deploy.sh --skip-build --to ~/cpa-core           # 用已有产物部署
#   ./deploy.sh --to ~/cpa-core --frontend ../cpa-web  # 编译后端 + 部署前端
#   ./deploy.sh --dry-run --to ~/cpa-core              # 预览部署操作
#
# 选项:
#   --to <目录>        部署目标目录（自动创建 static/ var/ logs/ auths/）
#   --frontend <目录>  前端项目路径（编译后部署 web.html）
#   --skip-build       跳过 Go 编译，使用已有 /tmp/cpa-core
#   --dry-run          只显示操作预览，不实际执行
#   -h, --help         显示此帮助信息
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
    -h|--help)
      awk '/^#/{print;next} /^$/{exit}' "$0" | sed 's/^# //;s/^#$//'
      exit 0
      ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
  shift
done

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuiltAt=${BUILT_AT}"

if [[ "$SKIP_BUILD" != true ]]; then
  go build -trimpath -ldflags="${LDFLAGS}" -o /tmp/cpa-core ./cmd/server
fi

if [[ -n "$TO_DIR" ]]; then
  mkdir -p "$TO_DIR/static" "$TO_DIR/var" "$TO_DIR/logs" "$TO_DIR/auths"
  if [[ -f "$TO_DIR/cpa-core" ]]; then
    cp "$TO_DIR/cpa-core" "$TO_DIR/cpa-core.bak"
  fi
  install -m 0755 /tmp/cpa-core "$TO_DIR/cpa-core"
  
  if [[ -n "$FRONTEND_DIR" && -f "$FRONTEND_DIR/dist/index.html" ]]; then
    install -m 0644 "$FRONTEND_DIR/dist/index.html" "$TO_DIR/static/web.html"
  fi
  
  if [[ ! -f "$TO_DIR/config.yaml" ]]; then
    if [[ -f config.example.yaml ]]; then
      cp config.example.yaml "$TO_DIR/config.yaml"
      echo "已复制 config.example.yaml → $TO_DIR/config.yaml"
      echo "请编辑 $TO_DIR/config.yaml 设置管理密钥等配置后启动"
    fi
  fi
fi

echo "部署完成"
