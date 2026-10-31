#!/bin/sh
cd "$(dirname "$0")" || exit 1
mkdir -p data
exec ./frpc-mesh --http "${FRPC_MESH_HTTP:-0.0.0.0:18080}" --data "$PWD/data"
