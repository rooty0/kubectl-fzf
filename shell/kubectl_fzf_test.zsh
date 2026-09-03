#!/usr/bin/env zsh
# Drives the completion widget inside a real interactive zsh (through zpty) and
# checks what it leaves on the command line. The completion binary is replaced by
# a stub so the cases stay hermetic: no cluster, no cache, no fzf.
#
# Usage: zsh shell/kubectl_fzf_test.zsh

set -u
zmodload zsh/zpty

typeset -g pluginDir="${0:A:h}"
typeset -g workDir
typeset -gi failures=0 testCount=0

workDir=$(mktemp -d)
trap 'zpty -d kfzf 2>/dev/null; rm -rf "$workDir"' EXIT

# The stub records how it was called and answers with a canned response, so each
# case can drive the plugin down a chosen path.
cat >"$workDir/stub" <<'STUB'
#!/usr/bin/env zsh
# One argument per line, bracketed, so an argument holding a space is told apart
# from two arguments and a trailing empty one is not lost.
{ for arg in "$@"; do print -r -- "[$arg]"; done } >"$STUB_ARGS_FILE"
if [[ -s "$STUB_EXIT_FILE" ]]; then
  exit "$(<"$STUB_EXIT_FILE")"
fi
cat "$STUB_RESPONSE_FILE"
STUB
chmod +x "$workDir/stub"

# Completion falls back to the stock zsh completion in several cases. Running
# from an empty directory keeps that from inserting filenames of its own.
mkdir -p "$workDir/empty" "$workDir/zdotdir"

cat >"$workDir/zdotdir/.zshrc" <<RC
# The harness waits for this prompt: typing before zle has drawn it loses input.
PS1='KFZFREADY>' PS2='' RPS1=''
export STUB_ARGS_FILE='$workDir/args'
export STUB_RESPONSE_FILE='$workDir/response'
export STUB_EXIT_FILE='$workDir/exit'
KUBECTL_FZF_COMPLETION_BIN='$workDir/stub'
cd '$workDir/empty'
autoload -Uz compinit && compinit -u -d '$workDir/zcompdump'
# A case can ask for a fallback it can watch. This binding is what the plugin
# picks up as the previous Tab widget, and it records the call while leaving the
# line alone. Without it the fallback lands on whatever completion the host has
# for kubectl, which is not the same on a laptop as it is on CI.
if [[ -n "\${KFZF_MARKER_FALLBACK:-}" ]]; then
  __kfzf_marker_fallback() { print -r -- called >'$workDir/fallback' }
  zle -N __kfzf_marker_fallback
  bindkey '^I' __kfzf_marker_fallback
fi
source '$pluginDir/kubectl_fzf.plugin.zsh'
# Reports the line the widget produced, so the checks never have to parse what
# the terminal painted.
__report_buffer() { print -r -- "<<<\$BUFFER>>>" >'$workDir/buffer' }
zle -N __report_buffer
bindkey '^X^R' __report_buffer
RC

export ZDOTDIR="$workDir/zdotdir"
# zle stays out of the way when the terminal is unknown, and the pty inherits
# whatever the caller had.
export TERM=${TERM:-xterm-256color}

# Consumes pty output until the pattern shows up.
wait_for()
{
  local pattern="$1" seen="" chunk
  local -i waited=0
  while (( waited < 200 )); do
    if zpty -r -t kfzf chunk 2>/dev/null; then
      seen+="$chunk"
      [[ "$seen" == *${~pattern}* ]] && return 0
    else
      sleep 0.05
      (( waited++ ))
    fi
  done
  return 1
}

# press_tab <buffer> [reportNeeded]: types the buffer, presses Tab and prints the
# resulting line. Cases that only look at how the binary was called pass a 0, as
# the stock completion may open a listing that eats the reporting key.
press_tab()
{
  local buffer="$1" out
  local -i reportNeeded=${2:-1} waited=0

  rm -f "$workDir/buffer" "$workDir/args"
  zpty -d kfzf 2>/dev/null
  # -d skips the global rc files: only the harness zshrc should shape the shell.
  zpty kfzf zsh -d -i
  if ! wait_for 'KFZFREADY>'; then
    print -r -- "harness: the interactive shell never prompted"
    return 1
  fi

  zpty -w -n kfzf "$buffer"
  sleep 0.3
  zpty -w -n kfzf $'\t'
  sleep 1
  zpty -w -n kfzf $'\C-X\C-R'
  while (( waited < 60 )) && [[ ! -s "$workDir/buffer" ]]; do
    sleep 0.05
    (( waited++ ))
  done
  zpty -d kfzf 2>/dev/null

  if [[ ! -s "$workDir/buffer" ]]; then
    (( reportNeeded )) || return 0
    print -r -- "harness: the widget never reported a line"
    return 1
  fi
  out=$(<"$workDir/buffer")
  out=${out##*<<<}
  out=${out%%>>>*}
  print -r -- "$out"
}

fail()
{
  print -r -- "FAIL $1"
  shift
  local detail
  for detail in "$@"; do
    print -r -- "     $detail"
  done
  (( failures++ ))
}

# check_line <description> <buffer> <expected line>
check_line()
{
  local description="$1" buffer="$2" expected="$3" got
  (( testCount++ ))
  got=$(press_tab "$buffer") || { fail "$description" "$got"; return; }
  if [[ "$got" == "$expected" ]]; then
    print -r -- "ok   $description"
  else
    fail "$description" "buffer:   '$buffer'" "expected: '$expected'" "got:      '$got'"
  fi
}

# check_not_called <description> <buffer>: the completion binary must not be
# reached at all, the line belongs to something other than kubectl.
check_not_called()
{
  local description="$1" buffer="$2" got
  (( testCount++ ))
  got=$(press_tab "$buffer" 0) || { fail "$description" "$got"; return; }
  if [[ -e "$workDir/args" ]]; then
    fail "$description" "buffer: '$buffer'" \
      "the completion binary was called with: '$(<"$workDir/args")'"
  else
    print -r -- "ok   $description"
  fi
}

# check_args <description> <buffer> <expected argv>
check_args()
{
  local description="$1" buffer="$2" expected="$3" got
  local -a gotArgs
  (( testCount++ ))
  # The reported line is of no interest here, and a case that ends in a fallback
  # may open a listing that eats the reporting key.
  press_tab "$buffer" 0 >/dev/null || { fail "$description" "harness failure"; return; }
  if [[ ! -e "$workDir/args" ]]; then
    fail "$description" "buffer: '$buffer'" "the completion binary was never called"
    return
  fi
  gotArgs=("${(@f)$(<"$workDir/args")}")
  got="${(pj: :)gotArgs}"
  if [[ "$got" == "$expected" ]]; then
    print -r -- "ok   $description"
  else
    fail "$description" "expected argv: '$expected'" "got argv:      '$got'"
  fi
}

# check_fallback <description> <buffer>: the binary declined the position, so the
# widget has to hand the line to the completion that held Tab before it, and hand
# it over exactly as typed.
check_fallback()
{
  local description="$1" buffer="$2" got
  local -i harnessRc
  (( testCount++ ))
  rm -f "$workDir/fallback"
  export KFZF_MARKER_FALLBACK=1
  got=$(press_tab "$buffer")
  harnessRc=$?
  unset KFZF_MARKER_FALLBACK
  if (( harnessRc )); then
    fail "$description" "$got"
    return
  fi
  if [[ ! -e "$workDir/fallback" ]]; then
    fail "$description" "buffer: '$buffer'" \
      "the widget never reached the previous Tab completion"
    return
  fi
  if [[ "$got" != "$buffer" ]]; then
    fail "$description" "the line was altered on the way to the fallback" \
      "expected: '$buffer'" "got:      '$got'"
    return
  fi
  print -r -- "ok   $description"
}

respond()
{
  print -rl -- "$@" >"$workDir/response"
  : >"$workDir/exit"
}

respond_exit()
{
  : >"$workDir/response"
  print -r -- "$1" >"$workDir/exit"
}

# -A is consumed once the selection pins a namespace: kubectl refuses to retrieve
# a resource by name across all namespaces.
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
  "[k8s_completion] [--protocol=2] [--] [get] [pods] [-A] []"

# The point of the words protocol: a quoted value keeps its space and arrives as
# one argument, with the quoting stripped the way kubectl would see it.
respond "completion=mypod -n web"
check_args "a quoted value holding a space stays one argument" \
  "kubectl get pods -l \"app=my app\" " \
  "[k8s_completion] [--protocol=2] [--] [get] [pods] [-l] [app=my app] []"

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
  "[k8s_completion] [--protocol=2] [--] [get] [pods] []"

# A command that is not kubectl is none of our business, whatever precedes it.
respond "completion=should-not-be-used"
check_not_called "a non-kubectl command after a pipe is left to the shell" \
  "kubectl get pods -oyaml|gre"

respond "completion=should-not-be-used"
check_not_called "an operator inside quotes does not split the command" \
  "grep -l 'a|b' "

# A command substitution the user merely typed must not run on a Tab press.
# The payload writes its file through a redirection so it holds no space, which
# keeps it a single word once the plugin re-splits the line.
respond "completion=web-server-1 -n web"
check_line 'a typed $(...) is passed through inertly' \
  "kubectl get pods -l \"\$(>$workDir/pwned)\" " \
  "kubectl get pods -l \"\$(>$workDir/pwned)\" web-server-1 -n web "
(( testCount++ ))
if [[ -e "$workDir/pwned" ]]; then
  fail "the typed command substitution must not be executed"
else
  print -r -- "ok   the typed command substitution was not executed"
fi

respond_exit 6
check_line "exit code 6 leaves the line to the default completion" \
  "kubectl get frobnicate " \
  "kubectl get frobnicate "

# The value of a flag whose candidates kubectl-fzf has no idea about, --sort-by
# for instance, belongs to kubectl's own completion. What the shell has to get
# right is handing that position over untouched.
respond_exit 6
check_fallback "a declined flag value goes to the shell's own completion" \
  "kubectl get pods --sort-by "

# Finding nothing is a different refusal, and it has to end up in the same place.
respond_exit 5
check_fallback "finding nothing hands the line over the same way" \
  "kubectl get pods "

respond_exit 6
check_fallback "a declined value after a pipe keeps the prefix" \
  "cat f.yaml | kubectl get pods --sort-by "

# Which side owns the position is decided in the binary, from the flag name, so
# the flag and the word being started both have to arrive intact. --context used
# to be read as a boolean here, which offered a pod list for a context name.
respond "completion=prod"
check_args "the flag governing the position is reported with it" \
  "kubectl get pods -n kube-system --context " \
  "[k8s_completion] [--protocol=2] [--] [get] [pods] [-n] [kube-system] [--context] []"

# Completing the flag itself is not completing its value, and kubectl knows the
# flag names.
respond_exit 6
check_args "the attached form arrives whole" \
  "kubectl get pods --context=" \
  "[k8s_completion] [--protocol=2] [--] [get] [pods] [--context=]"

# A context is named by the kubeconfig, so it comes back as a bare name: no
# namespace is pinned by picking one, and nothing on the line contradicts it.
respond "completion=prod"
check_line "a context is completed from the kubeconfig" \
  "kubectl get pods --context " \
  "kubectl get pods --context prod "

respond "completion=prod"
check_line "the namespace the user typed survives the context they pick" \
  "kubectl get pods -n kube-system --context " \
  "kubectl get pods -n kube-system --context prod "

# A flag that stands alone leaves a resource position behind it, so that one is
# still ours: the two cases part company on the flag name alone.
respond "completion=mypod -n web" "remove-word=-A"
check_line "a boolean flag still completes a resource" \
  "kubectl get pods -A " \
  "kubectl get pods mypod -n web "

respond "completion=mypod"
check_line "a value already given leaves the next word to us" \
  "kubectl get pods --context minikube " \
  "kubectl get pods --context minikube mypod "

# Splitting the line needs no terminal, so it is checked directly.
source "$pluginDir/kubectl_fzf.plugin.zsh" 2>/dev/null

# check_split <buffer> <expected prefix> <expected command>
check_split()
{
  local buffer="$1" wantPrefix="$2" wantCommand="$3"
  local commandPrefix commandLine
  (( testCount++ ))
  __kubectl_fzf_split_last_command "$buffer"
  if [[ "$commandPrefix" != "$wantPrefix" || "$commandLine" != "$wantCommand" ]]; then
    fail "split of '$buffer'" \
      "expected prefix '$wantPrefix' and command '$wantCommand'" \
      "got      prefix '$commandPrefix' and command '$commandLine'"
    return
  fi
  # Whatever the split decides, the two halves have to be the original line.
  if [[ "${commandPrefix}${commandLine}" != "$buffer" ]]; then
    fail "split of '$buffer' does not reassemble into the original line"
    return
  fi
  print -r -- "ok   split of '$buffer'"
}

check_split 'kubectl get pods ' '' 'kubectl get pods '
check_split 'k get pods -oyaml|gre' 'k get pods -oyaml|' 'gre'
check_split 'cat f.yaml | k apply -f ' 'cat f.yaml | ' 'k apply -f '
check_split 'cat f.yaml |k get pods ' 'cat f.yaml |' 'k get pods '
check_split 'k get pods && k get svc ' 'k get pods && ' 'k get svc '
check_split 'k get pods ; k get svc ' 'k get pods ; ' 'k get svc '
check_split 'a|b|k get pods ' 'a|b|' 'k get pods '
check_split 'k get pods > out.tx' 'k get pods > ' 'out.tx'
check_split 'k get pods 2>/dev/null ' 'k get pods 2>' '/dev/null '
check_split 'k get pods |' 'k get pods |' ''
# An operator inside quotes is part of a value, not a command boundary.
check_split 'k get pods -l "a|b" ' '' 'k get pods -l "a|b" '
check_split "k get pods -l 'x;y' " '' "k get pods -l 'x;y' "
# Spacing the user typed has to survive untouched.
check_split 'k   get   pods ' '' 'k   get   pods '
check_split '' '' ''

# The Tab binding that was in place has to be remembered, otherwise sourcing the
# plugin silently throws away a widget the user had set up. Also a load time
# property, so no terminal needed either.
# check_previous_binding <description> <setup> <expected captured widget>
check_previous_binding()
{
  local description="$1" setup="$2" expected="$3" got
  (( testCount++ ))
  got=$(zsh -f -c "
    zmodload zsh/zle 2>/dev/null
    $setup
    source '$pluginDir/kubectl_fzf.plugin.zsh'
    print -r -- \"\$kubectl_fzf_default_completion\"")
  if [[ "$got" == "$expected" ]]; then
    print -r -- "ok   $description"
  else
    fail "$description" "expected captured widget '$expected'" "got '$got'"
  fi
}

check_previous_binding "a custom Tab widget is kept as the fallback" \
  'my_tab_widget() { : }; zle -N my_tab_widget 2>/dev/null; bindkey "^I" my_tab_widget' \
  'my_tab_widget'
check_previous_binding "the stock Tab widget is kept as the fallback" \
  'bindkey "^I" expand-or-complete' \
  'expand-or-complete'
# Nothing to remember, the widget then falls back to expand-or-complete itself.
check_previous_binding "an unbound Tab captures nothing" \
  'bindkey -r "^I" 2>/dev/null' \
  ''
# Sourcing twice must not make the fallback call this very widget.
check_previous_binding "sourcing twice does not capture this widget" \
  "bindkey '^I' expand-or-complete; source '$pluginDir/kubectl_fzf.plugin.zsh'" \
  'expand-or-complete'

print -r -- ""
print -r -- "$((testCount - failures))/$testCount passed"
exit $(( failures > 0 ))
