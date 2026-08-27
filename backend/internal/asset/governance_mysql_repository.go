package asset

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"
)

func (r *MySQLRepository) GroupCode(ctx context.Context, groupID uint64) (string, error) {
	var code string
	err := r.db.QueryRowContext(ctx, `SELECT code FROM study_groups WHERE id=? AND status=1`, groupID).Scan(&code)
	return code, err
}

func (r *MySQLRepository) RemoveImport(ctx context.Context, groupID, importedAssetID, actorID uint64, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := removeImportedAssetTx(ctx, tx, groupID, importedAssetID, actorID, at, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) BatchDelete(ctx context.Context, groupID, actorID uint64, input BatchDeleteInput, at time.Time) (*BatchDeleteResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &BatchDeleteResult{AssetIDs: input.AssetIDs, DeletedIDs: make([]uint64, 0, len(input.AssetIDs))}
	detail := `{"batch":true}`
	for _, assetID := range input.AssetIDs {
		_, binding, err := r.assetWithBinding(ctx, tx, groupID, assetID)
		if err != nil {
			return nil, err
		}
		switch binding.AssetKind {
		case AssetKindImported:
			if _, err := removeImportedAssetTx(ctx, tx, groupID, assetID, actorID, at, detail); err != nil {
				return nil, err
			}
			result.ImportedCount++
		case AssetKindOwned:
			if err := deleteOwnedAssetTx(ctx, tx, groupID, assetID, actorID, at); err != nil {
				return nil, err
			}
			result.OwnedCount++
		default:
			return nil, sql.ErrNoRows
		}
		result.DeletedIDs = append(result.DeletedIDs, assetID)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result.Count = len(result.DeletedIDs)
	return result, nil
}

func removeImportedAssetTx(ctx context.Context, tx *sql.Tx, groupID, importedAssetID, actorID uint64, at time.Time, detail any) (uint64, error) {
	var sourceAssetID uint64
	if err := tx.QueryRowContext(ctx, `SELECT source_asset_id FROM asset_bindings
		WHERE asset_id=? AND group_id=? AND asset_kind=? AND deleted_at IS NULL FOR UPDATE`,
		importedAssetID, groupID, AssetKindImported).Scan(&sourceAssetID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_bindings SET deleted_at=?,updated_at=? WHERE asset_id=? AND group_id=?`,
		at, at, importedAssetID, groupID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE assets SET updated_at=? WHERE id=? AND group_id=?`, at, importedAssetID, groupID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_dependencies SET status='removed',updated_at=?
		WHERE consumer_group_id=? AND consumer_asset_id=?`, at, groupID, importedAssetID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_import_events
		(target_group_id,imported_asset_id,source_asset_id,event_type,actor_user_id,detail,created_at)
		VALUES (?,?,?,'removed',?,?,?)`, groupID, importedAssetID, sourceAssetID, actorID, detail, at); err != nil {
		return 0, err
	}
	return sourceAssetID, nil
}

func deleteOwnedAssetTx(ctx context.Context, tx *sql.Tx, groupID, assetID, actorID uint64, at time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE asset_bindings
		SET deleted_at=?,updated_at=?
		WHERE asset_id=? AND group_id=? AND asset_kind=? AND deleted_at IS NULL`,
		at, at, assetID, groupID, AssetKindOwned); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE assets SET updated_at=? WHERE id=? AND group_id=?`, at, assetID, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_share_grants
		SET status=?,revoked_by=?,revoked_at=?
		WHERE asset_id=? AND owner_group_id=? AND permission=? AND status=?`,
		shareStatusRevoked, actorID, at, assetID, groupID, sharePermissionImport, shareStatusActive); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_dependencies
		SET status='removed',updated_at=?
		WHERE provider_group_id=? AND provider_asset_id=?`,
		at, groupID, assetID); err != nil {
		return err
	}
	return nil
}

func (r *MySQLRepository) DependencyGraph(
	ctx context.Context,
	groupID uint64,
	isSuperAdmin bool,
	filter DependencyFilter,
) (*DependencyGraph, error) {
	scopeGroupID := groupID
	if isSuperAdmin && filter.GroupID > 0 {
		scopeGroupID = filter.GroupID
	}
	query := `SELECT d.id,d.consumer_group_id,cg.name,d.consumer_asset_id,ca.title,ca.category,
		d.provider_group_id,pg.name,d.provider_asset_id,pa.title,pa.category,
		CASE
		  WHEN d.status<>'active' THEN d.status
		  WHEN cb.deleted_at IS NOT NULL OR pb.deleted_at IS NOT NULL THEN 'broken'
		  WHEN NOT EXISTS (
		    SELECT 1 FROM asset_share_grants g
		    WHERE g.asset_id=d.provider_asset_id AND g.permission='import' AND g.status='active'
		      AND (g.consumer_group_id IS NULL OR g.consumer_group_id=d.consumer_group_id)
		  ) THEN 'revoked'
		  ELSE 'active'
		END AS resolved_status
		FROM asset_dependencies d
		JOIN study_groups cg ON cg.id=d.consumer_group_id
		JOIN study_groups pg ON pg.id=d.provider_group_id
		JOIN assets ca ON ca.id=d.consumer_asset_id
		JOIN assets pa ON pa.id=d.provider_asset_id
		LEFT JOIN asset_bindings cb ON cb.asset_id=d.consumer_asset_id
		LEFT JOIN asset_bindings pb ON pb.asset_id=d.provider_asset_id
		WHERE (d.consumer_group_id=? OR d.provider_group_id=?)
		  AND ca.storage_path LIKE ? AND pa.storage_path LIKE ?`
	args := []any{scopeGroupID, scopeGroupID, newResourceStorageSQLPattern, newResourceStorageSQLPattern}
	if filter.Category != "" {
		query += " AND (ca.category=? OR pa.category=?)"
		args = append(args, filter.Category, filter.Category)
	}
	query += " ORDER BY d.provider_group_id,d.consumer_group_id,d.id"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodes := map[string]DependencyNode{}
	providers := map[uint64]bool{}
	pairStrength := map[string]int{}
	type rawEdge struct {
		edge DependencyEdge
		pair string
	}
	rawEdges := []rawEdge{}
	for rows.Next() {
		var dependencyID, consumerGroupID, consumerAssetID, providerGroupID, providerAssetID uint64
		var consumerGroupName, consumerTitle, consumerCategory string
		var providerGroupName, providerTitle, providerCategory, status string
		if err := rows.Scan(
			&dependencyID, &consumerGroupID, &consumerGroupName, &consumerAssetID, &consumerTitle, &consumerCategory,
			&providerGroupID, &providerGroupName, &providerAssetID, &providerTitle, &providerCategory, &status,
		); err != nil {
			return nil, err
		}
		providerGroupNode := "group:" + strconv.FormatUint(providerGroupID, 10)
		consumerGroupNode := "group:" + strconv.FormatUint(consumerGroupID, 10)
		providerAssetNode := "asset:" + strconv.FormatUint(providerAssetID, 10)
		consumerAssetNode := "asset:" + strconv.FormatUint(consumerAssetID, 10)
		nodes[providerGroupNode] = DependencyNode{ID: providerGroupNode, Kind: "group", Label: providerGroupName, GroupID: providerGroupID}
		nodes[consumerGroupNode] = DependencyNode{ID: consumerGroupNode, Kind: "group", Label: consumerGroupName, GroupID: consumerGroupID}
		nodes[providerAssetNode] = DependencyNode{ID: providerAssetNode, Kind: "asset", Label: providerTitle, Type: providerCategory, GroupID: providerGroupID, AssetID: providerAssetID}
		nodes[consumerAssetNode] = DependencyNode{ID: consumerAssetNode, Kind: "asset", Label: consumerTitle, Type: consumerCategory, GroupID: consumerGroupID, AssetID: consumerAssetID}
		pair := fmt.Sprintf("%d:%d", providerGroupID, consumerGroupID)
		pairStrength[pair]++
		rawEdges = append(rawEdges, rawEdge{
			pair: pair,
			edge: DependencyEdge{
				ID: "dependency:" + strconv.FormatUint(dependencyID, 10), Source: providerAssetNode, Target: consumerAssetNode,
				SourceGroupID: providerGroupID, TargetGroupID: consumerGroupID, Status: status,
			},
		})
		providers[providerAssetID] = true
		graph.Metrics.Imports++
		if status != "active" {
			graph.Metrics.BrokenReferences++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, raw := range rawEdges {
		raw.edge.Strength = pairStrength[raw.pair]
		if matchesStrength(raw.edge.Strength, filter.Strength) {
			graph.Edges = append(graph.Edges, raw.edge)
		}
	}
	for _, node := range nodes {
		graph.Nodes = append(graph.Nodes, node)
	}
	graph.Metrics.Dependents = len(providers)
	if len(graph.Edges) > 0 {
		graph.Metrics.MaxDepth = 1
	}
	return graph, nil
}

func matchesStrength(value int, filter string) bool {
	switch filter {
	case "", "all":
		return true
	case "weak":
		return value <= 2
	case "medium":
		return value >= 3 && value <= 5
	case "strong":
		return value >= 6
	default:
		return true
	}
}
