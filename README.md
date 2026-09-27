# anyresume

Resume any Claude Code session from one list. You do not need to know the
folder where the session started: anyresume resumes each session in its own
folder for you. It runs in any terminal and as a [herdr](https://herdr.dev)
plugin.

anyresume is for Claude Code, but it is not an Anthropic product.

```
session>
  4/4 ───────────────────────────────────────────────────
  Enter: resume the session. Esc: close. ● = open in a live Claude process.
▌ 09-26 23:05  ● Fix the login form
  09-26 21:49    Plan the next release
  09-26 14:00    日本語のドキュメントを翻訳
  09-25 17:13    Clean up the build scripts (archived)
```

## What it does

- Reads every Claude Code session on the computer: the transcripts in
  `~/.claude/projects` and the chats of the Claude desktop app.
- Shows one list, newest first, in [fzf](https://github.com/junegunn/fzf).
  Each row has the date, the title, and a mark for a session that a live
  Claude process holds.
- Resumes the chosen session with `claude --resume <id>`, in the folder of the
  session. A desktop chat keeps its model, effort, and permission mode.
- In herdr, opens the session in a named tab, or shows the tab that already
  runs it.
- Can make one herdr tab for every session (`anyresume import`). Each tab has
  the resume command on its prompt: open the tab and press Enter.

## Install

You need Claude Code (`claude` on `PATH`) and fzf.

### As a herdr plugin

```sh
herdr plugin install AIToolSharing/anyresume
```

The install downloads the release binary for your system. Then bind a key in
the herdr `config.toml`:

```toml
[[keys.command]]
key = "prefix+f"
type = "plugin_action"
command = "aitoolsharing.anyresume.pick"
description = "resume a Claude Code session"
```

Run `herdr server reload-config`. Press the key to open the list in a popup.

### As a command

Download the archive for your system from
[Releases](https://github.com/AIToolSharing/anyresume/releases) and put
`anyresume` on your `PATH`. Or build it with Go 1.26 or newer:

```sh
go install github.com/AIToolSharing/anyresume@latest
```

## Use

| Command | What it does |
|---|---|
| `anyresume` | Show the list and resume the chosen session. |
| `anyresume --loop` | Keep the list open after each choice. Good for a herdr tab. |
| `anyresume --query TEXT` | Start with a search. One match resumes at once. |
| `anyresume list [--json]` | Print all sessions, newest first. |
| `anyresume open <id>` | Resume one session. An ID prefix of 8 characters is enough. |
| `anyresume resume <id>` | Resume one session in this terminal, also in herdr. |
| `anyresume import` | Make a herdr tab for each session. |
| `anyresume import --refresh` | Type the resume command again in the imported tabs. |

In herdr (`HERDR_ENV=1`), a session opens in the workspace "claude chats". In
other terminals, it opens in the current terminal.

## Import all sessions into herdr

`anyresume import` makes one tab for each session that does not run in herdr.
It puts the tabs in one workspace for each day, for example "chats 09-26". It
types the resume command on the prompt of each tab, but it does not press
Enter. No Claude process starts until you open a tab and press Enter.

Each tab keeps one idle shell. A PowerShell tab uses about 75 MB of memory,
and a `cmd.exe` tab uses about 5 MB. For hundreds of tabs on Windows, set the
herdr shell before the import:

```toml
[terminal]
default_shell = "cmd.exe"
```

On Windows, import stops when less than 1 GB of memory commit is free. Run it
again to continue. `--limit N` makes at most N new tabs.

herdr restores tabs with empty prompts after a restart. The plugin runs
`anyresume import --refresh` at herdr startup, and that types the commands
again. anyresume records the imported tabs in `herdr-tabs.json` in your user
config folder.

## Safety

- **No copies.** anyresume never starts Claude Code with `--fork-session`. A
  session always continues under its own ID.
- **One writer.** Two processes that write one session split the
  conversation. Claude Code does not stop a resume when the desktop app holds
  the session. anyresume reads the live-session registry of Claude Code
  (`~/.claude/sessions/<pid>.json`) and refuses to resume a session that
  another live process holds.
- **No bypass.** anyresume never restores `bypassPermissions` or `plan` mode.
  Claude Code also does not restore them on resume.

## How it finds sessions

- Transcripts: `~/.claude/projects/<folder>/<session id>.jsonl`.
  `CLAUDE_CONFIG_DIR` changes `~/.claude`. anyresume skips subagent
  transcripts and transcripts without messages.
- Titles: the title that you or the desktop app gave the session, then the
  title that Claude Code generated, then the agent name, the summary, and the
  last prompt.
- Desktop chats: the `claude-code-sessions` folder of the Claude desktop app
  (`%APPDATA%\Claude` on Windows, `~/Library/Application Support/Claude` on
  macOS). anyresume reads the title, the folder, the archived flag, and the
  settings.

These files are internal to Claude Code and to the desktop app. A new version
can change them. If anyresume shows wrong data after an update, open an issue.

## Development

```sh
go test ./...
```

The tests include property tests with [rapid](https://github.com/flyingmutant/rapid).

To release:

1. Change the version in `herdr-plugin.toml` in three places: the `version`
   line, the Windows download URL, and the argument of `fetch-binary.sh`.
2. Commit the change.
3. Push a tag with the same version, for example `v0.2.0`.

The release workflow checks the three places, runs the tests, and publishes
the archives with GoReleaser.

## License

MIT
