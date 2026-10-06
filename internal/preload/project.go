package preload

import (
	"regexp"
	"slices"
	"strings"
)

const RunnerTool = "Bash(henia preload *)"

var (
	SlotsCommand   = regexp.MustCompile(`^henia slots --(markdown|inline)((?: [A-Za-z0-9_.-]+)+)$`)
	ShowCommand    = regexp.MustCompile(`^henia show (\S+) --(head|toc)$`)
	ContextCommand = regexp.MustCompile(`^henia context (\S+)$`)
)

func Own(command string) bool {
	return SlotsCommand.MatchString(command) || ShowCommand.MatchString(command) || ContextCommand.MatchString(command)
}

func Tool(command string) string {
	if !Own(command) {
		return RunnerTool
	}
	return "Bash(henia " + strings.Fields(command)[1] + " *)"
}

func Tools(body string) []string {
	var tools []string
	for _, p := range Find([]byte(body)) {
		if tool := Tool(p.Command); !slices.Contains(tools, tool) {
			tools = append(tools, tool)
		}
	}
	return tools
}

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
		invocation := p.Command
		if !Own(p.Command) {
			invocation = Invocation(skill, p.Command)
		}
		invocation = strings.ReplaceAll(invocation, "\n", "\n"+p.Indent)
		switch {
		case native && p.Block:
			b.WriteString(p.Indent + "```!\n" + p.Indent + invocation + "\n" + p.Indent + "```\n")
		case native:
			b.WriteString("!" + span(invocation))
		case p.Block:
			b.WriteString(p.Indent + "Run first:\n\n" + p.Indent + "```bash\n" + p.Indent + invocation + "\n" + p.Indent + "```\n")
		case !ownLine(body, p):
			b.WriteString("the output of " + span(invocation))
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
