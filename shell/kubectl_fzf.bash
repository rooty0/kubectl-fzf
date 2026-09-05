# shellcheck shell=bash
# kubectl-fzf bash support. Source this file after "kubectl completion bash".
#
# bash splits COMP_WORDS at every COMP_WORDBREAKS character, and the default
# set includes '=': --context=prod would arrive here as three harmless-looking
# words, each a lie. Taking '=' out fixes attached flag values for every
# completion, ours included; what breaks is the (rare, and bash-completion
# already re-splits them internally) style of completing after a bare '='.
COMP_WORDBREAKS="${COMP_WORDBREAKS//=/}"

KUBECTL_FZF_COMPLETION_BIN=${KUBECTL_FZF_COMPLETION_BIN:-kubectl-fzf-completion}

__kubectl_fzf_debug()
{
    local file="$KUBECTL_FZF_COMP_DEBUG_FILE"
    if [[ -n ${file} ]]; then
        echo "$*" >> "${file}"
    fi
}

# Calls the completion binary and reads its structured answer. Reports through
# the globals completionOutput, fallback and removeWords: bash completion
# cannot rewrite the command line, so a word the binary wants gone (like -A in
# the company of a namespaced name) hands the line to kubectl's own completion
# instead of leaving something kubectl would reject.
__kubectl_fzf_get_completions()
{
    local -a requestComp requestWords
    local rawOutput line
    local -i exitCode relCursor

    # What the binary is told about is kubectl, whatever the user calls it. An
    # alias is expanded for the binary only: the words on the line stay as
    # typed, so the alias is not spelled out behind the user's back.
    local first="${COMP_WORDS[0]}"
    requestWords=("${COMP_WORDS[@]}")
    relCursor=$COMP_CWORD
    if [[ "$first" != "kubectl" ]]; then
        local aliasDef body
        aliasDef=$(alias "$first" 2>/dev/null)
        # aliasDef: alias k='kubectl --context x'
        body="${aliasDef#*=}"
        body="${body#[\'\"]}"
        body="${body%[\'\"]}"
        local -a expanded
        # read splits on IFS without globbing a stray * in the alias body.
        IFS=' ' read -ra expanded <<< "$body"
        if [[ "${expanded[0]:-}" != "kubectl" ]]; then
            __kubectl_fzf_debug "first word '$first' is not kubectl or an alias to it"
            fallback="true"
            return
        fi
        requestWords=("${expanded[@]}" "${COMP_WORDS[@]:1}")
        # An alias standing for several words pushes the cursor right.
        relCursor=$(( COMP_CWORD + ${#expanded[@]} - 1 ))
    fi

    # The cursor on the command name or on the verb belongs to the shell's own
    # completion; kubectl knows its verbs better than we would.
    if (( relCursor < 2 )); then
        fallback="true"
        return
    fi
    # The verb is the first word the binary hears about, so the cursor shifts
    # one left: 0 there means the verb is being completed, and the binary
    # declines that position itself.
    relCursor=$(( relCursor - 1 ))

    # COMP_WORDS keeps the quotes of a closed quoted segment. One level is
    # stripped so the binary sees the value the way kubectl would; a segment
    # split at "=" already arrived whole thanks to the COMP_WORDBREAKS tweak.
    local w
    local -a quotedChecked=()
    for w in "${requestWords[@]:1}"; do
        case $w in
            \"*\")
                (( ${#w} >= 2 )) && w="${w:1:${#w}-2}"
                ;;
            \'*\')
                (( ${#w} >= 2 )) && w="${w:1:${#w}-2}"
                ;;
        esac
        quotedChecked+=("$w")
    done

    # The command line must never be eval'ed: a "$(...)" or a backtick the user
    # has merely typed would run on a Tab press. One word per argv entry after
    # --, so a value holding a space arrives in one piece.
    requestComp=($KUBECTL_FZF_COMPLETION_BIN k8s_completion --protocol=2 "--cursor=$relCursor" -- "${quotedChecked[@]}")
    __kubectl_fzf_debug "About to call: ${requestComp[*]}"
    rawOutput=$("${requestComp[@]}")
    exitCode=$?
    __kubectl_fzf_debug "raw output: ${rawOutput}, exit code ${exitCode}"

    case $exitCode in
        5) # No completion available
            __kubectl_fzf_debug "No completion available, fallback to default completion"
            fallback="true"
            return
            ;;
        6) # Unknown resource type
            __kubectl_fzf_debug "Unknown resource type, fallback to default completion"
            fallback="true"
            return
            ;;
        0) ;;
        *)
            # Error on completion: keep Tab working via kubectl's own completion.
            __kubectl_fzf_debug "error when calling ${requestComp[*]}, output: ${rawOutput}"
            fallback="true"
            return
            ;;
    esac

    # One key=value per line. Unknown keys are ignored so the binary can grow
    # new ones without breaking an older plugin.
    while IFS= read -r line; do
        case $line in
            completion=*) completionOutput="${line#completion=}" ;;
            remove-word=*) removeWords+=("${line#remove-word=}") ;;
            "") ;;
            *) __kubectl_fzf_debug "Ignoring unknown response line '$line'" ;;
        esac
    done <<< "$rawOutput"

    if [[ "$completionOutput" == error* ]]; then
        fallback="true"
        return
    fi
    if (( ${#removeWords[@]} )); then
        # bash cannot drop words mid-line from a completion function, so a
        # completion that needs one removed is declined entirely; kubectl's own
        # completion at least leaves the line valid.
        __kubectl_fzf_debug "the binary wants words removed; bash cannot, falling back"
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
        "$defaultCompletion"
        return
    fi
    __kubectl_fzf_debug "No default completion function '$defaultCompletion' to fall back to"
    COMPREPLY=()
}

__kubectl_fzf_kubectl()
{
    local fallback completionOutput
    local -a removeWords=()

    __kubectl_fzf_debug
    __kubectl_fzf_debug "========= starting completion logic =========="
    __kubectl_fzf_debug "COMP_CWORD is ${COMP_CWORD}, COMP_WORDS[*] is ${COMP_WORDS[*]}"

    # ${words[0]} goes along to the binary, so aliases need no special case here.
    __kubectl_fzf_get_completions

    if [[ -n "$fallback" ]]; then
        __kubectl_fzf_default_completion
        return
    fi
    if [[ -z "$completionOutput" ]]; then
        __kubectl_fzf_debug "Empty completion output"
        COMPREPLY=()
        return
    fi
    # completion word splitting of the answer is unwanted; quote it wholesale.
    COMPREPLY=("$completionOutput")
}

if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __kubectl_fzf_kubectl kubectl
else
    complete -o default -o nospace -F __kubectl_fzf_kubectl kubectl
fi

# Completion only fires for commands complete(1) knows about, so the kubectl
# aliases defined by the time this file is sourced are registered too: their
# first word is expanded in __kubectl_fzf_get_completions.
while read -r aliasLine; do
    aliasName="${aliasLine#alias }"
    aliasName="${aliasName%%=*}"
    aliasBody="${aliasLine#*=}"
    aliasBody="${aliasBody#[\'\"]}"
    aliasBody="${aliasBody%[\'\"]}"
    if [[ "$aliasBody" == "kubectl" || "$aliasBody" == "kubectl "* ]]; then
        if [[ $(type -t compopt) = "builtin" ]]; then
            complete -o default -F __kubectl_fzf_kubectl "$aliasName"
        else
            complete -o default -o nospace -F __kubectl_fzf_kubectl "$aliasName"
        fi
    fi
done < <(alias)

# ex: ts=4 sw=4 et filetype=sh
