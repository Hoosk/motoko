package types

type EntryKind string

const (
	EntryUser      EntryKind = "user"
	EntryAssistant EntryKind = "assistant"
	EntryReasoning EntryKind = "reasoning"
	EntrySystem    EntryKind = "system"
	EntryCommand   EntryKind = "command"
	EntryOutput    EntryKind = "output"
	EntryError     EntryKind = "error"
	EntryHelp      EntryKind = "help"
)

type Entry struct {
	Kind EntryKind
	Text string
}
