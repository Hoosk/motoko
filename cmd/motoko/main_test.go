package main

import (
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/ui"
)

func TestMainWiringUsesRuntimeTachikomas(t *testing.T) {
	runtime := app.NewRuntime(app.RuntimeOptions{})
	model := ui.NewModel(runtime)
	if model.Init() == nil {
		t.Fatal("expected init command")
	}
}
