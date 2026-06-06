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
		return []app.Entry{{Kind: app.EntrySystem, Text: "El provider no devolvio modelos."}}
	}
	return []app.Entry{{
		Kind: app.EntrySystem,
		Text: fmt.Sprintf("%d modelos cargados. Usa /models para verlos o /models <modelo> para seleccionarlo.", len(models)),
	}}
}
