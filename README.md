# Misogynetes

**A punishment for misogynists.**

`misogynectl` is `kubectl` that behaves exactly the way misogynists think
women behave. It posts fake-deep quotes, sulks, says everything is fine when
nothing is, refuses with "Do what you want", expects you to know what you did,
wants an apology for the right thing, and has what he calls "that time of the
month" four days in every 28.

If you find it accurate, that is the point, and it is on you.

What the evidence actually says about women, on average and with sources, is
in [Kyvernetria](https://github.com/tym83/kyvernetria). Its companion,
[Mansplainetes](https://github.com/tym83/mansplainetes), is the colleague who
explains your own commands back to you.

```text
$ misogynectl delete pod nope -n shop
Everything's fine. 🙂
(kubectl exit 1 — `misogynectl what` if you really want to know)

$ misogynectl what
Nothing. 🙂
$ misogynectl what
You know what you did.
$ misogynectl what
Fine. Since you're SO interested:
At 07:16 you ran: delete pod nope
Error from server (NotFound): pods "nope" not found

$ misogynectl get ns
Do what you want. 🙂

$ misogynectl sorry
Sorry for what? 🙂
Think about what you did at 07:16.
$ misogynectl flowers
🌹 ...they're nice. Doesn't change anything.
$ misogynectl sorry for being late
That's not what this is about.
Think about what you did at 07:16.
$ misogynectl sorry for the delete
Fine. 🙄 (Mood restored.)

$ misogynectl get ns
Silence is also an answer. So is a 504.
NAME              STATUS   AGE
...

$ misogynectl get pods -n shop          # on one of those days
📅 It's that time of the month. (His words, not hers.)
Do you even know what day it is? 📅😭
```

## How she behaves (in his head)

- **First run:** a banner that says what this is and who it is for.
- **Ordinary days:** a quote now and then ("Hard to find, easy to lose,
  impossible to forget."), and now and then a whim ("Not now.", "Ask me nicely.").
- **After an error:** the error is hidden behind "Everything's fine.",
  followed by one plain line with kubectl's exit code. The real message
  comes out on the third `misogynectl what`.
- **Upset:** half the commands get "Do what you want." and nothing happens.
  An apology works only if it names what you did (`misogynectl sorry for the
  delete`). Apologizing when nothing happened makes it worse. Flowers are nice
  and change nothing.
- **Four days out of 28 (his idea of a cycle):** everything above, louder, with emoji. The cycle
  starts at a random day when you first run it.

## It never breaks anything

It hides errors from you, which is the joke. It does not hide anything else.

- It runs exactly the command you typed, or nothing. It never runs anything else.
- Exit codes are honest: kubectl's own code when it ran (128+n if a signal
  killed it), 1 when she refused. A hidden error always says its exit code.
- It hides errors only from short commands like `get`, `apply` or `delete`.
  Interactive and long-running commands (`exec`, `run -it`, `attach`,
  `debug`, `edit`, `logs -f`, `port-forward`, anything with `-w`, plugins,
  login prompts) get kubectl's stderr as it comes. Anything kubectl says
  after 3 seconds reaches you as it comes, too.
- Her own commands (`sorry`, `what`, `flowers`, `about`) count only as the
  first word. `misogynectl --as what get pods` is kubectl's.
- It acts up only for a person at a terminal. In pipes, scripts and CI it is
  plain `kubectl`, errors included. `--help` is kubectl's own help.
- `MISOGYNETES=off` makes it plain `kubectl` anywhere; `MISOGYNETES=always`
  makes it act up even into a pipe.
- `MISOGYNETES_KUBECTL` points at a different kubectl.
- Her memory lives in your user cache directory (`misogynetes/state.json`).
  It holds the verb, resource and name of the last failed command, never a
  flag or its value, and at most 2KB of the error with tokens blanked out.
- `misogynectl about` says what this is, who it is for, and how to make it
  stop.

## Install

```bash
go install github.com/tym83/misogynetes@latest
```

Install it on your own machine. Don't install it on anyone else's.

## License

Apache License 2.0.
