package linkcheck_test

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iderex/kanzlei/internal/linkcheck"
)

func file(name, body string) linkcheck.File {
	return linkcheck.File{Name: name, Bytes: []byte(body)}
}

func urls(links []linkcheck.Link) []string {
	out := make([]string, 0, len(links))
	for _, l := range links {
		out = append(out, l.URL)
	}
	return out
}

func TestALinkInProseIsReadWithItsLine(t *testing.T) {
	links := linkcheck.Links(file("docs/a.md", "first\nsee [the text](https://example.org/one) for it\n"))
	if len(links) != 1 {
		t.Fatalf("want one link, got %v", urls(links))
	}
	if links[0].Line != 2 || links[0].URL != "https://example.org/one" || links[0].File != "docs/a.md" {
		t.Fatalf("the link does not carry where it is: %+v", links[0])
	}
}

func TestAnAutolinkIsReadTheWayAReaderLinksIt(t *testing.T) {
	links := linkcheck.Links(file("docs/a.md", "report it at <https://example.org/new>\n"))
	if got := urls(links); len(got) != 1 || got[0] != "https://example.org/new" {
		t.Fatalf("want the autolink, got %v", got)
	}
}

// The boundary this shares with internal/doclint, from both sides. A command
// block holds addresses that are arguments, and a document showing the link
// syntax is showing it rather than writing one.
func TestWhatIsNotProseIsNotRead(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"an indented block", "prose\n\n    curl https://example.org/gone\n"},
		{"a tab-indented block", "prose\n\n\tcurl https://example.org/gone\n"},
		{"a fenced block", "prose\n\n```\n[text](https://example.org/gone)\n```\n"},
		{"a code span", "write it as `[text](https://example.org/gone)` in a document\n"},
		{"a relative target", "see [the readme](../README.md)\n"},
		{"a mail address", "write to [us](mailto:nobody@example.org)\n"},
		{"a fragment", "see [the section](#style)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if links := linkcheck.Links(file("docs/a.md", tc.body)); len(links) != 0 {
				t.Fatalf("read %v out of %q", urls(links), tc.body)
			}
		})
	}
}

// A span holding a backtick is why the run lengths have to match, and an
// unclosed run is a stray backtick rather than a span that swallows the rest of
// the line.
func TestASpanEndsWhereItsOwnRunEnds(t *testing.T) {
	links := linkcheck.Links(file("docs/a.md", "``a ` b`` then [text](https://example.org/after)\n"))
	if got := urls(links); len(got) != 1 || got[0] != "https://example.org/after" {
		t.Fatalf("want the link after the span, got %v", got)
	}
	links = linkcheck.Links(file("docs/a.md", "a stray ` and [text](https://example.org/after)\n"))
	if got := urls(links); len(got) != 1 || got[0] != "https://example.org/after" {
		t.Fatalf("an unclosed run swallowed the line: %v", got)
	}
}

func TestALinkTitleIsNotPartOfTheAddress(t *testing.T) {
	links := linkcheck.Links(file("docs/a.md", "see [text](https://example.org/one \"a title\")\n"))
	if got := urls(links); len(got) != 1 || got[0] != "https://example.org/one" {
		t.Fatalf("the title was read as part of the address: %v", got)
	}
}

func TestAnUnfinishedLinkShapeIsNotAnAddress(t *testing.T) {
	for _, body := range []string{
		"see [text](https://example.org/one\n",
		"see <https://example.org/one\n",
		"see <a b> and [text]\n",
	} {
		if links := linkcheck.Links(file("docs/a.md", body)); len(links) != 0 {
			t.Fatalf("read %v out of %q", urls(links), body)
		}
	}
}

func TestClassifySeparatesTheTwoWaysALinkDiesFromEveryOtherAnswer(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", Name: "example.invalid", IsNotFound: true}
	temporary := &net.DNSError{Err: "server misbehaving", Name: "example.org", IsTemporary: true}

	for _, tc := range []struct {
		name   string
		answer linkcheck.Answer
		want   linkcheck.Verdict
	}{
		{"not found", linkcheck.Answer{Status: 404}, linkcheck.Gone},
		{"gone", linkcheck.Answer{Status: 410}, linkcheck.Gone},
		{"the host does not resolve", linkcheck.Answer{Err: notFound}, linkcheck.Gone},
		{"a name lookup that failed for another reason", linkcheck.Answer{Err: temporary}, linkcheck.Transient},
		{"a timeout", linkcheck.Answer{Err: errors.New("context deadline exceeded")}, linkcheck.Transient},
		{"a server error", linkcheck.Answer{Status: 503}, linkcheck.Transient},
		{"too many requests", linkcheck.Answer{Status: 429}, linkcheck.Transient},
		{"an answer", linkcheck.Answer{Status: 200}, linkcheck.Reachable},
		{"a redirect that was followed", linkcheck.Answer{Status: 301}, linkcheck.Reachable},
		{"a credential is needed", linkcheck.Answer{Status: 403}, linkcheck.Reachable},
		{"the method is declined", linkcheck.Answer{Status: 405}, linkcheck.Reachable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := linkcheck.Classify(tc.answer)
			if got != tc.want {
				t.Fatalf("classified as %v, want %v (%s)", got, tc.want, detail)
			}
			if detail == "" {
				t.Fatal("an answer was classified with no reason beside it")
			}
		})
	}
}

// The bite the contributor guide asks to be shown by running rather than
// asserted. A document pointing at an address the other end says it does not
// have has to be red, and the finding has to carry the document and the line.
func TestALinkTheOtherEndSaysIsGoneIsRefusedWithItsFileAndItsLine(t *testing.T) {
	c := linkcheck.Checker{
		Probe:    func(string) linkcheck.Answer { return linkcheck.Answer{Status: 404} },
		Attempts: 3,
	}
	findings, report := c.Check([]linkcheck.File{file("docs/a.md", "one\ntwo [text](https://example.org/gone)\n")})
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %v", findings)
	}
	if !findings[0].Refused {
		t.Fatal("a gone link was reported as not judged rather than refused")
	}
	if got := findings[0].String(); !strings.Contains(got, "docs/a.md:2") || !strings.Contains(got, "is gone") {
		t.Fatalf("the finding does not say where or what: %q", got)
	}
	if report.Gone != 1 || report.Links != 1 || report.Reachable != 0 {
		t.Fatalf("the report does not count what happened: %s", report)
	}
}

// The other half of the same rule, and the reason this is not a request in a
// loop: a host that was slow once is not a dead link.
func TestATransientAnswerIsRetriedAndTheLinkPasses(t *testing.T) {
	calls := 0
	c := linkcheck.Checker{
		Probe: func(string) linkcheck.Answer {
			calls++
			if calls < 3 {
				return linkcheck.Answer{Status: 503}
			}
			return linkcheck.Answer{Status: 200}
		},
		Attempts: 3,
	}
	findings, report := c.Check([]linkcheck.File{file("docs/a.md", "[text](https://example.org/slow)\n")})
	if len(findings) != 0 {
		t.Fatalf("a link that answered on the third attempt was reported: %v", findings)
	}
	if calls != 3 {
		t.Fatalf("want three attempts, got %d", calls)
	}
	if report.Reachable != 1 {
		t.Fatalf("the report does not count the recovery: %s", report)
	}
}

// A run that could not judge says so and is not a refusal. Silence here would
// read as a green gate over a link nothing reached.
func TestALinkNoAttemptCouldJudgeIsPrintedAndIsNotARefusal(t *testing.T) {
	c := linkcheck.Checker{
		Probe:    func(string) linkcheck.Answer { return linkcheck.Answer{Err: errors.New("connection reset")} },
		Attempts: 2,
	}
	findings, report := c.Check([]linkcheck.File{file("docs/a.md", "[text](https://example.org/down)\n")})
	if len(findings) != 1 || findings[0].Refused {
		t.Fatalf("want one finding that is not a refusal, got %v", findings)
	}
	if got := findings[0].String(); !strings.Contains(got, "was not judged") || !strings.Contains(got, "after 2 attempt(s)") {
		t.Fatalf("the finding does not say it was not judged, or how hard it tried: %q", got)
	}
	if report.Unjudged != 1 {
		t.Fatalf("the report does not count it: %s", report)
	}
}

// A permanent answer is taken on the first attempt. Asking a server that has
// already said it does not have this a second time changes nothing.
func TestAPermanentAnswerIsNotRetried(t *testing.T) {
	calls := 0
	c := linkcheck.Checker{
		Probe: func(string) linkcheck.Answer {
			calls++
			return linkcheck.Answer{Status: 410}
		},
		Attempts: 5,
	}
	if _, _ = c.Check([]linkcheck.File{file("docs/a.md", "[text](https://example.org/gone)\n")}); calls != 1 { // the count of probes is the whole assertion here, and the findings are proved by the case above
		t.Fatalf("a permanent answer was asked for %d times", calls)
	}
}

func TestAnAddressIsProbedOnceHoweverManyDocumentsCarryIt(t *testing.T) {
	calls := 0
	c := linkcheck.Checker{
		Probe: func(string) linkcheck.Answer {
			calls++
			return linkcheck.Answer{Status: 200}
		},
		Attempts: 1,
	}
	findings, report := c.Check([]linkcheck.File{
		file("docs/a.md", "[text](https://example.org/one)\n"),
		file("docs/b.md", "[text](https://example.org/one)\n"),
	})
	if len(findings) != 0 {
		t.Fatalf("a reachable address was reported: %v", findings)
	}
	if calls != 1 {
		t.Fatalf("the same address was probed %d times", calls)
	}
	if report.Links != 2 || report.Reachable != 2 || report.Documents != 2 {
		t.Fatalf("the report counts probes rather than links: %s", report)
	}
}

func TestFindingsComeBackInTheOrderAReaderWouldOpenThem(t *testing.T) {
	c := linkcheck.Checker{
		Probe:    func(string) linkcheck.Answer { return linkcheck.Answer{Status: 404} },
		Attempts: 1,
	}
	findings, _ := c.Check([]linkcheck.File{ // the order of the findings is the assertion, and the report is proved by the cases above
		file("docs/b.md", "[text](https://example.org/three)\n"),
		file("docs/a.md", "one\n[text](https://example.org/two)\n"),
		file("docs/a.md", "[text](https://example.org/one)\n"),
	})
	var got []string
	for _, f := range findings {
		got = append(got, f.File+":"+f.URL)
	}
	want := []string{"docs/a.md:https://example.org/one", "docs/a.md:https://example.org/two", "docs/b.md:https://example.org/three"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("findings came back as %v, want %v", got, want)
	}
}

// The pause between attempts is a field rather than a constant so that a case
// can prove the retry without waiting for it, and so that the waiting itself is
// exercised by something rather than only configured.
func TestThePauseBetweenAttemptsIsTakenBeforeTheSecondOne(t *testing.T) {
	calls := 0
	c := linkcheck.Checker{
		Probe: func(string) linkcheck.Answer {
			calls++
			if calls == 1 {
				return linkcheck.Answer{Status: 503}
			}
			return linkcheck.Answer{Status: 200}
		},
		Attempts: 2,
		Pause:    time.Millisecond,
	}
	start := time.Now()
	findings, _ := c.Check([]linkcheck.File{file("docs/a.md", "[text](https://example.org/slow)\n")}) // the elapsed time is the assertion, and the report is proved by the cases above
	if len(findings) != 0 {
		t.Fatalf("a link that answered on the second attempt was reported: %v", findings)
	}
	if elapsed := time.Since(start); elapsed < time.Millisecond {
		t.Fatalf("the second attempt was made after %v, which is no pause at all", elapsed)
	}
}

func TestAnAttemptCountBelowOneStillAsksOnce(t *testing.T) {
	calls := 0
	c := linkcheck.Checker{Probe: func(string) linkcheck.Answer {
		calls++
		return linkcheck.Answer{Status: 200}
	}}
	if _, report := c.Check([]linkcheck.File{file("docs/a.md", "[text](https://example.org/one)\n")}); report.Reachable != 1 {
		t.Fatalf("the report does not count the one probe: %s", report)
	}
	if calls != 1 {
		t.Fatalf("want one probe, got %d", calls)
	}
}

// A tree whose documents carry no external link at all is this repository
// today, and the count is what says so. A gate that printed nothing here would
// be indistinguishable from one that read nothing.
func TestATreeWithNoExternalLinkReportsThatItReadTheDocuments(t *testing.T) {
	c := linkcheck.Checker{Probe: func(string) linkcheck.Answer {
		t.Fatal("a probe was made for a tree with no external link")
		return linkcheck.Answer{}
	}}
	findings, report := c.Check([]linkcheck.File{file("docs/a.md", "prose with `internal/authz` in it\n")})
	if len(findings) != 0 {
		t.Fatalf("findings out of a document with no link: %v", findings)
	}
	if got := report.String(); !strings.Contains(got, "0 external link(s) across 1 tracked document(s)") {
		t.Fatalf("the report does not say what it read: %q", got)
	}
}
