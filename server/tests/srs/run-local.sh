#!/usr/bin/env bash
set -euo pipefail
srs_test_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
srs_test_image="ossrs/srs:7.0.165"
srs_test_name="tb-live-srs-smoke-$$"
srs_test_network="${srs_test_name}-net"
srs_test_container_created=0
srs_test_network_created=0
command -v ffmpeg >/dev/null
command -v ffprobe >/dev/null
# No implicit image pull; no callbacks or existing container commands.
docker image inspect "$srs_test_image" >/dev/null
srs_test_cleanup() {
  if [[ "$srs_test_container_created" == 1 ]]; then docker rm -f "$srs_test_name" >/dev/null; fi
  if [[ "$srs_test_network_created" == 1 ]]; then docker network rm "$srs_test_network" >/dev/null; fi
}
trap srs_test_cleanup EXIT INT TERM
docker network create --label tb-live-test=srs-smoke "$srs_test_network" >/dev/null
srs_test_network_created=1
docker run -d --name "$srs_test_name" --network "$srs_test_network" \
  --label tb-live-test=srs-smoke --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 128 --memory 256m --cpus 1 \
  -p 127.0.0.1::1935 -p 127.0.0.1::1985 -p 127.0.0.1::8080 \
  -v "$srs_test_root/tests/srs/local.conf:/usr/local/srs/conf/local-smoke.conf:ro" \
  "$srs_test_image" ./objs/srs -c conf/local-smoke.conf >/dev/null
srs_test_container_created=1
if [[ "$(docker inspect --format '{{.State.Running}}' "$srs_test_name")" != true ]]; then
  docker logs --tail 60 "$srs_test_name" >&2
  exit 1
fi
srs_test_api="$(docker port "$srs_test_name" 1985/tcp)" || { docker logs --tail 60 "$srs_test_name" >&2; exit 1; }
srs_test_rtmp="$(docker port "$srs_test_name" 1935/tcp)"
srs_test_flv="$(docker port "$srs_test_name" 8080/tcp)"
for srs_test_endpoint in "$srs_test_api" "$srs_test_rtmp" "$srs_test_flv"; do
  [[ "$srs_test_endpoint" == 127.0.0.1:* ]] || { echo 'Refusing non-loopback published port' >&2; exit 1; }
done
SRS_LOCAL_SMOKE=1 SRS_LOCAL_API="http://$srs_test_api" SRS_LOCAL_RTMP="rtmp://$srs_test_rtmp" SRS_LOCAL_FLV="http://$srs_test_flv" \
  go -C "$srs_test_root" test ./internal/srs -run '^TestLocalMediaSmoke$' -count=1 -v
