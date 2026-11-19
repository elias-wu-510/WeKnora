package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type capturingDocumentEnqueuer struct {
	payloads [][]byte
}

func (e *capturingDocumentEnqueuer) Enqueue(
	task *asynq.Task,
	_ ...asynq.Option,
) (*asynq.TaskInfo, error) {
	e.payloads = append(e.payloads, append([]byte(nil), task.Payload()...))
	return &asynq.TaskInfo{ID: "task-1", Queue: types.QueueDefault}, nil
}

func TestApplyDocumentTaskCorrelationCopiesRequestID(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.RequestIDContextKey, "req-upload-123")
	payload := &types.DocumentProcessPayload{KnowledgeID: "knowledge-1"}

	applyDocumentTaskCorrelation(ctx, payload)

	require.Equal(t, "req-upload-123", payload.RequestId)
}

func TestApplyDocumentTaskCorrelationWithoutRequestID(t *testing.T) {
	payload := &types.DocumentProcessPayload{KnowledgeID: "knowledge-1"}

	applyDocumentTaskCorrelation(context.Background(), payload)

	require.Empty(t, payload.RequestId)
}

// The upload request id must reach the async payload, otherwise the parse
// worker and docreader log an empty request id and a failed document cannot
// be traced back from the request the user reported.
func TestCreateKnowledgeFromFileCarriesRequestIDIntoProcessTask(t *testing.T) {
	t.Parallel()

	enqueuer := &capturingDocumentEnqueuer{}
	svc := &knowledgeService{
		repo:      &createKnowledgeFileRepoStub{},
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   &createKnowledgeFileServiceStub{},
		task:      enqueuer,
	}
	ctx := context.WithValue(newCreateKnowledgeFileContext(), types.RequestIDContextKey, "req-upload-42")

	_, err := svc.CreateKnowledgeFromFile(
		ctx,
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.Len(t, enqueuer.payloads, 1)
	var payload types.DocumentProcessPayload
	require.NoError(t, json.Unmarshal(enqueuer.payloads[0], &payload))
	require.Equal(t, "req-upload-42", payload.RequestId)
}
