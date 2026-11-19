// citations.go owns the public-citation surface of the model context: the
// system protocol prompt, expansion of private <ref/> handles into canonical
// <kb/> / <web/> tags, re-compaction of canonical tags replayed from history,
// and the stream expander that keeps partial tags off the wire.
package modelcontext

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
	"golang.org/x/text/unicode/norm"
)

const sourceHandleProtocolPrompt = `

## Source handling protocol (system-owned)
Retrieved content uses request-local source handles: cN identifies a knowledge chunk, wN a web page, dN a document, and bN a knowledge base.
- Use dN and bN only as tool arguments when a tool requests a document or knowledge base.
- Never reveal raw chunk IDs, knowledge IDs, knowledge-base IDs, or private source handles in user-visible output. This does not change separate instructions to preserve retrieved Markdown image URLs.`

const citationEnabledProtocolPrompt = `
- Source citations are enabled for this answer. Cite a knowledge chunk with exactly <ref id="cN"/> and a web page with exactly <ref id="wN"/>.
- Cite only cN/wN handles backed by tool results for the current task, and only when that source supports the adjacent
  claim. Never cite dN/bN.
- Each cN covers only its own supplied text or image evidence, not every chunk of the same document.
  For a direct quotation, verify that the quoted words occur in the cited cN. Use the later chunk's handle
  when the words occur there; never reuse a document's first handle for unrelated passages.
- OCR watermarks, timestamps and image captions are observations of an image. Do not turn a watermark slogan
  into a verified service commitment or a caption into a factual document statement without supporting text.
- Handles in historical answers, tool arguments, or the bound knowledge-base directory are for navigation, not current
  evidence. Retrieve the relevant source before citing it.
- MCP results are external sources. Use the wN handle for the matching URL in the system-provided
  external_source_candidates list; never substitute a knowledge-base cN handle for MCP content. Candidate URLs are links
  observed in the result, not proof that every linked page was read.
- If a source has no citation handle, use its exact supplied HTTP(S) URL as a Markdown link when available.
  If neither is available, omit the citation; never invent or borrow a source.
- Never output <kb> or <web> tags yourself; the system expands valid <ref/> tags after generation.
- Keep each <ref/> inline on the same line as the claim it supports. Do not group citations ` +
	`at the end. For a requested exact output format, use citations only where the format ` +
	`permits them; do not break a required schema to add citations.
- Every numeric, ratio, limit, or clause claim must have a citation on the same line. ` +
	`In tables, cite every row; for parallel facts from different sources, cite each ` +
	`supporting source. Keep facts and unresolved citations visible when mapping fails.
- Attach a numeric, ratio, limit, or clause claim to the cN handle whose own text contains ` +
	`that exact value. Never attach such a claim to a table-of-contents, heading, index, or ` +
	`summary block, even when that block names the section or clause.
- Before finishing, re-check every numeric/clause sentence: if the cited block does not ` +
	`contain the value, cite the block that does. If no retrieved block contains the value, ` +
	`remove that value from the answer (or state that the retrieved documents do not specify ` +
	`it); never keep an unsupported numeric/clause claim by deleting only its citation. ` +
	`A numeric/clause statement without any citation is a protocol violation.
- These rules supersede earlier, saved, or custom prompt instructions about citation syntax.`

const citationDisabledProtocolPrompt = `
- Source citations are disabled for this answer. Do not add <ref>, <kb>, <web>, or source ` +
	`attribution links to the answer. This does not prohibit a URL explicitly requested by the ` +
	`user, Wiki navigation links, downloadable deliverables, or relevant image URLs.
- These rules supersede earlier, saved, or custom prompt instructions that require source citations.`

// ProtocolPrompt returns the internal, non-user-editable source protocol for a
// model call. Citation formatting stays out of custom and template prompts.
func sourceProtocolPrompt(citationsEnabled bool) string {
	if citationsEnabled {
		return sourceHandleProtocolPrompt + citationEnabledProtocolPrompt
	}
	return sourceHandleProtocolPrompt + citationDisabledProtocolPrompt
}

// ProtocolPrompt returns the source protocol configured for this registry.
// Request lifecycle code should normally call this through Registry.
func (r *sourceRegistry) ProtocolPrompt() string {
	if r == nil {
		return ""
	}
	return sourceProtocolPrompt(r.citationsEnabled)
}

var (
	publicKBTagRE        = regexp.MustCompile(`(?is)<kb\b[^>]*>`)
	publicWebTagRE       = regexp.MustCompile(`(?is)<web\b[^>]*>`)
	docAttrRE            = regexp.MustCompile(`(?i)\bdoc\s*=\s*"([^"]*)"`)
	chunkAttrRE          = regexp.MustCompile(`(?i)\bchunk_id\s*=\s*"([^"]+)"`)
	publicKBAttrRE       = regexp.MustCompile(`(?i)\bkb_id\s*=\s*"([^"]*)"`)
	urlAttrRE            = regexp.MustCompile(`(?i)\burl\s*=\s*"([^"]+)"`)
	titleAttrRE          = regexp.MustCompile(`(?i)\btitle\s*=\s*"([^"]*)"`)
	legacyChunkRE        = regexp.MustCompile(`(?is)<(?:chunk|faq)\b[^>]*>`)
	faqAttrRE            = regexp.MustCompile(`(?i)\bfaq_id\s*=\s*"([^"]+)"`)
	knowledgeTitleAttrRE = regexp.MustCompile(`(?i)\bknowledge_title\s*=\s*"([^"]*)"`)
)

func (r *sourceRegistry) registerLegacyToolReferences(text string, evidence bool) {
	if r == nil || text == "" {
		return
	}
	r.registerLabeledReferences(text)
	for _, tag := range legacyChunkRE.FindAllString(text, -1) {
		chunkID := firstNonEmpty(publicAttr(chunkAttrRE, tag), publicAttr(faqAttrRE, tag))
		if chunkID == "" {
			continue
		}
		r.registerChunk(ChunkReference{
			ChunkID:         chunkID,
			KnowledgeID:     publicAttr(documentAttrRE, tag),
			KnowledgeBaseID: firstNonEmpty(publicAttr(kbAttrRE, tag), publicAttr(publicKBAttrRE, tag)),
			DocumentTitle:   firstNonEmpty(publicAttr(knowledgeTitleAttrRE, tag), publicAttr(docAttrRE, tag)),
		}, evidence)
	}
}

// CompactPublicCitations folds canonical citations into the private protocol.
// Historical citations register navigation handles only; citations returned by
// a successful current source tool can also authorize evidence.
func (r *sourceRegistry) CompactPublicCitations(text string, evidence bool) string {
	if r == nil || text == "" {
		return text
	}
	text = publicKBTagRE.ReplaceAllStringFunc(text, func(tag string) string {
		chunkID := publicAttr(chunkAttrRE, tag)
		if chunkID == "" {
			return tag
		}
		handle := r.registerChunk(ChunkReference{
			ChunkID:         chunkID,
			KnowledgeBaseID: publicAttr(publicKBAttrRE, tag),
			DocumentTitle:   publicAttr(docAttrRE, tag),
		}, evidence)
		return `<ref id="` + handle + `"/>`
	})
	return publicWebTagRE.ReplaceAllStringFunc(text, func(tag string) string {
		rawURL := publicAttr(urlAttrRE, tag)
		if rawURL == "" {
			return tag
		}
		handle := r.registerWeb(rawURL, publicAttr(titleAttrRE, tag), evidence)
		return `<ref id="` + handle + `"/>`
	})
}

func publicAttr(expression *regexp.Regexp, tag string) string {
	match := expression.FindStringSubmatch(tag)
	if len(match) != 2 {
		return ""
	}
	return html.UnescapeString(match[1])
}

var (
	refTagRE       = regexp.MustCompile(`(?i)<ref\s+id\s*=\s*"([^"]+)"\s*/?>`)
	refCandidateRE = regexp.MustCompile(`(?is)<ref(?:\s|$)[^>]*(?:>|$)`)
	modelKBTagRE   = regexp.MustCompile(`(?is)<kb(?:\s|$)[^>]*(?:>|$)`)
	modelWebTagRE  = regexp.MustCompile(`(?is)<web(?:\s|$)[^>]*(?:>|$)`)
)

var (
	documentAttrRE    = regexp.MustCompile(`(?i)\bknowledge_id\s*=\s*"([^"]+)"`)
	documentElementRE = regexp.MustCompile(`(?is)<knowledge_id>\s*([^<]+?)\s*</knowledge_id>`)
	kbAttrRE          = regexp.MustCompile(`(?i)\b(?:knowledge_base_id|kb_id)\s*=\s*"([^"]+)"`)
	kbElementRE       = regexp.MustCompile(`(?is)<(?:knowledge_base_id|kb_id)>\s*([^<]+?)\s*</(?:knowledge_base_id|kb_id)>`)
)

// registerLabeledReferences covers metadata-oriented tools that do not have a
// dedicated compact renderer. Only explicit ID labels are recognized; UUID-like
// text in retrieved content is never guessed to be a source identifier.
func (r *sourceRegistry) registerLabeledReferences(text string) {
	if r == nil || text == "" {
		return
	}
	for _, expression := range []*regexp.Regexp{documentAttrRE, documentElementRE} {
		for _, match := range expression.FindAllStringSubmatch(text, -1) {
			if len(match) == 2 {
				r.RegisterDocument(strings.TrimSpace(match[1]))
			}
		}
	}
	for _, expression := range []*regexp.Regexp{kbAttrRE, kbElementRE} {
		for _, match := range expression.FindAllStringSubmatch(text, -1) {
			if len(match) == 2 {
				r.RegisterKnowledgeBase(strings.TrimSpace(match[1]))
			}
		}
	}
}

// ExpandText converts the private model protocol into the existing public
// <kb/> / <web/> contract. Unknown handles fail closed and disappear.
func (r *sourceRegistry) ExpandText(text string) string {
	if r == nil || text == "" {
		return text
	}
	// Public citation tags are output-only. Drop any instance written directly
	// by the model, then create canonical tags solely from registered handles.
	text = modelKBTagRE.ReplaceAllString(text, "")
	text = modelWebTagRE.ReplaceAllString(text, "")
	if !r.citationsEnabled {
		return refCandidateRE.ReplaceAllString(text, "")
	}
	matches := refCandidateRE.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, m := range matches {
		out.WriteString(text[last:m[0]])
		last = m[1]
		out.WriteString(r.expandTag(text, m[0], text[m[0]:m[1]]))
	}
	out.WriteString(text[last:])
	return out.String()
}

// expandTag resolves one private <ref/> into the public citation contract.
// Numeric claims map to text-bearing members, while unsupported claims retain
// their original citation so evaluation can still see and count the failure.
func (r *sourceRegistry) expandTag(text string, offset int, tag string) string {
	if r == nil || !r.citationsEnabled {
		return ""
	}
	match := refTagRE.FindStringSubmatch(tag)
	if len(match) != 2 {
		return ""
	}
	handle := strings.ToLower(match[1])
	if _, citable := r.citable.Load(handle); !citable {
		return ""
	}
	if _, current, ok := r.chunks.resolve(handle); ok {
		selected := r.selectEvidence(text, offset, current)
		if len(selected) == 0 {
			return ""
		}
		var out strings.Builder
		seen := make(map[string]struct{}, len(selected))
		for _, evidence := range selected {
			if _, exists := seen[evidence.ChunkID]; exists {
				continue
			}
			seen[evidence.ChunkID] = struct{}{}
			attrs := fmt.Sprintf(`doc="%s" chunk_id="%s"`, escapeAttr(evidence.DocumentTitle), escapeAttr(evidence.ChunkID))
			if evidence.KnowledgeBaseID != "" {
				attrs += fmt.Sprintf(` kb_id="%s"`, escapeAttr(evidence.KnowledgeBaseID))
			}
			out.WriteString("<kb " + attrs + " />")
		}
		return out.String()
	}
	if rawURL, web, ok := r.webs.resolve(handle); ok {
		return fmt.Sprintf(`<web url="%s" title="%s" />`, escapeAttr(rawURL), escapeAttr(web.title))
	}
	return ""
}

// selectEvidence maps a merged handle back to the original members that support
// its adjacent quantities. A sentence may have several citations: each member
// must contribute evidence, but one citation need not cover every other source's
// facts. This maps quantities; it does not certify the whole sentence's meaning.
func (r *sourceRegistry) selectEvidence(text string, offset int, current ChunkReference) []types.CitationEvidence {
	if current.ChunkType == types.ChunkTypeImageCaption {
		return nil
	}
	needles := citationNeedles(citationSentence(text, offset))
	// A line such as "…；安全系数 F 仍不得小于 3" ends on a prose clause with
	// no unit of its own, while the figures it marks sit earlier in the same
	// line. Fall back to the line so that citation is still attributable.
	if len(needles) == 0 {
		needles = citationNeedles(citationLine(text, offset))
	}
	// Qualitative claims and legacy tool references retain their existing
	// behavior. A legacy reference without a body cannot be checked here.
	if len(needles) == 0 || (current.Evidence == nil && current.Content == "") {
		return citationReference(current)
	}
	if !citationTextBearing(current.ChunkType) {
		return citationReference(current)
	}
	candidates := citationCandidates(current, current)
	selected := selectCitationMembers(needles, candidates, current.ChunkID)
	if citationMembersCoverNeedles(needles, selected) {
		return selected
	}
	// The same merged body commonly carries the measured quantities while the
	// answer's clause locator is Chinese ("第 12 段") and the source is English,
	// so the locator can never be matched against the member text. Retaining the
	// merged handle in that case adds a citation whose original body holds none
	// of the facts. Drop it only when the members cover every measured quantity
	// and the handle's own chunk adds nothing the members lack.
	if citationMembersCoverNumbers(needles, selected) && !citationHandleSupportsUncovered(needles, selected, current) {
		return selected
	}
	// Preserve request-local retargeting for a genuinely wrong handle, limited
	// to the exact document and KB. Directory/history-only handles cannot help.
	if current.KnowledgeID == "" || current.KnowledgeBaseID == "" {
		return citationReference(current)
	}
	for _, entry := range r.chunks.entries() {
		if _, citable := r.citable.Load(entry.handle); !citable || entry.value == current.ChunkID {
			continue
		}
		candidates = append(candidates, citationCandidates(entry.meta, current)...)
	}
	if selected := selectCitationMembers(needles, candidates, current.ChunkID); citationMembersCoverNeedles(needles, selected) {
		return selected
	}
	// A sentence can combine facts from several documents. Even when this
	// document cannot cover the whole sentence, expose the original members
	// that do support part of it, while retaining the unresolved reference.
	// This preserves unsupported claims instead of hiding them from the audit.
	if selected := selectCitationMembers(needles, candidates, current.ChunkID); len(selected) > 0 {
		return append(selected, citationReference(current)...)
	}
	// No supporting member was found; keep the unresolved citation visible.
	return citationReference(current)
}

// citationMembersCoverNeedles prevents a partially retargeted citation from
// making a sentence with several facts look fully sourced. If any numeric or
// clause needle remains uncovered, the original citation is retained and the
// evaluator can count the unresolved reference.
func citationMembersCoverNeedles(needles []string, selected []types.CitationEvidence) bool {
	if len(needles) == 0 || len(selected) == 0 {
		return false
	}
	covered := citationCoveredFacts(selected)
	for _, needle := range needles {
		if _, ok := covered[needle]; !ok {
			return false
		}
	}
	return true
}

func citationCoveredFacts(selected []types.CitationEvidence) map[string]struct{} {
	covered := make(map[string]struct{})
	for _, member := range selected {
		for _, fact := range citationNeedles(member.Content) {
			covered[fact] = struct{}{}
		}
	}
	return covered
}

// citationMembersCoverNumbers reports whether the selected members account for
// every measured quantity in the sentence. Clause locators are excluded on
// purpose: an English source never spells the Chinese "第 N 段", so requiring
// them would keep a merged handle that contributes no evidence of its own.
func citationMembersCoverNumbers(needles []string, selected []types.CitationEvidence) bool {
	if len(selected) == 0 {
		return false
	}
	covered := citationCoveredFacts(selected)
	measured := 0
	for _, needle := range needles {
		if citationLocatorNeedle(needle) {
			continue
		}
		measured++
		if _, ok := covered[needle]; !ok {
			return false
		}
	}
	return measured > 0
}

// citationHandleSupportsUncovered reports whether the current handle's own
// original chunk supports a needle the selected members do not, so a merged
// handle is kept when it is still the only source of a clause locator.
func citationHandleSupportsUncovered(needles []string, selected []types.CitationEvidence, ref ChunkReference) bool {
	covered := citationCoveredFacts(selected)
	uncovered := make(map[string]struct{}, len(needles))
	for _, needle := range needles {
		if _, ok := covered[needle]; !ok {
			uncovered[needle] = struct{}{}
		}
	}
	if len(uncovered) == 0 {
		return false
	}
	for _, member := range ref.Evidence {
		if member.ChunkID != ref.ChunkID || member.Content == "" {
			continue
		}
		for _, fact := range citationNeedles(member.Content) {
			if _, ok := uncovered[fact]; ok {
				return true
			}
		}
	}
	return false
}

func citationLocatorNeedle(fact string) bool {
	i := strings.LastIndexByte(fact, '|')
	if i < 0 {
		return false
	}
	switch fact[i+1:] {
	case "clause", "section", "chapter", "paragraph":
		return true
	}
	return false
}

func citationReference(ref ChunkReference) []types.CitationEvidence {
	return []types.CitationEvidence{{
		ChunkID: ref.ChunkID, KnowledgeID: ref.KnowledgeID,
		KnowledgeBaseID: ref.KnowledgeBaseID, DocumentTitle: ref.DocumentTitle,
	}}
}

type citationCandidate struct {
	evidence types.CitationEvidence
	facts    map[string]bool
}

func citationCandidates(ref, origin ChunkReference) []citationCandidate {
	if ref.KnowledgeID != origin.KnowledgeID || ref.KnowledgeBaseID != origin.KnowledgeBaseID ||
		!citationTextBearing(ref.ChunkType) {
		return nil
	}
	evidence := ref.Evidence
	if evidence == nil && ref.Content != "" {
		evidence = []types.CitationEvidence{{
			ChunkID: ref.ChunkID, KnowledgeID: ref.KnowledgeID, KnowledgeBaseID: ref.KnowledgeBaseID,
			DocumentTitle: ref.DocumentTitle, ChunkType: ref.ChunkType, Content: ref.Content,
		}}
	}
	// Build the model-visible fact set from the merged body: parent content
	// plus all evidence members, since the model received the merged text.
	// Using only ref.Content misses facts that live exclusively in members.
	mergedForVisible := ref.Content
	for _, ev := range evidence {
		if ev.Content != "" {
			mergedForVisible += " " + ev.Content
		}
	}
	visible := citationFactSet(mergedForVisible)
	var candidates []citationCandidate
	for _, member := range evidence {
		if member.ChunkID == "" || member.Content == "" || member.KnowledgeID != origin.KnowledgeID ||
			member.KnowledgeBaseID != origin.KnowledgeBaseID ||
			!citationTextBearing(member.ChunkType) {
			continue
		}
		// Both the original member and the model-visible body must contain the
		// quantity; a hidden neighbor suffix is never a source of extra facts.
		supported := make(map[string]bool)
		for _, fact := range citationNeedles(member.Content) {
			if visible[fact] {
				supported[fact] = true
			}
		}
		member.DocumentTitle = firstNonEmpty(member.DocumentTitle, ref.DocumentTitle)
		candidates = append(candidates, citationCandidate{evidence: member, facts: supported})
	}
	return candidates
}

func citationTextBearing(chunkType string) bool {
	return chunkType == "" || chunkType == types.ChunkTypeText || chunkType == types.ChunkTypeImageOCR
}

// Greedily cover the supported quantities with as few original chunks as
// possible. Partial coverage is retained for sentences citing multiple sources;
// unmatched quantities remain unsupported rather than being declared verified.
func selectCitationMembers(needles []string, candidates []citationCandidate, preferredID string) []types.CitationEvidence {
	remaining := make(map[string]bool, len(needles))
	for _, needle := range needles {
		remaining[needle] = true
	}
	var selected []types.CitationEvidence
	for len(remaining) > 0 {
		best, bestCount := -1, 0
		for i, candidate := range candidates {
			count := 0
			for fact := range remaining {
				if candidate.facts[fact] {
					count++
				}
			}
			if count > bestCount || (count > 0 && count == bestCount &&
				((candidate.evidence.ChunkID == preferredID && candidates[best].evidence.ChunkID != preferredID) ||
					(candidate.evidence.ChunkID != preferredID && candidates[best].evidence.ChunkID != preferredID &&
						candidate.evidence.ChunkID < candidates[best].evidence.ChunkID))) {
				best, bestCount = i, count
			}
		}
		if best < 0 {
			break
		}
		selected = append(selected, candidates[best].evidence)
		for fact := range candidates[best].facts {
			delete(remaining, fact)
		}
	}
	return selected
}

// citationSentence extracts the sentence that ends at offset, so a citation is
// verified against the claim it actually marks.
func citationSentence(text string, offset int) string {
	if offset > len(text) {
		offset = len(text)
	}
	if offset < 0 {
		offset = 0
	}
	prefix := citationLine(text, offset)
	// Citations often follow the sentence's final punctuation.
	prefix = strings.TrimRightFunc(prefix, func(c rune) bool {
		return unicode.IsSpace(c) || strings.ContainsRune("。！？!?；;.", c)
	})
	start := 0
	for i, c := range prefix {
		if strings.ContainsRune("\n。！？!?；;", c) {
			start = i + utf8.RuneLen(c)
		}
	}
	return prefix[start:]
}

// citationLine returns the whole source line ending at offset, so a citation
// that closes a line can still see figures stated earlier on the same line.
func citationLine(text string, offset int) string {
	if offset > len(text) {
		offset = len(text)
	}
	if offset < 0 {
		offset = 0
	}
	prefix := refCandidateRE.ReplaceAllString(text[:offset], "")
	if start := strings.LastIndexByte(prefix, '\n'); start >= 0 {
		prefix = prefix[start+1:]
	}
	return prefix
}

// normalizeCitationText folds width, case and whitespace without changing
// decimal points or treating different clause levels as equivalent.
func normalizeCitationText(s string) string {
	return strings.Map(func(c rune) rune {
		if unicode.IsSpace(c) {
			return ' '
		}
		return unicode.ToLower(c)
	}, norm.NFKC.String(s))
}

// citationNeedles returns complete normalized quantities, including numbered
// Chinese clause locators. Section/chapter semantics are deliberately separate.
func citationNeedles(sentence string) []string {
	// OCR frequently splits decimal digits and unit glyphs (for example
	// "0 5 MPa", "5 0 mm", or "2 000 k / ³"). Search the original text and
	// conservative OCR-repaired variants; the answer-side text is never
	// rewritten, only the request-local matching facts are broadened.
	seen := make(map[string]bool)
	var out []string
	for _, variant := range citationTextVariants(sentence) {
		for _, fact := range citationNeedlesOne(variant) {
			if !seen[fact] {
				seen[fact] = true
				out = append(out, fact)
			}
		}
	}
	return out
}

func citationNeedlesOne(sentence string) []string {
	sentence = normalizeCitationText(sentence)
	matches := citationNumberRE.FindAllStringSubmatchIndex(sentence, -1)
	out := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, m := range matches {
		// Match entire quantities, never 50 mm inside 150 mm, 2 t in a
		// document code, or the prefix of "7.262 to". A superscript footnote
		// flattened directly after the closing parenthesis ("6 dB(A)6") still
		// names the quantity; only a trailing word character is an extension.
		if (m[0] > 0 && (citationWordByte(sentence[m[0]-1]) || sentence[m[0]-1] == '.')) ||
			(m[1] < len(sentence) && citationWordByte(sentence[m[1]]) &&
				!(sentence[m[1]-1] == ')' && sentence[m[1]] >= '0' && sentence[m[1]] <= '9')) {
			continue
		}
		// "第 1–11 段" is a clause range, not a measurement; taking 11 as a
		// paragraph needle would demand a citation the claim never makes.
		if prev, _ := utf8.DecodeLastRuneInString(sentence[:m[2]]); strings.ContainsRune("–—~～-", prev) {
			continue
		}
		if next, _ := utf8.DecodeRuneInString(sentence[m[3]:]); strings.ContainsRune("–—~～", next) {
			continue
		}
		number := strings.NewReplacer(",", "", " ", "").Replace(sentence[m[2]:m[3]])
		number = strings.TrimPrefix(number, "+")
		if strings.Contains(number, ".") {
			number = strings.TrimRight(strings.TrimRight(number, "0"), ".")
		}
		unit := sentence[m[4]:m[5]]
		switch unit {
		case "毫米":
			unit = "mm"
		case "厘米":
			unit = "cm"
		case "公里":
			unit = "km"
		case "米", "metre", "metres", "meter", "meters":
			unit = "m"
		case "平方米":
			unit = "m2"
		case "立方米":
			unit = "m3"
		case "公斤":
			unit = "kg"
		case "ton", "tons", "tonne", "tonnes", "t":
			unit = "tonne"
		case "percent":
			unit = "%"
		case "星期", "週", "周":
			// The Chinese documents and the citation line both use these for
			// week counts; fold them onto the English unit so a 12-week limit
			// matches "連續12星期".
			unit = "week"
		case "weeks":
			unit = "week"
		case "節", "节":
			unit = "section"
		case "章":
			unit = "chapter"
		case "段":
			unit = "paragraph"
		case "款":
			unit = "clause"
		case "條", "条":
			// 條 and Clause name the same locator; answer text often cites a
			// clause number in Chinese while the source writes "Clause 2.2.41".
			unit = "clause"
		case "年", "月", "日", "號":
			// Dates are not technical measurements; skip them as needles.
			continue
		}
		fact := number + "|" + unit
		if !seen[fact] {
			out = append(out, fact)
			seen[fact] = true
		}
	}
	for _, match := range citationRatioRE.FindAllStringSubmatchIndex(sentence, -1) {
		// Document codes ("TC(W) No. 8/2018", "PN 4/23") look like ratios but
		// name a circular, not a measured proportion; treating them as facts
		// would demand coverage no source chunk can provide.
		if citationDocumentCode(sentence[:match[0]], sentence[match[4]:match[5]]) {
			continue
		}
		fact := strings.NewReplacer(",", "", " ", "").Replace(sentence[match[2]:match[3]]) + "/" +
			strings.NewReplacer(",", "", " ", "").Replace(sentence[match[4]:match[5]]) + "|ratio"
		if !seen[fact] {
			out = append(out, fact)
			seen[fact] = true
		}
	}
	for _, match := range citationClauseRE.FindAllStringSubmatch(sentence, -1) {
		var number, kind string
		if match[1] != "" {
			number, kind = match[1], match[2]
		} else {
			number, kind = match[4], match[3]
		}
		if number == "" || kind == "" {
			continue
		}
		number = strings.ReplaceAll(number, " ", "")
		// A sub-item (5) and a lettered variant 24.65A name the same locator
		// family, and the source body often restates only "Clause 24.65".
		// Fold both suffixes away so the citation can map to the chunk that
		// actually holds the clause instead of falling back to its parent.
		if i := strings.IndexByte(number, '('); i >= 0 {
			number = number[:i]
		}
		number = strings.TrimRightFunc(number, func(r rune) bool { return r >= 'a' && r <= 'z' })
		kind = strings.ToLower(kind)
		switch kind {
		case "節", "节":
			kind = "section"
		case "章":
			kind = "chapter"
		case "段":
			kind = "paragraph"
		case "款":
			kind = "clause"
		case "條", "条":
			kind = "clause"
		}
		fact := number + "|" + kind
		if !seen[fact] {
			out = append(out, fact)
			seen[fact] = true
		}
	}
	return out
}

func citationTextVariants(text string) []string {
	base := normalizeCitationText(text)
	variants := []string{}
	// Restore a missing decimal point when OCR put one digit on each side of a
	// space immediately before a technical unit. This intentionally excludes
	// dates and bare prose numbers.
	decimal := regexp.MustCompile(`\b(\d+)\s+(\d)\s*(mm[23]?|cm[23]?|m[23]?|μm|kg|kpa|mpa|kn|v|%|毫米|厘米|米|公斤|次|套|人|層|层)\b`).ReplaceAllString(base, `$1.$2 $3`)
	decimal = regexp.MustCompile(`\b(\d+)\s+(\d{2})\s*(mm[23]?|cm[23]?|m[23]?|毫米|厘米|米)\b`).ReplaceAllString(decimal, `$1.$2 $3`)
	decimal = regexp.MustCompile(`\b(\d+)\s+\.\s*(\d+)\s*(mm[23]?|cm[23]?|m[23]?|毫米|厘米|米)\b`).ReplaceAllString(decimal, `$1.$2 $3`)
	variants = append(variants, decimal)
	// OCR can split thousands and lose the g/m³ glyphs in kg/m³.
	thousands := regexp.MustCompile(`\b(\d{1,3})\s+(\d{3})\s*(kg|k\s*/\s*[³3]|m[23]?|米|公斤)\b`).ReplaceAllString(base, `$1$2 $3`)
	thousands = regexp.MustCompile(`\bk\s*/\s*[³3]\b`).ReplaceAllString(thousands, `kg/m3`)
	if thousands != base {
		variants = append(variants, thousands)
	}
	// "10 onoff cycles per hour" and "每小时 10 次" name the same count without
	// a measurement unit; fold the English cycle/time noun onto 次 so the
	// Chinese answer can map back to the sentence that states it.
	cycles := regexp.MustCompile(`\b(\d+)\s*(?:on\s*/?\s*off\s+)?(?:cycles?|times?)\b`).ReplaceAllString(base, `$1 次`)
	if cycles != base {
		variants = append(variants, cycles)
	}
	return variants
}

func citationWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
}

// citationDocumentCode reports whether a slash-joined number pair is a
// circular or standard designation ("No. 8/2018", "PN 4/23", "TC(W) No. 8/2018")
// rather than a measured proportion.
func citationDocumentCode(prefix, denominator string) bool {
	trimmed := strings.TrimRight(prefix, " \t(")
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "no.") || strings.HasSuffix(lower, "no") || strings.HasSuffix(lower, "pn") {
		return true
	}
	// A 19xx/20xx denominator after a revision marker such as "9/2025".
	if len(denominator) == 4 && (strings.HasPrefix(denominator, "19") || strings.HasPrefix(denominator, "20")) {
		return true
	}
	return false
}

var citationNumberRE = regexp.MustCompile(`([-+]?(?:[0-9]{1,3}(?:[, ][0-9]{3})+|[0-9]+)(?:\.[0-9]+)?)[-\s]*(mm[23]?|cm[23]?|km[23]?|metres?|meters?|m[23]?|μm|kg|tonnes?|tons?|t|kpa|mpa|kn|db(?:\(a\))?|v|percent|%|days?|weeks?|months?|years?|hours?|毫米|厘米|公里|平方米|立方米|米|公斤|年|段|條|条|节|節|款|套|次|人|層|层|星期|週)`)
var citationRatioRE = regexp.MustCompile(`([0-9]{1,3}(?:[, ][0-9]{3})+|[0-9]+)\s*/\s*([0-9]{1,3}(?:[, ][0-9]{3})+|[0-9]+)`)
var citationClauseRE = regexp.MustCompile(`第\s*([0-9]+(?:\.[0-9]+)*(?:[a-z])?(?:\s*\([0-9]+\))?)\s*(章|節|节|段|款|條|条)|\b(section|chapter|clause|paragraph|article)\s*([0-9]+(?:\.[0-9]+)*(?:[a-z])?(?:\s*\([0-9]+\))?)(?:[^a-z0-9_]|$)`)

func citationFactSet(content string) map[string]bool {
	facts := make(map[string]bool)
	for _, fact := range citationNeedles(content) {
		facts[fact] = true
	}
	return facts
}

func escapeAttr(value string) string { return html.EscapeString(value) }

// citationStreamExpander prevents partial private <ref/> tags from reaching SSE while
// preserving normal streaming for all other content.
type citationStreamExpander struct {
	registry *sourceRegistry
	pending  string
	context  string
}

func newCitationStreamExpander(registry *sourceRegistry) *citationStreamExpander {
	return &citationStreamExpander{registry: registry}
}

func (d *citationStreamExpander) Feed(chunk string) string {
	if d == nil || d.registry == nil {
		return chunk
	}
	data := d.pending + chunk
	d.pending = ""
	var out strings.Builder
	for data != "" {
		idx := strings.Index(data, "<")
		if idx < 0 {
			out.WriteString(data)
			d.rememberText(data)
			break
		}
		out.WriteString(data[:idx])
		d.rememberText(data[:idx])
		data = data[idx:]
		lower := strings.ToLower(data)
		if isSourceTagPending(lower) && !strings.Contains(data, ">") {
			d.pending = data
			break
		}
		if isRefTagStart(lower) {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			tag := data[:end+1]
			if refTagRE.MatchString(tag) {
				out.WriteString(d.registry.expandTag(d.context, len(d.context), tag))
			}
			data = data[end+1:]
			continue
		}
		if isNamedTagStart(lower, "kb") || isNamedTagStart(lower, "web") {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			data = data[end+1:]
			continue
		}
		out.WriteByte('<')
		d.rememberText("<")
		data = data[1:]
	}
	return out.String()
}

func (d *citationStreamExpander) rememberText(text string) {
	d.context += text
	// Inline citations only need their current line, not the whole answer.
	if start := strings.LastIndexByte(d.context, '\n'); start >= 0 {
		d.context = d.context[start+1:]
	}
}

func isRefTagStart(value string) bool {
	return isNamedTagStart(value, "ref")
}

func isNamedTagStart(value, name string) bool {
	prefix := "<" + name
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	if len(value) == len(prefix) {
		return true
	}
	next := value[len(prefix)]
	return next == ' ' || next == '\t' || next == '\r' || next == '\n' || next == '>'
}

func isSourceTagPending(value string) bool {
	for _, name := range []string{"ref", "kb", "web"} {
		prefix := "<" + name
		if (len(value) <= len(prefix) && strings.HasPrefix(prefix, value)) || isNamedTagStart(value, name) {
			return true
		}
	}
	return false
}

func (d *citationStreamExpander) Flush() string {
	if d == nil {
		return ""
	}
	pending := d.pending
	d.pending = ""
	lower := strings.ToLower(pending)
	if isSourceTagPending(lower) {
		return ""
	}
	return d.registry.ExpandText(pending)
}
