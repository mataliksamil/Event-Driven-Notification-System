#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=========================================="
echo "  Notification System - k6 Load Test"
echo "=========================================="

echo ""
echo "[1/3] Starting docker compose stack (LOG_LEVEL=error)..."
LOG_LEVEL=error docker compose -f "$PROJECT_DIR/docker-compose.yaml" up -d

echo ""
echo "[2/3] Waiting for services (15s)..."
sleep 15

echo ""
echo "[3/3] Running k6 load test..."
echo "  Scenarios:"
echo "    warmup:    0 → 20 VUs over 15s"
echo "    sustained: 20 VUs for 1m"
echo "    spike:     20 → 60 VUs, hold 15s, ramp down 10s"
echo ""

docker compose -f "$PROJECT_DIR/docker-compose.yaml" \
  --profile load-test \
  run --rm k6 \
  run /tests/load_test.js \
  --out experimental-prometheus-rw

echo ""
echo "=========================================="
echo "  Load test completed!"
echo "  View results in Grafana: http://localhost:3000"
echo "  Dashboard: Load Test"
echo "=========================================="