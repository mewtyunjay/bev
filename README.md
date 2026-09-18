# bev

Type a shell command or a natural-language request at your normal zsh prompt.
Bev asks Jev which it is: commands run in the current shell, requests run headlessly
with `codex exec` using `gpt-5.6-luna` and reasoning effort `none`. Only the final
answer appears in the terminal; startup logs and progress are hidden. Failed runs
show their diagnostics. Control then returns to your shell. Uncertain results and
API failures leave your input editable.

Go standard library only. Ghostty needs no configuration changes. The integration
supports zsh's emacs and vi insert keymaps; vi command mode keeps its usual bindings.

## Install

Requires Go 1.22+, zsh, and an installed, signed-in `codex` executable on your PATH.
From this checkout:

```sh
make install
```

This builds and installs `~/.local/bin/bev` and
`~/.local/share/bev/bev.zsh`. It does **not** edit your shell configuration.
For another destination, use `make install PREFIX=/your/prefix`, set `BEV_BIN` to
that prefix's `bin/bev`, and source its `share/bev/bev.zsh` instead.

## Configure your Jev key and try it

Put your Jev key in the checkout's `.env` file:

```zsh
export TYPESAFE_API_KEY='your-jev-api-key'
```

`.env` is ignored by Git. On a fresh clone, create it from `.env.example`, restrict
its permissions, and edit it before loading:

```zsh
cp -n .env.example .env
chmod 600 .env
# Edit .env, then load it into this shell:
source .env
```

Bev reads the exported environment variable. It does not search the current
directory for `.env` files when you run commands in other projects.

Check the API before attaching the hook:

```zsh
printf '%s' 'git status' | "$HOME/.local/bin/bev" classify
printf '%s' 'explain why the build is failing' | "$HOME/.local/bin/bev" classify
```

Each successful call prints `shell`, `codex`, or `hold`. Then attach to this shell:

```zsh
source "$HOME/.local/share/bev/bev.zsh"
```

Now type a command such as `git status`, or a request such as
`explain the architecture of this project`, and press Enter. A Codex route submits
the original text as a headless Codex request:

```sh
codex exec --model gpt-5.6-luna -c model_reasoning_effort=none --skip-git-repo-check -- "your request"
```

The command uses your current directory, works outside Git repositories, and returns
to the shell when finished. Bev shows only its final answer on success, retaining
error diagnostics for failed runs. Your other Codex settings still apply.

| Input | Behavior |
| --- | --- |
| Enter / Ctrl-J | Classify the current line, then route it |
| Ctrl-X, then `s` | Run the current line directly in zsh; bypass Jev |
| Ctrl-X, then `a` | Run the current line through headless Codex; bypass Jev |
| `bev-disable` | Restore the previous Enter and shortcut bindings |
| `bev-enable` | Attach again in this shell |

Ctrl-X followed by a letter is a two-step key sequence. The exact command
`bev-disable` always bypasses Jev, so detaching also works offline.
The shortcuts temporarily replace any bindings you previously had on those keys.

## Enable in new terminals

Add this block **at the end of `~/.zshrc`, after your other shell plugins**.
Replace `/absolute/path/to/bev` with the location of this checkout:

```zsh
# >>> bev >>>
source /absolute/path/to/bev/.env
source "$HOME/.local/share/bev/bev.zsh"
# <<< bev <<<
```

Open a new Ghostty tab. These lines enable Bev in every interactive zsh that reads
your `.zshrc`, including terminals other than Ghostty. To limit it to Ghostty, put
the block inside `if [[ -n ${GHOSTTY_RESOURCES_DIR-} ]]; then ...; fi`.

## Detach or uninstall

For the current shell, run `bev-disable`. This restores the bindings that existed
before Bev was enabled; no terminal restart is required.

To detach permanently, remove the marked Bev block from `~/.zshrc`, then disable
Bev in each existing tab or close those tabs. Reopen a tab to check your normal shell.
Removing the block does not affect already-running shells.

To also remove the installed binary and integration file, run from this checkout:

```sh
make uninstall
```

Use the same `PREFIX` if you installed elsewhere. Uninstall does not edit `.zshrc`
or delete your key. To remove the saved key too:

```zsh
rm -f /absolute/path/to/bev/.env
unset TYPESAFE_API_KEY
```

## Settings

Set these environment variables before using the hook:

| Variable | Default | Meaning |
| --- | --- | --- |
| `TYPESAFE_API_KEY` | Required | Your TypeSafe/Jev bearer token |
| `BEV_MODEL` | `jev-1.13.0` | Jev model ID; pinned for repeatable evaluation |
| `BEV_MIN_PROBABILITY` | `0.90` | Required winning probability; greater than 0.5 and at most 1 |
| `BEV_TIMEOUT` | `2s` | HTTP request deadline; positive Go duration, at most 30s |
| `BEV_BIN` | `~/.local/bin/bev`, then PATH lookup | Classifier executable path |

For example, `export BEV_TIMEOUT=3s` gives a slow connection more time. A higher
probability threshold produces more `hold` results. Thresholds are initial policy,
not a measured accuracy guarantee; evaluate your own command/request examples.

## How it works and limits

The zsh hook reads the literal edit buffer before Enter executes it. The Go binary
makes one request to `https://api.typesafe.ai/v1/systemone` using a single Choice
question with `shell`, `natural_language`, and `ambiguous` options. It validates the
answer and probability distribution before returning a routing decision.

Shell input is handed back to the original Enter widget. Codex input is escaped
as one literal argument and handed back through that same widget, so `codex exec`
runs in the foreground without opening the interactive interface. No generated
command is evaluated. Codex's stderr is captured in a private temporary file and
shown only on failure; the file is removed when the command exits. The resulting quoted
Codex invocation is what the shell records in its normal command history.

- Each classified line is sent to TypeSafe. Shell history, directory contents, and
  environment values are not included. For input containing secrets, use the
  force-shell shortcut to bypass the API. The binary keeps no local request log.
- Empty input and secondary prompts (for example, an unfinished `for` loop) keep
  normal shell behavior. Scripts, running programs, and Codex's execution are
  outside the hook. Buffers above 32 KiB or invalid UTF-8 are rejected.
- There is no retry loop. Network failures, missing credentials, malformed answers,
  and low probability leave the buffer editable. The overrides work without Jev.
- Jev judges intent, not command safety. An intended command can still be harmful
  or contain a typo. Ordinary words can also be command names; ambiguity is expected.
- Classification adds a network round trip to each nonempty top-level Enter.
  This version has no cache or daemon. Real Jev accuracy and latency need evaluation
  with your key and your own examples.

API contract: [TypeSafe HTTP API](https://docs.typesafe.ai/api),
[Choice](https://docs.typesafe.ai/primitives/choice),
[models](https://docs.typesafe.ai/models).
Codex behavior: [non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode),
[GPT-5.6 Luna reasoning settings](https://developers.openai.com/api/docs/models/gpt-5.6-luna).

## Development checks

```sh
make test
```

Tests require Go, zsh, and Python 3 (standard library only). They use mock HTTP
responses and fake classifier/Codex executables; no real API key is required.
The terminal check runs an actual zsh in a pseudo-terminal and verifies literal
prompt arguments, foreground terminal access, current-directory and exit-status
preservation, uncertain/error cases, continuation prompts, and detach/reattach.

To check installed shell plugins as well:

```sh
python3 tests/terminal.py \
  --plugin "$HOME/.zsh/fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh" \
  --plugin "$HOME/.zsh/zsh-autosuggestions/zsh-autosuggestions.zsh"
```
