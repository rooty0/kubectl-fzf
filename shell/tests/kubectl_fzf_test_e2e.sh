#!/usr/bin/env bash
# End-to-end: the real kubectl-fzf-completion binary, a real fzf on a real
# terminal, in each supported shell. The cache fixture stands in for a cluster
# (fixturegen wrote it; fixtures/kubeconfig points the binary at it).
#
# Usage: bash shell/tests/kubectl_fzf_test_e2e.sh
# Needs: tmux, fzf, fish, zsh, and KFZF_E2E_BIN pointing at a
# kubectl-fzf-completion built for this platform; fixtures reachable at
# $KFZF_E2E_KUBECONFIG and $KFZF_E2E_CACHE.

set -u

scriptDir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bin="${KFZF_E2E_BIN:-kubectl-fzf-completion}"
kubeconfig="${KFZF_E2E_KUBECONFIG:-$scriptDir/fixtures/kubeconfig}"
cache="${KFZF_E2E_CACHE:?KFZF_E2E_CACHE must point at the fixturegen cache root}"

workDir="$(mktemp -d)"
failures=0
testCount=0
SES="kfzfe2e$$"
trap 'tmux kill-session -t "$SES" 2>/dev/null; rm -rf "$workDir"' EXIT

fail() {
    echo "FAIL $1"
    shift
    local detail
    for detail in "$@"; do
        echo "     $detail"
    done
    (( failures++ ))
}

write_fish_cfg() {
    local cfg="$1"
    mkdir -p "$cfg/fish/conf.d"
    cat >"$cfg/fish/conf.d/harness.fish" <<RC
function fish_prompt; echo -n 'KFZFREADY> '; end
function __kfzf_report
    commandline -b >"$workDir/buffer"
end
bind \cx __kfzf_report
RC
    echo "source '$scriptDir/../kubectl_fzf.fish'" >>"$cfg/fish/conf.d/plugin.fish"
}

write_zsh_rc() {
    local rc="$1"
    cat >"$rc" <<RC
PS1='KFZFREADY> ' PS2='' RPS1=''
source '$scriptDir/../kubectl_fzf.plugin.zsh'
__kfzf_report() { print -r -- "\$BUFFER" >'$workDir/buffer' }
zle -N __kfzf_report
bindkey -e
bindkey '^X^R' __kfzf_report
RC
}

write_bash_rc() {
    local rc="$1"
    cat >"$rc" <<RC
PS1='KFZFREADY> '
source '$scriptDir/../kubectl_fzf.bash'
__kfzf_report() { printf '%s' "\$READLINE_LINE" >'$workDir/buffer'; }
bind -x '"\C-x": __kfzf_report'
RC
}

# e2e_case <shell> <shell-specific session command> <typed query for fzf>
e2e_case() {
    local shellName="$1" sessionCmd="$2" filter="$3" expected="$4" got waited
    (( testCount++ ))
    rm -f "$workDir/buffer"

    tmux kill-session -t "$SES" 2>/dev/null
    tmux new-session -d -s "$SES" -x 200 -y 50 "$sessionCmd"

    waited=0
    while (( waited < 100 )); do
        tmux capture-pane -t "$SES" -p 2>/dev/null | grep -q 'KFZFREADY>' && break
        sleep 0.05; (( waited++ ))
    done
    if (( waited >= 100 )); then
        tmux capture-pane -t "$SES" -p | tail -5
        fail "an fzf pick in $shellName completes the line" "shell never prompted"
        return
    fi

    tmux send-keys -t "$SES" -l -- "kubectl get pods "
    sleep 0.4
    tmux send-keys -t "$SES" Tab
    sleep 1.2
    if ! tmux capture-pane -t "$SES" -p | grep -q 'web-server-1'; then
        tmux capture-pane -t "$SES" -p | tail -8
        tmux kill-session -t "$SES" 2>/dev/null
        fail "an fzf pick in $shellName completes the line" "fzf never opened or the fixture pods are missing"
        return
    fi
    tmux send-keys -t "$SES" -l -- "$filter"
    sleep 0.4
    tmux send-keys -t "$SES" Enter
    sleep 1
    if [[ "$shellName" == zsh ]]; then
        tmux send-keys -t "$SES" C-x C-r
    else
        tmux send-keys -t "$SES" C-x
    fi
    waited=0
    while (( waited < 60 )); do
        [[ -s "$workDir/buffer" ]] && break
        sleep 0.05; (( waited++ ))
    done
    tmux kill-session -t "$SES" 2>/dev/null

    got=$(cat "$workDir/buffer" 2>/dev/null)
    if [[ "$got" == "$expected" ]]; then
        echo "ok   an fzf pick in $shellName completes the line"
    else
        fail "an fzf pick in $shellName completes the line" \
            "expected: '$expected'" "got:      '$got'"
    fi
}

fish_cfg="$workDir/fishcfg"
write_fish_cfg "$fish_cfg"
# zsh reads $ZDOTDIR/.zshrc
write_zsh_rc "$workDir/.zshrc"
bash_rc="$workDir/bashrc"; write_bash_rc "$bash_rc"

ENV="env KUBECONFIG='$kubeconfig' KUBECTL_FZF_CACHE_DIR='$cache' KUBECTL_FZF_COMPLETION_BIN='$bin'"
e2e_case fish "$ENV XDG_CONFIG_HOME='$fish_cfg' fish" "web-server-1" "kubectl get pods web-server-1 "
e2e_case zsh "$ENV ZDOTDIR='$workDir' zsh" "web-server-1" "kubectl get pods web-server-1 "
e2e_case bash "$ENV bash --rcfile '$bash_rc' -i" "web-server-1" "kubectl get pods web-server-1 "

printf '\n%d/%d passed\n' $((testCount - failures)) "$testCount"
exit $(( failures > 0 ))
