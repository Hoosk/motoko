package types

type Response struct {
	Action  *Action
	Signal  string
	Entries []Entry
	Clear   bool
}
