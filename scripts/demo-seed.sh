#!/usr/bin/env bash
# Fills a local Acksy with synthetic demo notifications (homelab-style).
# Everything this script sends is made-up demo data.
#
#   ACKSY_URL=http://localhost:8080 ACKSY_USER=admin ACKSY_PASSWORD=... scripts/demo-seed.sh
set -euo pipefail

URL=${ACKSY_URL:-http://localhost:8080}
USER=${ACKSY_USER:-admin}
PASS=${ACKSY_PASSWORD:-acksy-dev-password}
JAR=$(mktemp "${TMPDIR:-/tmp}/acksy-seed.XXXXXX")
trap 'rm -f "$JAR"' EXIT

curl -fsS -c "$JAR" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$URL/api/v1/auth/login" >/dev/null

TOKEN=$(curl -fsS -b "$JAR" -H 'Content-Type: application/json' \
  -d '{"name":"demo-seed","scopes":["notifications:publish","topics:manage"]}' \
  "$URL/api/v1/tokens" | sed -E 's/.*"secret":"([^"]+)".*/\1/')

for t in 'backups|Backups|5' 'monitoring|Monitoring|' 'ci|CI|2' 'home|Home|' 'updates|Updates|2'; do
  IFS='|' read -r name title prio <<<"$t"
  body="{\"name\":\"$name\",\"title\":\"$title\""
  [ -n "$prio" ] && body+=",\"default_priority\":$prio"
  curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$body}" "$URL/api/v1/topics" >/dev/null || true
done

pub() { curl -fsS -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$1" "$URL/api/v1/notifications" >/dev/null; sleep 0.05; }

pub '{"topic":"updates","title":"Watchtower updated 3 containers","body":"- jellyfin `10.10.3` → `10.10.4`\n- paperless-ngx `2.13.1` → `2.13.2`\n- vaultwarden `1.32.4` → `1.32.5`","priority":2}'
pub '{"topic":"backups","title":"Nightly snapshot ok","body":"`zfs tank@2026-11-09` · 12.4 GB · 3m 41s","priority":2}'
pub '{"topic":"ci","title":"acksy main #141 failed","body":"`go test ./internal/store` failed on postgres:\n\n```\n--- FAIL: TestContractPostgres/dedup_upsert (0.02s)\n```","priority":4,"click_url":"https://example.com/ci/141"}'
pub '{"topic":"home","title":"Front door open for 10 min","body":"`binary_sensor.front_door` has been open since **09:21**.","priority":3}'
pub '{"topic":"ci","title":"acksy main #142 passed","body":"go test 9/9 · vitest 10/10 · image pushed to `ghcr.io/oowliq/acksy:main`"}'
pub '{"topic":"monitoring","title":"/dev/sda 92% on proxmox-01","body":"`local-lvm` will be full in **~6 days** at the current rate.\n\n| volume | used | free |\n|---|---|---|\n| local-lvm | 860 GB | 72 GB |\n| local | 41 GB | 53 GB |","priority":4}'
pub '{"topic":"monitoring","title":"UPS input voltage low","body":"rack-ups: input **196 V**, threshold 200 V","group_key":"rack-ups","priority":4}'
for i in 1 2 3; do
  pub '{"topic":"monitoring","title":"UPS on battery","body":"rack-ups switched to battery. Runtime **18 min**, load 41%.\n\n- [x] NAS notified\n- [ ] Proxmox shutdown scheduled","group_key":"rack-ups","dedup_key":"ups-on-battery","priority":5,"actions":[{"label":"Open NUT dashboard","url":"https://example.com/nut"}]}'
done
pub '{"topic":"backups","title":"Backup failed: restic nas → b2","body":"The nightly backup to `b2:homelab-restic` exited with code **1** after 4m12s.\n\n```\nFatal: unable to create lock in backend:\nrepository is already locked by PID 2231 on nas\n```\n\nRun `restic unlock` once the stale process is gone.","priority":5,"tags":["restic","b2"]}'

echo "Seeded demo notifications into $URL"
