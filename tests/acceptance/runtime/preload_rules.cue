package rules

import (
	"github.com/srnnkls/fas/cue/bash"
	"github.com/srnnkls/fas/cue/henia"
)

no_clock_preload: {
	when: henia.#Preload & (bash.#command & {#name: "date"})
	then: deny: {
		rule_id:  "no-clock-preload"
		reason:   "Clock preloads are off in this fixture"
		severity: "LOW"
	}
}
