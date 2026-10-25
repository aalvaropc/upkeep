package sandbox

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExecSuccess(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"sh", "-c", "pwd; echo done"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || !strings.Contains(res.Output, "done") {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecFailureKeepsOutputAndCode(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Args: []string{"sh", "-c", "echo broken >&2; exit 3"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || res.ExitCode != 3 || !strings.Contains(res.Output, "broken") {
		t.Fatalf("result = %+v", res)
	}
}

func TestExecTimeout(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Spec{Args: []string{"sleep", "5"}, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.OK() {
		t.Fatalf("result = %+v, want a timeout", res)
	}
}

func TestExecErrors(t *testing.T) {
	if _, err := (Exec{}).Run(t.Context(), Spec{}); err == nil {
		t.Error("Run with no command returned no error")
	}
	if _, err := (Exec{}).Run(t.Context(), Spec{Args: []string{"definitely-not-a-command-xyz"}}); err == nil {
		t.Error("Run of a missing binary returned no error")
	}
}

func TestExecEnvironment(t *testing.T) {
	t.Setenv("NEBIUS_API_KEY", "very-secret-value")
	t.Setenv("GITHUB_TOKEN", "another-secret")
	t.Setenv("UPKEEP_VISIBLE", "visible")
	res, err := Exec{}.Run(t.Context(), Spec{Args: []string{"sh", "-c", `echo "key=[$NEBIUS_API_KEY] token=[$GITHUB_TOKEN] visible=[$UPKEEP_VISIBLE] extra=[$EXTRA]"`}, Env: []string{"EXTRA=added"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "key=[] token=[] visible=[visible] extra=[added]"
	if strings.TrimSpace(res.Output) != want {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(res.Output), want)
	}
}

func TestCleanEnviron(t *testing.T) {
	in := []string{"PATH=/bin", "NEBIUS_API_KEY=x", "my_password=y", "DB_Secret=z", "CLOUD_CREDENTIALS=q", "HOME=/h", "KEYBOARD=us", "malformed"}
	got := CleanEnviron(in)
	want := []string{"PATH=/bin", "HOME=/h", "malformed"}
	if !slices.Equal(got, want) {
		t.Fatalf("CleanEnviron = %v, want %v", got, want)
	}
}

func TestBoundedBufferKeepsSmallOutput(t *testing.T) {
	b := newBoundedBuffer()
	_, _ = b.Write([]byte("hello "))
	_, _ = b.Write([]byte("world"))
	if b.String() != "hello world" || b.truncated() {
		t.Fatalf("buffer = %q truncated %v", b.String(), b.truncated())
	}
}

func TestBoundedBufferAtTheBoundary(t *testing.T) {
	b := newBoundedBuffer()
	_, _ = b.Write([]byte(strings.Repeat("a", headLimit)))
	_, _ = b.Write([]byte(strings.Repeat("b", tailLimit)))
	got := b.String()
	if b.truncated() || len(got) != headLimit+tailLimit || strings.Contains(got, "truncated") {
		t.Fatalf("exactly at the limit nothing is dropped: len %d truncated %v", len(got), b.truncated())
	}
	if !strings.HasPrefix(got, "aaa") || !strings.HasSuffix(got, "bbb") {
		t.Fatal("order was lost")
	}
}

func TestBoundedBufferTruncatesTheMiddle(t *testing.T) {
	b := newBoundedBuffer()
	_, _ = b.Write([]byte("HEAD-"))
	for i := 0; i < 300; i++ {
		_, _ = b.Write([]byte(strings.Repeat("x", 1000)))
	}
	_, _ = b.Write([]byte("-TAIL"))
	got := b.String()
	if !b.truncated() || !strings.HasPrefix(got, "HEAD-") || !strings.HasSuffix(got, "-TAIL") || !strings.Contains(got, "output truncated") {
		t.Fatalf("truncated %v, prefix %q, suffix %q", b.truncated(), got[:8], got[len(got)-8:])
	}
	if len(got) > headLimit+tailLimit+len(truncationNote) {
		t.Fatalf("kept %d bytes", len(got))
	}
}

func TestBoundedBufferTailIsTheLastBytes(t *testing.T) {
	b := newBoundedBuffer()
	_, _ = b.Write([]byte(strings.Repeat("h", headLimit)))
	var all strings.Builder
	for i := 0; i < 5000; i++ {
		line := strings.Repeat("0123456789", 3) + "\n"
		all.WriteString(line)
		_, _ = b.Write([]byte(line))
	}
	_, _ = b.Write([]byte("LAST-LINE\n"))
	all.WriteString("LAST-LINE\n")
	tail := b.tailBytes()
	if len(tail) != tailLimit || string(tail) != all.String()[all.Len()-tailLimit:] {
		t.Fatalf("tail has %d bytes and is not the end of the output", len(tail))
	}
}

func TestResultAndReport(t *testing.T) {
	ok := Result{}
	failed := Result{ExitCode: 1}
	if !ok.OK() || failed.OK() || (Result{TimedOut: true}).OK() {
		t.Fatal("OK() is wrong")
	}
	if (Result{Skipped: true}).OK() {
		t.Fatal("a skipped step must never count as passing")
	}
	if !(Report{Build: ok, Vet: ok, Test: ok}).Passed() {
		t.Error("a clean report did not pass")
	}
	if (Report{Build: ok, Vet: ok, Test: failed}).Passed() {
		t.Error("a report with failing tests passed")
	}
	if (Report{Build: ok, Vet: Result{TimedOut: true}, Test: ok}).Passed() {
		t.Error("a report with a timeout passed")
	}
	if (Report{Build: failed, Vet: skipped("go", "vet"), Test: skipped("go", "test")}).Passed() {
		t.Error("a report with skipped steps passed")
	}
	if first, found := (Report{Build: failed, Vet: skipped("v"), Test: skipped("t")}).FirstFailure(); !found || first.ExitCode != 1 {
		t.Errorf("FirstFailure = %+v %v", first, found)
	}
	if _, found := (Report{Build: ok, Vet: ok, Test: ok}).FirstFailure(); found {
		t.Error("a clean report has no failure")
	}
}
