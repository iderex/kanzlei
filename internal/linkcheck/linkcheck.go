// Package linkcheck reads this repository's own documents and refuses an
// external link that is permanently gone.
//
// internal/doclint already resolves a link that points inside this repository,
// so a path that moved is caught on the day it moves. An address pointing off
// this machine is the half nothing reads. It fails differently and more
// quietly: the sentence stays correct, the target is deleted or moved by
// somebody this repository has no relationship with, and the next reader
// follows it to nothing. #113 asks for both halves under one name.
//
// # Tolerant of a transient failure, not of a permanent one
//
// That sentence is the whole design and it is the reason this is not a request
// in a loop. A network is unreliable and a gate that reddens because a host was
// slow teaches people to re-run it until it is green, which is the same as
// having no gate. So an answer is placed in one of three states rather than
// two, in Classify, and only one of them is a refusal.
//
// A link that could not be judged is printed rather than passed over. A run
// that reached nothing and a run that found nothing look identical in an exit
// status, and the first would otherwise read as the second.
//
// # What it cannot see
//
// The residual is stated rather than left to be discovered. A host that is
// permanently unreachable answers the same way as one that is briefly
// unreachable, so it is reported as unjudged for as long as it stays down and
// is never refused. An address that answers with the wrong document answers
// correctly as far as this is concerned: nothing here reads what came back. And
// a link inside a command block is not read at all, which is the same boundary
// internal/doclint draws and for the same reason.
package linkcheck

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// A File is one document to read, named by its path from the repository root so
// that a finding carries a position a reader can open.
type File struct {
	Name  string
	Bytes []byte
}

// A Link is one place a document points off this machine.
type Link struct {
	File string
	Line int
	URL  string
}

// An Answer is what one request came back as, in the two things a caller can
// report without interpreting them. Status is zero where no response arrived.
//
// The interpretation is Classify's and is here rather than in the command, so
// that which answers refuse and which do not is decided by a function with
// fixtures in front of it rather than by whichever branch of an HTTP client
// somebody wrote last.
type Answer struct {
	Status int
	Err    error
}

// A Verdict is what one address is judged to be.
type Verdict int

const (
	// Reachable is an address the other end answered for. Nothing here reads
	// what came back, so this says the address exists and says nothing about
	// what is at it.
	Reachable Verdict = iota
	// Transient is an answer that says nothing about the address, because the
	// request did not get far enough to ask.
	Transient
	// Gone is an answer from the other end saying the address is not there.
	Gone
)

// Classify decides which of the three an answer is.
//
// Two states are permanent and both are the other end speaking. A status of 404
// or 410 is a server saying it does not have this, and a host that does not
// resolve is the naming system saying nobody has it. Those are the two ways a
// link dies and they are what this refuses.
//
// Everything else that failed is transient, including a timeout, a reset and a
// server error. A 429 belongs here too: it is a server declining to answer this
// minute rather than saying the address is wrong.
//
// A status this does not name is reachable, which is deliberate and is the
// direction to be wrong in. A 401 or a 403 is a document behind a credential
// and a 405 is a server that declines the method rather than the address; in
// all three the address is there, and refusing them would make the gate red for
// links that work in a browser.
func Classify(a Answer) (Verdict, string) {
	if a.Err != nil {
		var dns *net.DNSError
		if errors.As(a.Err, &dns) && dns.IsNotFound {
			return Gone, "the host does not resolve"
		}
		return Transient, a.Err.Error()
	}
	switch a.Status {
	case http.StatusNotFound, http.StatusGone:
		return Gone, fmt.Sprintf("the server answered %d", a.Status)
	case http.StatusTooManyRequests:
		return Transient, "the server answered 429"
	}
	if a.Status >= 500 {
		return Transient, fmt.Sprintf("the server answered %d", a.Status)
	}
	return Reachable, fmt.Sprintf("the server answered %d", a.Status)
}

// A Finding is a link that is gone, or one no attempt could judge. Refused says
// which, because only the first is a reason to be red.
type Finding struct {
	Link
	Detail  string
	Refused bool
}

func (f Finding) String() string {
	if f.Refused {
		return fmt.Sprintf("%s:%d: %s is gone: %s", f.File, f.Line, f.URL, f.Detail)
	}
	return fmt.Sprintf("%s:%d: %s was not judged: %s", f.File, f.Line, f.URL, f.Detail)
}

// A Report is what a run read. It is printed on the green route as well as the
// red one, because an exit status cannot tell a run that read nothing from one
// that found nothing.
type Report struct {
	Documents int
	Links     int
	Reachable int
	Gone      int
	Unjudged  int
}

func (r Report) String() string {
	return fmt.Sprintf("%d external link(s) across %d tracked document(s), %d reachable, %d gone, %d not judged",
		r.Links, r.Documents, r.Reachable, r.Gone, r.Unjudged)
}

// A Checker probes every link a set of documents carries.
//
// Attempts and Pause are the tolerance, and they are fields rather than
// constants so that a case can prove the retry without waiting for it. Probe is
// injected for the same reason the rule is a package rather than a shell block:
// what an answer means is decided here, under fixtures, and making the request
// is the command's job.
type Checker struct {
	Probe    func(url string) Answer
	Attempts int
	Pause    time.Duration
}

// Check probes every link once, retrying only a transient answer.
//
// The order is deterministic: one address is probed once however many documents
// carry it, and the findings come back sorted by file and line. A gate whose
// output moves between runs on an unchanged tree is one whose diffs cannot be
// read.
func (c Checker) Check(files []File) ([]Finding, Report) {
	report := Report{Documents: len(files)}

	var links []Link
	for _, f := range files {
		links = append(links, Links(f)...)
	}
	report.Links = len(links)

	type judgement struct {
		verdict Verdict
		detail  string
	}
	judged := map[string]judgement{}

	var findings []Finding
	for _, l := range links {
		j, seen := judged[l.URL]
		if !seen {
			verdict, detail := c.probeWithRetries(l.URL)
			j = judgement{verdict: verdict, detail: detail}
			judged[l.URL] = j
		}
		switch j.verdict {
		case Reachable:
			report.Reachable++
		case Gone:
			report.Gone++
			findings = append(findings, Finding{Link: l, Detail: j.detail, Refused: true})
		case Transient:
			report.Unjudged++
			findings = append(findings, Finding{Link: l, Detail: j.detail})
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, report
}

// probeWithRetries asks until an answer means something or the attempts run
// out. A permanent answer is taken on the first attempt: asking a server that
// has already said it does not have this a second time changes nothing and
// costs somebody else's bandwidth.
func (c Checker) probeWithRetries(url string) (Verdict, string) {
	attempts := c.Attempts
	if attempts < 1 {
		attempts = 1
	}
	var verdict Verdict
	var detail string
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && c.Pause > 0 {
			time.Sleep(c.Pause)
		}
		verdict, detail = Classify(c.Probe(url))
		if verdict != Transient {
			return verdict, detail
		}
	}
	return verdict, fmt.Sprintf("%s, after %d attempt(s)", detail, attempts)
}

// Links reads one document and reports every address it points at off this
// machine.
//
// The boundary is internal/doclint's, deliberately and not by coincidence. A
// fenced block and an indented block are passed over because they hold commands
// and a command carries addresses that are arguments rather than references,
// and a code span is separated from the prose around it because a document
// showing the link syntax is showing it rather than writing one.
//
// Two positions are read. A link target, which is what a reader clicks. And an
// autolink, `<https://example.org/>`, which is the other shape a markdown
// reader turns into a link.
func Links(f File) []Link {
	var links []Link
	fenced := false
	for i, raw := range strings.Split(string(f.Bytes), "\n") {
		line := i + 1
		if isFence(raw) {
			fenced = !fenced
			continue
		}
		if fenced || indented(raw) {
			continue
		}
		prose := outsideSpans(raw)
		for _, url := range append(linkTargets(prose), autolinks(prose)...) {
			if !external(url) {
				continue
			}
			links = append(links, Link{File: f.Name, Line: line, URL: url})
		}
	}
	return links
}

// external reports whether a target points off this machine. Only the two
// schemes a document links with are read: a mailto address has no request to
// make of it, and a scheme this does not name is not something to guess at.
func external(target string) bool {
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
}

func isFence(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

// indented reports a line held inside an indented code block. Four spaces is
// what markdown reads as one, and it is how this repository's own documents
// write a command.
func indented(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

// outsideSpans returns the parts of a line that are not inside an inline code
// span, joined in order. Backtick runs delimit a span and the run lengths have
// to match, which is what lets a span hold a backtick of its own.
func outsideSpans(line string) string {
	out := make([]byte, 0, len(line))
	rest := line
	for {
		open := strings.IndexByte(rest, '`')
		if open < 0 {
			return string(append(out, rest...))
		}
		out = append(out, rest[:open]...)
		rest = rest[open:]
		run := 0
		for run < len(rest) && rest[run] == '`' {
			run++
		}
		marker := rest[:run]
		body := rest[run:]
		end := strings.Index(body, marker)
		if end < 0 {
			// An unclosed run is a stray backtick rather than a span, so the
			// rest of the line stays prose and nothing is swallowed.
			return string(append(out, rest...))
		}
		rest = body[end+run:]
	}
}

// linkTargets returns the target of every inline link on a line.
func linkTargets(line string) []string {
	var targets []string
	rest := line
	for {
		open := strings.Index(rest, "](")
		if open < 0 {
			return targets
		}
		body := rest[open+2:]
		end := strings.IndexByte(body, ')')
		if end < 0 {
			return targets
		}
		target := strings.TrimSpace(body[:end])
		// A title after the target is part of the link and not part of the
		// address: [text](url "title").
		if i := strings.IndexAny(target, " \t"); i >= 0 {
			target = target[:i]
		}
		targets = append(targets, target)
		rest = body[end+1:]
	}
}

// autolinks returns every address written between angle brackets, which is the
// shape a markdown reader links without any text beside it.
func autolinks(line string) []string {
	var targets []string
	rest := line
	for {
		open := strings.IndexByte(rest, '<')
		if open < 0 {
			return targets
		}
		body := rest[open+1:]
		end := strings.IndexByte(body, '>')
		if end < 0 {
			return targets
		}
		target := strings.TrimSpace(body[:end])
		if !strings.ContainsAny(target, " \t") {
			targets = append(targets, target)
		}
		rest = body[end+1:]
	}
}
