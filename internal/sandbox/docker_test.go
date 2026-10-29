package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func containsRun(args, run []string) bool {
	for i := 0; i+len(run) <= len(args); i++ {
		if slices.Equal(args[i:i+len(run)], run) {
			return true
		}
	}
	return false
}

func TestDockerArgsIsolateTheCommand(t *testing.T) {
	d := Docker{CacheDir: "/var/cache/upkeep", UID: 1000, GID: 1001}
	args, err := d.args(Spec{Dir: "/srv/work/pr-5", Args: []string{"go", "build", "./..."}, Env: []string{"EXTRA=1"}}, "upkeep-abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]string{
		{"run", "--rm", "--init", "--name", "upkeep-abc"},
		{"--user", "1000:1001"},
		{"--cpus", "1"}, {"--memory", "768m"}, {"--memory-swap", "768m"}, {"--pids-limit", "512"},
		{"--cap-drop", "ALL"}, {"--security-opt", "no-new-privileges"}, {"--read-only"},
		{"--network", "none"},
		{"-v", "/srv/work/pr-5:/work:rw"},
		{"-v", "/var/cache/upkeep:/cache:rw"},
		{"-w", "/work"},
		{"-e", "GOFLAGS=-buildvcs=false -modcacherw"}, {"-e", "GOTOOLCHAIN=local"}, {"-e", "GOPATH=/cache/gopath"}, {"-e", "GOMODCACHE=/cache/gopath/pkg/mod"}, {"-e", "GOCACHE=/cache/build"},
		{"-e", "HOME=/tmp"}, {"-e", "EXTRA=1"},
	} {
		if !containsRun(args, want) {
			t.Errorf("arguments do not contain %v:\n%v", want, args)
		}
	}
	if containsRun(args, []string{"--network", "bridge"}) || containsRun(args, []string{"--privileged"}) {
		t.Errorf("a command without network must not get one: %v", args)
	}
	tail := args[len(args)-4:]
	if !slices.Equal(tail, []string{DefaultImage, "go", "build", "./..."}) {
		t.Errorf("the image and command must come last: %v", tail)
	}
}

func TestDockerArgsNetworkAndDefaults(t *testing.T) {
	d := Docker{CacheDir: "/c", Image: "golang:custom", CPUs: "2", Memory: "2g", Pids: 100, UID: 7, GID: 7}
	args, err := d.args(Spec{Dir: "/w", Args: []string{"go", "mod", "download"}, Network: true}, "n")
	if err != nil {
		t.Fatal(err)
	}
	if !containsRun(args, []string{"--network", "bridge"}) || containsRun(args, []string{"--network", "none"}) {
		t.Errorf("network was requested: %v", args)
	}
	for _, want := range [][]string{{"--cpus", "2"}, {"--memory", "2g"}, {"--pids-limit", "100"}} {
		if !containsRun(args, want) {
			t.Errorf("missing %v", want)
		}
	}
	if args[len(args)-4] != "golang:custom" {
		t.Errorf("image = %q", args[len(args)-4])
	}
}

func TestDockerNeverRunsAsRoot(t *testing.T) {
	d := Docker{CacheDir: "/c", UID: 0, GID: 0}
	uid, gid := d.ids()
	if os.Getuid() == 0 {
		if uid == 0 || gid == 0 {
			t.Fatalf("running as root must not pass root to the container: %d:%d", uid, gid)
		}
		return
	}
	if uid != os.Getuid() || gid != os.Getgid() {
		t.Fatalf("default ids = %d:%d, want the current user", uid, gid)
	}
	root := Docker{UID: 0, GID: 5}
	if u, _ := root.ids(); u == 0 {
		t.Fatal("uid 0 must be replaced")
	}
}

func TestDockerArgsErrors(t *testing.T) {
	if _, err := (Docker{}).args(Spec{Dir: "/w"}, "n"); err == nil {
		t.Error("no command returned no error")
	}
	args, err := (Docker{CacheDir: "relative/cache"}).args(Spec{Dir: "rel", Args: []string{"x"}}, "n")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range args {
		if strings.Contains(a, ":") && strings.HasPrefix(a, "rel") {
			t.Errorf("a relative path reached docker: %q", a)
		}
	}
}

func TestDockerFailureDetection(t *testing.T) {
	if got := dockerFailure("docker: Error response from daemon: pull access denied\nSee 'docker run --help'."); !strings.Contains(got, "pull access denied") {
		t.Errorf("failure = %q", got)
	}
	if dockerFailure("some build output") != "" {
		t.Error("ordinary output was taken for a docker failure")
	}
}

func dockerOrSkip(t *testing.T) Docker {
	t.Helper()
	if os.Getenv("UPKEEP_DOCKER_TESTS") != "1" {
		t.Skip("set UPKEEP_DOCKER_TESTS=1 to run containers")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	d := Docker{CacheDir: t.TempDir()}
	if err := d.EnsureImage(t.Context()); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDockerRunsGoWithoutNetworkAndAsTheCaller(t *testing.T) {
	d := dockerOrSkip(t)
	dir := t.TempDir()
	res, err := d.Run(t.Context(), Spec{Dir: dir, Args: []string{"go", "version"}})
	if err != nil || !res.OK() || !strings.Contains(res.Output, "go1.26") {
		t.Fatalf("go version: %+v %v", res, err)
	}

	res, err = d.Run(t.Context(), Spec{Dir: dir, Args: []string{"sh", "-c", "echo made > /work/made.txt"}})
	if err != nil || !res.OK() {
		t.Fatalf("writing in /work: %+v %v", res, err)
	}
	info, err := os.Stat(filepath.Join(dir, "made.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if owner := info.Sys().(*syscall.Stat_t).Uid; int(owner) != os.Getuid() {
		t.Fatalf("the file is owned by uid %d, want %d", owner, os.Getuid())
	}

	res, _ = d.Run(t.Context(), Spec{Dir: dir, Args: []string{"sh", "-c", "getent hosts example.com"}, Timeout: 20 * time.Second})
	if res.OK() {
		t.Fatalf("name resolution worked without a network: %+v", res)
	}
	res, _ = d.Run(t.Context(), Spec{Dir: dir, Args: []string{"sh", "-c", "touch /usr/should-not-exist"}})
	if res.OK() {
		t.Fatal("the root file system is writable")
	}
	res, _ = d.Run(t.Context(), Spec{Dir: dir, Args: []string{"sh", "-c", "touch /tmp/ok && echo writable"}})
	if !res.OK() {
		t.Fatalf("/tmp is not writable: %+v", res)
	}
}

func TestDockerHidesTheHostEnvironment(t *testing.T) {
	d := dockerOrSkip(t)
	t.Setenv("NEBIUS_API_KEY", "host-secret-value")
	res, err := d.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"env"}})
	if err != nil || !res.OK() {
		t.Fatalf("env: %+v %v", res, err)
	}
	if strings.Contains(res.Output, "host-secret-value") || strings.Contains(res.Output, "NEBIUS") {
		t.Fatal("the host environment reached the container")
	}
}

func TestDockerTimeoutKillsTheContainer(t *testing.T) {
	d := dockerOrSkip(t)
	res, err := d.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"sleep", "60"}, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.OK() {
		t.Fatalf("result = %+v, want a timeout", res)
	}
	time.Sleep(500 * time.Millisecond)
	out, err := exec.CommandContext(t.Context(), "docker", "ps", "-q", "--filter", "name=upkeep-").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("a container is still running after the timeout: %s", out)
	}
}

func TestDockerBoundsTheOutput(t *testing.T) {
	d := dockerOrSkip(t)
	res, err := d.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"sh", "-c", "yes upkeep | head -c 600000"}})
	if err != nil || !res.OK() {
		t.Fatalf("result = %+v %v", res, err)
	}
	if !res.Truncated || len(res.Output) > headLimit+tailLimit+len(truncationNote) {
		t.Fatalf("truncated %v, %d bytes kept", res.Truncated, len(res.Output))
	}
}

func TestDockerWithAMissingImage(t *testing.T) {
	d := dockerOrSkip(t)
	d.Image = "upkeep-image-that-does-not-exist:0"
	if _, err := d.Run(t.Context(), Spec{Dir: t.TempDir(), Args: []string{"true"}, Timeout: 60 * time.Second}); err == nil {
		t.Fatal("a missing image returned no error")
	}
}

func TestDockerRunnerDownloadsWithNetworkThenWorksWithout(t *testing.T) {
	d := dockerOrSkip(t)
	dir := module(t, map[string]string{
		"go.mod":    "module example.test/m\n\ngo 1.25\n\nrequire github.com/golang-jwt/jwt/v5 v5.3.1\n",
		"m.go":      "package m\n\nimport \"github.com/golang-jwt/jwt/v5\"\n\nfunc Method() string { return jwt.SigningMethodHS256.Alg() }\n",
		"m_test.go": "package m\n\nimport \"testing\"\n\nfunc TestMethod(t *testing.T) {\n\tif Method() != \"HS256\" {\n\t\tt.Fatal(Method())\n\t}\n}\n",
	})
	runner := GoRunner{Executor: d}

	before, err := runner.Check(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Passed() || !strings.Contains(before.Build.Output, "golang-jwt") {
		t.Fatalf("a module whose dependency is not downloaded must not build without a network: %+v", before.Build)
	}

	tidy, err := d.Run(t.Context(), Spec{Dir: dir, Args: []string{"go", "mod", "tidy"}, Network: true, Timeout: 90 * time.Second})
	if err != nil || !tidy.OK() {
		t.Fatalf("tidy = %+v %v", tidy, err)
	}
	download, err := runner.Download(t.Context(), dir)
	if err != nil || !download.OK() {
		t.Fatalf("download = %+v %v", download, err)
	}
	after, err := runner.Check(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Passed() {
		t.Fatalf("after the download the module should build offline: %+v", after)
	}
	t.Logf("cold cache: download %v, build %v, vet %v, test %v", download.Duration.Round(time.Millisecond), after.Build.Duration.Round(time.Millisecond), after.Vet.Duration.Round(time.Millisecond), after.Test.Duration.Round(time.Millisecond))
	warm, err := runner.Check(t.Context(), dir)
	if err != nil || !warm.Passed() {
		t.Fatalf("second check: %+v %v", warm, err)
	}
	t.Logf("warm cache: build %v, vet %v, test %v", warm.Build.Duration.Round(time.Millisecond), warm.Vet.Duration.Round(time.Millisecond), warm.Test.Duration.Round(time.Millisecond))
}

func TestDockerClientNoticesAreNotCommandOutput(t *testing.T) {
	notice := "WARNING: Error loading config file: open /home/upkeep/.docker/config.json: permission denied\n"
	got := withoutClientNotices(notice + "# pkg\nfile.go:1:1: undefined: x\n")
	if got != "# pkg\nfile.go:1:1: undefined: x\n" {
		t.Fatalf("the client notice stayed in the output: %q", got)
	}
	plain := "ok\nWARNING: this one is the command's own\n"
	if withoutClientNotices(plain) != plain {
		t.Fatalf("output without the notice changed: %q", withoutClientNotices(plain))
	}
}
