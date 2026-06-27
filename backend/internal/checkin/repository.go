package checkin

import "context"

type Repository interface {
	FindExistingWeeklyBook(ctx context.Context, groupID, userID, taskID, weekID uint64, part, detail string) (uint64, error)
	FindExistingWeeklyTask(ctx context.Context, groupID, userID, taskID, weekID uint64, taskType string) (uint64, error)
	List(ctx context.Context, groupID uint64, from, to string, userID uint64, limit int) ([]Record, error)
	Create(ctx context.Context, record *Record, actorID uint64) (uint64, error)
	DeleteOwn(ctx context.Context, groupID, userID, recordID uint64) error
	DeleteAny(ctx context.Context, groupID, recordID uint64) error
}
