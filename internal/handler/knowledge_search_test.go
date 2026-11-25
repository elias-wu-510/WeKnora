package handler

import "testing"

func TestQueryTokens(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"建筑高度限制", []string{"建筑", "筑高", "高度", "度限", "限制"}},
		{"OTTV 计算", []string{"ottv", "计算"}},
		{"zzzz-no-such-topic", []string{"zzzz", "such", "topic"}},
		{"a", nil},
	}
	for _, tc := range cases {
		got := queryTokens(tc.query)
		if len(got) != len(tc.want) {
			t.Fatalf("query %q: got %v want %v", tc.query, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		}
	}
}

func TestContainsAnyToken(t *testing.T) {
	tokens := queryTokens("建筑高度限制")
	if !containsAnyToken("本规范规定建筑高度不得超过…", tokens) {
		t.Fatal("expected match on 建筑")
	}
	if containsAnyToken("今天天气不错，适合户外活动。", tokens) {
		t.Fatal("unexpected match for off-topic text")
	}
	if containsAnyToken("This noise report explains nothing.", queryTokens("no plan")) {
		t.Fatal("short ASCII token must not match as a substring")
	}
}

func TestPerKBMatchesBounds(t *testing.T) {
	if got := perKBMatches(20); got != 6 {
		t.Fatalf("perKBMatches(20) = %d, want 6", got)
	}
	if got := perKBMatches(3); got != 5 {
		t.Fatalf("perKBMatches(3) = %d, want 5", got)
	}
	if got := perKBMatches(100); got != 15 {
		t.Fatalf("perKBMatches(100) = %d, want 15", got)
	}
}