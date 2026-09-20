#!/usr/bin/env bash
#
# Runs all unit tests for the hetty project.
# Usage: ./test.sh

set -euo pipefail

cd "$(dirname "$0")"

echo "==> pkg/scope (rule matching, serialization)"
go test -count=1 -v ./pkg/scope/...

echo "==> pkg/proxy/intercept (request scope matching)"
go test -count=1 -v ./pkg/proxy/intercept/...

echo "==> pkg/api (scope resolvers)"
go test -count=1 ./pkg/api/...

echo "==> all remaining packages"
go test -count=1 ./...

echo "All tests passed."
