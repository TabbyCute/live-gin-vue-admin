#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  echo "用法: $0 <production|test>"
  echo "环境变量: TARGET_ARCH=amd64|arm64 SKIP_PNPM_INSTALL=1"
}

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 2
fi

environment="$1"
case "$environment" in
  production)
    config_name="config.prod.yaml"
    runtime_dir="production"
    gin_mode="release"
    ;;
  test)
    config_name="config.dev.yaml"
    runtime_dir="test"
    gin_mode="debug"
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "$script_dir/.." && pwd)"
web_dir="$project_root/web"
server_dir="$project_root/server"
release_dir="$project_root/release"

target_os="linux"
target_arch="${TARGET_ARCH:-amd64}"
case "$target_arch" in
  amd64|arm64) ;;
  *)
    echo "不支持的 TARGET_ARCH: $target_arch（仅支持 amd64、arm64）" >&2
    exit 2
    ;;
esac

for command_name in go node pnpm tar; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name" >&2
    exit 1
  fi
done

if [[ ! -f "$server_dir/$config_name" ]]; then
  echo "配置文件不存在: $server_dir/$config_name" >&2
  exit 1
fi

mkdir -p "$release_dir"

package_name="tb-live-${environment}-${target_os}-${target_arch}"
archive_path="$release_dir/${package_name}.tar.gz"
archive_temp="$release_dir/.${package_name}.tar.gz.tmp"
staging_root="$(mktemp -d "$release_dir/.${package_name}.XXXXXX")"
package_dir="$staging_root/$package_name"

cleanup() {
  rm -rf -- "$staging_root"
  rm -f -- "$archive_temp"
}
trap cleanup EXIT

echo "==> 构建环境: $environment"
echo "==> 目标平台: $target_os/$target_arch"
echo "==> Go: $(go version)"
echo "==> Node: $(node --version)"
echo "==> pnpm: $(pnpm --version)"

if [[ "${SKIP_PNPM_INSTALL:-0}" != "1" ]]; then
  echo "==> 安装前端依赖"
  (
    cd "$web_dir"
    pnpm install --frozen-lockfile
  )
else
  echo "==> 跳过前端依赖安装（SKIP_PNPM_INSTALL=1）"
fi

# 测试包同样使用生产构建模式。这样 API 地址保持为同源的 /api/v1/admin，
# 不会把 .env.development 中的 127.0.0.1 编译进部署包。
echo "==> 构建管理后台"
(
  cd "$web_dir"
  pnpm build
)

if [[ ! -f "$server_dir/assets/admin/index.html" ]]; then
  echo "管理后台构建失败：未生成 server/assets/admin/index.html" >&2
  exit 1
fi

mkdir -p \
  "$package_dir/assets" \
  "$package_dir/resource" \
  "$package_dir/uploads/$runtime_dir" \
  "$package_dir/log/$runtime_dir"

echo "==> 编译 Linux 后端"
(
  cd "$server_dir"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
    go build -trimpath -ldflags="-s -w" -o "$package_dir/tb-live-server" .
)

cp "$server_dir/$config_name" "$package_dir/$config_name"
cp -R "$server_dir/assets/admin" "$package_dir/assets/"
cp -R "$server_dir/resource/." "$package_dir/resource/"
chmod 700 "$package_dir/tb-live-server"
chmod 600 "$package_dir/$config_name"

if [[ "$environment" == "production" ]]; then
  cp "$script_dir/restart-prod.sh" "$package_dir/restart-prod.sh"
  chmod 700 "$package_dir/restart-prod.sh"
fi

echo "==> 生成压缩包"
rm -f -- "$archive_path"
tar -C "$staging_root" -czf "$archive_temp" "$package_name"
mv "$archive_temp" "$archive_path"

echo
echo "打包完成: $archive_path"
echo "服务器启动命令:"
if [[ "$environment" == "production" ]]; then
  echo "  cd <解压后的目录> && ./restart-prod.sh"
else
  echo "  cd /opt/tb-live && GIN_MODE=$gin_mode ./tb-live-server -c ./$config_name"
fi

if command -v shasum >/dev/null 2>&1; then
  echo "SHA-256:"
  shasum -a 256 "$archive_path"
elif command -v sha256sum >/dev/null 2>&1; then
  echo "SHA-256:"
  sha256sum "$archive_path"
fi
