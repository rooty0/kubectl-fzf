KUBECTL_FZF_COMPLETION_BIN=${KUBECTL_FZF_COMPLETION_BIN:-kubectl-fzf-completion}

# Example debugging: kubectl-fzf-completion k8s_completion "get pods -n "

__kubectl_fzf_debug()
{
  local file="$KUBECTL_FZF_COMP_DEBUG_FILE"
  if [[ -n ${file} ]]; then
    echo "$*" >>"${file}"
  fi
}

# Where the completion widget below leaves zsh's reading of the command line.
typeset -ga kubectl_fzf_parsed_words
typeset -gi kubectl_fzf_parsed_current
typeset -g kubectl_fzf_parsed_prefix kubectl_fzf_parsed_suffix

# zsh already knows how to read a command line, and gets it right for pipes,
# quoting and a cursor left of the end alike. That reading is only handed to a
# completion widget, so here is one. It generates no matches: the candidates come
# from the binary further down, this exists purely to be told what the user typed.
__kubectl_fzf_capture_parse()
{
  kubectl_fzf_parsed_words=("${words[@]}")
  kubectl_fzf_parsed_current=$CURRENT
  kubectl_fzf_parsed_prefix=$PREFIX
  kubectl_fzf_parsed_suffix=$SUFFIX
  # Insert nothing, list nothing, leave the line exactly as it was found.
  compstate[insert]=''
  compstate[list]=''
}
zle -C __kubectl_fzf_capture .complete-word __kubectl_fzf_capture_parse

# complete_in_word makes zsh cut the word at the cursor instead of handing the
# whole of it over as the prefix, which is what says where in the line that word
# begins. Setting it here and nowhere else leaves the option the user chose alone
# everywhere it is theirs to decide, the fallback completion included. The word
# under the cursor is replaced whole either way.
__kubectl_fzf_capture_line()
{
  setopt localoptions completeinword
  zle __kubectl_fzf_capture
}

# Finds where the command under the cursor starts and ends in BUFFER. zsh has
# already decided which words belong to it, so a pipe or a redirection is out of
# the picture by then and this only has to locate words it was handed, walking
# out from the cursor. That the walk matches the buffer at every step is checked,
# and giving up is safe: the caller then leaves the line alone.
#
# Assigns to commandStart and commandEnd, which the caller declares local, as
# offsets before the first and after the last character of the command.
__kubectl_fzf_locate_command()
{
  setopt localoptions noshwordsplit noksh_arrays
  local word
  local -i pos i
  # PREFIX and SUFFIX are the two halves of the word the cursor sits in, so the
  # word itself starts and ends that far away from the cursor.
  commandStart=$(( CURSOR - ${#kubectl_fzf_parsed_prefix} ))
  commandEnd=$(( CURSOR + ${#kubectl_fzf_parsed_suffix} ))

  pos=$commandStart
  for (( i = kubectl_fzf_parsed_current - 1; i >= 1; i-- )); do
    word=${kubectl_fzf_parsed_words[i]}
    while (( pos > 0 )) && [[ ${BUFFER[pos]} == [[:space:]] ]]; do
      (( pos-- ))
    done
    if (( pos < ${#word} )) || [[ ${BUFFER[pos - ${#word} + 1, pos]} != "$word" ]]; then
      __kubectl_fzf_debug "Lost track of the buffer looking back for '$word'"
      return 1
    fi
    (( pos -= ${#word} ))
  done
  commandStart=$pos

  pos=$commandEnd
  for (( i = kubectl_fzf_parsed_current + 1; i <= ${#kubectl_fzf_parsed_words}; i++ )); do
    word=${kubectl_fzf_parsed_words[i]}
    while [[ ${BUFFER[pos+1]} == [[:space:]] ]]; do
      (( pos++ ))
    done
    if [[ ${BUFFER[pos+1, pos + ${#word}]} != "$word" ]]; then
      __kubectl_fzf_debug "Lost track of the buffer looking ahead for '$word'"
      return 1
    fi
    (( pos += ${#word} ))
  done
  commandEnd=$pos
  return 0
}

__kubectl_fzf_get_completions()
{
  local rawOutput
  local -a requestComp requestWords
  local -i cursor=$1
  shift
  requestWords=("$@")

  __kubectl_fzf_debug "Get completions: words: '${(q-)requestWords}', cursor: $cursor"
  # The command line must never be eval'ed: a "$(...)" or a backtick the user has
  # merely typed would run on a Tab press. Build the argv and call it directly.
  # ${=...} keeps a KUBECTL_FZF_COMPLETION_BIN carrying its own arguments working.
  # Everything after -- is one word per argv entry, so a value holding a space
  # arrives in one piece, and the cursor says which of them is being completed.
  requestComp=(${=KUBECTL_FZF_COMPLETION_BIN} k8s_completion --protocol=2 "--cursor=$cursor" -- "${requestWords[@]}")
  __kubectl_fzf_debug "About to call: ${(q-)requestComp}"
  zle -R "Calling completion '${requestComp[*]}'"
  rawOutput=$("${requestComp[@]}")
  exitCode=$?
  __kubectl_fzf_debug "raw output: ${rawOutput}, exit code ${exitCode}"

  # Exit code 5: kubectl-fzf-completion has no custom completion for this pattern
  if [[ $exitCode == 5 ]]; then
    __kubectl_fzf_debug "No completion available, fallback to default completion"
    fallback="true"
    return
  fi

  # Exit code 6: unknown resource type -> also fall back
  if [[ $exitCode == 6 ]]; then
    __kubectl_fzf_debug "Unknown resource type, fallback to default completion"
    fallback="true"
    return
  fi

  # Any other non-zero error: just bail, don't print
  if [[ $exitCode != 0 ]]; then
    __kubectl_fzf_debug "error on completion"
    return
  fi

  # One key=value per line. Unknown keys are ignored so the binary can grow new
  # ones without breaking an older plugin.
  local line
  completionOutput=""
  removeWords=()
  for line in "${(@f)rawOutput}"; do
    case $line in
      completion=*) completionOutput="${line#completion=}" ;;
      remove-word=*) removeWords+=("${line#remove-word=}") ;;
      "") ;;
      *) __kubectl_fzf_debug "Ignoring unknown response line '$line'" ;;
    esac
  done
  __kubectl_fzf_debug "completion: '$completionOutput', words to remove: '${removeWords[*]}'"
}

__kubectl_fzf_kubectl() {
  local currentWord
  local completionOutput
  local -a removeWords
  local fallback

  zle -R "Starting kubectl-fzf completion"
  __kubectl_fzf_debug "cmdCurrent: ${cmdCurrent}, cmdWords: '${cmdWords[*]}', ${#cmdWords[@]}"
  currentWord=${cmdWords[cmdCurrent]}

  # (Q) strips one level of quoting, so the binary is handed the values kubectl
  # would see. The words keep their original text for rebuilding the line. The
  # verb is word 2, so it is the zero of the cursor the binary is told about.
  __kubectl_fzf_get_completions $(( expandedCurrent - 2 )) "${(@Q)expandedWords[2,-1]}"
  zle -R "Processing completion output"

  # If kubectl-fzf doesn't know how to complete this, fall back to default
  if [[ -n "$fallback" ]]; then
    __kubectl_fzf_debug "Fallback detected: '$fallback'"
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  if [[ "$completionOutput" == "" ]]; then
    __kubectl_fzf_debug "Empty completion output"
    return
  fi

  if [[ "$completionOutput" == error* ]]; then
    __kubectl_fzf_debug "Output starts with error, falling back: $completionOutput"
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  __kubectl_fzf_debug "Replacing current word '$currentWord' with: '$completionOutput'"

  if (( ${#removeWords[@]} )); then
    __kubectl_fzf_rebuild_command || return
  else
    __kubectl_fzf_replace_current_word
  fi
  __kubectl_fzf_debug "New BUFFER: '$BUFFER', cursor at $CURSOR"
}

# The usual case: one word becomes another. PREFIX and SUFFIX say where that word
# lies, so it is cut out and the completion put in its place. Every other
# character of the line, the spacing around a pipe included, is left alone.
__kubectl_fzf_replace_current_word()
{
  local before="${BUFFER[1, CURSOR - ${#kubectl_fzf_parsed_prefix}]}"
  local after="${BUFFER[CURSOR + ${#kubectl_fzf_parsed_suffix} + 1, -1]}"
  local trailing=""
  # A completed word gets the usual trailing space, unless something already
  # follows it on the line.
  [[ $after != [[:space:]]* ]] && trailing=" "

  BUFFER="${before}${completionOutput}${trailing}${after}"
  CURSOR=$(( ${#before} + ${#completionOutput} + ${#trailing} ))
}

# Words the completion reported as incompatible with its result have to go, and
# they sit anywhere on the command. That means writing the command out again
# rather than splicing one word, so the command has to be found in the line
# first. Everything outside it is still kept verbatim.
__kubectl_fzf_rebuild_command()
{
  local -a newWords keptWords pendingRemovals
  local word
  local -i commandStart commandEnd newCurrent=$cmdCurrent
  local -i argsEnded=0 removalIndex index=0

  if ! __kubectl_fzf_locate_command; then
    # Leaving -A in place would produce a command kubectl rejects, so the line is
    # better left as it is.
    __kubectl_fzf_debug "Command not located, leaving the line alone"
    return 1
  fi

  newWords=("${cmdWords[@]}")
  newWords[$cmdCurrent]="$completionOutput"

  # One occurrence per reported word. Which words those are is the binary's
  # decision, this only applies it. Stops at "--" since the rest belongs to the
  # executed command.
  pendingRemovals=("${removeWords[@]}")
  for word in "${newWords[@]}"; do
    (( index++ ))
    if (( ! argsEnded )); then
      [[ $word == "--" ]] && argsEnded=1
      # (ie) matches the exact string: a word holding glob characters must not
      # be treated as a pattern.
      removalIndex=${pendingRemovals[(ie)$word]}
      if (( removalIndex <= ${#pendingRemovals[@]} )); then
        pendingRemovals[removalIndex]=()
        # Dropping a word in front of the completed one moves it left.
        (( index <= cmdCurrent )) && (( newCurrent-- ))
        continue
      fi
    fi
    keptWords+=("$word")
  done
  newWords=("${keptWords[@]}")
  __kubectl_fzf_debug "Words after removal: ${newWords[*]}"

  local before="${BUFFER[1,commandStart]}"
  local after="${BUFFER[commandEnd+1,-1]}"
  local completed="${(j: :)newWords[1,newCurrent]}"
  local rest=""
  (( newCurrent < ${#newWords} )) && rest=" ${(j: :)newWords[newCurrent+1,-1]}"
  local trailing=""
  [[ -z $rest && $after != [[:space:]]* ]] && trailing=" "

  BUFFER="${before}${completed}${trailing}${rest}${after}"
  CURSOR=$(( ${#before} + ${#completed} + ${#trailing} ))
}

# Completion entry point
kubectl_fzf_completion() {
  local firstWord
  local -a cmdWords expandedWords
  local -i cmdCurrent expandedCurrent
  setopt localoptions noshwordsplit noksh_arrays noposixbuiltins
  __kubectl_fzf_debug "\n========= starting completion logic =========="

  # Let zsh read the line. Only the command under the cursor comes back, so a
  # pipe in front of it is already none of our business, and a cursor left of the
  # end is placed for us.
  __kubectl_fzf_capture_line
  cmdWords=("${kubectl_fzf_parsed_words[@]}")
  cmdCurrent=$kubectl_fzf_parsed_current
  __kubectl_fzf_debug "BUFFER: '$BUFFER', words: '${cmdWords[*]}', current: $cmdCurrent"

  firstWord=${cmdWords[1]}

  # The command name or the verb is under the cursor, and kubectl completes its
  # own verbs better than we would.
  if (( cmdCurrent <= 2 )); then
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  # We only care about kubectl completion
  if [[ $firstWord != k* ]]; then
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  # What the binary is told about is kubectl, whatever the user calls it. The
  # words that go back on the line stay as they were typed, so an alias is not
  # spelled out behind the user's back.
  expandedWords=("${cmdWords[@]}")
  expandedCurrent=$cmdCurrent
  if [[ "$firstWord" != "kubectl" ]]; then
    local -a expanded
    expanded=(${(z)aliases[$firstWord]})
    if [ ${#expanded} -lt 1 ]; then
      zle "${kubectl_fzf_default_completion:-expand-or-complete}"
      return
    fi
    if [ "${expanded[1]}" != "kubectl" ]; then
      zle "${kubectl_fzf_default_completion:-expand-or-complete}"
      return
    fi
    # We have resolved a kubectl alias
    local -i aliasLength=${#expanded}
    for word in "${cmdWords[@]:1}"; do
      expanded+=("$word")
    done
    expandedWords=("${expanded[@]}")
    # An alias standing for several words pushes everything behind it to the
    # right, the completed word included.
    (( expandedCurrent += aliasLength - 1 ))
  fi
  __kubectl_fzf_kubectl
}

if [[ -z "$kubectl_fzf_default_completion" ]]; then
  # bindkey reports '"^I" widget-name', or '"^I" undefined-key' when Tab is
  # unbound. Whatever was there has to be remembered, otherwise a Tab widget the
  # user had set up, fzf-tab for instance, is lost the moment this is sourced.
  binding=(${(z)"$(bindkey '^I')"})
  case ${binding[2]} in
    # Sourcing this twice must not make the fallback call this widget again.
    ""|undefined-key|kubectl_fzf_completion) ;;
    *) kubectl_fzf_default_completion=${binding[2]} ;;
  esac
  unset binding
fi

zle -N kubectl_fzf_completion
bindkey '^I' kubectl_fzf_completion
