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

## It's not about the nail

A tribute to [*It's Not About the Nail*](https://www.youtube.com/watch?v=-4EDhdAHrOg)
(Jason Headley, 2013): she has a nail in her forehead and just wants to be
heard, and he keeps trying to fix it. Here, the nail is a pod in
`CrashLoopBackOff`.

```text
$ misogynectl get pods -n shop
NAME     READY   STATUS             RESTARTS      AGE
web-1    0/1     CrashLoopBackOff   6 (40s ago)   9m
I just wanted to share: pod/web-1 in shop, it's CrashLoopBackOff. 🥺
(she just wants to be heard: `misogynectl aga`. Fix it before that and nothing runs, exit 75.)

$ misogynectl delete pod web-1 -n shop
You're not listening to me. I just wanted to share with you. 😢
(nothing ran, exit 75 — `misogynectl aga` to listen, or type it again to insist)

$ misogynectl aga
Thank you. That's all I wanted. 🫶

$ misogynectl get pods -n shop
NAME     READY   STATUS    RESTARTS      AGE
web-1    1/1     Running   6 (25m ago)   40m
Oh, pod/web-1 in shop sorted itself out. Thanks for listening. 🥰
You didn't even try to fix it. That's all I ever wanted. 💕
```

- **She shares.** When the output of your own `get`, `describe`, `logs` or
  `events` shows trouble (`CrashLoopBackOff`, `Error`, `OOMKilled`,
  `ImagePullBackOff`, `NotReady`, a recent restart, a `Warning` event,
  errors in the logs), she says so after kubectl's output and remembers the
  object. She reads only table and description output: `-o yaml`, `-o json`
  and the like are left alone.
- **You listen.** `misogynectl aga` (also `ага`, `угу`, `uh-huh`, `mhm`, `aha`,
  `yeah`) runs no kubectl and tells her you heard. After that you may fix
  whatever you like.
- **You fix.** A command that changes something (`delete`, `apply`, `edit`,
  `patch`, `scale`, `rollout restart`, `set`, `drain`, `cordon`, `label`,
  `exec`, ...) within 15 minutes of an unheard share is refused: nothing runs
  and the exit code is **75**. Type the same command again within 2 minutes to
  insist: it runs with kubectl's own exit code, she says "Fine. Everything's
  fine. Do what you want." and answers the next two commands curtly.
  `misogynectl aga` ends that too.
- **It sorts itself out.** When a later read shows the object healthy, she
  says so and thanks you for listening, with something extra if all you did
  was listen.

She never asks the cluster anything herself: she only reads the output of the
commands you typed, while it reaches your terminal unchanged.

## It never breaks anything

It hides errors from you, which is the joke. It does not hide anything else.

- It runs exactly the command you typed, or nothing. It never runs anything else.
- Exit codes are honest: kubectl's own code when it ran (128+n if a signal
  killed it), 1 when she refused, 75 when she refused a fix because she only
  wanted to share. A hidden error and a refusal always say their exit code.
- It hides errors only from short commands like `get`, `apply` or `delete`.
  Interactive and long-running commands (`exec`, `run -it`, `attach`,
  `debug`, `edit`, `logs -f`, `port-forward`, anything with `-w`, plugins,
  login prompts) get kubectl's stderr as it comes. Anything kubectl says
  after 3 seconds reaches you as it comes, too.
- Her own commands (`sorry`, `what`, `flowers`, `aga` and its synonyms,
  `about`) count only as the first word. `misogynectl --as what get pods` is kubectl's.
- It acts up only for a person at a terminal, with both stdout and stderr on
  it. In pipes (`misogynectl get pods | grep web` included), scripts and CI it
  is plain `kubectl`, errors included. `--help` is kubectl's own help.
- `MISOGYNETES=off` makes it plain `kubectl` anywhere; `MISOGYNETES=always`
  makes it act up even into a pipe.
- `MISOGYNETES_KUBECTL` points at a different kubectl.
- Her memory lives in your user cache directory (`misogynetes/state.json`).
  It holds the verb, resource and name of the last failed command, never a
  flag or its value, and at most 2KB of the error with tokens blanked out.
  For the nail, it holds the namespace, kind and name of the objects she
  shared and a reason word, never an event message or a log line.
- `misogynectl about` says what this is, who it is for, and how to make it
  stop.

## Install

```bash
go install github.com/tym83/misogynetes@latest
```

Install it on your own machine. Don't install it on anyone else's.

## License

Apache License 2.0.
