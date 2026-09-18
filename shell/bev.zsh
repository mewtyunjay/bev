# Source this file after other plugins to enable Bev in an interactive zsh.
# bev-disable restores the previous bindings; bev-enable attaches again.

typeset -g _BEV_ENABLED=${_BEV_ENABLED:-0}
typeset -gA _BEV_SAVED_BINDINGS

_bev_binary() {
  if [[ -n ${BEV_BIN-} ]]; then print -r -- "$BEV_BIN"
  elif [[ -x ${HOME}/.local/bin/bev ]]; then print -r -- "${HOME}/.local/bin/bev"
  else whence -p bev 2>/dev/null
  fi
}

_bev_original_widget() {
  zle "${_BEV_SAVED_BINDINGS["$1:$2"]-accept-line}"
}

_bev_codex_path() { whence -p codex 2>/dev/null }

_bev_exec_codex() (
  local errors rc
  errors=$(command mktemp "${TMPDIR:-/tmp}/bev-codex.XXXXXX") || return 1
  trap 'command rm -f -- "$errors"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  trap 'exit 129' HUP
  if "$@" 2>"$errors"; then
    return 0
  else
    rc=$?
    if [[ -s $errors ]]; then
      command cat -- "$errors" >&2
    else
      builtin print -u2 -r -- "bev: Codex failed (exit $rc)"
    fi
    return "$rc"
  fi
)

_bev_route_codex() {
  local prompt=$1 map=$2 key=$3 codex_path
  codex_path=$(_bev_codex_path)
  if [[ -z $codex_path ]]; then
    zle -M 'bev: codex executable not found; input left editable'
    return 1
  fi
  BUFFER="_bev_exec_codex ${(q)codex_path} exec --model gpt-5.6-luna -c model_reasoning_effort=none --skip-git-repo-check -- ${(q)prompt}"
  _bev_original_widget "$map" "$key"
}

_bev_force_shell() {
  [[ ${CONTEXT-} == start && -n ${BUFFER-} ]] || return 0
  _bev_original_widget "$1" '^M'
}

_bev_force_codex() {
  [[ ${CONTEXT-} == start && -n ${BUFFER-} ]] || return 0
  _bev_route_codex "$BUFFER" "$1" '^M'
}

_bev_force_shell_emacs() { _bev_force_shell emacs; }
_bev_force_shell_viins() { _bev_force_shell viins; }
_bev_force_codex_emacs() { _bev_force_codex emacs; }
_bev_force_codex_viins() { _bev_force_codex viins; }

_bev_accept_line() {
  [[ ${CONTEXT-} == start && -n ${BUFFER-} && $BUFFER != bev-disable ]] || { _bev_original_widget "$1" "$2"; return; }
  local input=$BUFFER result rc bin
  bin=$(_bev_binary)
  if [[ -z $bin || ! -x $bin ]]; then
    zle -M 'bev: classifier not found; input left editable'
    return 1
  fi
  result=$(builtin printf '%s' "$input" | "$bin" classify 2>&1)
  rc=$?
  if (( rc != 0 )); then
    zle -M "${result:-bev: classifier failed}; input left editable (Ctrl-X s bypasses)"
    return $rc
  fi
  case $result in
    shell) _bev_original_widget "$1" "$2" ;;
    codex) _bev_route_codex "$input" "$1" "$2" ;;
    hold) zle -M 'bev: ambiguous input; use Ctrl-X s or Ctrl-X a' ;;
    *) zle -M 'bev: invalid classifier result; input left editable'; return 1 ;;
  esac
}

_bev_enter_emacs() { _bev_accept_line emacs '^M' }
_bev_enter_emacs_j() { _bev_accept_line emacs '^J' }
_bev_enter_viins() { _bev_accept_line viins '^M' }
_bev_enter_viins_j() { _bev_accept_line viins '^J' }

_bev_save_binding() {
  local map=$1 key=$2 line
  line=$(bindkey -M "$map" "$key" 2>/dev/null)
  _BEV_SAVED_BINDINGS["$map:$key"]=${line##* }
}

_bev_restore_binding() {
  local map=$1 key=$2 widget
  widget=${_BEV_SAVED_BINDINGS["$map:$key"]-accept-line}
  bindkey -M "$map" "$key" "$widget"
}

bev-enable() {
  (( _BEV_ENABLED )) && return 0
  local _bev_map
  zle -N _bev_enter_emacs
  zle -N _bev_enter_emacs_j
  zle -N _bev_enter_viins
  zle -N _bev_enter_viins_j
  zle -N _bev_force_shell_emacs
  zle -N _bev_force_shell_viins
  zle -N _bev_force_codex_emacs
  zle -N _bev_force_codex_viins
  for _bev_map in emacs viins; do
    _bev_save_binding "$_bev_map" '^M'
    _bev_save_binding "$_bev_map" '^J'
    _bev_save_binding "$_bev_map" '^Xs'
    _bev_save_binding "$_bev_map" '^Xa'
    bindkey -M "$_bev_map" '^M' "_bev_enter_${_bev_map}"
    bindkey -M "$_bev_map" '^J' "_bev_enter_${_bev_map}_j"
    bindkey -M "$_bev_map" '^Xs' "_bev_force_shell_${_bev_map}"
    bindkey -M "$_bev_map" '^Xa' "_bev_force_codex_${_bev_map}"
  done
  _BEV_ENABLED=1
}

bev-disable() {
  (( _BEV_ENABLED )) || return 0
  local _bev_map
  for _bev_map in emacs viins; do
    _bev_restore_binding "$_bev_map" '^M'
    _bev_restore_binding "$_bev_map" '^J'
    _bev_restore_binding "$_bev_map" '^Xs'
    _bev_restore_binding "$_bev_map" '^Xa'
  done
  zle -D _bev_enter_emacs 2>/dev/null || true
  zle -D _bev_enter_emacs_j 2>/dev/null || true
  zle -D _bev_enter_viins 2>/dev/null || true
  zle -D _bev_enter_viins_j 2>/dev/null || true
  zle -D _bev_force_shell_emacs 2>/dev/null || true
  zle -D _bev_force_shell_viins 2>/dev/null || true
  zle -D _bev_force_codex_emacs 2>/dev/null || true
  zle -D _bev_force_codex_viins 2>/dev/null || true
  _BEV_ENABLED=0
}

if [[ -o interactive ]]; then
  bev-enable
fi
