#!/bin/bash
set -euo pipefail

# Tenant SQL Isolation Checker
# Scans internal/store/pg/*.go for SQL queries on tenant-scoped tables
# and verifies they include a tenant_id filter.

STORE_DIR="internal/store/pg"
TENANT_TABLES="sessions|messages|users|audit_logs|api_keys|memories|user_profiles|cron_jobs|roles|role_permissions"
SKIP_FILES="migrate.go|pg.go"

ERRORS=0

for file in "$STORE_DIR"/*.go; do
  basename=$(basename "$file")

  # Skip non-query files
  if echo "$basename" | grep -qE "^($SKIP_FILES)$"; then
    continue
  fi

  # Extract SQL strings (backtick-delimited) and check each
  # Look for queries that reference tenant-scoped tables
  while IFS= read -r line_num; do
    line=$(sed -n "${line_num}p" "$file")

    # Check if this SQL line references a tenant-scoped table
    if echo "$line" | grep -qiE "FROM\s+($TENANT_TABLES)|INTO\s+($TENANT_TABLES)|UPDATE\s+($TENANT_TABLES)|DELETE\s+FROM\s+($TENANT_TABLES)|JOIN\s+($TENANT_TABLES)"; then

      # role_permissions joins roles (which has tenant_id) — skip direct check
      if echo "$line" | grep -qiE "role_permissions" && echo "$basename" | grep -q "roles"; then
        continue
      fi

      # Check for skip marker in the 5 lines above the query
      skip_context=$(sed -n "$((line_num > 5 ? line_num - 5 : 1)),${line_num}p" "$file")
      if echo "$skip_context" | grep -q "tenant_sql_check:skip"; then
        continue
      fi

      # Read the full SQL context (this line + next 15 lines) to check for tenant_id
      context=$(sed -n "${line_num},$((line_num + 15))p" "$file")
      if ! echo "$context" | grep -q "tenant_id"; then
        echo "WARNING: $file:$line_num — SQL on tenant-scoped table may lack tenant_id filter:"
        echo "  $line"
        ERRORS=$((ERRORS + 1))
      fi
    fi
  done < <(grep -nE "FROM\s|INTO\s|UPDATE\s|DELETE\s|JOIN\s" "$file" | cut -d: -f1)
done

if [ "$ERRORS" -gt 0 ]; then
  echo ""
  echo "FAIL: Found $ERRORS potential tenant isolation gaps."
  exit 1
else
  echo "OK: All SQL queries on tenant-scoped tables include tenant_id filter."
  exit 0
fi
