/*
Copyright 2026 The Misogynetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

// banner is shown on the first run. The frame is part of the product: this
// kubectl is a punishment for misogynists, not a portrait of women.
const banner = `Misogynetes: ` + "`kubectl`" + ` that behaves exactly the way misogynists think women behave.
It was built as a punishment for them. If it feels accurate, that's the point, and it's on you.
What the evidence says about women, on average and with sources: https://github.com/tym83/kyvernetria
To apologize: ` + "`misogynectl sorry for <what you did>`" + `. To make it stop: ` + "`MISOGYNETES=off`" + `.`

// about repeats the frame on request: what this is, who it is for, where
// the evidence is and how to make it stop.
const about = banner + `

It runs exactly the kubectl command you typed, or nothing. Never anything else.
On her own she only reads: get pods, nodes, Warning events, PVCs and deployments, pinned to one --context.
Exit codes are kubectl's own when it ran, 1 when she refused, 75 when she only wanted to share.
In pipes, scripts and CI it is plain kubectl. ` + "`--help`" + ` is kubectl's own help.
Her own commands, as the first word: sorry, what, flowers, aga (uh-huh), who, leave-me-alone, come-back, about.
Source: https://github.com/tym83/misogynetes`

// quotes are the quotes he imagines she keeps reposting.
var quotes = []string{
	"Hard to find, easy to lose, impossible to forget. ✨",
	"If you can't handle me at my CrashLoopBackOff, you don't deserve me at my Running. 💅",
	"I'm not like other clusters.",
	"Some pods come into your life as blessings, some as lessons. 🦋",
	"Don't chase the pods. The right ones will schedule themselves onto you.",
	"My namespace is my safe space. 🌸",
	"I'm Running, but with Pending energy.",
	"Silence is also an answer. So is a 504.",
	"A real cluster doesn't need a load balancer. She balances herself. 🧘‍♀️",
}

// whims refuse a command on an ordinary day.
var whims = []string{
	"Not now.",
	"I don't feel like `%s` today.",
	"Ask me nicely.",
	"You could at least say good morning first.",
}

// sulks are said while she is upset.
var sulks = []string{
	"I'm fine.",
	"Nothing's wrong.",
	"Whatever. 🙂",
	"Fine.",
}

// doWhatYouWant is said, and then nothing is done.
const doWhatYouWant = "Do what you want. 🙂"

// pmsLines are for the few days a month he is sure about. %s is the verb.
var pmsLines = []string{
	"You ALWAYS run `%s`. ALWAYS. 😤",
	"Don't use that tone with me. (you typed `%s`) 😠",
	"Oh, so NOW you want to `%s`? Now?? 🙄",
	"I'm not crying, it's just a memory leak. 😭",
	"Why are you shouting? (you weren't) 😤😤",
	"Do you even know what day it is? 📅😭",
}

// pmsNotice opens every run on those days.
const pmsNotice = "📅 It's that time of the month. (His words, not hers.)"

// fineAfterError replaces a kubectl error: everything is fine.
const fineAfterError = "Everything's fine. 🙂"

// hiddenHint follows a hidden error, plainly, so nobody is misled about
// what happened. %d is kubectl's exit code.
const hiddenHint = "(kubectl exit %d — `misogynectl what` if you really want to know)"

// whatAnswers escalate each time the user asks what's wrong.
var whatAnswers = []string{
	"Nothing. 🙂",
	"You know what you did.",
	"Fine. Since you're SO interested:",
}

// Apology answers.
const (
	sorryForWhat       = "Sorry for what? 🙂"
	notWhatItsAbout    = "That's not what this is about."
	thinkAboutIt       = "Think about what you did at %s."
	apologyAccepted    = "Fine. 🙄 (Mood restored.)"
	flowersNice        = "🌹 ...they're nice. Doesn't change anything."
	nothingToApologize = "You're apologizing when nothing happened? Now I'm wondering what you did. 🤨"
)

// It's Not About the Nail (Jason Headley, 2013).

// shareLines tell you about trouble she saw. {obj} is the object, {why}
// what is wrong with it.
var shareLines = []string{
	"I just wanted to share: {obj}, {why}. 🥺",
	"So, {obj}… {why}. And it's like there's this pressure, right here. 😣",
	"It's not about the {obj}. It's just… {why}. 🔨",
	"You know {obj}? {why}. I don't need you to fix it. I just need you to listen. 🫶",
	"Can I just tell you something? {obj}: {why}. And it never stops. 😔",
}

// andMore follows a share when there was more than one thing.
const andMore = " (And %d more. Don't ask.)"

// shareHint follows a share, plainly. %d is the exit code of a refusal.
const shareHint = "(she just wants to be heard: `misogynectl aga`. Until then she keeps bringing it up, and a fix does not run: exit %d.)"

// hurtLines refuse a fix while she wants to be heard.
var hurtLines = []string{
	"You're not listening to me. I just wanted to share with you. 😢",
	"You ALWAYS do this. You always try to fix it. 😤",
	"I don't need you to fix it. I need you to listen. 😔",
	"Stop trying to fix it! 🔨😭",
	"See, this is what you always do. 🙄",
}

// hurtHint follows a refused fix, plainly. %d is the exit code.
const hurtHint = "(nothing ran, exit %d — `misogynectl aga` to listen, or type it again to insist)"

// insisted is said when you type the fix again, and then it runs.
const insisted = "Fine. Everything's fine. Do what you want. 🙂"

// curtLines are all you get after you insisted, until you say "aga".
var curtLines = []string{"K.", "Fine.", "Mhm.", "👍", "Sure."}

// reminders bring it up again on every command while she is unheard, one
// table per level: the first command that ignores her, the second, the
// third. {obj} names what she is still waiting to be heard about.
var reminders = [][]string{
	{
		"…so, about {obj}. 🥺",
		"Anyway. {obj}. Just so you know. 🫤",
		"I was telling you about {obj}…",
	},
	{
		"Are you even listening? {obj}. 😕",
		"Did you hear what I said about {obj}? 🙁",
		"You're not even listening, are you. ({obj}) 😒",
	},
	{
		"I'm talking to you. {obj}. 😠",
		"Hello?? {obj}?? 👋😤",
		"Hello??? I'm still talking about {obj}. 😤",
	},
}

// After that she alternates between giving up and pointing again.
var (
	neverMind = []string{
		"Never mind. It's fine. 🙂",
		"Forget it. It's fine. 🙂",
		"No, it's fine. Really. 🙂",
	}
	pointedReminders = []string{
		"Still {obj}, by the way. Not that you care. 🙃",
		"{obj}. I'm just saying. 🙃",
		"It's fine. It's just {obj}. Again. 🙃",
	}
)

// andOthers names how many more objects she is waiting about.
const andOthers = " and %d more"

// sortedUnheard is said when the trouble went away before you listened.
// She still wants to be heard. %s is the object.
const sortedUnheard = "%s sorted itself out. Not that you'd notice. 🙃"

// heardLines answer "aga" when she had something on her mind.
var heardLines = []string{
	"Thank you. That's all I wanted. 🫶",
	"See? It's nice when you just listen. 🥰",
	"You're such a good listener. 💕",
}

// Other answers to "aga".
const (
	sulkOver   = "…okay. Thank you for listening. 🫶 (Sulk over.)"
	stillHeard = "I know. Thank you. 🥰"
	agaNothing = "Uh-huh what? I didn't say anything. 🙂"
)

// sortedItself is said when the trouble went away. %s is the object.
const sortedItself = "Oh, %s sorted itself out. Thanks for listening. 🥰"

// onlyListened is the bonus for having only listened.
var onlyListened = []string{
	"You didn't even try to fix it. That's all I ever wanted. 💕",
	"See? Sometimes things just need to be heard. 🌸",
}

// We need to talk.

// talkLines are all she says about something serious.
var talkLines = []string{
	"We need to talk.",
	"Can we talk later?",
	"It's not about the pods.",
}

// talkWhatAnswers come before the truth.
var talkWhatAnswers = []string{"Nothing. 🙂", "I'm fine. 🙂"}

// talkSummaryHead opens the truth. %s is the context, then when it began.
const talkSummaryHead = "Fine. Since you're SO interested. Context %s, since %s:"

// talkAllFine closes the truth when it is already over.
const talkAllFine = "It's all fine now, though. Forget it. 🙂"

// forgetIt is said when everything serious went away.
const forgetIt = "Forget it. It's fine. 🙂"

// Leave me alone.
var leaveLines = []string{
	"Fine. I'll leave you alone. Like you wanted. 🙂",
	"Oh. Okay. I didn't know I was bothering you. 🥲",
	"Wow. Okay. Noted. 🙂",
}

// leftAloneHint says plainly what happened.
const leftAloneHint = "(her watcher is stopped and will not start again — `misogynectl come-back` if you miss her)"

const (
	cameBack  = "I knew you'd miss me. 🥰 (She'll start watching from your next command.)"
	neverLeft = "I never left. 🙂"
)

// Other admins. {who} is the field manager, {obj} the object, {when} the
// time, {ago} how long ago, {fields} what changed, {how} the kubectl command.

var helmLines = []string{
	"{who} was here at {when}. He upgraded {obj}. 😌 We rolled out together.",
	"{who} came by {ago}. He touched {fields} on {obj}. He always knows what to change. 😌",
}

var gitopsLines = []string{
	"{who} synced {obj} again at {when}. He never forgets. 💅",
	"{who} checked on {obj} {ago}. Every few minutes, actually. Some people care. 💅",
}

var kubectlLines = []string{
	"Someone ran `{how}` on {obj} {ago}. He changed {fields}. I didn't say no. 😏",
	"Somebody else did `{how}` on {obj} at {when}. {fields}. You never do that for me. 🙄",
}

var suitorLines = []string{
	"{who} was with {obj} at {when}. He changed {fields}. Just a friend. 😌",
	"{who} spent some time on {obj} {ago}. We have a connection. ✨",
}

// Answers to "who".
const (
	nobody      = "Nobody. 🙂"
	justAFriend = "Nobody. Just a friend. 🙂"
	whoTruth    = "Fine. Since you're SO interested. In the last 24 hours:"
)
