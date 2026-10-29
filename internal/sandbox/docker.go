package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const clientNotice = "WARNING: Error loading config file:"

const (
	DefaultImage  = "golang:1.26.10-bookworm"
	defaultCPUs   = "1"
	defaultMemory = "768m"
	defaultPids   = 512
	workDir       = "/work"
	cacheDir      = "/cache"
)

type Docker struct {
	Bin      string
	Image    string
	CacheDir string
	CPUs     string
	Memory   string
	Pids     int
	UID      int
	GID      int
	Runtime  string
}

func (d Docker) bin() string {
	if d.Bin != "" {
		return d.Bin
	}
	return "docker"
}

func (d Docker) image() string {
	if d.Image != "" {
		return d.Image
	}
	return DefaultImage
}

func (d Docker) ids() (int, int) {
	uid, gid := d.UID, d.GID
	if uid == 0 && gid == 0 {
		uid, gid = os.Getuid(), os.Getgid()
	}
	if uid == 0 {
		uid, gid = 65532, 65532
	}
	return uid, gid
}

func (d Docker) args(spec Spec, name string) ([]string, error) {
	if len(spec.Args) == 0 {
		return nil, errors.New("sandbox: no command")
	}
	dir, err := filepath.Abs(spec.Dir)
	if err != nil {
		return nil, err
	}
	cache, err := filepath.Abs(d.cacheDir())
	if err != nil {
		return nil, err
	}
	cpus, memory, pids := d.CPUs, d.Memory, d.Pids
	if cpus == "" {
		cpus = defaultCPUs
	}
	if memory == "" {
		memory = defaultMemory
	}
	if pids <= 0 {
		pids = defaultPids
	}
	uid, gid := d.ids()
	args := []string{
		"run", "--rm", "--init", "--name", name,
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--cpus", cpus, "--memory", memory, "--memory-swap", memory, "--pids-limit", strconv.Itoa(pids),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--read-only", "--tmpfs", "/tmp:rw,exec,nosuid,size=512m",
	}
	if spec.Network {
		args = append(args, "--network", "bridge")
	} else {
		args = append(args, "--network", "none")
	}
	args = append(args,
		"-v", dir+":"+workDir+":rw",
		"-v", cache+":"+cacheDir+":rw",
		"-w", workDir,
		"-e", "HOME=/tmp",
		"-e", "GOPATH="+cacheDir+"/gopath",
		"-e", "GOMODCACHE="+cacheDir+"/gopath/pkg/mod",
		"-e", "GOCACHE="+cacheDir+"/build",
		"-e", "GOFLAGS=-buildvcs=false -modcacherw",
		"-e", "GOTOOLCHAIN=local",
		"-e", "GOTELEMETRY=off",
		"-e", "CGO_ENABLED=0",
	)
	for _, kv := range spec.Env {
		args = append(args, "-e", kv)
	}
	if d.Runtime != "" {
		args = append(args, "--runtime", d.Runtime)
	}
	args = append(args, d.image())
	args = append(args, spec.Args...)
	return args, nil
}

func (d Docker) cacheDir() string {
	if d.CacheDir != "" {
		return d.CacheDir
	}
	return filepath.Join(os.TempDir(), "upkeep-cache")
}

func (d Docker) HostPath(p string) string {
	rel, ok := strings.CutPrefix(path.Clean(p), cacheDir)
	if !ok || (rel != "" && !strings.HasPrefix(rel, "/")) {
		return p
	}
	cache, err := filepath.Abs(d.cacheDir())
	if err != nil {
		return p
	}
	return filepath.Join(cache, filepath.FromSlash(rel))
}

func (d Docker) prepare() error {
	return os.MkdirAll(d.cacheDir(), 0o755)
}

func (d Docker) Run(ctx context.Context, spec Spec) (Result, error) {
	if err := d.prepare(); err != nil {
		return Result{}, err
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return Result{}, err
	}
	name := "upkeep-" + hex.EncodeToString(suffix)
	args, err := d.args(spec, name)
	if err != nil {
		return Result{}, err
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, d.bin(), args...)
	out := newBoundedBuffer()
	cmd.Stdout = out
	cmd.Stderr = out
	start := time.Now()
	runErr := cmd.Run()
	if runCtx.Err() != nil {
		d.remove(name)
	}
	res, err := finish(runCtx, spec, out, time.Since(start), runErr)
	res.Output = withoutClientNotices(res.Output)
	if err != nil {
		return res, fmt.Errorf("running %s: %w", d.bin(), err)
	}
	if res.ExitCode == 125 || res.ExitCode == 126 || res.ExitCode == 127 {
		if daemon := dockerFailure(res.Output); daemon != "" {
			return res, errors.New(daemon)
		}
	}
	return res, nil
}

func withoutClientNotices(output string) string {
	if !strings.Contains(output, clientNotice) {
		return output
	}
	lines := strings.Split(output, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, clientNotice) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func dockerFailure(output string) string {
	for _, marker := range []string{"Cannot connect to the Docker daemon", "docker: Error response from daemon", "Unable to find image", "permission denied while trying to connect"} {
		if strings.Contains(output, marker) {
			return "docker could not run the command: " + firstLine(output)
		}
	}
	return ""
}

func (d Docker) remove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, d.bin(), "rm", "-f", name).Run()
}

func (d Docker) EnsureImage(ctx context.Context) error {
	if err := exec.CommandContext(ctx, d.bin(), "image", "inspect", d.image()).Run(); err == nil {
		return nil
	}
	out, err := exec.CommandContext(ctx, d.bin(), "pull", "--quiet", d.image()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pulling %s: %w: %s", d.image(), err, firstLine(string(out)))
	}
	return nil
}
