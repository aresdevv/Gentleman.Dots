# Tools Reference

Detailed overview of all tools configured by Gentleman.Dots.

## Terminal Emulators

| Tool | Description |
|------|-------------|
| **Ghostty** | GPU-accelerated, native, blazing fast |
| **Kitty** | Feature-rich, GPU-based rendering |
| **WezTerm** | Lua-configurable, cross-platform |
| **Alacritty** | Minimal, Rust-based, lightweight |

## Shells

| Tool | Description |
|------|-------------|
| **Nushell** | Structured data, modern syntax, pipelines |
| **Fish** | User-friendly, great defaults, no config needed |
| **Zsh** | Highly customizable, POSIX-compatible, Powerlevel10k |

## Multiplexers

| Tool | Description |
|------|-------------|
| **Tmux** | Battle-tested, widely used, lots of plugins |
| **Zellij** | Modern, WebAssembly plugins, floating panes |
| **Herdr** | Agent-focused terminal multiplexer for AI coding workflows |

## Editor

| Tool | Description |
|------|-------------|
| **Neovim** | LazyVim config with LSP, completions, AI integration |

## Prompts

| Tool | Description |
|------|-------------|
| **Starship** | Cross-shell prompt with Git integration |

## Known Issues

### Zellij: raw escape sequences printed at the prompt (WezTerm + WSL2)

**Symptom:** garbage resembling `rgb:8787/d7d7/8787...` is printed literally
at the shell prompt right after Zellij starts, most commonly reported with
WezTerm on Windows attaching to a WSL2 distro, with Nushell as the shell
([#181](https://github.com/Gentleman-Programming/Gentleman.Dots/issues/181)).

**Root cause:** this is an upstream Zellij bug, not a Gentleman.Dots
configuration issue. Panes inside Zellij don't talk to the real terminal
directly, so when a program running in the pane sends a terminal query — for
example an `OSC 4 ; N ; ?` color-palette query — Zellij's client has to
intercept it, forward it to the actual host terminal on the program's
behalf, and relay the reply back into the pane. The values in the garbled
output (`5f5f`, `8787`, `afaf`, `d7d7`, `ffff`) are exactly the doubled-byte
`rgb:RRRR/GGGG/BBBB` values of the standard xterm 256-color cube, confirming
this is a fragment of a 256-color palette query's reply stream, not random
corruption.

Zellij's `zellij-client/src/stdin_ansi_parser.rs` added this query-forwarding
mechanism (`active_forward`) in 0.44.2. A confirmed, still-open bug in it —
[zellij-org/zellij#5467](https://github.com/zellij-org/zellij/issues/5467),
open as of Zellij 0.45.1 — accumulates *every* classified host reply into
the currently-open forwarding slot with no check that it actually matches
the query that opened it, so an unrelated burst of replies (e.g. the rest of
a 256-entry palette query) can be misdelivered into a pane that only asked a
single, different question. This kind of timing-sensitive misdelivery is
more likely to occur when the round-trip to the terminal is slower than a
local pty, which is exactly what a WezTerm-on-Windows-relaying-through-WSL2
hop adds. We could not pin down which specific program in this stack (shell,
prompt, or another CLI tool) sends the triggering query without reproducing
the exact WezTerm + WSL2 + Zellij + Nushell combination live.

**What this repo does about it:** `GentlemanZellij/zellij/config.kdl` sets
`support_kitty_graphics_protocol false` (zellij >= 0.45.0 only), since this
setup doesn't render images in terminal panes and that protocol is one of
the *separate* capability queries Zellij performs for itself at startup.
This is a minor, harmless reduction of Zellij's own query traffic — it does
**not** touch the query-forwarding bug above, which is the actual cause of
the leak, and there is currently no documented Zellij config option or
environment variable to work around that bug directly.

**Workarounds if you hit this:** it's cosmetic — Zellij and the shell still
end up in the correct state once the burst finishes — and tends to only
affect the very first prompt after attaching. If it's disruptive, either
pin Zellij to a release before 0.44.2, or select Tmux or Herdr as your
multiplexer instead until upstream fixes #5467.
