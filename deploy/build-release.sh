#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  echo "用法: $0 <production|test>"
  echo "环境变量: TARGET_ARCH=amd64|arm64 SKIP_PNPM_INSTALL=1 BUILD_TIMEZONE=Asia/Kuala_Lumpur"
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
release_runner_template="$script_dir/run-release.sh"

target_os="linux"
target_arch="${TARGET_ARCH:-amd64}"
case "$target_arch" in
  amd64|arm64) ;;
  *)
    echo "不支持的 TARGET_ARCH: $target_arch（仅支持 amd64、arm64）" >&2
    exit 2
    ;;
esac

for command_name in go node pnpm tar mktemp awk sed date; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少命令: $command_name" >&2
    exit 1
  fi
done

if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "缺少 sha256sum 或 shasum，无法生成发布包校验值" >&2
  exit 1
fi

if [[ ! -f "$server_dir/$config_name" ]]; then
  echo "配置文件不存在: $server_dir/$config_name" >&2
  exit 1
fi

if [[ "$environment" == "production" && ! -f "$release_runner_template" ]]; then
  echo "部署脚本模板不存在: $release_runner_template" >&2
  exit 1
fi

mkdir -p "$release_dir"

build_timezone="${BUILD_TIMEZONE:-Asia/Kuala_Lumpur}"
build_date="$(TZ="$build_timezone" date +%Y%m%d)"
if [[ ! "$build_date" =~ ^[0-9]{8}$ ]]; then
  echo "无法按时区 $build_timezone 获取打包日期" >&2
  exit 1
fi

package_prefix="tb-live-${environment}-${target_os}-${build_date}"
package_name=""
archive_path=""
deploy_script_path=""
reservation_dir=""

# 用原子创建的保留目录分配当天序号，避免两个打包任务拿到同一个编号。
for ((sequence = 1; sequence <= 99; sequence++)); do
  printf -v sequence_text '%02d' "$sequence"
  candidate_name="${package_prefix}${sequence_text}"
  candidate_archive="$release_dir/${candidate_name}.tar"
  candidate_script="$release_dir/${candidate_name}.sh"
  candidate_reservation="$release_dir/.${candidate_name}.building"

  if [[ ! -e "$candidate_archive" && ! -e "$candidate_script" ]] && mkdir "$candidate_reservation" 2>/dev/null; then
    package_name="$candidate_name"
    archive_path="$candidate_archive"
    deploy_script_path="$candidate_script"
    reservation_dir="$candidate_reservation"
    break
  fi
done

if [[ -z "$package_name" ]]; then
  echo "当天打包序号已用完（01-99）：$build_date" >&2
  exit 1
fi

staging_root=""
package_dir=""
archive_temp=""
deploy_script_temp=""
published=0

cleanup() {
  [[ -z "$staging_root" || ! -d "$staging_root" ]] || rm -rf -- "$staging_root"
  [[ -z "$archive_temp" || ! -e "$archive_temp" ]] || rm -f -- "$archive_temp"
  [[ -z "$deploy_script_temp" || ! -e "$deploy_script_temp" ]] || rm -f -- "$deploy_script_temp"
  [[ -z "$reservation_dir" || ! -d "$reservation_dir" ]] || rmdir -- "$reservation_dir"
  if [[ "$published" != "1" ]]; then
    [[ -z "$archive_path" || ! -e "$archive_path" ]] || rm -f -- "$archive_path"
    [[ -z "$deploy_script_path" || ! -e "$deploy_script_path" ]] || rm -f -- "$deploy_script_path"
  fi
}
trap cleanup EXIT

staging_root="$(mktemp -d "$release_dir/.${package_name}.staging.XXXXXX")"
package_dir="$staging_root/$package_name"
archive_temp="$(mktemp "$release_dir/.${package_name}.archive.XXXXXX")"

echo "==> 构建环境: $environment"
echo "==> 目标平台: $target_os/$target_arch"
echo "==> 发布编号: $package_name"
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

echo "==> 生成发布包"
# macOS 打包时禁止写入 AppleDouble 元数据，避免 Linux 解压后出现额外的
# ._* 文件；其他系统会安全地忽略这个环境变量。
COPYFILE_DISABLE=1 tar -C "$staging_root" -cf "$archive_temp" "$package_name"

if command -v sha256sum >/dev/null 2>&1; then
  archive_sha256="$(sha256sum "$archive_temp" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  archive_sha256="$(shasum -a 256 "$archive_temp" | awk '{print $1}')"
else
  echo "缺少 sha256sum 或 shasum，无法生成发布包校验值" >&2
  exit 1
fi

if [[ "$environment" == "production" ]]; then
  deploy_script_temp="$(mktemp "$release_dir/.${package_name}.script.XXXXXX")"
  sed "s/__ARCHIVE_SHA256__/$archive_sha256/g" "$release_runner_template" > "$deploy_script_temp"
  chmod 700 "$deploy_script_temp"
fi

# 临时文件完整生成后才对外发布，避免成功目录中出现未完成的发布包。
mv -- "$archive_temp" "$archive_path"
archive_temp=""
if [[ "$environment" == "production" ]]; then
  mv -- "$deploy_script_temp" "$deploy_script_path"
  deploy_script_temp=""
fi
published=1

echo
echo "打包完成："
echo "  发布包：$archive_path"
if [[ "$environment" == "production" ]]; then
  echo "  部署脚本：$deploy_script_path"
  echo
  echo "将以上两个文件放到服务器的同一个目录，然后复制执行："
  echo "chmod +x ./${package_name}.sh && ./${package_name}.sh"
  echo
  echo "脚本会校验并解压发布包、停止全部旧 tb-live-server 进程，再启动新版本。"
else
  echo "服务器启动命令："
  echo "  tar -xf ${package_name}.tar && cd ${package_name} && GIN_MODE=$gin_mode ./tb-live-server -c ./$config_name"
fi

echo "SHA-256：$archive_sha256"
