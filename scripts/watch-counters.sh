#!/usr/bin/env bash
# Live encoder counts from closinuf (turn encoders while this runs).
# Usage: ./scripts/watch-counters.sh [host]
set -euo pipefail

HOST="${1:-http://127.0.0.1:3000}"

if ! command -v jq >/dev/null 2>&1; then
	echo "Install jq: sudo apt install jq" >&2
	exit 1
fi

echo "Watching ${HOST}/api/encoder (Ctrl+C to stop)"
echo "For SPI detail: curl -s ${HOST}/api/encoder/debug | jq"
echo "Motion probe (turn encoders during 2s): curl -s '${HOST}/api/encoder/debug/probe?seconds=2' | jq"
echo

while true; do
	curl -sf "${HOST}/api/encoder" | jq -r '[.x.count, .["x'"'"].count, .y.count, .z.count] | @tsv' 2>/dev/null \
		| awk '{printf "X=%s  Xp=%s  Y=%s  Z=%s\n", $1,$2,$3,$4}' \
		|| echo "(request failed)"
	sleep 0.25
done
