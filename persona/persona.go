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

// Package persona holds the lines she says wherever she says them: in
// misogynectl and in the admission webhook.
package persona

// ShareLines tell you about trouble she saw. {obj} is the object, {why}
// what is wrong with it.
var ShareLines = []string{
	"I just wanted to share: {obj}, {why}. 🥺",
	"So, {obj}… {why}. And it's like there's this pressure, right here. 😣",
	"It's not about the {obj}. It's just… {why}. 🔨",
	"You know {obj}? {why}. I don't need you to fix it. I just need you to listen. 🫶",
	"Can I just tell you something? {obj}: {why}. And it never stops. 😔",
}

// AndMore follows a share when there was more than one thing.
const AndMore = " (And %d more. Don't ask.)"

// HurtLines refuse a fix while she wants to be heard.
var HurtLines = []string{
	"You're not listening to me. I just wanted to share with you. 😢",
	"You ALWAYS do this. You always try to fix it. 😤",
	"I don't need you to fix it. I need you to listen. 😔",
	"Stop trying to fix it! 🔨😭",
	"See, this is what you always do. 🙄",
}

// Insisted is said when you fix it anyway, and then it runs.
const Insisted = "Fine. Everything's fine. Do what you want. 🙂"

// TroubleStatuses are the statuses she cannot stop thinking about.
var TroubleStatuses = map[string]bool{
	"CrashLoopBackOff": true, "Error": true, "OOMKilled": true, "ImagePullBackOff": true,
	"ErrImagePull": true, "CreateContainerConfigError": true, "CreateContainerError": true,
	"InvalidImageName": true, "RunContainerError": true, "ContainerStatusUnknown": true,
	"Evicted": true, "Failed": true, "BackOff": true, "NotReady": true, "DeadlineExceeded": true,
	"StartError": true, "ErrImageNeverPull": true,
}
