KUBECTL_FZF_COMPLETION_BIN=${KUBECTL_FZF_COMPLETION_BIN:-kubectl-fzf-completion}

__kubectl_fzf_debug()
{
    local file="$KUBECTL_FZF_COMP_DEBUG_FILE"
    if [[ -n ${file} ]]; then
        echo "$*" >> "${file}"
    fi
}

# Reports its result through the caller's completionOutput and fallback
# variables. Nothing may be written to stdout: the caller feeds this function's
# result to COMPREPLY, so a printed status line would be pasted into the command
# line instead of completing it.
__kubectl_fzf_get_completions()
{
    local cmdArgs currentWord exitCode
    local -a requestComp
    cmdArgs="$1"
    # TODO Handle query
    currentWord="$2"

    __kubectl_fzf_debug "Get completions: cmdArgs: '$cmdArgs', currentWord: '$currentWord'"
    # The command line must never be eval'ed: a "$(...)" or a backtick the user has
    # merely typed would run on a Tab press. Build the argv and call it directly.
    # Passing cmdArgs as a single argument also matters, the completion binary
    # wants the whole argument string as one argv entry.
    requestComp=($KUBECTL_FZF_COMPLETION_BIN k8s_completion "$cmdArgs")
    __kubectl_fzf_debug "About to call: ${requestComp[*]}"
    completionOutput=$("${requestComp[@]}")
    exitCode=$?
    __kubectl_fzf_debug "completion output: ${completionOutput}, exit code ${exitCode}"

    if [[ $exitCode == 5 ]]; then
        # No completion available
        __kubectl_fzf_debug "No completion available, fallback to default completion"
        fallback="true"
        return
    fi
    if [[ $exitCode == 6 ]]; then
        # Unknow resource type, fallback to default completion
        __kubectl_fzf_debug "Unknown resource type, fallback to default completion"
        fallback="true"
        return
    fi
    if [[ $exitCode != 0 ]]; then
        # Error on completion. Falling back keeps Tab working while the server or
        # the cache is unavailable, at the cost of kubectl querying the API.
        __kubectl_fzf_debug "error when calling ${requestComp[*]}, output: ${completionOutput}"
        completionOutput=""
        fallback="true"
        return
    fi
}

# Hands the completion back to kubectl's own bash completion, the one this file
# shadows when it is sourced after "kubectl completion bash".
__kubectl_fzf_default_completion()
{
    local defaultCompletion=${KUBECTL_FZF_DEFAULT_COMPLETION:-__start_kubectl}

    if declare -F "$defaultCompletion" >/dev/null 2>&1; then
        # __start_kubectl rebuilds cur/words/cword from COMP_WORDS, so the
        # truncation done by the caller does not reach it.
        "$defaultCompletion"
        return
    fi
    # Without kubectl's completion sourced there is nothing better to offer, and
    # "complete -o default" falls back to filenames on an empty COMPREPLY.
    __kubectl_fzf_debug "No default completion function '$defaultCompletion' to fall back to"
    COMPREPLY=()
}

__kubectl_fzf_get_completion_results() {
    local lastParam cmdArgs
    local completionOutput fallback

    # Prepare the command to request completions for the program.
    # Calling ${words[0]} instead of directly kubectl allows to handle aliases
    cmdArgs="${words[*]:1}"

    lastParam=${words[$((${#words[@]}-1))]}
    __kubectl_fzf_debug "lastParam ${lastParam}"

    # When completing a flag with an = (e.g., kubectl -n=<TAB>)
    # bash focuses on the part after the =, so we need to remove
    # the flag part from $cur
    if [[ "${cur}" == -*=* ]]; then
        cur="${cur#*=}"
    fi

    # Called directly and not through $(...): the fallback is reported through a
    # variable, which a subshell would throw away.
    __kubectl_fzf_get_completions "$cmdArgs" "$cur"

    if [[ -n "$fallback" ]]; then
        __kubectl_fzf_default_completion
        return
    fi
    if [[ -z "$completionOutput" ]]; then
        __kubectl_fzf_debug "Empty completion output"
        COMPREPLY=()
        return
    fi
    COMPREPLY=("$completionOutput")
}

__kubectl_fzf_kubectl()
{
    local cur words cword

    COMPREPLY=()

    # Call _init_completion from the bash-completion package
    # to prepare the arguments properly
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -n "=:" || return
    else
        __kubectl_init_completion -n "=:" || return
    fi

    __kubectl_fzf_debug
    __kubectl_fzf_debug "========= starting completion logic =========="
    __kubectl_fzf_debug "cur is ${cur}, words[*] is ${words[*]}, #words[@] is ${#words[@]}, cword is $cword"

    # The user could have moved the cursor backwards on the command-line.
    # We need to trigger completion from the $cword location, so we need
    # to truncate the command-line ($words) up to the $cword location.
    words=("${words[@]:0:$cword+1}")
    __kubectl_fzf_debug "Truncated words[*]: ${words[*]},"

    __kubectl_fzf_get_completion_results
}

if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __kubectl_fzf_kubectl kubectl
else
    complete -o default -o nospace -F __kubectl_fzf_kubectl kubectl
fi

# ex: ts=4 sw=4 et filetype=sh
