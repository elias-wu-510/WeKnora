package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// TestPurgeDeletedChunks_CutoffAndLiveRows pins the retention contract: only
// tombstoned rows older than the cutoff are removed, live rows and tombstones
// inside the grace window survive.
func TestPurgeDeletedChunks_CutoffAndLiveRows(t *testing.T) {
	db := setupChunkTestDB(t)
	repo := NewChunkRepository(db)
	ctx := context.Background()

	live := makeChunk("kb-1", "kn-1", "text")
	oldA := makeChunk("kb-1", "kn-1", "text")
	oldB := makeChunk("kb-1", "kn-2", "text")
	recent := makeChunk("kb-1", "kn-2", "text")
	require.NoError(t, db.Create([]*types.Chunk{live, oldA, oldB, recent}).Error)

	now := time.Now()
	require.NoError(t, db.Exec(
		"UPDATE chunks SET deleted_at = ? WHERE id IN ?",
		now.AddDate(0, 0, -10), []string{oldA.ID, oldB.ID}).Error)
	require.NoError(t, db.Exec(
		"UPDATE chunks SET deleted_at = ? WHERE id = ?",
		now.AddDate(0, 0, -1), recent.ID).Error)

	deleted, err := repo.PurgeDeletedChunks(ctx, now.AddDate(0, 0, -7), 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted, "batchSize=1 must still drain every eligible row")

	var ids []string
	require.NoError(t, db.Unscoped().Model(&types.Chunk{}).Pluck("id", &ids).Error)
	require.ElementsMatch(t, []string{live.ID, recent.ID}, ids,
		"live rows and recent tombstones must survive the sweep")
}

// TestPurgeDeletedChunks_NothingToDo covers the steady-state path: an empty
// sweep returns (0, nil) rather than looping.
func TestPurgeDeletedChunks_NothingToDo(t *testing.T) {
	db := setupChunkTestDB(t)
	repo := NewChunkRepository(db)

	deleted, err := repo.PurgeDeletedChunks(context.Background(), time.Now(), 100)
	require.NoError(t, err)
	require.Zero(t, deleted)
}
