package evaluator

type Tag uint

//go:generate stringer -type Tag
const (
	TagNone Tag = iota
	TagBool
	TagNumber
	TagString
	TagIdentifier
	TagFieldAccessor
	TagCommand
	TagDump
	TagGoroutine
	TagAddress
)

type Value struct {
	Tag  Tag
	Data any
}

var NoValue = Value{Tag: TagNone}
