package components

const (
	LevelError   = "error"
	LevelWarning = "warning"
)

const (
	UnknownComponent = "unknown-component"
	MissingAttribute = "missing-attribute"
	UnknownAttribute = "unknown-attribute"
	InvalidAttribute = "invalid-attribute"
	InvalidBody      = "invalid-body"
	DisallowedChild  = "disallowed-child"
	WrongForm        = "wrong-form"
	TemplateAction   = "template-action"
	InvalidTemplate  = "invalid-template"
	UnknownField     = "unknown-field"
	UnusedComponent  = "unused-component"
	InvalidMarkup    = "invalid-markup"
)

type Diagnostic struct {
	ID, Level, Path, Message string
	Line, Column             int
}
