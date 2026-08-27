package asset

import (
	"context"
	"io"
	"time"
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

type SharingRepository interface {
	Repository
	GroupCode(ctx context.Context, groupID uint64) (string, error)
	FindDownloadTarget(ctx context.Context, groupID, assetID uint64) (*Asset, error)
	ShareSettings(ctx context.Context, groupID, assetID uint64) (*SharingSettings, error)
	SaveShareSettings(ctx context.Context, groupID, assetID, actorID uint64, input ShareInput, at time.Time) error
	SharedResources(ctx context.Context, targetGroupID uint64, filter SharedFilter) ([]SharedResource, error)
	ImportPreview(ctx context.Context, targetGroupID, sourceAssetID uint64) (*ImportPreview, error)
	Import(ctx context.Context, targetGroupID, actorID uint64, input ImportInput, at time.Time) (*Asset, error)
	ImportHistory(ctx context.Context, groupID uint64, limit int) ([]ImportEvent, error)
	RemoveImport(ctx context.Context, groupID, importedAssetID, actorID uint64, at time.Time) error
	DependencyGraph(ctx context.Context, groupID uint64, isSuperAdmin bool, filter DependencyFilter) (*DependencyGraph, error)
	BatchSaveShareSettings(ctx context.Context, groupID, actorID uint64, input BatchShareInput, at time.Time) (*BatchShareResult, error)
	BatchDelete(ctx context.Context, groupID, actorID uint64, input BatchDeleteInput, at time.Time) (*BatchDeleteResult, error)
	BatchImport(ctx context.Context, targetGroupID, actorID uint64, input BatchImportInput, at time.Time) (*BatchImportResult, error)
}
