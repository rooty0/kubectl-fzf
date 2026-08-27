KUBECTL_FZF_COMPLETION_BIN=${KUBECTL_FZF_COMPLETION_BIN:-kubectl-fzf-completion}

# Example debugging: kubectl-fzf-completion k8s_completion "get pods -n "

__kubectl_fzf_debug()
{
  local file="$KUBECTL_FZF_COMP_DEBUG_FILE"
  if [[ -n ${file} ]]; then
    echo "$*" >>"${file}"
  fi
}

# Splits a command line into the part that is none of kubectl's business and the
# last command on it, since that is the only one being completed. Both
# `cat f.yaml | k apply -f <TAB>` and `k get pods -oyaml | gre<TAB>` then get the
# completion they deserve, the first from kubectl and the second from the shell.
#
# Assigns to commandPrefix and commandLine, which the caller declares local.
__kubectl_fzf_split_last_command()
{
  setopt localoptions extendedglob noshwordsplit noksh_arrays noposixbuiltins
  local buffer="$1" token
  local -a tokens
  local -i offset=0 tokenStart=0 commandStart=0 afterSeparator=0

  commandPrefix=""
  commandLine="$buffer"
  tokens=(${(z)buffer})
  for token in "${(@)tokens}"; do
    # (z) is zsh's own lexer and hands the tokens back in order and unchanged, so
    # each one is located by walking forward over the whitespace between them.
    while [[ ${buffer[offset+1]} == [[:space:]] ]]; do
      (( offset++ ))
    done
    tokenStart=$offset
    (( offset += ${#token} ))
    if [[ ${buffer[tokenStart+1,offset]} != "$token" ]]; then
      # The walk lost track of the buffer, so the offsets cannot be trusted.
      # Treating the whole line as one command is the old behaviour and is far
      # better than splicing the line at a wrong position.
      __kubectl_fzf_debug "Lost track of the buffer at '$token', taking the whole line"
      return
    fi
    if (( afterSeparator )); then
      # Starting at the first word after the operator leaves whatever spacing the
      # user typed around that operator untouched.
      commandStart=$tokenStart
      afterSeparator=0
    fi
    # A bare operator: | || && ; & |& or a redirection such as > >> < 2>. One
    # inside quotes is not matched, as (z) keeps a quoted string in one token.
    if [[ $token == [0-9]#[\|\;\&\<\>]## ]]; then
      afterSeparator=1
      commandStart=$offset
    fi
  done
  commandPrefix="${buffer[1,commandStart]}"
  commandLine="${buffer[commandStart+1,-1]}"
}

__kubectl_fzf_get_completions()
{
  local rawOutput
  local -a requestComp cmdWords
  # TODO Handle query
  currentWord="$1"
  shift
  cmdWords=("$@")

  __kubectl_fzf_debug "Get completions: cmdWords: '${(q-)cmdWords}', currentWord: '$currentWord'"
  # The command line must never be eval'ed: a "$(...)" or a backtick the user has
  # merely typed would run on a Tab press. Build the argv and call it directly.
  # ${=...} keeps a KUBECTL_FZF_COMPLETION_BIN carrying its own arguments working.
  # Everything after -- is one word per argv entry, so a value holding a space
  # arrives in one piece.
  requestComp=(${=KUBECTL_FZF_COMPLETION_BIN} k8s_completion --protocol=2 -- "${cmdWords[@]}")
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
  local currentWord previousWord
  local completionOutput
  local -a removeWords
  local fallback

  zle -R "Starting kubectl-fzf completion"
  __kubectl_fzf_debug "CURRENT: ${CURRENT}, words[*]: '${words[*]}', ${#words[@]}"
  currentWord=${words[CURRENT]}
  previousWord=${words[CURRENT-1]}
  __kubectl_fzf_debug "Current word: ${currentWord}, previous word: ${previousWord}"

  # We only have 'kubectl g#', fallback to default completion
  if [[ ${#words[@]} -le 2 ]]; then
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  # (Q) strips one level of quoting, so the binary is handed the values kubectl
  # would see. The words keep their original text for rebuilding the line.
  __kubectl_fzf_get_completions "$currentWord" "${(@Q)words[2,-1]}"
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

  # Rebuild the buffer from the parsed words, replacing only the current word
  local -a new_words
  new_words=("${words[@]}")
  new_words[$CURRENT]="$completionOutput"

  # Drop the words the completion reported as incompatible with its result, one
  # occurrence each. Which words those are is the binary's decision, this only
  # applies it. Stops at "--" since the rest belongs to the executed command.
  if (( ${#removeWords[@]} )); then
    local -a keptWords pendingRemovals
    local word
    local -i argsEnded=0 removalIndex
    pendingRemovals=("${removeWords[@]}")
    for word in "${new_words[@]}"; do
      if (( ! argsEnded )); then
        [[ $word == "--" ]] && argsEnded=1
        # (ie) matches the exact string: a word holding glob characters must not
        # be treated as a pattern.
        removalIndex=${pendingRemovals[(ie)$word]}
        if (( removalIndex <= ${#pendingRemovals[@]} )); then
          pendingRemovals[removalIndex]=()
          continue
        fi
      fi
      keptWords+=("$word")
    done
    new_words=("${keptWords[@]}")
    __kubectl_fzf_debug "Words after removal: ${new_words[*]}"
  fi

  LBUFFER="${commandPrefix}${(j: :)new_words} "
  __kubectl_fzf_debug "New LBUFFER: '$LBUFFER'"
}

# Completion entry point
kubectl_fzf_completion() {
  local words firstWord commandPrefix commandLine
  setopt localoptions noshwordsplit noksh_arrays noposixbuiltins
  __kubectl_fzf_debug "\n========= starting completion logic =========="

  # Only the last command on the line is being completed, everything before it is
  # kept verbatim.
  __kubectl_fzf_split_last_command "$LBUFFER"
  words=(${(z)commandLine})
  __kubectl_fzf_debug "LBUFFER: '$LBUFFER', prefix: '$commandPrefix', command: '$commandLine', words: '${words[*]}', ${#words}"

  firstWord=${words[1]}

  if [[ ${#words[@]} -le 1 && ${commandLine[-1]} != " " ]]; then
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  # We only care about kubectl completion
  if [[ $firstWord != k* ]]; then
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  if [[ $RBUFFER != "" ]]; then
    # TODO Handle right buffer
    zle "${kubectl_fzf_default_completion:-expand-or-complete}"
    return
  fi

  if [[ "$firstWord" != "kubectl" ]]; then
    # Try to resolve alias
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
    for word in "${words[@]:1}"; do
      expanded+=("$word")
    done
    words=("${expanded[@]}")
  fi
  # An empty trailing word marks a word that is only being started. It used to be
  # a single space, which a command line passed as one string could not tell
  # apart from padding; one argv entry per word can.
  if [[ ${commandLine[-1]} == " " ]]; then
    words+=("")
  fi
  CURRENT=${#words[@]}
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
