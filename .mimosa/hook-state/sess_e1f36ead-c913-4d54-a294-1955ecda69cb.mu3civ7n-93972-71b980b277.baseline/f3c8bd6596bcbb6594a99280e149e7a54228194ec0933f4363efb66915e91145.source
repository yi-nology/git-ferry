#!/bin/bash
set -e

BASE_URL="http://localhost:8890"

echo "=== Testing Git Sync Service API ==="
echo ""

# Test ping
echo "1. Testing /ping..."
curl -s "$BASE_URL/ping"
echo ""
echo ""

# Test repos list
echo "2. Testing /api/v1/repos..."
curl -s "$BASE_URL/api/v1/repos"
echo ""
echo ""

# Test sync tasks list
echo "3. Testing /api/v1/sync/tasks..."
curl -s "$BASE_URL/api/v1/sync/tasks"
echo ""
echo ""

# Test webhook rules list
echo "4. Testing /api/v1/webhook/rules..."
curl -s "$BASE_URL/api/v1/webhook/rules?repo_key=test"
echo ""
echo ""

# Test repo branches
echo "5. Testing /api/v1/repo/branches..."
curl -s "$BASE_URL/api/v1/repo/branches?key=test"
echo ""
echo ""

echo "=== All tests completed ==="
