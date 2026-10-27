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
process_name="tb-live-server"
force_stop_wait_seconds=5

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

for command_name in pgrep curl; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "缺少启动所需命令：$command_name" >&2
    exit 1
  fi
done

mkdir -p "$log_dir"

declare -a old_pids=()

pid_is_live_server() {
  local pid="$1"
  local process_comm
  local process_exe
  local process_state

  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  kill -0 "$pid" 2>/dev/null || return 1

  process_state="$(ps -o stat= -p "$pid" 2>/dev/null || true)"
  process_state="${process_state//[[:space:]]/}"
  [[ -n "$process_state" && "${process_state:0:1}" != "Z" ]] || return 1

  process_comm="$(cat "/proc/$pid/comm" 2>/dev/null || true)"
  if [[ "$process_comm" == "$process_name" ]]; then
    return 0
  fi

  process_exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
  process_exe="${process_exe% (deleted)}"
  [[ "${process_exe##*/}" == "$process_name" ]]
}

add_pid() {
  local pid="$1"
  local existing

  pid_is_live_server "$pid" || return 0

  for existing in "${old_pids[@]:-}"; do
    [[ "$existing" == "$pid" ]] && return 0
  done
  old_pids+=("$pid")
}

collect_live_server_pids() {
  local candidate_pid
  local saved_pid

  old_pids=()

  # PID 文件可能来自当前发布目录，但必须再次核对进程身份，避免 PID
  # 已复用时误杀无关进程。
  if [[ -s "$pid_file" ]]; then
    saved_pid="$(tr -d '[:space:]' < "$pid_file")"
    add_pid "$saved_pid"
  fi

  # 不再限定可执行文件必须位于当前发布目录。这样可以清理旧目录、已被
  # 覆盖而显示为 (deleted)，以及之前通过 nohup 手工启动的实例。
  while IFS= read -r candidate_pid; do
    [[ -n "$candidate_pid" ]] || continue
    add_pid "$candidate_pid"
  done < <(pgrep -x "$process_name" 2>/dev/null || true)
}

stop_old_live_servers() {
  local second

  collect_live_server_pids
  if (( ${#old_pids[@]} == 0 )); then
    echo "未发现旧服务"
    rm -f -- "$pid_file"
    return 0
  fi

  echo "正在停止旧服务：${old_pids[*]}"
  kill -TERM -- "${old_pids[@]}" 2>/dev/null || true

  for ((second = 0; second < stop_timeout; second++)); do
    collect_live_server_pids
    (( ${#old_pids[@]} == 0 )) && break
    # 同时处理停止期间被守护程序重新拉起的同名旧实例。
    kill -TERM -- "${old_pids[@]}" 2>/dev/null || true
    sleep 1
  done

  collect_live_server_pids
  if (( ${#old_pids[@]} > 0 )); then
    echo "旧服务未在 ${stop_timeout} 秒内退出，强制停止：${old_pids[*]}"
    kill -KILL -- "${old_pids[@]}" 2>/dev/null || true
  fi

  for ((second = 0; second < force_stop_wait_seconds; second++)); do
    collect_live_server_pids
    (( ${#old_pids[@]} == 0 )) && break
    sleep 1
  done

  collect_live_server_pids
  if (( ${#old_pids[@]} > 0 )); then
    echo "无法清理旧服务进程：${old_pids[*]}。请检查 systemd、Supervisor 等守护程序是否仍在自动拉起旧版本。" >&2
    exit 1
  fi

  rm -f -- "$pid_file"
  echo "旧服务已全部停止"
}

health_authority="${health_url#*://}"
health_authority="${health_authority%%/*}"
listen_port="${LISTEN_PORT:-}"
if [[ -z "$listen_port" && "$health_authority" =~ :([0-9]+)$ ]]; then
  listen_port="${BASH_REMATCH[1]}"
fi

if [[ -n "$listen_port" ]] &&
  { [[ ! "$listen_port" =~ ^[0-9]+$ ]] || (( listen_port < 1 || listen_port > 65535 )); }; then
  echo "LISTEN_PORT 或 HEALTH_URL 中的端口无效：$listen_port" >&2
  exit 2
fi

stop_old_live_servers

# 旧进程已经清理后，端口仍被监听说明存在非本服务进程；此时拒绝启动，
# 避免健康检查命中其他进程并错误报告启动成功。
if [[ -n "$listen_port" ]] && command -v ss >/dev/null 2>&1; then
  listener_info="$(ss -H -ltnp "sport = :$listen_port" 2>/dev/null || true)"
  if [[ -n "$listener_info" ]]; then
    echo "端口 $listen_port 仍被其他进程占用，已取消启动：" >&2
    echo "$listener_info" >&2
    exit 1
  fi
fi

cd "$app_dir"
echo "正在启动生产服务……"
nohup env GIN_MODE=release \
  "$binary" -c "$config" \
  >> "$log_file" 2>&1 < /dev/null &

new_pid=$!
printf '%s\n' "$new_pid" > "$pid_file"

for ((second = 0; second < start_timeout; second++)); do
  # nohup -> env -> 服务程序的 exec 切换存在极短窗口；此处只判断 PID
  # 是否存活，健康检查通过后再严格核对进程名称和唯一性。
  if ! kill -0 "$new_pid" 2>/dev/null; then
    rm -f -- "$pid_file"
    echo "服务启动失败，最近日志：" >&2
    tail -n 100 "$log_file" >&2
    exit 1
  fi

  if curl -fsS "$health_url" >/dev/null 2>&1; then
    collect_live_server_pids
    if (( ${#old_pids[@]} != 1 )) || [[ "${old_pids[0]:-}" != "$new_pid" ]]; then
      echo "健康检查虽然通过，但检测到非本次启动的服务进程：${old_pids[*]:-无}，已取消确认成功。" >&2
      kill -TERM -- "$new_pid" 2>/dev/null || true
      rm -f -- "$pid_file"
      exit 1
    fi
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
