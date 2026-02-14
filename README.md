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
export WTX_CONFIG_PATH="$HOME/.config/wtx/config.json"
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

### `wtx new [task] [base-branch] [codex|claude]`

Direct worktree creation + optional AI launch.
Aliases: `new`, `nw`, `new-worktree`

```bash
wtx new "fix lint errors" develop codex
wtx nw "fix lint errors" develop codex
```

### `wtx clean`

Removes local worktrees whose branches are already merged into `mainBranch`.

```bash
wtx clean
```

### `wtx switch [index|branch|path]`

Select a local worktree and open a shell in it.

```bash
wtx switch
wtx switch 2
wtx switch feature/my-branch
```

## Config

`wtx` reads config in this order:
- `WTX_CONFIG_PATH` env var (preferred)
- `./config.json` (current directory)
- `.tmyjoe/wtx/config.json` (repo-local fallback)
- `config.json` next to the binary

Main config keys:
- `mainBranch`
- `defaultBaseBranch`
- `worktreesDir`
- `copyFiles`
- `postCreateHooks`
- `llm.default`
- `llm.allowed`
- `llm.branchNamePromptTemplate`
- `llm.commands.*`

Example:

```json
{
  "copyFiles": [
    { "from": "apps/web/.env.local", "to": "apps/web/.env.local" }
  ],
  "postCreateHooks": [
    {
      "name": "Install frontend dependencies",
      "cwd": "apps/web",
      "command": ["pnpm", "install"],
      "skipIfMissing": true
    }
  ]
}
```

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

These wrappers set `WTX_CONFIG_PATH` automatically and call the Go CLI.

## License

MIT (or your preferred license)
