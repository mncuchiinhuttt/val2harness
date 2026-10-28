package valclient

import (
	"context"

	"val2harness/internal/analytics"
)

// CheckQuota is a compatibility read of local observations. It never probes Val.
func (c *Client) CheckQuota(ctx context.Context) (*analytics.QuotaStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status := c.tracker.SnapshotQuota()
	return &status, nil
}
