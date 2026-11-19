package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/access"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type recordingTaskInspector struct {
	interfaces.TaskInspector
	events *[]string
}

func (r *recordingTaskInspector) CancelTasksForKnowledge(
	_ context.Context,
	knowledgeID string,
) (int, int, error) {
	*r.events = append(*r.events, "dequeue:"+knowledgeID)
	return 1, 0, nil
}

type recordingReparseEnqueuer struct {
	events *[]string
}

func (e *recordingReparseEnqueuer) Enqueue(
	task *asynq.Task,
	_ ...asynq.Option,
) (*asynq.TaskInfo, error) {
	*e.events = append(*e.events, "enqueue:"+task.Type())
	return &asynq.TaskInfo{ID: "task-1", Type: task.Type(), Queue: types.QueueDefault}, nil
}

// A reparse must cancel the previous attempt's queued/running tasks before it
// enqueues the new one; otherwise the stale task can write a second copy of
// the chunks after reparse cleanup.
func TestReparseKnowledgeCancelsStaleTasksBeforeEnqueue(t *testing.T) {
	var events []string
	knowledge := &types.Knowledge{
		ID:              "knowledge-1",
		TenantID:        7,
		KnowledgeBaseID: "kb-1",
		Type:            types.KnowledgeTypeManual,
		ParseStatus:     types.ParseStatusCompleted,
		EnableStatus:    "enabled",
	}
	require.NoError(t, knowledge.SetManualMetadata(
		types.NewManualKnowledgeMetadata("# content", types.ManualKnowledgeStatusPublish, 1),
	))

	svc := &knowledgeService{
		repo:          &reparseFailureKnowledgeRepo{knowledge: knowledge},
		kbService:     &reparseFailureKBService{kb: &types.KnowledgeBase{ID: "kb-1", TenantID: 7}},
		task:          &recordingReparseEnqueuer{events: &events},
		taskInspector: &recordingTaskInspector{events: &events},
	}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx, grantErr := access.WithKBTaskWrite(ctx, &types.KnowledgeBase{ID: "kb-1", TenantID: 7}, 7)
	require.NoError(t, grantErr)

	got, err := svc.ReparseKnowledge(ctx, knowledge.ID, nil)

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, []string{"dequeue:knowledge-1", "enqueue:" + types.TypeManualProcess}, events)
}
