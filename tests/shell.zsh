#!/usr/bin/env zsh
if [[ ! -o interactive ]]; then
  exec zsh -f -i "$0" "$@"
fi

emulate -L zsh
set -eu
root=${0:A:h:h}

# Give every binding a known value, including the two intentionally undefined
# shortcuts. The test never invokes a widget or executes BUFFER.
bindkey -e
for map in emacs viins; do
  bindkey -M "$map" '^M' accept-line
  bindkey -M "$map" '^J' accept-line
  bindkey -M "$map" '^Xs' history-incremental-search-forward
  bindkey -M "$map" '^Xa' undefined-key
done

typeset -A before
for map in emacs viins; do
  for key in '^M' '^J' '^Xs' '^Xa'; do
    before["$map:$key"]=$(bindkey -M "$map" "$key")
  done
done

source "$root/shell/bev.zsh"
source "$root/shell/bev.zsh"
[[ $_BEV_ENABLED == 1 ]]
for map in emacs viins; do
  [[ $(bindkey -M "$map" '^M') == *'_bev_enter_'* ]]
  [[ $(bindkey -M "$map" '^J') == *'_bev_enter_'* ]]
  [[ $(bindkey -M "$map" '^Xs') == *"_bev_force_shell_${map}"* ]]
  [[ $(bindkey -M "$map" '^Xa') == *"_bev_force_codex_${map}"* ]]
done

bev-disable
bev-disable
[[ $_BEV_ENABLED == 0 ]]
for map in emacs viins; do
  for key in '^M' '^J' '^Xs' '^Xa'; do
    [[ $(bindkey -M "$map" "$key") == ${before["$map:$key"]} ]]
  done
done

bev-enable
[[ $_BEV_ENABLED == 1 ]]
bev-disable
[[ $_BEV_ENABLED == 0 ]]
print 'shell integration checks passed'
