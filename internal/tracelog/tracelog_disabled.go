//go:build !motoko_trace

package tracelog

func Available() bool { return false }

func Enabled() bool { return false }

func SetEnabled(bool) bool { return false }

func Path() string { return "" }

func Logf(string, ...any) {}
