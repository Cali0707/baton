# baton

> **WARNING: This project is experimental and has been vibe-coded without thorough review. Use at your own risk. Expect rough edges, bugs, and breaking changes.**

A terminal UI tool that uses AI agents (via the [Agent Client Protocol](https://agentclientprotocol.com/get-started/introduction)) to automatically analyze GitHub issues and pull requests.

This project serves as a demonstration of the utility of ACP -- because agents communicate over a standard protocol, you can easily swap different agents in and out of workflows without changing any application code.

## What it does

- Browse GitHub issues and PRs from configured repositories
- Launch AI agents to investigate bugs, evaluate feature requests, or review PRs
- Agents run in isolated git worktrees so they can safely explore code without affecting your working tree
- Run multiple agents in parallel with real-time progress tracking
- Review completed analysis sessions

## Building

```
make build
```

Or directly:

```
go build -o bin/baton ./cmd/baton
```

## Prerequisites

This tool assumes that the repositories you configure are set up as git worktrees with a bare checkout. Agents create isolated worktrees to safely explore code, so this layout is required.

To set up a repo this way:

```bash
mkdir my-repo && cd my-repo
git clone --bare <remote-url> .git
git config remote.origin.fetch "+refs/heads/*:refs/remotes/origin/*"
git fetch origin
git remote set-head origin --auto
DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD | sed 's@^refs/remotes/origin/@@')
git worktree add "$DEFAULT_BRANCH" "$DEFAULT_BRANCH"
```

## Configuration

Copy `config.example.toml` to `~/.config/baton/config.toml` and edit it to configure:

- GitHub repositories to monitor
- AI agents to use (e.g. Claude CLI with `--acp`)
- Label filters and safety policies

## Usage

```
./bin/baton [path/to/config.toml]
```

If no config path is given, it defaults to `~/.config/baton/config.toml`.

Session data and logs are stored in `~/.local/share/baton/`.
