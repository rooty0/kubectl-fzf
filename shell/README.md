### Using `k` with `kubecolor` and autocompletion

If you're like me and use `k` not just as an alias for `kubectl` but for something like:

```zsh
alias k="kubecolor --force-colors=truecolor"
```

then `k` will always force colors, while plain `kubectl` can still be used when you want to pipe uncolored output to files.

To get autocompletion working for `k` as well, remove the `alias k=...` line from your `~/.zshrc` and add the following instead:

```zsh
source ~/.kubectl_fzf.plugin.zsh # or whatever you named the plugin file
k() {
  kubecolor --force-colors=truecolor "$@"
}

# The plugin resolves a kubectl alias on its own, but "k" is a function here, so
# the command word is swapped for "kubectl" just long enough to complete and then
# put back. Where that word sits is asked of the plugin rather than assumed, so a
# "k" after a pipe, or a cursor left of the end of the line, works the same.
_kubectl_fzf_k_wrapper() {
  emulate -L zsh
  setopt localoptions noshwordsplit noksh_arrays noposixbuiltins
  local -i commandStart commandEnd cursorAfter

  __kubectl_fzf_capture_line
  if [[ ${kubectl_fzf_parsed_words[1]} != k ]] ||
     (( kubectl_fzf_parsed_current == 1 )) ||
     ! __kubectl_fzf_locate_command; then
    zle kubectl_fzf_completion
    return
  fi

  # "kubectl" is six characters longer than "k".
  BUFFER="${BUFFER[1,commandStart]}kubectl${BUFFER[commandStart+2,-1]}"
  CURSOR=$(( CURSOR + 6 ))

  zle kubectl_fzf_completion

  # Put "k" back. The cursor is worked out before the buffer is assigned, since
  # assigning a shorter line moves the cursor on its own.
  if [[ ${BUFFER[commandStart+1,commandStart+7]} == kubectl ]]; then
    cursorAfter=$(( CURSOR - 6 ))
    BUFFER="${BUFFER[1,commandStart]}k${BUFFER[commandStart+8,-1]}"
    CURSOR=$cursorAfter
  fi
}

# Register the wrapper as a ZLE widget and bind it to Tab
zle -N _kubectl_fzf_k_wrapper
bindkey '^I' _kubectl_fzf_k_wrapper
```
