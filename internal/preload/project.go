package preload

import (
	"regexp"
	"strings"
)

const RunnerTool = "Bash(henia preload *)"

func Invocation(skill, command string) string {
	return "henia preload --skill " + skill + " -- '" + strings.ReplaceAll(command, "'", `'\''`) + "'"
}

func Project(body, skill string, native bool) string {
	preloads := Find([]byte(body))
	if len(preloads) == 0 {
		return body
	}
	var b strings.Builder
	last := 0
	for _, p := range preloads {
		b.WriteString(body[last:p.Start])
		last = p.Stop
		invocation := strings.ReplaceAll(Invocation(skill, p.Command), "\n", "\n"+p.Indent)
		switch {
		case native && p.Block:
			b.WriteString(p.Indent + "```!\n" + p.Indent + invocation + "\n" + p.Indent + "```\n")
		case native:
			b.WriteString("!" + span(invocation))
		case p.Block:
			b.WriteString(p.Indent + "Run first:\n\n" + p.Indent + "```bash\n" + p.Indent + invocation + "\n" + p.Indent + "```\n")
		default:
			b.WriteString("run first: " + span(invocation))
		}
	}
	b.WriteString(body[last:])
	return b.String()
}

func span(text string) string {
	ticks := "`"
	for strings.Contains(text, ticks) {
		ticks += "`"
	}
	if ticks == "`" {
		return "`" + text + "`"
	}
	return ticks + " " + text + " " + ticks
}

var harnessPlaceholder = regexp.MustCompile(`\$\{CLAUDE_[A-Z_]+\}|\$ARGUMENTS(?:\[\d+\])?|\$\d+`)

func Declares(declared, command string) bool {
	if declared == command {
		return true
	}
	var pattern strings.Builder
	last := 0
	for _, loc := range harnessPlaceholder.FindAllStringIndex(declared, -1) {
		pattern.WriteString(regexp.QuoteMeta(declared[last:loc[0]]))
		if strings.HasPrefix(declared[loc[0]:], "${") {
			pattern.WriteString(`[^\s;&|$` + "`" + `'"()<>\\]*`)
		} else {
			pattern.WriteString(`[^\n;&|$` + "`" + `'"()<>\\]*`)
		}
		last = loc[1]
	}
	if last == 0 {
		return false
	}
	pattern.WriteString(regexp.QuoteMeta(declared[last:]))
	return regexp.MustCompile(`^` + pattern.String() + `$`).MatchString(command)
}
