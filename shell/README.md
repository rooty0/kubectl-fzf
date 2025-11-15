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
# Wrap kubectl_fzf_completion so that "k ..." is treated like "kubectl ..." for completion,
# but the command line still shows "k" and executes via the k() function.
_kubectl_fzf_k_wrapper() {
  emulate -L zsh
  setopt localoptions noshwordsplit noksh_arrays noposixbuiltins

  # If the first word is "k", temporarily pretend it is "kubectl" for completion
  local words=(${(z)LBUFFER})
  if [[ ${#words[@]} -gt 0 && ${words[1]} == k ]]; then
    # Swap "k" -> "kubectl" just for the completion widget
    local orig_lbuffer="$LBUFFER"
    LBUFFER="kubectl ${LBUFFER#k }"

    # Run the original kubectl-fzf completion
    zle kubectl_fzf_completion

    # After completion, if the buffer still starts with "kubectl ",
    # swap it back to "k " so the final command is "k ..."
    if [[ $LBUFFER == kubectl\ * ]]; then
      LBUFFER="k ${LBUFFER#kubectl }"
    fi
  else
    # For everything else, just use kubectl-fzf as normal
    zle kubectl_fzf_completion
  fi
}

# Register the wrapper as a ZLE widget and bind it to Tab
zle -N _kubectl_fzf_k_wrapper
bindkey '^I' _kubectl_fzf_k_wrapper
```
