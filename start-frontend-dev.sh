#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$SCRIPT_DIR/frontend"

if ! command -v npm >/dev/null 2>&1; then
  echo "Error: npm was not found. Install Node.js 20 or newer and try again." >&2
  exit 1
fi

if [[ ! -f "$FRONTEND_DIR/package.json" ]]; then
  echo "Error: frontend/package.json was not found." >&2
  exit 1
fi

cd "$FRONTEND_DIR"

if [[ ! -d node_modules ]]; then
  echo "Installing frontend dependencies..."
  npm ci
fi

echo "Starting Cedar Discipleship frontend development server..."
echo "Open: http://localhost:5173"
echo "Press Ctrl+C to stop the server."

exec npm run dev -- "$@"
