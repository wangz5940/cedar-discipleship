package progress

import "context"

type Repository interface {
	FindByTask(ctx context.Context, groupID, userID, taskID uint64) (*Progress, error)
	Save(ctx context.Context, item *Progress) error
	ListByUser(ctx context.Context, groupID, userID uint64) ([]Progress, error)
}
