# typeghast

A terminal typing game haunted by your coding agent's context window.

![typeghast: typing what the agent just did while ghosts named after its context swarm the text, until a compact exorcises them](assets/demo.gif)

You kick off a coding agent, then wait. typeghast gives you something to do
in that terminal while it works, and it plays off what the agent is doing:

- **You type what your agent just did.** A local open-weight model reads the
  agent's session as it happens and writes short changelog lines for you to
  type: *"Added retries with backoff to the webhook client so flaky
  deliveries recover."*
- **Ghosts are context rot.** The fuller the agent's context window gets, the
  more ghosts drift over the words and hide letters. Some hunt your cursor,
  and as the haunting nears its peak they turn a rotten green. Each one is
  named after something filling the context: `webhook retries`, `ci logs`.
- **`/compact` and `/clear` are exorcisms.** When the context drops, the
  ghosts flee.
- **Typing through the haunting pays.** Your score multiplier goes up to ×3
  at the peak.
- **🎃 The pumpkin keeps count.** The header shows what the session would
  cost at API prices.

There's also a **normal mode** with no agent and no ghosts. You type today's
top headlines from DEV, Lobsters and Hacker News. Type a headline without a
mistake and its link unlocks.

## Install

With Go 1.27 or later:

```sh
go install github.com/cliffkoome/typeghast@latest
```

Or download a binary for Linux, macOS or Windows from the
[releases page](https://github.com/cliffkoome/typeghast/releases).

## Play

```sh
typeghast            # pick haunted or normal mode
typeghast --demo     # haunted mode with a made-up agent, no setup needed
```

In haunted mode, typeghast finds the agent sessions active on your machine.
If exactly one was active in the last half hour, it follows that one. Run it
in a split pane or a second terminal next to your agent.

| Key | Does |
| --- | --- |
| type, space | type the words; backspace can go back into a word you got wrong |
| enter | finish a sentence's last word and start the next line |
| ↑ / ↓ | pick a headline you've typed (normal mode) |
| ctrl+o | open the shown link |
| ctrl+r | restart |
| esc | back to the menu, then quit |
| pgup / pgdn, home | raise or lower the context, compact (demo only) |

Correct words flash, and fast typing leaves speed lines behind the cursor.
Perfect sentences and 25-word streaks shake the screen. Use `--no-shake` if
you'd rather it didn't.

## The narrator: local and open

The lines you type come from a model running **on your machine** through
[Ollama](https://ollama.com). Nothing about your session leaves your
computer: the agent's transcript is read from disk, and the model that
summarises it runs locally.

typeghast uses Gemma if it's installed, otherwise whichever model you have:

```sh
ollama pull gemma4:12b
typeghast                      # picks up Gemma automatically
typeghast --model qwen2.5:7b-instruct
```

The model gets the agent's recent actions (files edited, commands run, what
it said) and returns 2 to 4 short sentences, plus names for the ghosts.
Narration runs in the background while you type lines that are already
queued, so a few seconds per batch never holds you up.

On a Radeon RX 6600, Gemma 4 12B writes the cleanest lines, taking 3 to 6
seconds per batch. Qwen 2.5 7B is two to three times faster and a little
rougher.

**No model installed?** typeghast still works. It uses the agent's own words:
the first sentence of its messages, its command descriptions, and the files
it edits. Pass `--no-model` to choose this on purpose.

## How it works

- **Context:** typeghast tails the agent's session transcript and reads the
  token usage of its latest reply: input, cache reads, cache writes and
  output.
- **Ghosts:** the haunting peaks at a fraction (`--haunt-at`, default 0.4) of
  the model's real context window. Window sizes and prices come from
  [LiteLLM's model catalog](https://github.com/BerriAI/litellm), cached for a
  day.
- **Cost:** the pumpkin adds up every reply in the session once, subagents
  included, at that model's API rates. On a subscription you don't pay per
  token, so read it as "what this would have cost".
- **Headlines:** fetched from the public DEV, Lobsters and Hacker News APIs
  and cached for an hour. A built-in set is used offline.

Caches live in your user cache directory under `typeghast/`.

## Supported agents

- **Claude Code**: sessions are read from `~/.claude/projects/`.

Other terminal agents are planned (Codex CLI, Gemini CLI, OpenCode) once each
one's session format is supported.

## Options

```
--demo              haunted mode without an agent
--mode haunted|normal  skip the menu
--session FILE      follow this session transcript (.jsonl)
--haunt-at 0.4      fraction of the context window where the haunting peaks
--threshold N       peak at N tokens instead
--max-ghosts 14     ghosts on screen at the peak
--model NAME        Ollama model to narrate with
--ollama URL        Ollama server (default http://localhost:11434)
--no-model          type the agent's own words instead
--no-shake          keep the screen still
--version           print the version
```

## Build from source

```sh
git clone https://github.com/cliffkoome/typeghast
cd typeghast
go build .
go test ./...
```

The demo GIF is recorded with [VHS](https://github.com/charmbracelet/vhs):
`vhs assets/demo.tape`.

## License

MIT
