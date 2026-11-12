package modelcontext

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func validateFixture() *Registry {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Acoustics",
		ChunkType: types.ChunkTypeText, Content: "Acoustics", ContentRewritten: true,
		SubChunkID: []string{"member"},
		CitationEvidence: []types.CitationEvidence{
			{
				ChunkID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Acoustics",
				ChunkType: types.ChunkTypeText, Content: "Acoustics",
			},
			{
				ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Acoustics",
				ChunkType: types.ChunkTypeText,
				Content:   "Noise reduction of the window shall be at least 6 dB(A).",
			},
		},
	}})
	return &Registry{sources: registry}
}

func TestValidateAnswerAcceptsSupportingChunk(t *testing.T) {
	registry := validateFixture()
	violations := registry.ValidateAnswer(
		`隔声窗至少降低 6 dB(A)<kb doc="Acoustics" chunk_id="member"/>`)
	require.Empty(t, violations)
}

func TestValidateAnswerFlagsUnsupportingChunk(t *testing.T) {
	registry := validateFixture()
	violations := registry.ValidateAnswer(
		`隔声窗至少降低 6 dB(A)<kb doc="Acoustics" chunk_id="merged"/>`)
	require.Len(t, violations, 1)
	require.Equal(t, "merged", violations[0].ChunkID)
}

func TestValidateAnswerSkipsLinesWithoutFacts(t *testing.T) {
	registry := validateFixture()
	violations := registry.ValidateAnswer(
		`这一点见相关章节<kb doc="Acoustics" chunk_id="merged"/>`)
	require.Empty(t, violations)
}

func TestValidateAnswerIgnoresFactsAbsentFromContext(t *testing.T) {
	registry := validateFixture()
	violations := registry.ValidateAnswer(
		`隔声窗至少降低 12 dB(A)<kb doc="Acoustics" chunk_id="merged"/>`)
	require.Empty(t, violations, "no retrieved chunk carries 12 dB(A); repair must not delete the claim")
}