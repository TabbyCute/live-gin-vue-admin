#!/usr/bin/env bash

set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
script_filename="${BASH_SOURCE[0]##*/}"
package_name="${script_filename%.sh}"
archive_path="$script_dir/$package_name.tar"
target_dir="$script_dir/$package_name"
expected_sha256="__ARCHIVE_SHA256__"
release_marker_name=".release.sha256"
unpack_root=""

fail() {
  echo "部署失败：$*" >&2
  exit 1
}

calculate_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    fail "服务器缺少 sha256sum 或 shasum，无法校验发布包"
  fi
}

validate_release_dir() {
  local release_dir="$1"

  [[ -d "$release_dir" ]] || fail "发布目录不存在：$release_dir"
  [[ -x "$release_dir/tb-live-server" ]] || fail "发布目录缺少可执行文件 tb-live-server"
  [[ -r "$release_dir/config.prod.yaml" ]] || fail "发布目录缺少 config.prod.yaml"
  [[ -f "$release_dir/assets/admin/index.html" ]] || fail "发布目录缺少管理后台文件"
  [[ -x "$release_dir/restart-prod.sh" ]] || fail "发布目录缺少可执行的 restart-prod.sh"
}

cleanup() {
  if [[ -n "$unpack_root" && -d "$unpack_root" && "$unpack_root" == "$script_dir/.${package_name}.unpack."* ]]; then
    rm -rf -- "$unpack_root"
  fi
}
trap cleanup EXIT

[[ -f "$archive_path" ]] || fail "未找到同级发布包 $archive_path"

for command_name in tar mktemp awk tr; do
  command -v "$command_name" >/dev/null 2>&1 || fail "服务器缺少命令：$command_name"
done

echo "正在校验发布包：${archive_path##*/}"
actual_sha256="$(calculate_sha256 "$archive_path")"
[[ "$actual_sha256" == "$expected_sha256" ]] || fail "发布包 SHA-256 校验不通过，文件可能未上传完整"

if [[ -e "$target_dir" ]]; then
  validate_release_dir "$target_dir"
  marker_path="$target_dir/$release_marker_name"
  deployed_sha256="$(tr -d '[:space:]' < "$marker_path" 2>/dev/null || true)"
  [[ "$deployed_sha256" == "$actual_sha256" ]] ||
    fail "目标目录已存在，但不是由当前发布包完整部署的版本：$target_dir"

  echo "当前发布包已经解压完成，直接重新启动服务。"
  exec "$target_dir/restart-prod.sh"
fi

unpack_root="$(mktemp -d "$script_dir/.${package_name}.unpack.XXXXXX")"
echo "正在解压到：$target_dir"
tar --no-same-owner -xf "$archive_path" -C "$unpack_root"

unpacked_dir="$unpack_root/$package_name"
validate_release_dir "$unpacked_dir"
printf '%s\n' "$actual_sha256" > "$unpacked_dir/$release_marker_name"
chmod 600 "$unpacked_dir/$release_marker_name"

# 校验和及内容检查全部通过后再发布目录，避免半解压目录被误用。
mv -- "$unpacked_dir" "$target_dir"
# Linux 文件系统或解压工具可能在临时目录留下元数据文件，不能假设目录
# 一定为空。cleanup 只会删除本脚本通过 mktemp 创建且路径已校验的目录。
cleanup
unpack_root=""

chmod 700 "$target_dir/tb-live-server" "$target_dir/restart-prod.sh"
echo "解压完成，正在停止旧服务并启动新版本……"
exec "$target_dir/restart-prod.sh"
