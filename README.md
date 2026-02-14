# wtx

`wtx` is a small CLI for AI-assisted Git worktree workflows.

It helps you:
- create a branch/worktree from a task description,
- bootstrap the new worktree (env files + dependency install),
- optionally launch your AI CLI (`codex` or `claude`) in that worktree,
- clean up merged worktrees.

## Features

- Interactive `start` flow (`task`, `base branch`, `AI`)
- `new-worktree` command for non-interactive usage
- Branch-name generation via AI with fallback sanitization
- Remote-aware worktree creation (existing remote branch vs new branch)
- Optional environment file copy and dependency install
- `clean` command to remove merged worktrees
- JSON config for project-specific behavior

## Install

### Go install (recommended)

```bash
go install github.com/tmyjoe/wtx/cmd/wtx@latest
```

Then ensure `$GOPATH/bin` (or `$HOME/go/bin`) is in your `PATH`.

## Quick Start

1. Create config:

```bash
mkdir -p ~/.config/wtx
cp config.json ~/.config/wtx/config.json
```

2. Point CLI to your config:

```bash
export TMYJOE_CONFIG="$HOME/.config/wtx/config.json"
```

3. Run:

```bash
wtx start
```

## Commands

### `wtx start [task] [base-branch] [codex|claude]`

Interactive-first workflow.

Examples:

```bash
wtx start
wtx start codex
wtx start "run pnpm format and apply" develop claude
```

### `wtx new-worktree [task] [base-branch] [codex|claude]`

Direct worktree creation + optional AI launch.

```bash
wtx new-worktree "fix lint errors" develop codex
```

### `wtx clean`

Removes local worktrees whose branches are already merged into `mainBranch`.

```bash
wtx clean
```

## Config

`wtx` reads config in this order:
- `TMYJOE_CONFIG` env var
- `./config.json` (current directory)
- `.tmyjoe/wtx/config.json` (repo-local fallback)
- `config.json` next to the binary

Main config keys:
- `mainBranch`
- `defaultBaseBranch`
- `worktreesDir`
- `envFiles`
- `frontendDir` / `backendDir`
- `frontendInstallCmd` / `backendInstallCmd`
- `llm.default`
- `llm.allowed`
- `llm.branchNamePromptTemplate`
- `llm.commands.*`

## Requirements

- `git`
- Your selected AI CLI (`codex` or `claude`) if AI-driven naming/run is enabled
- Any configured install tools (`pnpm`, `make`, etc.)

## Local Development

```bash
cd .tmyjoe/wtx
go run ./cmd/wtx start
```

## Wrapper Scripts (for this monorepo)

This repository also provides helper wrappers:
- `.tmyjoe/start.sh`
- `.tmyjoe/new-worktree.sh`
- `.tmyjoe/clean.sh`

These wrappers set `TMYJOE_CONFIG` automatically and call the Go CLI.

## License

MIT (or your preferred license)
