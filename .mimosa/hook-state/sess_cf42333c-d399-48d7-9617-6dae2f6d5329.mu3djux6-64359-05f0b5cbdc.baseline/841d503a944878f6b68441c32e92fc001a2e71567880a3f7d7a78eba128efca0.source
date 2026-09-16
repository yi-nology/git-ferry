#!/bin/bash
set -e

BASE_URL="http://localhost:8890"

echo "=== Testing Full Flow ==="
echo ""

# 1. Create a repo
echo "1. Creating repo..."
curl -s -X POST "$BASE_URL/api/v1/repo/create" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Repo","remote_url":"https://github.com/test/repo.git","access_token":"test_token"}'
echo ""
echo ""

# 2. List repos
echo "2. Listing repos..."
curl -s "$BASE_URL/api/v1/repos"
echo ""
echo ""

# 3. Create sync task
echo "3. Creating sync task..."
curl -s -X POST "$BASE_URL/api/v1/sync/task/create" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Sync Task",
    "source_repo_key": "test-source",
    "source_branch": "main",
    "target_repo_key": "test-target",
    "target_branch": "main",
    "sync_mode": "single",
    "cron": "0 * * * *",
    "enabled": true,
    "git_tags": false,
    "git_force": true,
    "git_prune": false,
    "git_no_verify": true,
    "push_options": ""
  }'
echo ""
echo ""

# 4. List tasks
echo "4. Listing tasks..."
curl -s "$BASE_URL/api/v1/sync/tasks"
echo ""
echo ""

echo "=== Flow test completed ==="
