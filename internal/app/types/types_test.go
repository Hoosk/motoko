package types

import "testing"

func TestPackageCompiles(t *testing.T) {
	_ = ModePlan
	_ = EntryUser
	_ = ActionAgent
	_ = Response{}
	_ = ShellResult{}
	_ = TaskEvent{}
	_ = AgentStreamEvent{}
	_ = RuntimeOptions{}
}
