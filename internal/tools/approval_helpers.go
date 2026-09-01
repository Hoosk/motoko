package tools

import (
	"context"

	approvalpkg "github.com/Hoosk/motoko/internal/tools/approval"
)

func requestFileChange(ctx context.Context, path, diff string) error {
	return GetApprovalBroker(ctx).Request(ctx, approvalpkg.FileChange{Path: path, Diff: diff})
}
