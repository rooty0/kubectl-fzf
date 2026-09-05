# kubectl-fzf fish support. Source this file from config.fish, or drop it in
# ~/.config/fish/conf.d/. It binds Tab to an fzf-driven kubectl completion and
# remembers whatever Tab did before as the fallback, mirroring the zsh plugin.

if not set -q KUBECTL_FZF_COMPLETION_BIN
    set -g KUBECTL_FZF_COMPLETION_BIN kubectl-fzf-completion
end

function __kubectl_fzf_debug
    if set -q KUBECTL_FZF_COMP_DEBUG_FILE; and test -n "$KUBECTL_FZF_COMP_DEBUG_FILE"
        echo $argv >>"$KUBECTL_FZF_COMP_DEBUG_FILE"
    end
end

# Hands the line to whatever Tab did before this file was sourced.
function __kubectl_fzf_default_completion
    set -l prev "$kubectl_fzf_default_completion"
    if test -z "$prev"
        set prev complete
    end
    # A user widget is a function: call it the way its binding would have (an
    # input function only answers commandline -f, and user functions are none
    # of those). A name that is no function is a readline input function like
    # "complete". Anything else (a binding made of arguments or a ';' script)
    # is the user's own configuration text and runs as such; the command line
    # itself is never eval'ed.
    if functions -q -- "$prev"
        $prev
    else if string match -qr '^[A-Za-z_][-A-Za-z0-9_]*$' -- "$prev"
        commandline -f $prev
    else
        eval $prev
    end
    __kubectl_fzf_debug "fallback to previous tab binding: $prev"
end

# Finds a token value in the raw buffer running left of pos0 (0-based,
# exclusive). commandline hands over unquoted tokens, so a quoted token is
# searched both bare and wrapped. Echoes the match start (0-based); returns 1
# when nothing matches.
function __kubectl_fzf_find_left -a buffer -a pos0 -a word
    while test $pos0 -gt 0
        and string match -qr '^\s' -- (string sub -s $pos0 -l 1 -- "$buffer")
        set pos0 (math $pos0 - 1)
    end
    for candidate in "$word" "\"$word\"" "'$word'"
        set -l n (string length -- "$candidate")
        if test $pos0 -ge $n
            set -l start (math $pos0 - $n)
            if test (string sub -s (math $start + 1) -l $n -- "$buffer") = "$candidate"
                echo $start
                return 0
            end
        end
    end
    return 1
end

# The mirror image running right of pos0 (0-based, where the previous token
# ended). Echoes the offset just past the match.
function __kubectl_fzf_find_right -a buffer -a pos0 -a word
    set -l len (string length -- "$buffer")
    while test $pos0 -lt $len
        and string match -qr '^\s' -- (string sub -s (math $pos0 + 1) -l 1 -- "$buffer")
        set pos0 (math $pos0 + 1)
    end
    for candidate in "$word" "\"$word\"" "'$word'"
        set -l n (string length -- "$candidate")
        if test (string sub -s (math $pos0 + 1) -l $n -- "$buffer") = "$candidate"
            echo (math $pos0 + $n)
            return 0
        end
    end
    return 1
end

# Applies the binary's answer to the command line. The usual case splices the
# completion over the current token. When the binary also names words to drop
# (-A once a namespace is pinned), the command is written out again without
# them, and everything outside the kubectl command stays byte-identical.
function __kubectl_fzf_apply -a completion -a wordIndex
    set -l tokens $argv[3..-1]
    set -l removeWords $__kubectl_fzf_remove_words
    set -l buffer (commandline -b)
    set -l cursor (commandline -C)
    set -l t (commandline -t)
    set -l tc (commandline -tc)
    # -t/-tc are raw, quotes included, so their lengths locate the token
    # exactly: tc is what precedes the cursor inside the token.
    set -l tokStart0 (math $cursor - (string length -- "$tc"))
    set -l tokEnd0 (math $tokStart0 + (string length -- "$t"))

    if test (count $removeWords) -eq 0
        set -l before (string sub -s 1 -l $tokStart0 -- "$buffer")
        set -l after (string sub -s (math $tokEnd0 + 1) -- "$buffer")
        # Like zsh: the completed word earns a trailing space, unless something
        # already follows it.
        set -l trailing ""
        if not string match -qr '^\s' -- "$after"
            set trailing " "
        end
        commandline -br "$before$completion$trailing$after"
        commandline -C (string length -- "$before$completion$trailing")
        commandline -f repaint
        __kubectl_fzf_debug "new BUFFER: "(commandline -b)
        return 0
    end

    # Rebuild: replace the completed word, drop one occurrence per reported
    # word, never look past "--", which kubectl reads as end of its arguments.
    set -l newWords
    set -l pending $removeWords
    set -l newIndex $wordIndex
    set -l argsEnded 0
    set -l i 0
    for word in $tokens
        set i (math $i + 1)
        set -l dropIt 0
        if test $argsEnded -eq 0
            if test "$word" = --
                set argsEnded 1
            else
                set -l ri (contains -i -- "$word" $pending)
                if test -n "$ri"
                    set -e pending[$ri]
                    set dropIt 1
                    # A word dropped ahead of the completed one shifts it left.
                    if test $i -le $wordIndex
                        set newIndex (math $newIndex - 1)
                    end
                end
            end
        end
        if test $dropIt -eq 0
            if test $i -eq $wordIndex
                set -a newWords "$completion"
            else
                set -a newWords "$word"
            end
        end
    end

    # Locate the kubectl command inside the full line by walking from the
    # current token outwards over the tokens of this pipeline segment.
    # Everything outside stays byte-identical. Giving up is safe: the buffer is
    # only replaced on success.
    set -l pos $tokStart0
    set -l j (math $wordIndex - 1)
    while test $j -ge 1
        set -l found (__kubectl_fzf_find_left "$buffer" $pos "$tokens[$j]")
        or begin
            __kubectl_fzf_debug "lost track of the buffer looking back for '$tokens[$j]'"
            commandline -f repaint
            return 1
        end
        set pos $found
        set j (math $j - 1)
    end
    set -l commandStart $pos

    set pos $tokEnd0
    set j (math $wordIndex + 1)
    while test $j -le (count $tokens)
        set -l found (__kubectl_fzf_find_right "$buffer" $pos "$tokens[$j]")
        or begin
            __kubectl_fzf_debug "lost track of the buffer looking ahead for '$tokens[$j]'"
            commandline -f repaint
            return 1
        end
        set pos $found
        set j (math $j + 1)
    end
    set -l commandEnd $pos

    set -l before (string sub -s 1 -l $commandStart -- "$buffer")
    set -l after (string sub -s (math $commandEnd + 1) -- "$buffer")

    # The token list is unquoted; string escape writes each word back in a form
    # fish reads identically. The completion is spliced raw, like zsh does.
    set -l parts
    set i 0
    for word in $newWords
        set i (math $i + 1)
        if test $i -eq $newIndex
            set -a parts "$word"
        else
            set -a parts (string escape -- "$word")
        end
    end
    set -l prefix (string join ' ' -- $parts[1..$newIndex])
    set -l rest ""
    if test (count $parts) -gt $newIndex
        set rest " "(string join ' ' -- $parts[(math $newIndex + 1)..-1])
    end
    set -l trailing ""
    if test -z "$rest"; and not string match -qr '^\s' -- "$after"
        set trailing " "
    end

    commandline -br "$before$prefix$trailing$rest$after"
    commandline -C (string length -- "$before$prefix$trailing")
    commandline -f repaint
    __kubectl_fzf_debug "new BUFFER: "(commandline -b)
    return 0
end

function __kubectl_fzf_completion
    set -l buffer (commandline -b)
    set -l cursor (commandline -C)
    set -l tokens (commandline -po)
    set -l poc (commandline -poc)
    __kubectl_fzf_debug
    __kubectl_fzf_debug "========= starting completion logic =========="
    __kubectl_fzf_debug "BUFFER: '$buffer', process tokens: "(string join '|' -- $tokens)", cursor: $cursor"

    # -po is the pipeline segment under the cursor: a pipe ahead of the command
    # is already out of the picture.
    if test (count $tokens) -eq 0
        __kubectl_fzf_default_completion
        return
    end

    # fish drops the in-progress word from -poc: what precedes it plus one is
    # where the completed word sits. No token under the cursor (-t is empty)
    # means the cursor sits on whitespace: that is a fresh, empty word. It is
    # spliced into the list at its position, since fish only reports the real
    # tokens. This is what lets "-A <cursor> -o yaml" name an empty slot rather
    # than the -o beyond it.
    set -l wordIndex (math (count $poc) + 1)
    if test -z (commandline -t)
        if test (count $poc) -eq (count $tokens)
            set -a tokens ""
        else
            set -l head $tokens[1..(count $poc)]
            set -l tail $tokens[(math (math (count $poc) + 1))..-1]
            set tokens $head "" $tail
        end
    end

    # The command name or the verb is under the cursor; kubectl's own
    # completion knows its verbs better than we would.
    if test $wordIndex -le 2
        __kubectl_fzf_default_completion
        return
    end

    set -l first $tokens[1]
    if not string match -q 'k*' -- "$first"
        __kubectl_fzf_default_completion
        return
    end

    # What the binary is told about is kubectl, whatever the user calls it. The
    # words on the line stay as typed, so an alias is not spelled out behind
    # the user's back.
    set -l expandedWords $tokens
    set -l expandedIndex $wordIndex
    if test "$first" != kubectl
        # fish aliases are thin wrapper functions; functions prints the one
        # line that matters, e.g. "    kubectl $argv". alias NAME alone is not
        # a query in fish, it errors instead.
        set -l body ""
        for line in (functions -- $first 2>/dev/null)
            set -l trimmed (string trim -- "$line")
            if string match -qr '^kubectl(\s|$)' -- "$trimmed"
                set body (string replace -r '\s*\$argv\s*$' '' -- "$trimmed")
                break
            end
        end
        if test -z "$body"
            __kubectl_fzf_default_completion
            return
        end
        set -l expanded (string split -n ' ' -- "$body")
        if test "$expanded[1]" != kubectl
            __kubectl_fzf_default_completion
            return
        end
        set expandedWords $expanded $tokens[2..-1]
        set expandedIndex (math $wordIndex + (count $expanded) - 1)
        __kubectl_fzf_debug "resolved alias $first: $body"
    end

    # The verb is word 2, so the cursor index the binary hears about starts
    # there: 0 over "get", 1 over the word after it.
    set -l words $expandedWords[2..-1]
    set -l relCursor (math $expandedIndex - 2)

    # The command line must never be eval'ed: a "$(...)" the user has merely
    # typed would run on a Tab press. One word per argv entry after --, and the
    # cursor says which of them is being completed.
    set -l binParts (string split -n ' ' -- "$KUBECTL_FZF_COMPLETION_BIN")
    set -l requestComp $binParts k8s_completion --protocol=2 "--cursor=$relCursor" -- $words
    __kubectl_fzf_debug "About to call: $requestComp"
    set -l rawOutput ($requestComp)
    set -l exitCode $status
    __kubectl_fzf_debug "raw output:"(string join '|' -- $rawOutput)", exit code $exitCode"

    if test $exitCode -eq 5; or test $exitCode -eq 6
        # No completion available, or unknown resource type.
        __kubectl_fzf_default_completion
        return
    end
    if test $exitCode -ne 0
        # Error on completion: leave the line alone instead of breaking Tab.
        __kubectl_fzf_debug "error on completion"
        commandline -f repaint
        return
    end

    # One key=value per line. Unknown keys are ignored so the binary can grow
    # new ones without breaking an older plugin.
    set -l completion ""
    set -l seenCompletion 0
    set -l removeWords
    for line in $rawOutput
        switch $line
            case 'completion=*'
                set completion (string sub -s 12 -- "$line")
                set seenCompletion 1
            case 'remove-word=*'
                set -a removeWords (string sub -s 13 -- "$line")
            case ''
            case '*'
                __kubectl_fzf_debug "Ignoring unknown response line '$line'"
        end
    end
    __kubectl_fzf_debug "completion: '$completion', words to remove: "(string join '|' -- $removeWords)

    if test $seenCompletion -eq 0; or test -z "$completion"
        commandline -f repaint
        return
    end
    if string match -q 'error*' -- "$completion"
        __kubectl_fzf_default_completion
        return
    end

    set -g __kubectl_fzf_remove_words $removeWords
    __kubectl_fzf_apply "$completion" $wordIndex $tokens
end

if not set -q kubectl_fzf_default_completion
    # bind reports one line per active binding, presets first:
    # "bind --preset \t complete", then "bind \t my_widget" or
    # "bind \t 'do; thing'". The last Tab line wins, as it would on a Tab press.
    set -l captured ""
    for line in (bind \t 2>/dev/null)
        set -l rest (string replace -r '^bind\s+(--preset\s+)?\\\\t\s+' '' -- "$line")
        if test "$rest" != "$line"
            set rest (string replace -r "^'" '' -- (string replace -r "'\$" '' -- "$rest"))
            if test -n "$rest"; and test "$rest" != __kubectl_fzf_completion
                set captured "$rest"
            end
        end
    end
    set -g kubectl_fzf_default_completion "$captured"
end

bind \t __kubectl_fzf_completion
bind -M insert \t __kubectl_fzf_completion
