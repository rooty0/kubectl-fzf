#!/usr/bin/env bash
# Runs every shell test (zsh unit suite, fish and bash behavior suites and the
# fzf end-to-end) in a pristine container. The completion binary is built for
# linux here, the fixture cache likewise; the container gets both mounted.
#
# Usage: bash shell/tests/run_in_docker.sh
# Needs: docker, go.

set -euo pipefail
scriptDir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repoRoot="$(cd "$scriptDir/../.." && pwd)"
image="kubectl-fzf-shell-tests:local"

docker build -q -t "$image" - >/dev/null <<'DOCKERFILE'
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq \
 && apt-get install -y -qq --no-install-recommends fish zsh tmux fzf ca-certificates \
 && rm -rf /var/lib/apt/lists/*
DOCKERFILE

case "$(uname -m)" in
    arm64|aarch64) GOARCH=arm64 ;;
    x86_64) GOARCH=amd64 ;;
    *) echo "unsupported host arch: $(uname -m)" >&2; exit 1 ;;
esac

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build \
    -o "$scratch/kubectl-fzf-completion" "$repoRoot/cmd/kubectl-fzf-completion"
(cd "$repoRoot" && go run ./shell/tests/fixturegen -out "$scratch/cache")

docker run --rm \
    -v "$repoRoot:/work:ro" \
    -v "$scratch:/scratch" \
    "$image" bash -c '
        set -e
        cd /work
        echo "== zsh behavior suite"
        zsh shell/kubectl_fzf_test.zsh
        echo "== fish behavior suite"
        bash shell/tests/kubectl_fzf_test_fish.sh
        echo "== bash behavior suite"
        bash shell/tests/kubectl_fzf_test_bash.sh
        echo "== end-to-end, real binary driving a real fzf"
        KFZF_E2E_BIN=/scratch/kubectl-fzf-completion \
        KFZF_E2E_KUBECONFIG=/work/shell/tests/fixtures/kubeconfig \
        KFZF_E2E_CACHE=/scratch/cache \
        bash shell/tests/kubectl_fzf_test_e2e.sh
    '
