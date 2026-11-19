package modelcontext

import (
	"regexp"
	"strings"
)

// CitationViolation is one line whose measured fact is carried by the retrieved
// context but not by any of the chunks it cites. Generation-time validation
// reports these before the answer leaves the server; it never rewrites a
// citation to a different chunk on its own.
type CitationViolation struct {
	Line    string
	ChunkID string
	Fact    string
}

var inlineChunkIDRE = regexp.MustCompile(`chunk_id="([^"]+)"`)

// ValidateAnswer reports citations that point at chunks without the line's
// numbers or clauses while another retrieved chunk does carry them. A fact that
// no retrieved chunk carries is not reported: there is nothing to re-point to,
// and deleting the claim to satisfy the check is exactly what must not happen.
func (r *Registry) ValidateAnswer(text string) []CitationViolation {
	if r == nil || r.sources == nil || text == "" {
		return nil
	}
	available := r.availableFacts()
	if len(available) == 0 {
		return nil
	}
	var out []CitationViolation
	for _, line := range strings.Split(text, "\n") {
		matches := inlineChunkIDRE.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			continue
		}
		body := strings.TrimSpace(inlineChunkIDRE.ReplaceAllString(line, ""))
		needles := citationNeedles(body)
		needles = measuredQuantities(needles)
		if len(needles) == 0 {
			continue
		}
		cited := make(map[string]bool)
		for _, match := range matches {
			content, ok := r.sources.chunkOriginal(match[1])
			if !ok {
				continue
			}
			for fact := range citationFactSet(content) {
				cited[fact] = true
			}
		}
		for _, needle := range needles {
			if cited[needle] || !available[needle] {
				continue
			}
			out = append(out, CitationViolation{Line: body, ChunkID: matches[0][1], Fact: needle})
			break
		}
	}
	return out
}

// availableFacts is the union of measured facts carried by the retrieved chunk
// bodies, i.e. what the model could legitimately cite.
func (r *Registry) availableFacts() map[string]bool {
	facts := make(map[string]bool)
	for _, entry := range r.sources.chunks.entries() {
		for fact := range citationFactSet(entry.meta.Content) {
			facts[fact] = true
		}
		for _, evidence := range entry.meta.Evidence {
			for fact := range citationFactSet(evidence.Content) {
				facts[fact] = true
			}
		}
	}
	return facts
}

// measuredQuantities drops section/clause locators: an answer may mention a
// clause that the cited chunk does not state (e.g. "section 9 comes from another
// document"), and re-pointing such mentions would be wrong.
func measuredQuantities(needles []string) []string {
	out := needles[:0]
	for _, needle := range needles {
		if citationLocatorNeedle(needle) {
			continue
		}
		out = append(out, needle)
	}
	return out
}

// FactLineCount counts lines that carry a citation and at least one measured
// number. Repair must never reduce it: dropping a claim shrinks the evaluation
// denominator instead of fixing the citation.
func FactLineCount(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if !inlineChunkIDRE.MatchString(line) {
			continue
		}
		body := inlineChunkIDRE.ReplaceAllString(line, "")
		if len(measuredQuantities(citationNeedles(strings.TrimSpace(body)))) > 0 {
			count++
		}
	}
	return count
}

// ChunkText returns the body a citation to id resolves to. Repair prompts use it
// to show the model what the cited chunk actually says.
func (r *Registry) ChunkText(id string) (string, bool) {
	if r == nil || r.sources == nil {
		return "", false
	}
	return r.sources.chunkOriginal(id)
}

// chunkOriginal resolves a durable chunk id to the original body: the evidence
// member carrying that id when the result is a merged handle, otherwise the
// result body itself. Evidence members are not registered as handles, so a
// direct handle miss falls back to scanning every registered result.
func (r *sourceRegistry) chunkOriginal(id string) (string, bool) {
	if handle, ok := r.chunks.handleForKey(id); ok {
		if _, ref, ok := r.chunks.resolve(handle); ok {
			if content, ok := chunkOriginalIn(ref, id); ok {
				return content, true
			}
		}
	}
	for _, entry := range r.chunks.entries() {
		if content, ok := chunkOriginalIn(entry.meta, id); ok {
			return content, true
		}
	}
	return "", false
}

func chunkOriginalIn(ref ChunkReference, id string) (string, bool) {
	for _, evidence := range ref.Evidence {
		if evidence.ChunkID != id {
			continue
		}
		if evidence.Content == "" {
			return "", false
		}
		return evidence.Content, true
	}
	if ref.ChunkID == id && ref.Content != "" {
		return ref.Content, true
	}
	return "", false
}