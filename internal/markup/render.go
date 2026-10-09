package markup

import "strings"

func RenderDocument(doc *Document, format string) error {
	if _, err := render("", format, 0); err != nil {
		return err
	}
	var errs errorList
	if err := renderItems(doc.Children, format, &errs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func renderItems(items []*Item, format string, errs *errorList) error {
	for i, it := range items {
		switch it.Kind {
		case "code", "html", "rule":
			continue
		case "directive", "paragraph", "heading", "table", "text":
		default:
			if err := renderItems(it.Children, format, errs); err != nil {
				return err
			}
			continue
		}
		text := markdown(it)
		out, err := render(text, format, 0)
		if err != nil {
			if !errs.relocate(err, it.Origin) {
				return err
			}
			continue
		}
		if out != text {
			replaceText(items, i, out)
		}
	}
	return nil
}

func (l *errorList) relocate(err error, at Origin) bool {
	var found errorList
	switch e := err.(type) {
	case errorList:
		found = e
	case *Error:
		found = errorList{e}
	default:
		return false
	}
	for _, e := range found {
		located := *e
		located.Line += max(at.Line, 1) - 1
		located.Column += max(at.Column, 1) - 1
		*l = l.add(&located)
	}
	return true
}

func markdown(it *Item) string {
	return unprefix(written(it), itemPrefix(it))
}

func written(it *Item) string {
	w := &writer{}
	w.item(it)
	return w.out.String()
}

func itemPrefix(it *Item) string {
	if it.parsed == nil {
		return ""
	}
	return linePrefix(it.parsed.src.text, it.parsed.start)
}

func unprefix(text, prefix string) string {
	if prefix == "" || !strings.Contains(text, "\n") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines[1:] {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			lines[i+1] = rest
		} else {
			lines[i+1] = strings.TrimPrefix(line, strings.TrimRight(prefix, " \t"))
		}
	}
	return strings.Join(lines, "\n")
}

func replaceText(items []*Item, i int, text string) {
	it := items[i]
	items[i] = it.Replace(&Item{Kind: "text", Text: text, Origin: it.Origin, Backtrace: it.Backtrace})[0]
}
