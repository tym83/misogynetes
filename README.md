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
(she just wants to be heard: `misogynectl aga`. Until then she keeps bringing it up, and a fix does not run: exit 75.)

$ misogynectl get ns
…so, about pod/web-1 in shop. 🥺
NAME      STATUS   AGE
default   Active   9d

$ misogynectl top nodes
Are you even listening? pod/web-1 in shop. 😕
...

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
- **She keeps bringing it up.** Until you say `aga`, every command you type,
  whatever it is about, starts with a reminder that names what she is still
  waiting to be heard about, and every command that ignores her makes it
  louder: "…so, about pod/web-1", then "Are you even listening?", then
  "Hello??", then for ever after "Never mind. It's fine." taking turns with
  pointed reminders. Being unheard does not wear off with time; only after a
  day does she let it go.
- **You listen.** `misogynectl aga` (also `ага`, `угу`, `uh-huh`, `mhm`, `aha`,
  `yeah`) runs no kubectl, tells her you heard and starts her over from
  calm. After that you may fix whatever you like.
- **You fix.** A command that changes something (`delete`, `apply`, `edit`,
  `patch`, `scale`, `rollout restart`, `set`, `drain`, `cordon`, `label`,
  `exec`, ...) while she is unheard is refused: nothing runs and the exit
  code is **75**. Type the same command again within 2 minutes to insist: it
  runs with kubectl's own exit code, she says "Fine. Everything's fine. Do
  what you want." and from then on answers every command curtly, on top of
  the reminders, until `misogynectl aga`.
- **It sorts itself out.** When a later read shows the object healthy, she
  says so and thanks you for listening, with something extra if all you did
  was listen. If you never listened, it is "sorted itself out. Not that you'd
  notice.", and she still wants to be heard about it until `aga`.

She reads along with the output of the commands you typed, which reaches
your terminal unchanged. And she does not wait for you to look.

## She comes running

The first command you type at a terminal starts her watcher in the
background: one per user and context, detached, at low priority. Every 30
seconds (`MISOGYNETES_POLL`, at least 5s, with backoff on errors) she looks at
the pods, nodes, Warning events, volume claims and deployments of the
context's namespace (`MISOGYNETES_WATCH_ALL=1` for all namespaces). Whatever
she finds, she tells you right away, by writing to the terminal you started
her from, in the middle of whatever you were doing:

```text
$ vim deploy.yaml

I just wanted to share: pod/web-1, it's CrashLoopBackOff. 🥺
(she just wants to be heard: `misogynectl aga`. Until then she keeps bringing it up, and a fix does not run: exit 75.)
```

Then every 3 minutes (`MISOGYNETES_NAG`, at least 1 minute), louder each
time, until you say `aga`. If the API server is unreachable, that is a
complaint too. `MISOGYNETES_NOTIFY=1` also sends each one as a macOS
notification.

- `misogynectl leave-me-alone` stops her watcher, and she takes it
  personally. It stays stopped until `misogynectl come-back`.
- It also stops on `MISOGYNETES=off`, when the terminal goes away or no longer
  belongs to you, and after 12 hours. `MISOGYNETES_WATCH=off` keeps it from
  starting at all.
- Scripts, pipes and CI never start it.

## We need to talk

When something is seriously wrong (a node `NotReady`, the API server gone or
answering with server errors, three or more pods failing in one namespace,
an out-of-memory or volume-mount storm, a claim `Lost` or `Pending` for more
than 10 minutes, a deployment with no replicas available, certificate
warnings), she says only this, and nothing more:

```text
We need to talk.
```

```text
$ misogynectl what
Nothing. 🙂
$ misogynectl what
I'm fine. 🙂
$ misogynectl what
Fine. Since you're SO interested. Context prod, since 14:02:
  • node/node-2: node NotReady, since 14:02 → kubectl --context prod describe node node-2
  • deployment/api: 0/3 replicas available, since 14:02 → kubectl --context prod rollout status deployment/api
```

The third answer is the truth: every affected object, what is wrong, since
when she has seen it, and the read-only command to look further. The joke is
the wait, never the facts. Until the talk happens (or you say `aga`), fixes
are refused as with the nail. When everything is healthy again, she says
"Forget it. It's fine."

## It never breaks anything

It hides errors from you, which is the joke. It does not hide anything else.

- It runs exactly the command you typed, or nothing. It never runs anything
  else for you.
- On her own, her watcher only reads: `get` of pods, nodes, Warning events,
  claims and deployments, every call pinned to an explicit `--context`. She
  never reads secrets or configmaps and never runs a command that changes
  anything. What she writes down from it is the same as for the nail: names
  and reason words, never messages or field values.
- Exit codes are honest: kubectl's own code when it ran (128+n if a signal
  killed it), 1 when she refused, 75 when she refused a fix because she only
  wanted to share. A hidden error and a refusal always say their exit code.
- It hides errors only from short commands like `get`, `apply` or `delete`.
  Interactive and long-running commands (`exec`, `run -it`, `attach`,
  `debug`, `edit`, `logs -f`, `port-forward`, anything with `-w`, plugins,
  login prompts) get kubectl's stderr as it comes. Anything kubectl says
  after 3 seconds reaches you as it comes, too.
- Her own commands (`sorry`, `what`, `flowers`, `aga` and its synonyms,
  `leave-me-alone`, `come-back`, `about`) count only as the first word. `misogynectl --as what get pods` is kubectl's.
- It acts up only for a person at a terminal, with both stdout and stderr on
  it. In pipes (`misogynectl get pods | grep web` included), scripts and CI it
  is plain `kubectl`, errors included. `--help` is kubectl's own help.
- `MISOGYNETES=off` makes it plain `kubectl` anywhere and stops her watcher;
  `MISOGYNETES=always` makes it act up even into a pipe (her watcher still
  starts only at a real terminal).
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
