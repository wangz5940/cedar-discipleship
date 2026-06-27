package asset

import (
	"context"
	"io"
)

type Repository interface {
	FindByID(ctx context.Context, groupID, id uint64) (*Asset, error)
	List(ctx context.Context, groupID uint64, limit int) ([]Asset, error)
	Create(ctx context.Context, item *Asset, actorID uint64) (uint64, error)
	Delete(ctx context.Context, groupID, id uint64) error
}

type Storage interface {
	Save(ctx context.Context, relativeDir, fileName string, src io.Reader) (*StoredObject, error)
	Resolve(ctx context.Context, storagePath string) (*ResolvedObject, error)
	Delete(ctx context.Context, objectKey string) error
}
