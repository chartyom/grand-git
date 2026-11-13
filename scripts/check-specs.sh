#!/bin/sh
set -eu
CDPATH= cd -- "$(dirname -- "$0")/.."
export OPENSPEC_TELEMETRY=0
export DO_NOT_TRACK=1
npx --yes --package=@fission-ai/openspec@2.0.0 openspec validate --all --strict --no-interactive
