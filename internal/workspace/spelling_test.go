package workspace

import (
	"strings"
	"testing"
)

func TestLocalSpellingReview(t *testing.T) {
	p := NewProject("", true)
	p.Chapters[0].Text = "她迫不急待地打开信封。\n他终于走头无路，只能再接再励。\n小芳提心吊胆，却没有走投无路。"
	review, err := ReviewChapter(p, p.Chapters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int]int{}
	for _, issue := range review.Issues {
		if issue.Kind == "疑似错别字" {
			counts[issue.Paragraph]++
			if !strings.Contains(issue.Message, "建议") {
				t.Fatal("missing correction suggestion")
			}
		}
	}
	if counts[1] != 1 || counts[2] != 2 || counts[3] != 0 {
		t.Fatalf("incorrect matches/locations: %v", counts)
	}
	if p.Chapters[0].Text != "她迫不急待地打开信封。\n他终于走头无路，只能再接再励。\n小芳提心吊胆，却没有走投无路。" {
		t.Fatal("review mutated manuscript")
	}
	for _, pair := range spellingPairs {
		if pair[0] == pair[1] {
			t.Fatalf("invalid rule: %v", pair)
		}
	}
}
