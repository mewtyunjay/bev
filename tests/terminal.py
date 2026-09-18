#!/usr/bin/env python3
"""Exercise the real zsh editor and foreground terminal; no Jev calls are made."""

import argparse
import json
import os
from pathlib import Path
import pty
import re
import select
import shlex
import signal
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
CODEX_ARGS = ["exec", "--model", "gpt-5.6-luna", "-c", "model_reasoning_effort=none",
              "--skip-git-repo-check", "--"]


class Terminal:
    def __init__(self, directory, plugins, vi):
        self.directory = directory
        self.log = directory / "events.jsonl"
        self.output = bytearray()
        self.cursor = 0
        stub = directory / "stub.py"
        stub.write_text(
            "#!" + sys.executable + "\n"
            + '''import json, os, sys, time
from pathlib import Path
is_codex = Path(sys.argv[0]).name == "codex"
line = None if is_codex else sys.stdin.read()
event = {"kind": "codex" if is_codex else "classify", "line": line,
         "args": sys.argv[1:], "cwd": os.getcwd(),
         "tty": [os.isatty(fd) for fd in (0, 1, 2)]}
with open(os.environ["BEV_TEST_LOG"], "a") as f:
    f.write(json.dumps(event) + "\\n")
if is_codex:
    print("CODEX_STUB_FINISHED")
elif line.startswith("api-down"):
    print("bev: test API unavailable", file=sys.stderr)
    sys.exit(1)
elif line.startswith("stall"):
    time.sleep(10)
    print("shell")
elif line.startswith("uncertain"):
    print("hold")
else:
    print("codex" if line.startswith("explain ") else "shell")
'''
        )
        stub.chmod(0o755)
        (directory / "bev").symlink_to(stub)
        (directory / "codex").symlink_to(stub)
        rc = ["PROMPT='BEV_TEST> '", "RPROMPT=''", "bindkey -v" if vi else "bindkey -e", "KEYTIMEOUT=1"]
        rc += ["source " + shlex.quote(str(path)) for path in plugins]
        rc += ["source " + shlex.quote(str(ROOT / "shell/bev.zsh"))]
        (directory / ".zshrc").write_text("\n".join(rc) + "\n")
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            env = dict(os.environ, ZDOTDIR=str(directory), TERM="xterm-256color",
                       BEV_BIN=str(directory / "bev"), BEV_TEST_LOG=str(self.log),
                       PATH=str(directory) + os.pathsep + os.defpath)
            os.chdir(directory)
            os.execvpe("zsh", ["zsh", "-d", "-i"], env)
        self.expect(b"BEV_TEST> ")

    def send(self, text):
        os.write(self.fd, text.encode())

    def pump(self):
        if select.select([self.fd], [], [], 0.02)[0]:
            try:
                self.output.extend(os.read(self.fd, 65536))
            except OSError:
                raise AssertionError("zsh exited unexpectedly") from None

    def until(self, predicate, description):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            self.pump()
            if predicate():
                return
        raise AssertionError(description + "\n" + self.output[-6000:].decode(errors="replace"))

    def expect(self, text):
        def rendered():
            return re.sub(rb"\x1b\[[0-?]*[ -/]*[@-~]", b"", self.output)
        self.until(lambda: text in rendered()[self.cursor:], "Missing terminal output: " + repr(text))
        self.cursor = rendered().index(text, self.cursor) + len(text)

    def events(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def event(self, kind, field, value, after=0):
        def matches():
            return [event for event in self.events()[after:]
                    if event["kind"] == kind and event[field] == value]
        self.until(matches, "Missing event: " + repr((kind, field, value)))
        return matches()[-1]

    def run(self, command, expected):
        self.send(command + "\r")
        self.expect(expected.encode())
        self.expect(b"BEV_TEST> ")

    def close(self):
        os.kill(self.pid, signal.SIGKILL)
        os.waitpid(self.pid, 0)
        os.close(self.fd)


def check(plugins, vi):
    with tempfile.TemporaryDirectory(prefix="bev-terminal-") as path:
        directory = Path(path).resolve()
        terminal = Terminal(directory, plugins, vi)
        try:
            terminal.run("print 'SHELL_''OK'", "\r\nSHELL_OK\r\n")
            terminal.run("false; print 'STATUS_'$?", "\r\nSTATUS_1\r\n")
            terminal.send("false\r")
            terminal.expect(b"BEV_TEST> ")
            terminal.run("print 'PREVIOUS_'$?", "\r\nPREVIOUS_1\r\n")
            (directory / "child").mkdir()
            terminal.send("cd child\r")
            terminal.expect(b"BEV_TEST> ")
            terminal.run("print 'CWD_'\"$PWD\"", "\r\nCWD_" + str(directory / "child") + "\r\n")

            prompt = 'explain "quotes" \'apostrophes\' $(touch INJECTED) `touch INJECTED` ! & --help'
            terminal.send(prompt + "\r")
            event = terminal.event("codex", "args", CODEX_ARGS + [prompt])
            assert event["tty"] == [True, True, True], event
            assert event["cwd"] == str(directory / "child"), event
            terminal.expect(b"CODEX_STUB_FINISHED")
            terminal.expect(b"BEV_TEST> ")
            assert not (directory / "child/INJECTED").exists()

            for line in ["uncertain $(touch HELD)", "api-down $(touch HELD)"]:
                count = len(terminal.events())
                terminal.send(line + "\r")
                terminal.event("classify", "line", line, count)
                terminal.send("\x18a")
                terminal.event("codex", "args", CODEX_ARGS + [line], count)
                terminal.expect(b"CODEX_STUB_FINISHED")
                terminal.expect(b"BEV_TEST> ")
                assert not (directory / "child/HELD").exists()

            # A forced shell command must bypass even an unavailable classifier.
            count = len(terminal.events())
            terminal.send("print 'FORCED_''SHELL'\x18s")
            terminal.expect(b"\r\nFORCED_SHELL\r\n")
            terminal.expect(b"BEV_TEST> ")
            assert len(terminal.events()) == count

            # A missing classifier also leaves the line available for raw Enter.
            (directory / "bev").unlink()
            terminal.send("print 'MISSING_''CLASSIFIER'\r")
            terminal.expect(b"classifier not found")
            terminal.send("\x18s")
            terminal.expect(b"\r\nMISSING_CLASSIFIER\r\n")
            terminal.expect(b"BEV_TEST> ")
            (directory / "bev").symlink_to(directory / "stub.py")

            # Never consume the original prompt when Codex cannot be launched.
            (directory / "codex").unlink()
            line = "explain missing codex"
            terminal.send(line + "\r")
            terminal.expect(b"codex executable not found")
            (directory / "codex").symlink_to(directory / "stub.py")
            terminal.send("\x18a")
            terminal.event("codex", "args", CODEX_ARGS + [line])
            terminal.expect(b"CODEX_STUB_FINISHED")
            terminal.expect(b"BEV_TEST> ")

            # Ctrl-C must cancel a pending classifier and restore prompt input.
            count = len(terminal.events())
            terminal.send("stall $(touch INTERRUPTED)\r")
            terminal.event("classify", "line", "stall $(touch INTERRUPTED)", count)
            terminal.send("\x03")
            terminal.expect(b"BEV_TEST> ")
            terminal.run("print 'CANCELLED_''OK'", "\r\nCANCELLED_OK\r\n")
            assert not (directory / "child/INTERRUPTED").exists()

            # Pasted multiline prompts remain one literal Codex argument.
            prompt = "explain this code\nwith a second line"
            terminal.send("\x1b[200~" + prompt + "\x1b[201~\r")
            terminal.event("codex", "args", CODEX_ARGS + [prompt])
            terminal.expect(b"CODEX_STUB_FINISHED")
            terminal.expect(b"BEV_TEST> ")

            # Secondary shell prompts are never classified.
            count = len(terminal.events())
            terminal.send("for n in 1; do\r")
            terminal.event("classify", "line", "for n in 1; do", count)
            terminal.send("print 'CONTINUATION_''OK'; done\r")
            terminal.expect(b"\r\nCONTINUATION_OK\r\n")
            terminal.expect(b"BEV_TEST> ")
            assert len(terminal.events()) == count + 1

            # Repeated source and disable/enable must not stack wrappers.
            quoted_hook = shlex.quote(str(ROOT / "shell/bev.zsh"))
            terminal.run("source " + quoted_hook + "; source " + quoted_hook + "; print 'SOURCE_''OK'", "\r\nSOURCE_OK\r\n")
            (directory / "bev").unlink()
            terminal.send("bev-disable\r")
            terminal.expect(b"BEV_TEST> ")
            count = len(terminal.events())
            terminal.run("print 'DETACHED_''OK'", "\r\nDETACHED_OK\r\n")
            assert len(terminal.events()) == count
            (directory / "bev").symlink_to(directory / "stub.py")
            terminal.run("bev-enable; print 'ENABLED_''OK'", "\r\nENABLED_OK\r\n")
            count = len(terminal.events())
            terminal.run("print 'REATTACHED_''OK'", "\r\nREATTACHED_OK\r\n")
            assert len(terminal.events()) == count + 1
        finally:
            terminal.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plugin", type=Path, action="append", default=[])
    arguments = parser.parse_args()
    for vi in (False, True):
        check(arguments.plugin, vi)
    print("terminal integration: passed (emacs and vi insert)" + (" with plugins" if arguments.plugin else ""))
