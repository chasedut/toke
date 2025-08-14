package models

import (
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/components/dialogs/backend"
)

// NewLocalModelDialogAutoDownload creates an auto-download dialog that immediately starts downloading the best model
func NewLocalModelDialogAutoDownload() dialogs.DialogModel {
	return backend.NewAutoDownloadDialog()
}