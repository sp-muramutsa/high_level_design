#!/usr/bin/env bash
set -e

GATEWAY_URL=${1:-"http://localhost"}
echo "=================================================================="
echo "      EdgeLink Distributed URL Shortener Cluster Verification     "
echo "=================================================================="
echo "Target Gateway: $GATEWAY_URL"
echo ""

# 1. Health check
echo "[1/4] Checking Load Balancer & Health Endpoint..."
HEALTH_RESP=$(curl -s -f "$GATEWAY_URL/health" || echo "FAIL")
if [[ "$HEALTH_RESP" == *"status"* ]]; then
  echo "  ✓ Gateway healthy: $HEALTH_RESP"
else
  echo "  ✗ Gateway unreachable. Ensure 'docker-compose up' is running."
  exit 1
fi

# 2. Concurrently create 20 links across app1 and app2 to verify zero collisions
echo ""
echo "[2/4] Testing Range Allocator Uniqueness Across Nodes..."
CODES_FILE=$(mktemp)

for i in {1..20}; do
  (
    RESP=$(curl -s -X POST "$GATEWAY_URL/api/shorten" \
      -H "Content-Type: application/json" \
      -d "{\"url\":\"https://example.com/test-page-$i\"}")
    CODE=$(echo "$RESP" | grep -o '"short_code":"[^"]*' | cut -d'"' -f4)
    if [[ -n "$CODE" ]]; then
      echo "$CODE" >> "$CODES_FILE"
    fi
  ) &
done
wait

TOTAL_GENERATED=$(wc -l < "$CODES_FILE" | tr -d ' ')
UNIQUE_GENERATED=$(sort -u "$CODES_FILE" | wc -l | tr -d ' ')
rm -f "$CODES_FILE"

echo "  Total links created concurrently: $TOTAL_GENERATED"
echo "  Unique short codes:               $UNIQUE_GENERATED"
if [[ "$TOTAL_GENERATED" -eq "$UNIQUE_GENERATED" && "$TOTAL_GENERATED" -gt 0 ]]; then
  echo "  ✓ 100% collision-free by design! Zero write collisions."
else
  echo "  ✗ Collision detected! Expected $TOTAL_GENERATED unique codes, got $UNIQUE_GENERATED."
  exit 1
fi

# 3. Test HTTP 302 Found Redirect Semantics
echo ""
echo "[3/4] Verifying HTTP 302 Found Redirect & Cache Header..."
CREATE_RESP=$(curl -s -X POST "$GATEWAY_URL/api/shorten" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://deepmind.google/technologies/gemini/","custom_alias":"gemini-test"}')
CODE="gemini-test"

# Make redirect request and inspect response headers (do not follow redirect)
HEADERS=$(curl -s -I "$GATEWAY_URL/$CODE")
STATUS_LINE=$(echo "$HEADERS" | head -n 1)
LOCATION_LINE=$(echo "$HEADERS" | grep -i "location:" | tr -d '\r')
CACHE_STATUS=$(echo "$HEADERS" | grep -i "x-cache-status:" | tr -d '\r')

echo "  Status Line:   $STATUS_LINE"
echo "  $LOCATION_LINE"
echo "  $CACHE_STATUS"

if [[ "$STATUS_LINE" == *"302"* ]]; then
  echo "  ✓ Correct HTTP 302 Found returned (prevents browser client caching)."
else
  echo "  ✗ Expected HTTP 302 Found, got: $STATUS_LINE"
  exit 1
fi

# 4. Run Cache Stampede Test (Singleflight Defense)
echo ""
echo "[4/4] Executing Cache Stampede Simulator (100 concurrent misses)..."
STAMPEDE_RESP=$(curl -s -X POST "$GATEWAY_URL/api/simulate-stampede" \
  -H "Content-Type: application/json" \
  -d '{"short_code":"gemini-test","concurrency":100}')

echo "  Response: $STAMPEDE_RESP"

if [[ "$STAMPEDE_RESP" == *'"database_hits":1'* ]]; then
  echo "  ✓ SUCCESS: Exactly 1 database query executed! 99 requests coalesced by Go singleflight."
else
  echo "  ✗ Stampede check failed: expected exactly 1 database hit."
  exit 1
fi

echo ""
echo "=================================================================="
echo "    ALL SYSTEM DESIGN CRITERIA VERIFIED AND OPERATIONAL           "
echo "=================================================================="
