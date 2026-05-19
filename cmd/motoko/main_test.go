package main

import (
	"testing"

	"github.com/Hoosk/motoko/internal/app"
)

func TestNewTachikomaManagerWiresDefaultWorkers(t *testing.T) {
	mgr := newTachikomaManager(app.NewRuntime())
	if mgr == nil {
		t.Fatal("expected manager")
	}
}
