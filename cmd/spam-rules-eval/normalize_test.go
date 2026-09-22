package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

type normalizationCase struct {
	name, text, label string
	raw, normalized   []string
}

func TestNormalizedDocumentsThroughCommand(t *testing.T) {
	cases := []normalizationCase{
		{"advertisement", "π <p>money <b>back</b>&nbsp;guarantee</p>", promotionalLabel, nil, []string{moneyBackRule}},
		{"quoted-documentation", "<p>Example claim for a spam classifier:</p><pre><code>money <b>back</b>&nbsp;guarantee</code></pre>", legitimateLabel, nil, []string{moneyBackRule}},
		{"encoded-example", "Documentation example: &lt;p&gt;money&#32;back&#32;guarantee&lt;/p&gt;", legitimateLabel, nil, []string{moneyBackRule}},
		{"split-word", "m&#111;ney back guarantee", promotionalLabel, nil, []string{moneyBackRule}},
		{"wrapped-line", "money\r\n  back\tguarantee", promotionalLabel, nil, []string{moneyBackRule}},
		{"separate-blocks", "<p>money</p><p>back guarantee</p>", legitimateLabel, nil, nil},
		{"separate-paragraphs", "money\r\n\r\nback guarantee", legitimateLabel, nil, nil},
		{"script", "<script>money back guarantee</script><p>library docs</p>", legitimateLabel, []string{moneyBackRule}, nil},
		{"style", "<style>/* money back guarantee */</style><p>library docs</p>", legitimateLabel, []string{moneyBackRule}, nil},
		{"self-closing-script", "<script/>money back guarantee</script><p>library docs</p>", legitimateLabel, []string{moneyBackRule}, nil},
		{"self-closing-style", "<style/>money back guarantee</style><p>library docs</p>", legitimateLabel, []string{moneyBackRule}, nil},
		{"unclosed-script", "<script/>money back guarantee", legitimateLabel, []string{moneyBackRule}, nil},
		{"unclosed-style", "<style/>money back guarantee", legitimateLabel, []string{moneyBackRule}, nil},
		{"after-self-closing-script", "<SCRIPT/>money back guarantee</SCRIPT><p>money back guarantee</p>", promotionalLabel, []string{moneyBackRule}, []string{moneyBackRule}},
		{"after-self-closing-style", "<STYLE/>money back guarantee</STYLE><p>money back guarantee</p>", promotionalLabel, []string{moneyBackRule}, []string{moneyBackRule}},
		{"self-closing-template", "<template/><template/>library docs</template>money back guarantee</template>", legitimateLabel, []string{moneyBackRule}, nil},
		{"template", "<template><template>money back guarantee</template></template>library docs", legitimateLabel, []string{moneyBackRule}, nil},
		{"attribute", `<span title="money back guarantee">library docs</span>`, legitimateLabel, []string{moneyBackRule}, nil},
		{"raw-rule", `<img src="data:text/html;base64,AAAA">`, legitimateLabel, []string{"RSPAMD_HAS_DATA_URI", "RSPAMD_DATA_URI_OBFU"}, []string{"RSPAMD_HAS_DATA_URI", "RSPAMD_DATA_URI_OBFU"}},
		{"uri-rule", `<a href="http://203.0.113.9/click">library docs</a>`, legitimateLabel, []string{numericHostRule, ipLinkRule}, []string{numericHostRule, ipLinkRule}},
	}
	rulesPath, err := filepath.Abs(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte(tc.text), 0o600); err != nil {
				t.Fatal(err)
			}
			catalogue := []map[string]any{{"registry": "synthetic", "name": tc.name, "version": "1", "label": tc.label, "documents": []map[string]string{{"path": "README.txt", "member": "package/README.md"}}}}
			data, err := json.Marshal(catalogue)
			if err != nil {
				t.Fatal(err)
			}
			catalogPath := filepath.Join(dir, "catalog.json")
			if err := os.WriteFile(catalogPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			for _, view := range []string{rawMode, normalizedMode} {
				result := runTestCorpus(t, rulesPath, catalogPath, view)
				verifyNormalizedCase(t, tc, view, result)
			}

		})
	}
}

func TestNormalizedMappingAndBoundaries(t *testing.T) {
	data := []byte("<p>α m<b>oney</b>\u00a0back &amp; &#x20AC; &NotEqualTilde;</p><p>next</p>")
	views, err := normalizeText(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 || string(views[0].Text) != "α money back & € ≂̸" || string(views[1].Text) != "next" {
		t.Fatalf("wrong paragraphs: %+v", views)
	}
	for _, view := range views {
		if len(view.Text) != len(view.Sources) || !utf8.Valid(view.Text) {
			t.Fatal("invalid byte mapping")
		}
		lastStart, lastEnd := 0, 0
		for _, span := range view.Sources {
			if span.Start < lastStart || span.End < lastEnd || span.Start >= span.End || span.End > len(data) {
				t.Fatal("nonmonotonic source mapping")
			}
			lastStart, lastEnd = span.Start, span.End
		}
	}
}

func verifyNormalizedCase(t *testing.T, tc normalizationCase, view string, result output) {
	t.Helper()
	if len(result.Rows) != 1 || result.Rows[0].Label != tc.label {
		t.Fatal("label or row changed")
	}
	var ids []string
	for _, h := range result.Rows[0].Hits {
		ids = append(ids, h.Rule)
		if h.Start < 0 || h.End > len(tc.text) || h.Start >= h.End || !strings.HasPrefix(tc.text[h.Start:h.End], h.Excerpt) {
			t.Fatalf("bad original excerpt %+v", h)
		}
		if h.View != "" {
			verifyNormalizedHit(t, tc, h)
		}
	}
	expected := tc.raw
	if view == normalizedMode {
		expected = tc.normalized
	}
	slices.Sort(ids)
	expected = slices.Clone(expected)
	slices.Sort(expected)
	if !slices.Equal(ids, expected) {
		t.Fatalf("%s: got %v want %v", view, ids, expected)
	}
}

func verifyNormalizedHit(t *testing.T, tc normalizationCase, h hit) {
	t.Helper()
	if h.View != normalizedView || h.MatchedText != "money back guarantee" {
		t.Fatalf("bad normalized match %+v", h)
	}
	if strings.HasPrefix(tc.name, "after-self-closing-") && h.Start != strings.LastIndex(tc.text, "money back guarantee") {
		t.Fatalf("match should follow the suppressed element: %+v", h)
	}
	previousEnd := h.Start
	var fragments []string
	for _, span := range h.OriginalSpans {
		if span.Start < previousEnd || span.End <= span.Start || span.End > h.End || !utf8.ValidString(tc.text[span.Start:span.End]) {
			t.Fatalf("bad original span %+v", span)
		}
		previousEnd = span.End
		fragments = append(fragments, tc.text[span.Start:span.End])
	}
	if previousEnd != h.End {
		t.Fatal("mapping does not reach match end")
	}
	if tc.name == "advertisement" && !slices.Equal(fragments, []string{"money ", "back", "&nbsp;guarantee"}) {
		t.Fatalf("unexpected mapped fragments %q", fragments)
	}
}
