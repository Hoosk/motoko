package tools

import (
	"context"

	approvalpkg "github.com/Hoosk/motoko/internal/tools/approval"
)

func requestFileChange(ctx context.Context, path, diff string) error {
	if broker := GetApprovalBroker(ctx); broker != nil {
		return broker.Request(ctx, approvalpkg.FileChange{Path: path, Diff: diff})
	}
	return nil
}
