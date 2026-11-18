#!/usr/bin/env bash
set -euo pipefail

base="${1:-origin/main}"
chart="charts/argocd-mcp"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

render() {
  local dir="$1" out="$2"
  helm dependency build "$dir" >/dev/null
  : >"$out"
  for profile in default embeddings; do
    local args=(--set argocd.baseURL=https://argocd.example.com --set argocd.token=test)
    if [ "$profile" = embeddings ]; then
      args+=(--set embeddings.enabled=true)
    fi
    echo "# ---- profile: $profile ----" >>"$out"
    helm template argocd-mcp "$dir" --namespace argocd-mcp "${args[@]}" >>"$out"
  done
}

mkdir -p "$tmp/base" "$tmp/head"
git archive "$base" "$chart" | tar -x -C "$tmp/base"
cp -R "$chart" "$tmp/head/"
rm -rf "$tmp/head/$(basename "$chart")/charts"

render "$tmp/base/$chart" "$tmp/base.yaml"
render "$tmp/head/$(basename "$chart")" "$tmp/head.yaml"

diff -u --label "$base" --label "HEAD" "$tmp/base.yaml" "$tmp/head.yaml" || true
