package ui

import (
	"fmt"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/provider"
)

func entriesForProviderModels(models []provider.ModelInfo, err error) []app.Entry {
	if err != nil {
		return []app.Entry{{Kind: app.EntryError, Text: err.Error()}}
	}
	if len(models) == 0 {
		return []app.Entry{{Kind: app.EntrySystem, Text: "The provider returned no models."}}
	}
	return []app.Entry{{
		Kind: app.EntrySystem,
		Text: fmt.Sprintf("%d models loaded. Use /models list to see them or /models use <model> to select one.", len(models)),
	}}
}
