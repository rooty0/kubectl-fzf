#!/usr/bin/env bash
# Drives the fish plugin inside real interactive fish sessions (through tmux)
# and checks what it leaves on the command line. The completion binary is
# replaced by a stub so the cases stay hermetic: no cluster, no cache, no fzf.
#
# Usage: bash shell/tests/kubectl_fzf_test_fish.sh
# Needs: tmux, fish.

set -u

scriptDir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pluginFile="$scriptDir/../kubectl_fzf.fish"
workDir="$(mktemp -d)"
failures=0
testCount=0
SES="kfzffish$$"

cleanup() {
    tmux kill-session -t "$SES" 2>/dev/null
    rm -rf "$workDir"
}
trap cleanup EXIT

# The stub records how it was called and answers with a canned response, so
# each case can drive the plugin down a chosen path.
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

# Completion falls back to the previous Tab binding in several cases. A marker
# binding the harness can watch keeps that deterministic, whatever the host
# fishes around it would complete.
write_fish_cfg() {
    local cfg="$1"
    mkdir -p "$cfg/fish/conf.d"
    cat >"$cfg/fish/conf.d/harness.fish" <<RC
function fish_prompt; echo -n 'KFZFREADY> '; end
function __kfzf_report
    commandline -b >"$workDir/buffer"
    commandline -C >"$workDir/cursor"
end
bind \cx __kfzf_report
alias k kubectl
RC
    if [[ "${KFZF_MARKER_FALLBACK:-}" == 1 ]]; then
        cat >>"$cfg/fish/conf.d/harness.fish" <<RC
function __kfzf_marker_fallback
    echo called >"$workDir/fallback"
end
bind \t __kfzf_marker_fallback
RC
    fi
    case "${KFZF_RC_VARIANT:-}" in
        source-twice)
            echo "source '$pluginFile'" >>"$cfg/fish/conf.d/zzy_plug.fish"
            echo "source '$pluginFile'" >>"$cfg/fish/conf.d/zzy_plug2.fish"
            ;;
        custom-tab)
            echo "bind \t 'echo custom'" >>"$cfg/fish/conf.d/customtab.fish"
            echo "source '$pluginFile'" >>"$cfg/fish/conf.d/zzy_plug.fish"
            ;;
        stole-tab)
            # no user binding before the plugin: fish's preset complete is what
            # gets captured
            echo "source '$pluginFile'" >>"$cfg/fish/conf.d/zzy_plug.fish"
            ;;
        *)
            echo "source '$pluginFile'" >>"$cfg/fish/conf.d/zzy_plug.fish"
            ;;
    esac
    echo "echo \$kubectl_fzf_default_completion >'$workDir/captured'" \
        >>"$cfg/fish/conf.d/zzz_capture.fish"
}

# press_tab <buffer> [reportNeeded] [leftMoves]: types the buffer, walks the
# cursor back over leftMoves characters, presses Tab and prints the line left
# in the buffer.
press_tab() {
    local buffer="$1"
    local reportNeeded="${2:-1}"
    local leftMoves="${3:-0}"
    local cfg="$workDir/cfg" i waited

    rm -f "$workDir/buffer" "$workDir/args" "$workDir/cursor" "$workDir/fallback"
    rm -rf "$cfg"
    write_fish_cfg "$cfg"

    tmux kill-session -t "$SES" 2>/dev/null
    tmux new-session -d -s "$SES" -x 200 -y 50 \
        "env XDG_CONFIG_HOME='$cfg' STUB_ARGS_FILE='$workDir/args' STUB_RESPONSE_FILE='$workDir/response' STUB_EXIT_FILE='$workDir/exit' KUBECTL_FZF_COMPLETION_BIN='$workDir/stub' fish"

    waited=0
    while (( waited < 100 )); do
        tmux capture-pane -t "$SES" -p 2>/dev/null | grep -q 'KFZFREADY>' && break
        sleep 0.05; (( waited++ ))
    done
    if (( waited >= 100 )); then
        echo "harness: fish never prompted"
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

# check_line <description> <buffer> <expected line>
check_line() {
    local description="$1" buffer="$2" expected="$3" got
    (( testCount++ ))
    got=$(press_tab "$buffer") || { fail "$description" "$got"; return; }
    # the report file has no trailing newline, so $() keeps meaningful spaces
    if [[ "$got" == "$expected" ]]; then
        echo "ok   $description"
    else
        fail "$description" "buffer:   '$buffer'" "expected: '$expected'" "got:      '$got'"
    fi
}

# check_line_at <description> <left of cursor> <right of cursor> <expected
# left> <expected right>: cursor position afterwards is part of the contract.
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

# check_args / check_args_at <description> <buffer parts> <expected argv dump>
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

# check_not_called <description> <buffer>: the line belongs to something other
# than kubectl, the binary must not hear about it at all.
check_not_called() {
    local description="$1" buffer="$2"
    (( testCount++ ))
    press_tab "$buffer" 0 >/dev/null || { fail "$description" "harness failure"; return; }
    if [[ -e "$workDir/args" ]]; then
        fail "$description" "buffer: '$buffer'" \
            "the completion binary was called with: '$(cat "$workDir/args" | paste -sd' ' -)'"
    else
        echo "ok   $description"
    fi
}

# check_fallback <description> <buffer>: the binary declined, so the line goes
# to the completion that held Tab before, exactly as typed.
check_fallback() {
    local description="$1" buffer="$2" got harnessRc
    (( testCount++ ))
    KFZF_MARKER_FALLBACK=1 press_tab "$buffer" && harnessRc=0 || harnessRc=$?
    if (( harnessRc )); then
        fail "$description" "harness failure"
        return
    fi
    if [[ ! -e "$workDir/fallback" ]]; then
        fail "$description" "buffer: '$buffer'" \
            "the widget never reached the previous Tab completion"
        return
    fi
    got=$(cat "$workDir/buffer")
    if [[ "$got" != "$buffer" ]]; then
        fail "$description" "the line was altered on the way to the fallback" \
            "expected: '$buffer'" "got:      '$got'"
        return
    fi
    echo "ok   $description"
}

# check_captured <variant> <expected captured fallback>
check_captured() {
    local description="$1" variant="$2" expected="$3" got
    (( testCount++ ))
    KFZF_RC_VARIANT="$variant" press_tab "kubectl get pods " 0 >/dev/null || { fail "$description" "harness failure"; return; }
    got=$(cat "$workDir/captured")
    if [[ "$got" == "$expected" ]]; then
        echo "ok   $description"
    else
        fail "$description" "expected captured fallback '$expected'" "got '$got'"
    fi
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

# -A is consumed once the selection pins a namespace: kubectl refuses to
# retrieve a resource by name across all namespaces.
respond "completion=web-server-1 -n web" "remove-word=-A"
check_line "-A is dropped when the completion pins a namespace" \
    "kubectl get pods -A " \
    "kubectl get pods web-server-1 -n web "

respond "completion=web-server-1" "remove-word=--all-namespaces"
check_line "the long form is dropped too, with no namespace suffix" \
    "kubectl get pods --all-namespaces " \
    "kubectl get pods web-server-1 "

respond "completion=web-server-1 -n web"
check_line "without a removal the rest of the line is untouched" \
    "kubectl get pods -A " \
    "kubectl get pods -A web-server-1 -n web "

respond "completion=tier=control-plane"
check_line "a selector value keeps -A" \
    "kubectl get pods -A -l " \
    "kubectl get pods -A -l tier=control-plane "

# The word to drop is named by the binary, so unrelated flags survive.
respond "completion=mypod -n web" "remove-word=-A"
check_line "only the reported word is dropped" \
    "kubectl get pods -A --context minikube " \
    "kubectl get pods --context minikube mypod -n web "

respond "completion=mypod -n web"
check_args "the words are handed over one argv entry each" \
    "kubectl get pods -A " \
    "[k8s_completion] [--protocol=2] [--cursor=3] [--] [get] [pods] [-A] []"

# The point of the words protocol: a quoted value keeps its space and arrives
# as one argument, with the quoting stripped the way kubectl would see it.
respond "completion=mypod -n web"
check_args "a quoted value holding a space stays one argument" \
    "kubectl get pods -l \"app=my app\" " \
    "[k8s_completion] [--protocol=2] [--cursor=4] [--] [get] [pods] [-l] [app=my app] []"

# ... and the line keeps the quotes the user typed.
respond "completion=mypod -n web"
check_line "a quoted value is written back as typed" \
    "kubectl get pods -l \"app=my app\" " \
    "kubectl get pods -l \"app=my app\" mypod -n web "

# Only the last command on the line is completed, the rest is left alone.
respond "completion=mypod -n web" "remove-word=-A"
check_line "the command after a pipe is completed, prefix untouched" \
    "cat f.yaml | kubectl get pods -A " \
    "cat f.yaml | kubectl get pods mypod -n web "

respond "completion=mypod -n web"
check_line "spacing before the pipe is not reflowed" \
    "cat  f.yaml   |kubectl get pods " \
    "cat  f.yaml   |kubectl get pods mypod -n web "

respond "completion=mypod -n web"
check_args "only the last command is sent to the binary" \
    "cat f.yaml | kubectl get pods " \
    "[k8s_completion] [--protocol=2] [--cursor=2] [--] [get] [pods] []"

# A command that is not kubectl is none of our business, whatever precedes it.
respond "completion=should-not-be-used"
check_not_called "a non-kubectl command after a pipe is left to the shell" \
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
check_fallback "exit code 6 leaves the line to the default completion" \
    "kubectl get frobnicate "

respond_exit 6
check_fallback "a declined flag value goes to the shell's own completion" \
    "kubectl get pods --sort-by "

respond_exit 5
check_fallback "finding nothing hands the line over the same way" \
    "kubectl get pods "

respond_exit 6
check_fallback "a declined value after a pipe keeps the prefix" \
    "cat f.yaml | kubectl get pods --sort-by "

# Which side owns the position is decided in the binary, from the flag name, so
# the flag and the word being started both have to arrive intact.
respond "completion=prod"
check_args "the flag governing the position is reported with it" \
    "kubectl get pods -n kube-system --context " \
    "[k8s_completion] [--protocol=2] [--cursor=5] [--] [get] [pods] [-n] [kube-system] [--context] []"

# Completing the flag itself is not completing its value.
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

respond "completion=mypod -n web" "remove-word=-A"
check_line "a boolean flag still completes a resource" \
    "kubectl get pods -A " \
    "kubectl get pods mypod -n web "

respond "completion=mypod"
check_line "a value already given leaves the next word to us" \
    "kubectl get pods --context minikube " \
    "kubectl get pods --context minikube mypod "

# Completing wherever the cursor happens to be, not only at the end of line.
respond "completion=scaledobjects.keda.sh"
check_line_at "a resource type is completed in front of a namespace flag" \
    "kubectl get sca" " -n kube-system" \
    "kubectl get scaledobjects.keda.sh" " -n kube-system"

respond "completion=scaledobjects.keda.sh"
check_args_at "the words behind the cursor are sent, and the cursor with them" \
    "kubectl get sca" " -n kube-system" \
    "[k8s_completion] [--protocol=2] [--cursor=1] [--] [get] [sca] [-n] [kube-system]"

respond "completion=coredns-64897985d-nrblm"
check_line_at "the whole word under the cursor is replaced, not just its start" \
    "kubectl get pods cor" "edns" \
    "kubectl get pods coredns-64897985d-nrblm " ""

respond "completion=coredns-1"
check_line_at "a command after a pipe is completed in the middle of the line" \
    "cat f.yaml |kubectl get pods cor" " -n kube-system" \
    "cat f.yaml |kubectl get pods coredns-1" " -n kube-system"

# Removing a word means writing the command out again, so the words behind the
# cursor have to come through that intact.
respond "completion=mypod -n web" "remove-word=-A"
check_line_at "-A is dropped with the line continuing behind the cursor" \
    "kubectl get pods -A " " -o yaml" \
    "kubectl get pods mypod -n web" " -o yaml"

respond "completion=mypod -n web" "remove-word=-A"
check_line_at "a removal after a pipe keeps what came before it" \
    "cat f.yaml | kubectl get pods -A " " -o yaml" \
    "cat f.yaml | kubectl get pods mypod -n web" " -o yaml"

# A cursor sitting against the next word is on that word.
respond "completion=mypod"
check_line_at "the word the cursor abuts is the one completed" \
    "kubectl get pods " "-o yaml" \
    "kubectl get pods mypod" " yaml"

# The verb belongs to kubectl's own completion, wherever the cursor sits.
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

# The Tab binding that was in place has to be remembered. These are load-time
# properties observed through the fallback the plugin captured.
KFZF_MARKER_FALLBACK=1 check_captured "a custom Tab binding is kept as the fallback" \
    "" "__kfzf_marker_fallback"
check_captured "a code-style Tab binding is kept whole as the fallback" \
    "custom-tab" "echo custom"
check_captured "sourcing twice does not capture the plugin itself" \
    "source-twice" "complete"
check_captured "with no earlier user binding the fish preset is kept" \
    "stole-tab" "complete"

echo
echo "$((testCount - failures))/$testCount passed"
exit $(( failures > 0 ))
