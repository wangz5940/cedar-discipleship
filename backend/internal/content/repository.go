package content

import "context"

type Repository interface {
	FindByID(ctx context.Context, groupID, id uint64) (*Content, error)
	List(ctx context.Context, groupID uint64) ([]Content, error)
	Save(ctx context.Context, item *Content) error
	Delete(ctx context.Context, groupID, id uint64) error
}
