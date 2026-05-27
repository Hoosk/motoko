package main

import (
	"context"
	"testing"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/ui"
)

func TestMainWiringUsesRuntimeTachikomas(t *testing.T) {
	runtime := app.NewRuntime(app.RuntimeOptions{})
	model := ui.NewModel(runtime, func() {}, context.Background())
	if model.Init() == nil {
		t.Fatal("expected init command")
	}
}
