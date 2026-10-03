package security

import (
	"list"

	"github.com/srnnkls/fas/cue/bash"
	"github.com/srnnkls/fas/cue/hook"
	"github.com/srnnkls/fas/cue/tool"
)

_heniaLibrary: =~#"(^|/)(\.henia/skills|henia/sources)(/|$)"#

_libraryReadVerb: "cat" | "less" | "more" | "head" | "tail" | "bat" | "nl" | "tac" |
	"xxd" | "od" | "hexdump" | "strings" | "base64" | "cp" | "scp" | "rsync" |
	"grep" | "egrep" | "fgrep" | "rg" | "ag" | "awk" | "sed" | "sort" | "uniq" |
	"ls" | "find" | "fd" | "tree"

_denyHeniaLibrary: {
	rule_id:  "read-henia-library"
	reason:   "Library skills are served by henia, not read as files: use `henia show <skill>[#section]`, and `henia ls` to list them."
	severity: "MEDIUM"
}

read_henia_library: {
	when: hook.#PreToolUse & tool.#Read & {
		tool_input: file_path: _heniaLibrary
	}
	then: deny: _denyHeniaLibrary
}

grep_henia_library: {
	when: hook.#PreToolUse & tool.#Grep & {
		tool_input: path: _heniaLibrary
	}
	then: deny: _denyHeniaLibrary
}

glob_henia_library: {
	when: hook.#PreToolUse & tool.#Glob & {
		tool_input: path: _heniaLibrary
	}
	then: deny: _denyHeniaLibrary
}

bash_read_henia_library: {
	when: hook.#PreToolUse & tool.#Bash & (bash.#call & {
		#match: {command: _libraryReadVerb, targets: list.MatchN(>0, _heniaLibrary)}
	})
	then: deny: _denyHeniaLibrary
}
