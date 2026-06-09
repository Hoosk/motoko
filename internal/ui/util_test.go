package ui

import (
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/styles"
	"github.com/Hoosk/motoko/internal/tools"
)

func TestUtilityHelpers(t *testing.T) {
	if got := pendingLabel(""); got != "none" {
		t.Fatalf("unexpected pending label %q", got)
	}
	if got := trimLastRune("hola"); got != "hol" {
		t.Fatalf("unexpected trimLastRune result %q", got)
	}
	if got := clamp(10, 0, 5); got != 5 {
		t.Fatalf("unexpected clamp result %d", got)
	}
	if got := stripANSI("\x1b[31mboom\x1b[0m"); got != "boom" {
		t.Fatalf("unexpected stripANSI result %q", got)
	}
}

func TestRenderHelpers(t *testing.T) {
	specs := []tools.Spec{{Name: "read", Summary: "Lee", Usage: "read <ruta>"}}
	toolList := renderToolList(specs)
	if !strings.Contains(toolList, "read") || !strings.Contains(toolList, "Lee") {
		t.Fatalf("unexpected tool list %q", toolList)
	}
	tachikomaList := renderTachikomaList(map[string]string{"Git": "clean"})
	if !strings.Contains(tachikomaList, "Git") || !strings.Contains(tachikomaList, "clean") {
		t.Fatalf("unexpected tachikoma list %q", tachikomaList)
	}
	palette := renderToolPalette(specs, map[string]string{"Git": "clean"})
	if !strings.Contains(palette, "read") {
		t.Fatalf("expected tool palette content, got %q", palette)
	}
	if got := renderTachikomaList(nil); !strings.Contains(got, stripANSI(styles.SystemStyle.Render("No background workers active."))) && !strings.Contains(stripANSI(got), "No background workers active.") {
		t.Fatalf("unexpected empty tachikoma output %q", got)
	}
}

func TestRightPartANSI(t *testing.T) {
	str := "\x1b[31mHello\x1b[0m world"
	got := rightPartANSI(str, 2)
	gotPlain := stripANSI(got)
	if gotPlain != "llo world" {
		t.Fatalf("expected 'llo world', got %q (plain: %q)", got, gotPlain)
	}
	if !strings.HasPrefix(got, "\x1b[31m") {
		t.Fatalf("expected ANSI color code preserved, got %q", got)
	}

	base := "Left side              Right side"
	popup := "POPUP"
	res := overlayCenter(base, popup, 33, 1)
	if !strings.Contains(res, "Right side") {
		t.Fatalf("expected right side preserved in overlayCenter, got %q", res)
	}
	if !strings.Contains(res, "POPUP") {
		t.Fatalf("expected popup rendered in overlayCenter, got %q", res)
	}
}
