package sync

import (
	"context"

	"val2harness/internal/omp"
	"val2harness/internal/valclient"
)

func SyncOMP(ctx context.Context, client *valclient.Client, proxyBaseURL, proxyAPIKey string) (int, string, error) {
	return omp.SyncModels(ctx, client, proxyBaseURL, proxyAPIKey)
}
