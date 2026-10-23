package workspace

import (
	"encoding/json"
	"strings"
)

// Ignore non-text metadata (such as encrypted reasoning) without breaking the
// ordinary answer stream or rendering provider-internal objects as thoughts.
func modelThinkingText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	return ""
}

// Some local gateways put reasoning in content instead of a separate field.
// Keep partial tags across chunks so they never leak into the final manuscript.
type thinkingStream struct {
	pending string
	inside  bool
	answer  func(string) error
	thought func(string) error
}

func (s *thinkingStream) write(part string, final bool) error {
	s.pending += part
	for s.pending != "" {
		tags := []string{"<think>", "<thinking>"}
		if s.inside {
			tags = []string{"</think>", "</thinking>"}
		}
		// ASCII-only folding preserves byte offsets in Unicode manuscript text.
		folded := []byte(s.pending)
		for i, b := range folded {
			if b >= 'A' && b <= 'Z' {
				folded[i] = b + ('a' - 'A')
			}
		}
		lower := string(folded)
		at, tag := -1, ""
		for _, candidate := range tags {
			if i := strings.Index(lower, candidate); i >= 0 && (at < 0 || i < at) {
				at, tag = i, candidate
			}
		}
		emit := s.answer
		if s.inside {
			emit = s.thought
		}
		if at >= 0 {
			if at > 0 && emit != nil {
				if err := emit(s.pending[:at]); err != nil {
					return err
				}
			}
			s.pending = s.pending[at+len(tag):]
			s.inside = !s.inside
			continue
		}
		keep := 0
		if !final {
			for _, candidate := range tags {
				for n := 1; n < len(candidate) && n <= len(lower); n++ {
					if strings.HasSuffix(lower, candidate[:n]) && n > keep {
						keep = n
					}
				}
			}
		}
		n := len(s.pending) - keep
		if n > 0 && emit != nil {
			if err := emit(s.pending[:n]); err != nil {
				return err
			}
		}
		s.pending = s.pending[n:]
		break
	}
	return nil
}
