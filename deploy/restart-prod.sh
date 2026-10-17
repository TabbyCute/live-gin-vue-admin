#!/usr/bin/env bash

set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
app_dir="${APP_DIR:-$script_dir}"
binary="$app_dir/tb-live-server"
config="$app_dir/config.prod.yaml"
log_dir="$app_dir/log/production"
log_file="$log_dir/server-console.log"
pid_file="$app_dir/tb-live-server.pid"
health_url="${HEALTH_URL:-http://127.0.0.1:8888/api/health}"
stop_timeout="${STOP_TIMEOUT_SECONDS:-15}"
start_timeout="${START_TIMEOUT_SECONDS:-20}"

if [[ ! "$stop_timeout" =~ ^[1-9][0-9]*$ ]] || [[ ! "$start_timeout" =~ ^[1-9][0-9]*$ ]]; then
  echo "STOP_TIMEOUT_SECONDS 和 START_TIMEOUT_SECONDS 必须是正整数" >&2
  exit 2
fi

if [[ ! -x "$binary" ]]; then
  echo "服务程序不存在或不可执行：$binary" >&2
  exit 1
fi

if [[ ! -r "$config" ]]; then
  echo "生产配置不存在或不可读：$config" >&2
  exit 1
fi

mkdir -p "$log_dir"

declare -a old_pids=()

pid_matches_binary() {
  local pid="$1"
  local process_exe

  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1
  process_exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
  process_exe="${process_exe% (deleted)}"
  [[ "$process_exe" == "$binary" ]]
}

add_pid() {
  local pid="$1"
  local existing

  pid_matches_binary "$pid" || return 0

  for existing in "${old_pids[@]:-}"; do
    [[ "$existing" == "$pid" ]] && return 0
  done
  old_pids+=("$pid")
}

# 优先使用脚本保存的 PID。
if [[ -s "$pid_file" ]]; then
  saved_pid="$(<"$pid_file")"
  add_pid "$saved_pid"
fi

# 兼容第一次使用本脚本前，通过 nohup 手工启动的旧进程。
while IFS= read -r candidate_pid; do
  [[ -n "$candidate_pid" ]] || continue
  if pid_matches_binary "$candidate_pid"; then
    add_pid "$candidate_pid"
  fi
done < <(pgrep -x tb-live-server 2>/dev/null || true)

if (( ${#old_pids[@]} > 0 )); then
  echo "正在停止旧服务：${old_pids[*]}"
  kill "${old_pids[@]}"

  for ((second = 0; second < stop_timeout; second++)); do
    declare -a running_pids=()
    for pid in "${old_pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        running_pids+=("$pid")
      fi
    done
    (( ${#running_pids[@]} == 0 )) && break
    sleep 1
  done

  declare -a remaining_pids=()
  for pid in "${old_pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      remaining_pids+=("$pid")
    fi
  done
  if (( ${#remaining_pids[@]} > 0 )); then
    echo "旧服务未在 ${stop_timeout} 秒内退出，强制停止：${remaining_pids[*]}"
    kill -KILL "${remaining_pids[@]}"
  fi
else
  echo "未发现旧服务，直接启动"
fi

cd "$app_dir"
echo "正在启动生产服务……"
nohup env GIN_MODE=release \
  "$binary" -c "$config" \
  >> "$log_file" 2>&1 < /dev/null &

new_pid=$!
printf '%s\n' "$new_pid" > "$pid_file"

for ((second = 0; second < start_timeout; second++)); do
  if ! kill -0 "$new_pid" 2>/dev/null; then
    echo "服务启动失败，最近日志：" >&2
    tail -n 100 "$log_file" >&2
    exit 1
  fi

  if ! command -v curl >/dev/null 2>&1 || curl -fsS "$health_url" >/dev/null 2>&1; then
    echo "服务启动成功，PID：$new_pid"
    echo "健康检查：$health_url"
    echo "日志文件：$log_file"
    exit 0
  fi
  sleep 1
done

echo "服务进程已启动，但 ${start_timeout} 秒内健康检查未通过。最近日志：" >&2
tail -n 100 "$log_file" >&2
exit 1
