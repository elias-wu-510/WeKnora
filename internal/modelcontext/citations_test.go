package modelcontext

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestCitationLineFallbackAttributesFiguresStatedEarlierOnTheLine(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Amendments",
		ChunkType: types.ChunkTypeText, Content: "### Amendments to the Code",
		CitationEvidence: []types.CitationEvidence{{
			ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Amendments",
			ChunkType: types.ChunkTypeText,
			Content:   "Clause 2.2.41 qu should be limited to 3 000 kPa; basement on granular soil, increased to 4 500 kPa. F = factor of safety not less than 3",
		}},
	}})
	got := registry.ExpandText(`该修订将上限由 3 000 kPa 提高至 4 500 kPa；安全系数 F 仍不得小于 3<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.NotContains(t, got, `chunk_id="merged"`)
}

func TestCitationLineFallbackNormalizesChineseWeekUnit(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Asbestos",
		ChunkType: types.ChunkTypeText, Content: "Section 14 Washing and changing facilities 50",
		CitationEvidence: []types.CitationEvidence{{
			ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Asbestos",
			ChunkType: types.ChunkTypeText,
			Content:   "Section 2 Interpretation. “action level” means one of the following cumulative exposures to asbestos over a continuous 12-week period",
		}},
	}})
	got := registry.ExpandText(`Section 2 释义：连续12星期内，暴露为每毫升空气96纤维小时<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.Equal(t, citationNeedles("12 weeks"), citationNeedles("12星期"))
}

func TestCitationLineFallbackKeepsUnmatchedFiguresVisible(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Doc",
		ChunkType: types.ChunkTypeText, Content: "Body without the figure",
		CitationEvidence: []types.CitationEvidence{{
			ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Doc",
			ChunkType: types.ChunkTypeText, Content: "Body also without the figure",
		}},
	}})
	got := registry.ExpandText(`该值出自附件；限值为 1.0%<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="merged"`)
}

func TestCitationPartialEvidenceKeepsUnresolvedReference(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "amendments", KnowledgeBaseID: "kb", KnowledgeTitle: "Amendments",
		ChunkType: types.ChunkTypeText, Content: "Amendments to the Code",
		CitationEvidence: []types.CitationEvidence{{
			ChunkID: "member", KnowledgeID: "amendments", KnowledgeBaseID: "kb", DocumentTitle: "Amendments",
			ChunkType: types.ChunkTypeText, Content: "Clause 3.1.3 clarifies the definitions of Ys and Us.",
		}},
	}})
	// The density belongs to a different source cited earlier on this line.
	// Keep the unresolved handle, but expose the original amendment evidence.
	got := registry.ExpandText(`Density is 7850 kg, see amendments to Clause 3.1.3<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.Contains(t, got, `chunk_id="merged"`)

	// No member supports this claim: do not append an unrelated source.
	got = registry.ExpandText(`Density is 7850 kg<ref id="c1"/>`)
	require.NotContains(t, got, `chunk_id="member"`)
	require.Contains(t, got, `chunk_id="merged"`)
}

func TestCitationMergedHandleDroppedWhenMembersCoverMeasuredFacts(t *testing.T) {
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
	// The clause locator is Chinese and absent from the English member, but the
	// merged body adds no evidence of its own: cite the member only.
	got := registry.ExpandText(`隔声窗可使室内噪声至少降低 6 dB(A)，见第 9 段<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.NotContains(t, got, `chunk_id="merged"`)
}

func TestCitationMergedHandleKeptWhenItOwnsTheClauseLocator(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Acoustics",
		ChunkType: types.ChunkTypeText, Content: "Acoustics", ContentRewritten: true,
		SubChunkID: []string{"member"},
		CitationEvidence: []types.CitationEvidence{
			{
				ChunkID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Acoustics",
				ChunkType: types.ChunkTypeText, Content: "See paragraph 9 for the ventilation requirement.",
			},
			{
				ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Acoustics",
				ChunkType: types.ChunkTypeText,
				Content:   "Noise reduction of the window shall be at least 6 dB(A).",
			},
		},
	}})
	// The handle's own chunk carries the paragraph locator, so keep it.
	got := registry.ExpandText(`隔声窗可使室内噪声至少降低 6 dB(A)，见第 9 段<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.Contains(t, got, `chunk_id="merged"`)
}

func TestCitationCountFoldsEnglishCycleNoun(t *testing.T) {
	registry := newSourceRegistry()
	registry.RegisterSearchResults([]*types.SearchResult{{
		ID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", KnowledgeTitle: "Drainage",
		ChunkType: types.ChunkTypeText, Content: "Drainage", ContentRewritten: true,
		SubChunkID: []string{"member"},
		CitationEvidence: []types.CitationEvidence{
			{
				ChunkID: "merged", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Drainage",
				ChunkType: types.ChunkTypeText, Content: "Drainage",
			},
			{
				ChunkID: "member", KnowledgeID: "doc", KnowledgeBaseID: "kb", DocumentTitle: "Drainage",
				ChunkType: types.ChunkTypeText,
				Content: "Duty pumps should not be required to operate more than 10 onoff cycles per hour. " +
					"Location of each level switch: a minimum distance of 200 mm.",
			},
		},
	}})
	got := registry.ExpandText(`污水泵每小时启停不超过 10 次，液位开关之间最小距离 200 mm<ref id="c1"/>`)
	require.Contains(t, got, `chunk_id="member"`)
	require.NotContains(t, got, `chunk_id="merged"`)
	require.Contains(t, citationNeedles("10 onoff cycles per hour"), "10|次")
}
