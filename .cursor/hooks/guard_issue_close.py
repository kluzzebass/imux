#!/usr/bin/env python3
"""Cursor beforeShellExecution: block direct dcat close / git merge in this repo.

scripts/close-issue.sh runs dcat close and git merge as subprocesses; those are
not visible to this hook, so the sanctioned close path still works.

Bypass (human escape hatch): include IMUX_SHELL_HOOK_BYPASS=1 anywhere in the
full shell command string.
"""
from __future__ import annotations

import json
import os
import re
import sys


def _repo_root() -> str:
    here = os.path.dirname(os.path.realpath(__file__))
    return os.path.realpath(os.path.join(here, "..", ".."))


def _under_repo(path: str, repo_root: str) -> bool:
    path = os.path.realpath(path)
    repo_root = os.path.realpath(repo_root)
    if path == repo_root:
        return True
    prefix = repo_root + os.sep
    return path.startswith(prefix)


def _split_top_level_chain(cmd: str) -> list[str]:
    """Split on && / ; only outside quotes (so JSON/string payloads are not split)."""
    segs: list[str] = []
    cur: list[str] = []
    i = 0
    sq = dq = False
    depth_paren = 0
    n = len(cmd)
    while i < n:
        c = cmd[i]
        if sq:
            cur.append(c)
            if c == "'":
                sq = False
            i += 1
            continue
        if dq:
            cur.append(c)
            if c == "\\" and i + 1 < n:
                cur.append(cmd[i + 1])
                i += 2
                continue
            if c == '"':
                dq = False
            i += 1
            continue
        if c == "'":
            sq = True
            cur.append(c)
            i += 1
            continue
        if c == '"':
            dq = True
            cur.append(c)
            i += 1
            continue
        if c == "(":
            depth_paren += 1
            cur.append(c)
            i += 1
            continue
        if c == ")" and depth_paren > 0:
            depth_paren -= 1
            cur.append(c)
            i += 1
            continue
        if depth_paren == 0 and i + 1 < n and cmd[i : i + 2] == "&&":
            s = "".join(cur).strip()
            if s:
                segs.append(s)
            cur = []
            i += 2
            continue
        if depth_paren == 0 and c == ";":
            s = "".join(cur).strip()
            if s:
                segs.append(s)
            cur = []
            i += 1
            continue
        cur.append(c)
        i += 1
    tail = "".join(cur).strip()
    if tail:
        segs.append(tail)
    return segs


def _resolve_cd(cwd: str, target: str) -> str:
    target = target.strip().strip("'\"")
    if target.startswith("/"):
        return os.path.realpath(target)
    return os.path.realpath(os.path.join(cwd, target))


# Leading env assignments only (keeps "echo dcat close" from tripping the guard).
_PREFIX = r"^\s*(?:\w+=[^\s]+\s+)*"


def _segment_hits_dcat_close(seg: str) -> bool:
    return re.match(_PREFIX + r"dcat\s+close(?:\s|$)", seg) is not None


def _segment_hits_git_merge(seg: str) -> bool:
    # Require whitespace or end after "merge" so git merge-base does not match.
    return re.match(_PREFIX + r"git\s+merge(?:\s|$)", seg) is not None


def _shell_c_wraps_dcat_close(seg: str) -> bool:
    """Catch bash -c 'dcat close …' style evasion (single-quoted -c payload only)."""
    return bool(
        re.search(
            r"\b(?:bash|zsh|sh)\s+-c\s+'[^']*\bdcat\s+close\b",
            seg,
        )
    )


def _shell_c_wraps_git_merge(seg: str) -> bool:
    return bool(
        re.search(
            r"\b(?:bash|zsh|sh)\s+-c\s+'[^']*\bgit\s+merge(?:\s|')",
            seg,
        )
    )


def _deny(reason: str, detail: str) -> None:
    msg = (
        f"Blocked: {reason}. "
        "Use the enforced transaction: just close-issue … (after CLAUDE.md steps 1–3). "
        "Emergency bypass: prefix the command with IMUX_SHELL_HOOK_BYPASS=1 "
        "(not for agent use)."
    )
    out = {
        "permission": "deny",
        "user_message": msg,
        "agent_message": f"{msg} ({detail})",
    }
    sys.stdout.write(json.dumps(out))
    sys.exit(2)


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.stdout.write(json.dumps({"permission": "allow"}))
        return

    cmd = data.get("command") or ""
    cwd = data.get("cwd") or os.getcwd()

    if "IMUX_SHELL_HOOK_BYPASS=1" in cmd:
        sys.stdout.write(json.dumps({"permission": "allow"}))
        return

    repo_root = _repo_root()
    vcwd = os.path.realpath(cwd)

    for seg in _split_top_level_chain(cmd):
        if seg.startswith("cd "):
            rest = seg[3:].strip()
            if rest:
                try:
                    vcwd = _resolve_cd(vcwd, rest)
                except OSError:
                    pass
            continue

        if not _under_repo(vcwd, repo_root):
            continue

        if _segment_hits_dcat_close(seg) or _shell_c_wraps_dcat_close(seg):
            _deny("dcat close is not allowed from the agent shell", "dcat close")
        if _segment_hits_git_merge(seg) or _shell_c_wraps_git_merge(seg):
            _deny("git merge is not allowed from the agent shell", "git merge")

    sys.stdout.write(json.dumps({"permission": "allow"}))


if __name__ == "__main__":
    main()
