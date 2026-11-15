#!/usr/bin/env bash

set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_root="$(cd -- "$script_dir/.." && pwd)"
h5_source_dir="${H5_SOURCE_DIR:-$HOME/Desktop/IM/H5Player}"
h5_output_dir="$project_root/server/assets/h5"
h5_base="/app/v1/assets/h5/"

for command_name in node pnpm mktemp; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少 H5 构建命令：$command_name" >&2
    exit 1
  fi
done

if [[ ! -f "$h5_source_dir/package.json" || ! -f "$h5_source_dir/pnpm-lock.yaml" ]]; then
  echo "H5 源码或 pnpm-lock.yaml 不存在：$h5_source_dir，请通过 H5_SOURCE_DIR 指定 H5Player 目录。" >&2
  exit 1
fi
h5_source_dir="$(cd -- "$h5_source_dir" && pwd)"
if [[ -L "$h5_output_dir" || ( -e "$h5_output_dir" && ! -d "$h5_output_dir" ) ]]; then
  echo "H5 输出路径必须是普通目录：$h5_output_dir" >&2
  exit 1
fi

staging_root="$(mktemp -d "$project_root/server/assets/.h5-build.XXXXXX")"
cleanup() {
  # 发布失败时恢复上次构建，只有新构建就绪才替换整个输出目录。
  if [[ ! -e "$h5_output_dir" && -d "$staging_root/previous" ]]; then
    mv -- "$staging_root/previous" "$h5_output_dir"
  fi
  if [[ -d "$staging_root" && "$staging_root" == "$project_root/server/assets/.h5-build."* ]]; then
    rm -rf -- "$staging_root"
  fi
}
trap cleanup EXIT

echo "==> 构建 H5 客户端：$h5_source_dir"
(
  cd "$h5_source_dir"
  if [[ "${SKIP_PNPM_INSTALL:-0}" != "1" ]]; then
    pnpm install --frozen-lockfile
  fi
  # 同源部署，覆盖客户端环境中的根路径和固定 API 主机地址。
  VITE_APP_BASE="$h5_base" VITE_API_BASE_URL="/api" \
    pnpm exec vite build --mode production --base "$h5_base" --outDir "$staging_root/h5"
)

if [[ ! -s "$staging_root/h5/index.html" ]]; then
  echo "H5 构建失败：未生成 index.html" >&2
  exit 1
fi

if [[ -d "$h5_output_dir" ]]; then
  mv -- "$h5_output_dir" "$staging_root/previous"
fi
mv -- "$staging_root/h5" "$h5_output_dir"

echo "H5 打包完成：$h5_output_dir"
echo "访问路径：$h5_base"
