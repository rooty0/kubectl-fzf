#!/usr/bin/env bash
# Drives the bash completion inside real interactive bash sessions (through
# tmux) and checks what it leaves on the command line. The completion binary
# is replaced by a stub so the cases stay hermetic: no cluster, no cache, no
# fzf.
#
# Usage: bash shell/tests/kubectl_fzf_test_bash.sh (in bash where tmux exists)
# Needs: tmux, bash >= 5.
#
# A known divergence from the zsh/fish plugins, called out rather than hidden:
# when the binary wants a word dropped from the line (-A next to a namespaced
# name), this plugin hands the line to kubectl's own completion. The
# programmable-completion API can only replace the current word, it cannot
# rewrite the command line.

set -u

scriptDir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pluginFile="$scriptDir/../kubectl_fzf.bash"
workDir="$(mktemp -d)"
failures=0
testCount=0
SES="kfzfbash$$"

cleanup() {
    tmux kill-session -t "$SES" 2>/dev/null
    rm -rf "$workDir"
}
trap cleanup EXIT

cat >"$workDir/stub" <<'STUB'
#!/bin/sh
# One argument per line, bracketed, so an argument holding a space is told
# apart from two arguments and a trailing empty one is not lost.
for arg in "$@"; do printf '[%s]\n' "$arg"; done >"$STUB_ARGS_FILE"
if [ -s "$STUB_EXIT_FILE" ]; then
    exit "$(cat "$STUB_EXIT_FILE")"
fi
cat "$STUB_RESPONSE_FILE"
STUB
chmod +x "$workDir/stub"

write_bash_rc() {
    local rc="$1"
    cat >"$rc" <<RC
PS1='KFZFREADY> '
export STUB_ARGS_FILE='$workDir/args'
export STUB_RESPONSE_FILE='$workDir/response'
export STUB_EXIT_FILE='$workDir/exit'
KUBECTL_FZF_COMPLETION_BIN='$workDir/stub'
alias k=kubectl
__kfzf_report() {
    printf '%s' "\$READLINE_LINE" >'$workDir/buffer'
    printf '%s' "\$READLINE_POINT" >'$workDir/cursor'
}
bind -x '"\C-x": __kfzf_report'
RC
    if [[ "${KFZF_MARKER_FALLBACK:-}" == 1 ]]; then
        # stands in for kubectl's own completion: wins only when delegated to
        echo "__start_kubectl() { echo called >'$workDir/fallback'; COMPREPLY=(); }" >>"$rc"
    fi
    if [[ -n "${KFZF_DEFAULT_COMPLETION_FN:-}" ]]; then
        # a differently-named fallback the user points at from outside
        echo "$KFZF_DEFAULT_COMPLETION_FN() { echo called >'$workDir/fallback'; COMPREPLY=(); }" >>"$rc"
        echo "KUBECTL_FZF_DEFAULT_COMPLETION='$KFZF_DEFAULT_COMPLETION_FN'" >>"$rc"
    fi
    echo "source '$pluginFile'" >>"$rc"
}

press_tab() {
    local buffer="$1"
    local reportNeeded="${2:-1}"
    local leftMoves="${3:-0}"
    local rc="$workDir/bashrc" i waited

    rm -f "$workDir/buffer" "$workDir/args" "$workDir/cursor" "$workDir/fallback"
    write_bash_rc "$rc"

    tmux kill-session -t "$SES" 2>/dev/null
    tmux new-session -d -s "$SES" -x 200 -y 50 "bash --rcfile '$rc' -i"

    waited=0
    while (( waited < 100 )); do
        tmux capture-pane -t "$SES" -p 2>/dev/null | grep -q 'KFZFREADY>' && break
        sleep 0.05; (( waited++ ))
    done
    if (( waited >= 100 )); then
        echo "harness: bash never prompted"
        tmux kill-session -t "$SES" 2>/dev/null
        return 1
    fi

    tmux send-keys -t "$SES" -l -- "$buffer"
    sleep 0.4
    for (( i=0; i<leftMoves; i++ )); do
        tmux send-keys -t "$SES" Left
        sleep 0.05
    done
    tmux send-keys -t "$SES" Tab
    sleep 0.6
    tmux send-keys -t "$SES" C-x

    waited=0
    while (( waited < 60 )); do
        [[ -s "$workDir/buffer" ]] && break
        sleep 0.05; (( waited++ ))
    done
    tmux kill-session -t "$SES" 2>/dev/null

    if [[ ! -e "$workDir/buffer" ]]; then
        (( reportNeeded )) || return 0
        echo "harness: the report key never wrote a buffer"
        return 1
    fi
    cat "$workDir/buffer"
}

fail() {
    echo "FAIL $1"
    shift
    local detail
    for detail in "$@"; do
        echo "     $detail"
    done
    (( failures++ ))
}

check_line() {
    local description="$1" buffer="$2" expected="$3" got
    (( testCount++ ))
    got=$(press_tab "$buffer") || { fail "$description" "$got"; return; }
    if [[ "$got" == "$expected" ]]; then
        echo "ok   $description"
    else
        fail "$description" "buffer:   '$buffer'" "expected: '$expected'" "got:      '$got'"
    fi
}

check_line_at() {
    local description="$1" left="$2" right="$3" wantLeft="$4" wantRight="$5" got
    local gotCursor
    (( testCount++ ))
    got=$(press_tab "$left$right" 1 "${#right}") || { fail "$description" "$got"; return; }
    gotCursor=$(cat "$workDir/cursor")
    if [[ "$got" == "$wantLeft$wantRight" && "$gotCursor" == "${#wantLeft}" ]]; then
        echo "ok   $description"
    else
        fail "$description" "typed:    '$left<cursor>$right'" \
            "expected: '$wantLeft<cursor>$wantRight'" \
            "got:      '${got:0:gotCursor}<cursor>${got:gotCursor}'"
    fi
}

check_args() {
    local description="$1" buffer="$2" expected="$3" got
    (( testCount++ ))
    press_tab "$buffer" 0 >/dev/null || { fail "$description" "harness failure"; return; }
    if [[ ! -e "$workDir/args" ]]; then
        fail "$description" "buffer: '$buffer'" "the completion binary was never called"
        return
    fi
    got=$(paste -sd' ' "$workDir/args")
    if [[ "$got" == "$expected" ]]; then
        echo "ok   $description"
    else
        fail "$description" "expected argv: '$expected'" "got argv:      '$got'"
    fi
}

check_args_at() {
    local description="$1" left="$2" right="$3" expected="$4" got
    (( testCount++ ))
    press_tab "$left$right" 0 "${#right}" >/dev/null || { fail "$description" "harness failure"; return; }
    if [[ ! -e "$workDir/args" ]]; then
        fail "$description" "line: '$left<cursor>$right'" "the completion binary was never called"
        return
    fi
    got=$(paste -sd' ' "$workDir/args")
    if [[ "$got" == "$expected" ]]; then
        echo "ok   $description"
    else
        fail "$description" "expected argv: '$expected'" "got argv:      '$got'"
    fi
}

check_not_called() {
    local description="$1" buffer="$2"
    (( testCount++ ))
    press_tab "$buffer" 0 >/dev/null || { fail "$description" "harness failure"; return; }
    if [[ -e "$workDir/args" ]]; then
        fail "$description" "buffer: '$buffer'" \
            "the completion binary was called with: '$(paste -sd' ' "$workDir/args")'"
    else
        echo "ok   $description"
    fi
}

check_fallback() {
    local description="$1" buffer="$2" harnessRc
    local got=""
    (( testCount++ ))
    if KFZF_MARKER_FALLBACK=1 press_tab "$buffer" >/dev/null; then harnessRc=0; else harnessRc=$?; fi
    if (( harnessRc )); then
        fail "$description" "harness failure"
        return
    fi
    if [[ ! -e "$workDir/fallback" ]]; then
        fail "$description" "buffer: '$buffer'" \
            "the completion never reached kubectl's own completion"
        return
    fi
    if [[ -e "$workDir/buffer" ]]; then
        got=$(cat "$workDir/buffer")
    fi
    if [[ "$got" != "$buffer" ]]; then
        fail "$description" "the line was altered on the way to the fallback" \
            "expected: '$buffer'" "got:      '$got'"
        return
    fi
    echo "ok   $description"
}

respond() {
    printf '%s\n' "$@" >"$workDir/response"
    : >"$workDir/exit"
}

respond_exit() {
    : >"$workDir/response"
    printf '%s\n' "$1" >"$workDir/exit"
}
: >"$workDir/exit"; : >"$workDir/response"

# A completion that would need -A gone cannot be applied: progcomp cannot
# rewrite the line, so the position goes to kubectl's own completion instead.
respond "completion=web-server-1 -n web" "remove-word=-A"
check_fallback "a completion wanting -A gone defers to kubectl" \
    "kubectl get pods -A "

respond "completion=web-server-1 -n web"
check_line "a plain completion is spliced over the empty word" \
    "kubectl get pods " \
    "kubectl get pods web-server-1 -n web "

respond "completion=tier=control-plane"
check_line "a selector value keeps -A" \
    "kubectl get pods -A -l " \
    "kubectl get pods -A -l tier=control-plane "

respond "completion=mypod -n web"
check_args "the words are handed over one argv entry each" \
    "kubectl get pods -A " \
    "[k8s_completion] [--protocol=2] [--cursor=3] [--] [get] [pods] [-A] []"

# A quoted value keeps its space and arrives as one argument.
respond "completion=mypod -n web"
check_args "a quoted value holding a space stays one argument" \
    "kubectl get pods -l \"app=my app\" " \
    "[k8s_completion] [--protocol=2] [--cursor=4] [--] [get] [pods] [-l] [app=my app] []"

respond "completion=mypod -n web"
check_line "a quoted value is written back as typed" \
    "kubectl get pods -l \"app=my app\" " \
    "kubectl get pods -l \"app=my app\" mypod -n web "

# Only the pipeline segment under the cursor counts.
respond "completion=mypod -n web"
check_line "the command after a pipe is completed, prefix untouched" \
    "cat f.yaml | kubectl get pods " \
    "cat f.yaml | kubectl get pods mypod -n web "

respond "completion=mypod -n web"
check_args "only the last command is sent to the binary" \
    "cat f.yaml | kubectl get pods " \
    "[k8s_completion] [--protocol=2] [--cursor=2] [--] [get] [pods] []"

respond "completion=mypod -n web"
check_line "spacing before the pipe is not reflowed" \
    "cat  f.yaml   |kubectl get pods " \
    "cat  f.yaml   |kubectl get pods mypod -n web "

respond "completion=should-not-be-used"
check_not_called "a non-kubectl command after a pipe is left alone" \
    "kubectl get pods -oyaml|gre"

respond "completion=should-not-be-used"
check_not_called "an operator inside quotes does not split the command" \
    "grep -l 'a|b' "

# A command substitution the user merely typed must not run on a Tab press.
respond "completion=web-server-1 -n web"
check_line 'a typed $(...) is passed through inertly' \
    "kubectl get pods -l \"\$(touch $workDir/pwned)\" " \
    "kubectl get pods -l \"\$(touch $workDir/pwned)\" web-server-1 -n web "
(( testCount++ ))
if [[ -e "$workDir/pwned" ]]; then
    fail "the typed command substitution must not be executed"
else
    echo "ok   the typed command substitution was not executed"
fi

respond_exit 6
check_fallback "a declined flag value goes to the shell's own completion" \
    "kubectl get pods --sort-by "

respond_exit 5
check_fallback "finding nothing hands the line over the same way" \
    "kubectl get pods "

respond_exit 6
check_fallback "a declined value after a pipe keeps the prefix" \
    "cat f.yaml | kubectl get pods --sort-by "

respond "completion=prod"
check_args "the flag governing the position is reported with it" \
    "kubectl get pods -n kube-system --context " \
    "[k8s_completion] [--protocol=2] [--cursor=5] [--] [get] [pods] [-n] [kube-system] [--context] []"

# '=' was taken out of the word breaks at source time, so the attached form
# arrives as one word.
respond_exit 6
check_args "the attached form arrives whole" \
    "kubectl get pods --context=" \
    "[k8s_completion] [--protocol=2] [--cursor=2] [--] [get] [pods] [--context=]"

respond "completion=prod"
check_line "a context is completed from the kubeconfig" \
    "kubectl get pods --context " \
    "kubectl get pods --context prod "

respond "completion=prod"
check_line "the namespace the user typed survives the context they pick" \
    "kubectl get pods -n kube-system --context " \
    "kubectl get pods -n kube-system --context prod "

respond "completion=mypod"
check_line "a value already given leaves the next word to us" \
    "kubectl get pods --context minikube " \
    "kubectl get pods --context minikube mypod "

respond "completion=scaledobjects.keda.sh"
check_line_at "a resource type is completed in front of a namespace flag" \
    "kubectl get sca" " -n kube-system" \
    "kubectl get scaledobjects.keda.sh" " -n kube-system"

respond "completion=scaledobjects.keda.sh"
check_args_at "the words behind the cursor are sent, and the cursor with them" \
    "kubectl get sca" " -n kube-system" \
    "[k8s_completion] [--protocol=2] [--cursor=1] [--] [get] [sca] [-n] [kube-system]"

# readline's programmable completion replaces the typed part of the word only;
# what stands right of the cursor is the user's to fix. zsh and fish replace
# the whole word; bash can express neither that nor word removal. Both cases
# are kept here as documentation of the divergence.
respond "completion=coredns-64897985d-nrblm"
check_line_at "mid-token, the typed prefix is completed and the suffix kept" \
    "kubectl get pods cor" "edns" \
    "kubectl get pods coredns-64897985d-nrblm" "edns"

respond "completion=coredns-1"
check_line_at "a command after a pipe is completed in the middle of the line" \
    "cat f.yaml |kubectl get pods cor" " -n kube-system" \
    "cat f.yaml |kubectl get pods coredns-1" " -n kube-system"

respond "completion=mypod"
check_line_at "a word the cursor abuts is completed as an empty prefix" \
    "kubectl get pods " "-o yaml" \
    "kubectl get pods mypod" "-o yaml"

respond "completion=should-not-be-used"
check_not_called "a verb being typed is left to the shell" \
    "kubectl ge"

# An alias to kubectl is completed as kubectl, without being spelled out.
respond "completion=web-server-1"
check_line "an alias is completed without expanding it on the line" \
    "k get pods " \
    "k get pods web-server-1 "

respond "completion=web-server-1"
check_args "an alias is resolved to kubectl for the binary" \
    "k get pods " \
    "[k8s_completion] [--protocol=2] [--cursor=2] [--] [get] [pods] []"

# An empty answer (exit 0, no completion line) touches nothing.
respond ""
check_line "an empty answer leaves the line alone" \
    "kubectl get pods " \
    "kubectl get pods "

# The fallback function is configurable from outside.
respond_exit 6
(( testCount++ ))
if KFZF_MARKER_FALLBACK= KFZF_DEFAULT_COMPLETION_FN=my_kubectl press_tab "kubectl get pods --sort-by " >/dev/null \
    && [[ -e "$workDir/fallback" ]]; then
    echo "ok   the fallback function comes from KUBECTL_FZF_DEFAULT_COMPLETION"
else
    fail "the fallback function comes from KUBECTL_FZF_DEFAULT_COMPLETION" \
        "the configured fallback was never called"
fi

printf '\n%d/%d passed\n' $((testCount - failures)) "$testCount"
exit $(( failures > 0 ))
