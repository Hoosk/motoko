package tools

import (
	"context"
)

func requestFileChange(ctx context.Context, path, diff string) error {
	return GetBroker(ctx).RequestFileChange(ctx, FileChange{Path: path, Diff: diff})
}
