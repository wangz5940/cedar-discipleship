package audit

import "context"

type Repository interface {
	Create(ctx context.Context, log Log) error
	ListByGroup(ctx context.Context, groupID uint64, limit int) ([]Log, error)
}
