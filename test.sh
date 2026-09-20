#!/usr/bin/env bash
#
# Runs all unit tests and static checks for the repository.
#
# Usage:
#   ./test.sh           # build, vet, and run all unit tests
#   ./test.sh -v        # same, with verbose test output
#
set -euo pipefail

cd "$(dirname "$0")"

VERBOSE="${1:-}"

echo "==> go build ./..."
go build ./...

echo "==> go vet ./..."
go vet ./...

echo "==> go test ./... ${VERBOSE}"
go test -count=1 ${VERBOSE} ./...

echo "==> OK: all builds, vet checks and unit tests passed"
