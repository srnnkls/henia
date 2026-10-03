package preload

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type Refusal struct {
	Rule   string
	Reason string
}

func (r *Refusal) String() string { return "blocked by " + r.Rule + ": " + r.Reason }

type Rule struct {
	Command     string   `toml:"command,omitempty"`
	Subcommands []string `toml:"subcommands,omitempty"`
	Pattern     string   `toml:"pattern,omitempty"`
	Reason      string   `toml:"reason,omitempty"`
}

func refuse(rule, format string, args ...any) *Refusal {
	return &Refusal{Rule: "henia/" + rule, Reason: fmt.Sprintf(format, args...)}
}

type word struct {
	text    string
	literal bool
}

type pattern struct {
	re     *regexp.Regexp
	reason string
}

type checker struct {
	dir      string
	rules    []Rule
	patterns []pattern
	depth    int
}

func Check(command, dir string, rules []Rule) *Refusal {
	c := &checker{dir: dir}
	for _, rule := range rules {
		switch {
		case (rule.Pattern == "") == (rule.Command == ""):
			return &Refusal{Rule: "henia.toml", Reason: "a preload.refuse rule needs either a command or a pattern"}
		case rule.Pattern != "":
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return &Refusal{Rule: "henia.toml", Reason: fmt.Sprintf("invalid preload.refuse pattern %q: %v", rule.Pattern, err)}
			}
			c.patterns = append(c.patterns, pattern{re, cmp.Or(rule.Reason, "matches "+rule.Pattern)})
		default:
			c.rules = append(c.rules, rule)
		}
	}
	for _, p := range c.patterns {
		if p.re.MatchString(command) {
			return &Refusal{Rule: "henia.toml", Reason: p.reason}
		}
	}
	return c.script(command)
}

func (c *checker) script(source string) *Refusal {
	if c.depth > 8 {
		return refuse("uninspectable", "commands nest too deeply to inspect")
	}
	c.depth++
	defer func() { c.depth-- }()
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(source), "")
	if err != nil {
		return refuse("unparseable", "cannot parse the command: %v", err)
	}
	var refusal *Refusal
	syntax.Walk(file, func(node syntax.Node) bool {
		if refusal != nil {
			return false
		}
		if stmt, ok := node.(*syntax.Stmt); ok {
			for _, r := range stmt.Redirs {
				if refusal = redirect(r); refusal != nil {
					return false
				}
			}
			if call, ok := stmt.Cmd.(*syntax.CallExpr); ok && len(call.Args) > 0 {
				words := make([]word, len(call.Args))
				for i, arg := range call.Args {
					words[i].text, words[i].literal = literal(arg)
				}
				refusal = c.command(words, stmt.Redirs)
			}
		}
		return refusal == nil
	})
	return refusal
}

func literal(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(unescape(p.Value))
		case *syntax.SglQuoted:
			if p.Dollar && strings.Contains(p.Value, `\`) {
				return "", false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				lit, ok := q.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(unescape(lit.Value))
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			if s[i] == '\n' {
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var harmlessTargets = []string{"/dev/null", "/dev/stdout", "/dev/stderr"}

func redirect(r *syntax.Redirect) *Refusal {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrInOut, syntax.RdrClob, syntax.AppClob, syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob:
	case syntax.DplOut:
		if target, ok := literal(r.Word); ok && (target == "-" || strings.Trim(target, "0123456789") == "") {
			return nil
		}
	default:
		return nil
	}
	target, ok := literal(r.Word)
	if !ok {
		return refuse("redirect", "redirects output to a target that is not literal")
	}
	if slices.Contains(harmlessTargets, target) {
		return nil
	}
	return refuse("redirect", "redirects output into %s", target)
}

func (c *checker) stdin(redirs []*syntax.Redirect) (string, bool) {
	for _, r := range slices.Backward(redirs) {
		switch r.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			if r.Hdoc == nil {
				return "", true
			}
			return literal(r.Hdoc)
		case syntax.WordHdoc:
			text, ok := literal(r.Word)
			return text + "\n", ok
		case syntax.RdrIn:
			name, ok := literal(r.Word)
			if !ok {
				return "", false
			}
			return c.read(name)
		}
	}
	return "", false
}

func (c *checker) read(name string) (string, bool) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(c.dir, name)
	}
	data, err := os.ReadFile(name)
	return string(data), err == nil
}

func (c *checker) command(words []word, redirs []*syntax.Redirect) *Refusal {
	if !words[0].literal || strings.ContainsAny(words[0].text, "*?[") {
		return refuse("dynamic-command", "the command name is not literal")
	}
	name := path.Base(words[0].text)
	args := words[1:]
	for _, rule := range c.rules {
		if rule.Command != name {
			continue
		}
		sub, ok := subcommand(args, nil)
		if len(rule.Subcommands) == 0 || !ok || slices.Contains(rule.Subcommands, sub) {
			return &Refusal{Rule: "henia.toml", Reason: cmp.Or(rule.Reason, strings.TrimSpace(name+" "+sub)+" is refused by preload.refuse")}
		}
	}
	switch {
	case slices.Contains(privileged, name):
		return refuse("privilege", "%s runs a command with other privileges", name)
	case wrappers[name] != nil:
		return c.wrapped(name, args, redirs)
	case slices.Contains(shells, name):
		return c.shell(name, args, redirs)
	case name == "eval":
		var parts []string
		for _, a := range args {
			if !a.literal {
				return refuse("uninspectable", "eval runs a command that is not literal")
			}
			parts = append(parts, a.text)
		}
		return c.script(strings.Join(parts, " "))
	case name == "source" || name == ".":
		return refuse("uninspectable", "%s runs a script Henia cannot inspect", name)
	case name == "find":
		return c.find(args)
	case slices.Contains(fileWriters, name):
		return refuse("file-write", "%s changes files", name)
	case name == "sed" || name == "gsed" || name == "perl":
		return inPlace(name, args)
	case name == "gh":
		return c.gh(args, redirs)
	case name == "curl":
		return curl(args)
	case name == "wget":
		return wget(args)
	case slices.Contains(httpie, name):
		return httpieCall(name, args)
	case slices.Contains(remoteShells, name):
		return refuse("remote", "%s runs commands on or copies files to another host", name)
	case slices.Contains(processControl, name):
		return refuse("process-control", "%s signals or stops processes", name)
	case name == "osascript":
		return refuse("daemon", "osascript drives other applications")
	case name == "emacsclient":
		for _, a := range args {
			if a.text == "-e" || a.text == "--eval" || strings.HasPrefix(a.text, "--eval=") {
				return refuse("daemon", "emacsclient --eval changes the running Emacs")
			}
		}
		return nil
	case name == "security":
		sub, ok := subcommand(args, nil)
		if !ok {
			return refuse("dynamic-command", "the security subcommand is not literal")
		}
		for _, prefix := range []string{"add-", "delete-", "set-", "create-", "import", "unlock", "lock", "remove-", "authorizationdb", "trust-settings-import"} {
			if strings.HasPrefix(sub, prefix) {
				return refuse("daemon", "security %s changes the keychain", sub)
			}
		}
		return nil
	}
	if family, ok := subcommandRules[name]; ok {
		if family.always {
			return refuse(family.rule, "%s %s", name, family.effect)
		}
		sub, ok := subcommand(args, family.valued)
		if !ok {
			return refuse("dynamic-command", "the %s subcommand is not literal", name)
		}
		if slices.Contains(family.subcommands, sub) {
			return refuse(family.rule, "%s %s %s", name, sub, family.effect)
		}
		if allowed, nested := family.nested[sub]; nested {
			next, ok := subcommand(afterSubcommand(args, family.valued), nil)
			if !ok || !slices.Contains(allowed, next) {
				return refuse(family.rule, "%s %s %s %s", name, sub, cmp.Or(next, "(default)"), family.effect)
			}
		}
	}
	return nil
}

func subcommand(args []word, valued []string) (string, bool) {
	rest := positionals(args, valued)
	if len(rest) == 0 {
		return "", true
	}
	return rest[0].text, rest[0].literal
}

func afterSubcommand(args []word, valued []string) []word {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !a.literal || !strings.HasPrefix(a.text, "-") || a.text == "-":
			return args[i+1:]
		case a.text == "--":
			return args[min(i+2, len(args)):]
		case slices.Contains(valued, a.text):
			i++
		}
	}
	return nil
}

func positionals(args []word, valued []string) []word {
	var rest []word
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !a.literal || !strings.HasPrefix(a.text, "-") || a.text == "-":
			rest = append(rest, a)
		case a.text == "--":
			return append(rest, args[i+1:]...)
		case slices.Contains(valued, a.text):
			i++
		}
	}
	return rest
}

func skipOptions(args []word, valued []string) []word {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !a.literal || !strings.HasPrefix(a.text, "-") || a.text == "-":
			return args[i:]
		case a.text == "--":
			return args[i+1:]
		case slices.Contains(valued, a.text):
			i++
		}
	}
	return nil
}

func (c *checker) wrapped(name string, args []word, redirs []*syntax.Redirect) *Refusal {
	w := wrappers[name]
	for _, a := range args {
		if slices.Contains(w.inspectNothing, a.text) {
			return nil
		}
		if slices.Contains(w.uninspectable, a.text) || slices.ContainsFunc(w.uninspectable, func(flag string) bool { return strings.HasPrefix(a.text, flag+"=") }) {
			return refuse("uninspectable", "%s %s runs a command Henia cannot inspect", name, a.text)
		}
	}
	inner := skipOptions(args, w.valued)
	if name == "env" {
		for len(inner) > 0 && inner[0].literal && strings.Contains(inner[0].text, "=") && !strings.HasPrefix(inner[0].text, "=") {
			inner = skipOptions(inner[1:], w.valued)
		}
	}
	if w.positional && len(inner) > 0 {
		inner = inner[1:]
	}
	if len(inner) == 0 {
		return nil
	}
	if name == "xargs" {
		inner = append(slices.Clone(inner), word{text: "<stdin>"})
	}
	return c.command(inner, redirs)
}

func (c *checker) shell(name string, args []word, redirs []*syntax.Redirect) *Refusal {
	command := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.literal {
			return refuse("uninspectable", "%s runs a script Henia cannot inspect", name)
		}
		switch {
		case a.text == "--":
			args = args[i+1:]
			i = len(args)
		case a.text == "-o" || a.text == "+o" || a.text == "-O" || a.text == "+O" || a.text == "--rcfile" || a.text == "--init-file":
			i++
		case (strings.HasPrefix(a.text, "-") || strings.HasPrefix(a.text, "+")) && len(a.text) > 1:
			if !strings.HasPrefix(a.text, "--") && strings.Contains(a.text, "c") {
				command = true
			}
		default:
			if command {
				return c.script(a.text)
			}
			return refuse("uninspectable", "%s runs the script %s, which Henia does not inspect", name, a.text)
		}
	}
	if command {
		return refuse("uninspectable", "%s -c has no command", name)
	}
	script, ok := c.stdin(redirs)
	if !ok {
		return refuse("uninspectable", "%s reads commands Henia cannot inspect", name)
	}
	return c.script(script)
}

func (c *checker) find(args []word) *Refusal {
	for i := 0; i < len(args); i++ {
		switch args[i].text {
		case "-delete":
			return refuse("file-write", "find -delete deletes files")
		case "-fprint", "-fprint0", "-fprintf", "-fls":
			return refuse("file-write", "find %s writes a file", args[i].text)
		case "-exec", "-execdir", "-ok", "-okdir":
			end := i + 1
			for end < len(args) && args[end].text != ";" && args[end].text != "+" {
				end++
			}
			if end > i+1 {
				if refusal := c.command(args[i+1:end], nil); refusal != nil {
					return refusal
				}
			}
			i = end
		}
	}
	return nil
}

func inPlace(name string, args []word) *Refusal {
	for _, a := range args {
		if a.text == "--in-place" || strings.HasPrefix(a.text, "--in-place=") {
			return refuse("file-write", "%s --in-place edits files", name)
		}
		if strings.HasPrefix(a.text, "-") && !strings.HasPrefix(a.text, "--") && strings.ContainsRune(a.text[1:], 'i') && a.literal {
			if name == "sed" || name == "gsed" {
				if i := strings.IndexAny(a.text[1:], "ef"); i >= 0 && i < strings.IndexRune(a.text[1:], 'i') {
					continue
				}
			}
			return refuse("file-write", "%s -i edits files in place", name)
		}
	}
	return nil
}

var readMethods = []string{"GET", "HEAD", "OPTIONS", "POST", "QUERY"}

func method(tool, value string, literal bool) *Refusal {
	if !literal {
		return refuse("http-method", "%s sends a request whose method is not literal", tool)
	}
	if !slices.Contains(readMethods, strings.ToUpper(value)) {
		return refuse("http-method", "%s sends a %s request", tool, strings.ToUpper(value))
	}
	return nil
}

const curlValued = "AbcCdDeEFHKmoPQrtTuUwxXyYz"

func curl(args []word) *Refusal {
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() word {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return word{literal: true}
		}
		switch {
		case !a.literal || !strings.HasPrefix(a.text, "-") || a.text == "-":
		case a.text == "--request":
			v := next()
			if r := method("curl", v.text, v.literal); r != nil {
				return r
			}
		case strings.HasPrefix(a.text, "--request="):
			if r := method("curl", strings.TrimPrefix(a.text, "--request="), true); r != nil {
				return r
			}
		case a.text == "--upload-file" || strings.HasPrefix(a.text, "--upload-file="):
			return refuse("http-method", "curl --upload-file sends a PUT request")
		case a.text == "--config" || strings.HasPrefix(a.text, "--config="):
			return refuse("uninspectable", "curl --config reads options Henia cannot inspect")
		case strings.HasPrefix(a.text, "--"):
		default:
		cluster:
			for j := 1; j < len(a.text); j++ {
				flag := a.text[j]
				if !strings.ContainsRune(curlValued, rune(flag)) {
					continue
				}
				value := word{text: a.text[j+1:], literal: true}
				if value.text == "" {
					value = next()
				}
				switch flag {
				case 'X':
					if r := method("curl", value.text, value.literal); r != nil {
						return r
					}
				case 'T':
					return refuse("http-method", "curl -T sends a PUT request")
				case 'K':
					return refuse("uninspectable", "curl -K reads options Henia cannot inspect")
				}
				break cluster
			}
		}
	}
	return nil
}

func wget(args []word) *Refusal {
	for i, a := range args {
		switch {
		case a.text == "--method" && i+1 < len(args):
			if r := method("wget", args[i+1].text, args[i+1].literal); r != nil {
				return r
			}
		case strings.HasPrefix(a.text, "--method="):
			if r := method("wget", strings.TrimPrefix(a.text, "--method="), a.literal); r != nil {
				return r
			}
		case a.text == "-i" || a.text == "--input-file" || strings.HasPrefix(a.text, "--input-file=") || a.text == "-e" || a.text == "--execute" || a.text == "--config" || strings.HasPrefix(a.text, "--config="):
			return refuse("uninspectable", "wget %s reads instructions Henia cannot inspect", a.text)
		}
	}
	return nil
}

func httpieCall(name string, args []word) *Refusal {
	rest := positionals(args, []string{"-a", "--auth", "-A", "--auth-type", "-o", "--output", "--session", "--session-read-only", "--verify", "--cert", "--cert-key", "--proxy", "-p", "--print", "--pretty", "-s", "--style", "--format-options", "--timeout", "--max-redirects", "--boundary", "--raw"})
	if len(rest) >= 2 && rest[0].literal && regexp.MustCompile(`^[A-Za-z]+$`).MatchString(rest[0].text) && !strings.Contains(rest[0].text, ".") {
		return method(name, rest[0].text, true)
	}
	return nil
}

var ghValued = []string{"-R", "--repo", "--hostname"}

var ghAPIValued = []string{"-X", "--method", "-f", "--raw-field", "-F", "--field", "-H", "--header", "--input", "-q", "--jq", "-t", "--template", "--hostname", "--cache", "-p", "--preview"}

func (c *checker) gh(args []word, redirs []*syntax.Redirect) *Refusal {
	rest := positionals(args, ghValued)
	if len(rest) == 0 {
		return nil
	}
	if !rest[0].literal {
		return refuse("dynamic-command", "the gh command is not literal")
	}
	group := rest[0].text
	if group == "api" {
		return c.ghAPI(afterSubcommand(args, ghValued), redirs)
	}
	subs, ok := ghMutations[group]
	if !ok {
		return nil
	}
	if len(subs) == 0 {
		return refuse("gh", "gh %s changes GitHub state", group)
	}
	if len(rest) < 2 {
		return nil
	}
	if !rest[1].literal {
		return refuse("dynamic-command", "the gh %s subcommand is not literal", group)
	}
	if slices.Contains(subs, rest[1].text) {
		return refuse("gh", "gh %s %s changes GitHub state", group, rest[1].text)
	}
	return nil
}

func (c *checker) ghAPI(args []word, redirs []*syntax.Redirect) *Refusal {
	type field struct {
		key, value string
		typed, ok  bool
	}
	var fields []field
	var endpoint *word
	input, hasInput := word{}, false
	addField := func(v word, typed bool) {
		key, value, _ := strings.Cut(v.text, "=")
		fields = append(fields, field{key, value, typed, v.literal})
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() word {
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return word{literal: true}
		}
		name, value, attached := strings.Cut(a.text, "=")
		switch {
		case !a.literal && endpoint == nil:
			return refuse("dynamic-command", "the gh api endpoint is not literal")
		case !a.literal || !strings.HasPrefix(a.text, "-") || a.text == "-":
			if endpoint == nil {
				endpoint = &args[i]
			}
		case name == "-X" || name == "--method":
			v := word{text: value, literal: a.literal}
			if !attached {
				v = next()
			}
			if r := method("gh api", v.text, v.literal); r != nil {
				return r
			}
		case strings.HasPrefix(a.text, "-X"):
			if r := method("gh api", a.text[2:], true); r != nil {
				return r
			}
		case name == "-f" || name == "--raw-field" || name == "-F" || name == "--field":
			typed := name == "-F" || name == "--field"
			if attached {
				addField(word{text: value, literal: a.literal}, typed)
			} else {
				addField(next(), typed)
			}
		case strings.HasPrefix(a.text, "-f") || strings.HasPrefix(a.text, "-F"):
			addField(word{text: a.text[2:], literal: true}, a.text[1] == 'F')
		case name == "--input":
			input, hasInput = word{text: value, literal: a.literal}, true
			if !attached {
				input = next()
			}
		case slices.Contains(ghAPIValued, a.text):
			i++
		}
	}
	if endpoint == nil || strings.TrimSuffix(path.Base(endpoint.text), "/") != "graphql" {
		return nil
	}
	document, ok := "", false
	for _, f := range fields {
		if f.key != "query" {
			continue
		}
		switch {
		case !f.ok:
			return refuse("graphql", "the GraphQL document is not literal")
		case f.typed && f.value == "@-":
			document, ok = c.stdin(redirs)
		case f.typed && strings.HasPrefix(f.value, "@"):
			document, ok = c.read(f.value[1:])
		default:
			document, ok = f.value, true
		}
	}
	if !ok && hasInput && input.literal {
		var body string
		if input.text == "-" {
			body, ok = c.stdin(redirs)
		} else {
			body, ok = c.read(input.text)
		}
		var request struct {
			Query string `json:"query"`
		}
		ok = ok && json.Unmarshal([]byte(body), &request) == nil && request.Query != ""
		document = request.Query
	}
	if !ok {
		return refuse("graphql", "cannot read the GraphQL document gh api graphql sends")
	}
	mutation, err := graphQLMutation(document)
	if err != nil {
		return refuse("graphql", "cannot read the GraphQL document: %v", err)
	}
	if mutation {
		return refuse("graphql", "gh api graphql runs a mutation")
	}
	return nil
}
