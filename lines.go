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
Exit codes are kubectl's own when it ran, 1 when she refused.
In pipes, scripts and CI it is plain kubectl. ` + "`--help`" + ` is kubectl's own help.
Her own commands, as the first word: sorry, what, flowers, about.
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
