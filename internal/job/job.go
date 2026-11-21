// Package job runs submissions inside isolate sandboxes.
package job

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/config"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/globals"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/runtime"
)

const (
	maxBoxID    = 999
	isolatePath = "/usr/local/bin/isolate"
)

// Runner owns the concurrency limit and box id allocation shared by all jobs.
type Runner struct {
	cfg    *config.Config
	sem    chan struct{}
	boxSeq atomic.Uint32
}

// NewRunner creates a Runner allowing cfg.MaxConcurrentJobs parallel jobs.
func NewRunner(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg, sem: make(chan struct{}, cfg.MaxConcurrentJobs)}
}

func (r *Runner) nextBoxID() int { return int(r.boxSeq.Add(1) % maxBoxID) }

// File is a source file of a submission.
type File struct {
	Name     string
	Content  string
	Encoding string // base64, hex or utf8
}

// DecodeContent returns the raw bytes of the file according to its encoding.
func (f File) DecodeContent() ([]byte, error) {
	switch f.Encoding {
	case "base64":
		return base64.StdEncoding.DecodeString(f.Content)
	case "hex":
		return hex.DecodeString(f.Content)
	default:
		return []byte(f.Content), nil
	}
}

// Validate checks that the file stays inside the submission directory and that
// its content decodes with the declared encoding.
func (f File) Validate() error {
	const base = "/submission"
	rel, err := filepath.Rel(base, filepath.Join(base, f.Name))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file path %q tries to escape parent directory: %s", f.Name, rel)
	}
	if _, err := f.DecodeContent(); err != nil {
		return fmt.Errorf("content is not valid %s: %v", f.Encoding, err)
	}
	return nil
}

// Params describes a job.
type Params struct {
	Runtime      *runtime.Runtime
	Files        []File
	Args         []string
	Stdin        string
	Timeouts     runtime.Stage
	CPUTimes     runtime.Stage
	MemoryLimits runtime.Stage
}

// Box is an isolate sandbox.
type Box struct {
	ID           int
	MetadataPath string
	Dir          string
}

type state int

const (
	stateReady state = iota
	statePrimed
	stateExecuted
)

func (s state) String() string {
	return [...]string{"ready", "primed", "executed"}[s]
}

// Job is one submission.
type Job struct {
	UUID string

	r            *Runner
	rt           *runtime.Runtime
	files        []File
	args         []string
	stdin        string
	timeouts     runtime.Stage
	cpuTimes     runtime.Stage
	memoryLimits runtime.Stage

	mu       sync.Mutex
	state    state
	acquired bool
	dirty    []*Box
}

// New creates a job in the ready state.
func (r *Runner) New(p Params) *Job {
	files := make([]File, len(p.Files))
	for i, f := range p.Files {
		if f.Name == "" {
			f.Name = fmt.Sprintf("file%d.code", i)
		}
		switch f.Encoding {
		case "base64", "hex", "utf8":
		default:
			f.Encoding = "utf8"
		}
		files[i] = f
	}

	stdin := p.Stdin
	if !strings.HasSuffix(stdin, "\n") {
		stdin += "\n"
	}

	return &Job{
		UUID:         newUUID(),
		r:            r,
		rt:           p.Runtime,
		files:        files,
		args:         p.Args,
		stdin:        stdin,
		timeouts:     p.Timeouts,
		cpuTimes:     p.CPUTimes,
		memoryLimits: p.MemoryLimits,
	}
}

// Runtime returns the runtime the job will use.
func (j *Job) Runtime() *runtime.Runtime { return j.rt }

func (j *Job) createBox() (*Box, error) {
	id := j.r.nextBoxID()
	cmd := exec.Command("isolate", "--init", "--cg", fmt.Sprintf("-b%d", id))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run isolate --init: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}
	dir := strings.TrimSpace(stdout.String())
	if dir == "" {
		return nil, errors.New("received empty stdout from isolate --init")
	}
	box := &Box{
		ID:           id,
		MetadataPath: fmt.Sprintf("/tmp/%d-metadata.txt", id),
		Dir:          filepath.Join(dir, "box"),
	}
	j.mu.Lock()
	j.dirty = append(j.dirty, box)
	j.mu.Unlock()
	return box, nil
}

// Prime waits for a free job slot, creates the sandbox and writes the files.
func (j *Job) Prime() (*Box, error) {
	slog.Debug("waiting for job slot", "job", j.UUID)
	j.r.sem <- struct{}{}
	j.mu.Lock()
	j.acquired = true
	j.mu.Unlock()

	slog.Info("priming job", "job", j.UUID)
	box, err := j.createBox()
	if err != nil {
		return nil, err
	}

	submission := filepath.Join(box.Dir, "submission")
	if err := os.Mkdir(submission, 0o755); err != nil {
		return nil, err
	}
	for _, f := range j.files {
		if err := f.Validate(); err != nil {
			return nil, fmt.Errorf("file %q: %w", f.Name, err)
		}
		path := filepath.Join(submission, f.Name)
		content, err := f.DecodeContent()
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", f.Name, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, err
		}
	}

	j.mu.Lock()
	j.state = statePrimed
	j.mu.Unlock()
	return box, nil
}

// StageResult is the outcome of the compile or run stage.
type StageResult struct {
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	Output   string   `json:"output"`
	Code     *int     `json:"code"`
	Signal   *string  `json:"signal"`
	Message  *string  `json:"message"`
	Status   *string  `json:"status"`
	CPUTime  *float64 `json:"cpu_time"`
	WallTime *float64 `json:"wall_time"`
	Memory   *int64   `json:"memory"`
}

// ExecuteResult is the outcome of a whole job.
type ExecuteResult struct {
	Compile  *StageResult `json:"compile,omitempty"`
	Run      *StageResult `json:"run,omitempty"`
	Language string       `json:"language"`
	Version  string       `json:"version"`
}

func seconds(ms int) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64)
}

func (j *Job) safeCall(box *Box, file string, args []string,
	timeout, cpuTime, memoryLimit int, bus *Bus) (*StageResult, error) {

	rt := j.rt
	envVars, err := rt.EnvVars()
	if err != nil {
		return nil, fmt.Errorf("read runtime environment: %w", err)
	}

	argv := []string{
		"--run",
		fmt.Sprintf("-b%d", box.ID),
		"--meta=" + box.MetadataPath,
		"--cg",
		"-s",
		"-c", "/box/submission",
		"-E", "HOME=/tmp",
	}
	for _, v := range envVars {
		argv = append(argv, "-E", v)
	}
	argv = append(argv,
		"-E", "PISTON_LANGUAGE="+rt.Language, // read by package scripts
		"-E", "CFM_LANGUAGE="+rt.Language,
		"--dir="+rt.PkgDir,
		"--dir=/etc:noexec",
		fmt.Sprintf("--processes=%d", rt.MaxProcessCount),
		fmt.Sprintf("--open-files=%d", rt.MaxOpenFiles),
		fmt.Sprintf("--fsize=%d", rt.MaxFileSize/1000),
		"--wall-time="+seconds(timeout),
		"--time="+seconds(cpuTime),
		"--extra-time=0",
	)
	if memoryLimit >= 0 {
		argv = append(argv, fmt.Sprintf("--cg-mem=%d", memoryLimit/1000))
	}
	if !j.r.cfg.DisableNetworking {
		argv = append(argv, "--share-net")
	}
	argv = append(argv, "--", "/bin/bash", filepath.Join(rt.PkgDir, file))
	argv = append(argv, args...)

	cmd := exec.Command(isolatePath, argv...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start isolate: %w", err)
	}

	if bus == nil {
		go func() {
			defer stdinPipe.Close()
			_, _ = io.WriteString(stdinPipe, j.stdin)
		}()
	} else {
		bus.attach(cmd.Process, stdinPipe)
		defer bus.detach()
	}

	var (
		mu                     sync.Mutex
		stdout, stderr, output strings.Builder
		message, status        *string
	)

	reader := func(r io.Reader, isStderr bool) {
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				switch {
				case bus != nil && isStderr:
					bus.stderr(chunk)
				case bus != nil:
					bus.stdout(chunk)
				default:
					dst, msg, st := &stdout, "stdout length exceeded", "OL"
					if isStderr {
						dst, msg, st = &stderr, "stderr length exceeded", "EL"
					}
					mu.Lock()
					if dst.Len()+len(chunk) > rt.OutputMaxSize {
						message, status = &msg, &st
						mu.Unlock()
						slog.Info(msg, "job", j.UUID)
						// Isolate may already be gone; that is fine.
						_ = cmd.Process.Signal(syscall.SIGABRT)
					} else {
						dst.WriteString(chunk)
						output.WriteString(chunk)
						mu.Unlock()
					}
				}
			}
			if err != nil {
				return
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); reader(stdoutPipe, false) }()
	go func() { defer wg.Done(); reader(stderrPipe, true) }()
	wg.Wait()
	_ = cmd.Wait() // isolate exits non-zero when the program fails; metadata tells the story

	res := &StageResult{}
	mu.Lock()
	res.Stdout, res.Stderr, res.Output = stdout.String(), stderr.String(), output.String()
	res.Message, res.Status = message, status
	mu.Unlock()

	meta, err := os.ReadFile(box.MetadataPath)
	if err != nil {
		return nil, fmt.Errorf("error reading metadata file: %s\nerror: %v\nisolate run stdout: %s\nisolate run stderr: %s",
			box.MetadataPath, err, res.Stdout, res.Stderr)
	}
	for _, line := range strings.Split(string(meta), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("failed to parse metadata file, received: %s", line)
		}
		key, value := parts[0], parts[1]
		switch key {
		case "cg-mem":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				m := n * 1000
				res.Memory = &m
			}
		case "exitcode":
			if n, err := strconv.Atoi(value); err == nil {
				res.Code = &n
			}
		case "exitsig":
			if n, err := strconv.Atoi(value); err == nil {
				if name, ok := globals.SignalName(n); ok {
					res.Signal = &name
				}
			}
		case "message":
			if res.Message == nil {
				v := value
				res.Message = &v
			}
		case "status":
			if res.Status == nil {
				v := value
				res.Status = &v
			}
		case "time":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				f *= 1000
				res.CPUTime = &f
			}
		case "time-wall":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				f *= 1000
				res.WallTime = &f
			}
		}
	}

	if res.Status != nil {
		switch *res.Status {
		case "TO", "OL", "EL":
			kill := "SIGKILL"
			res.Signal = &kill
		}
	}
	return res, nil
}

// Execute compiles (when needed) and runs the job.
func (j *Job) Execute(box *Box, bus *Bus) (*ExecuteResult, error) {
	j.mu.Lock()
	st := j.state
	j.mu.Unlock()
	if st != statePrimed {
		return nil, fmt.Errorf("job must be in primed state, current state: %s", st)
	}

	slog.Info("executing job", "job", j.UUID, "runtime", j.rt.String())

	var codeFiles []File
	for _, f := range j.files {
		if j.rt.Language == "file" || f.Encoding == "utf8" {
			codeFiles = append(codeFiles, f)
		}
	}
	if len(codeFiles) == 0 {
		return nil, errors.New("no code files to execute")
	}

	var compile, run *StageResult
	compileErrored := false

	if j.rt.Compiled() {
		slog.Debug("compiling", "job", j.UUID)
		bus.stage("compile")
		names := make([]string, len(codeFiles))
		for i, f := range codeFiles {
			names[i] = f.Name
		}
		var err error
		compile, err = j.safeCall(box, "compile", names,
			j.timeouts.Compile, j.cpuTimes.Compile, j.memoryLimits.Compile, bus)
		if err != nil {
			return nil, err
		}
		bus.exit("compile", compile)

		compileErrored = compile.Code == nil || *compile.Code != 0
		if !compileErrored {
			// Run in a fresh box so the compile stage cannot tamper with it.
			oldDir := box.Dir
			box, err = j.createBox()
			if err != nil {
				return nil, err
			}
			if err := os.Rename(filepath.Join(oldDir, "submission"),
				filepath.Join(box.Dir, "submission")); err != nil {
				return nil, err
			}
		}
	}

	if !compileErrored {
		slog.Debug("running", "job", j.UUID)
		bus.stage("run")
		var err error
		run, err = j.safeCall(box, "run",
			append([]string{codeFiles[0].Name}, j.args...),
			j.timeouts.Run, j.cpuTimes.Run, j.memoryLimits.Run, bus)
		if err != nil {
			return nil, err
		}
		bus.exit("run", run)
	}

	j.mu.Lock()
	j.state = stateExecuted
	j.mu.Unlock()

	return &ExecuteResult{
		Compile:  compile,
		Run:      run,
		Language: j.rt.Language,
		Version:  j.rt.Version.Original(),
	}, nil
}

// Cleanup releases the job slot and destroys every sandbox of the job.
func (j *Job) Cleanup() {
	slog.Info("cleaning up job", "job", j.UUID)

	j.mu.Lock()
	acquired := j.acquired
	j.acquired = false
	boxes := j.dirty
	j.dirty = nil
	j.mu.Unlock()

	if acquired {
		<-j.r.sem
	}
	for _, box := range boxes {
		cmd := exec.Command("isolate", "--cleanup", "--cg", fmt.Sprintf("-b%d", box.ID))
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Error("failed to run isolate --cleanup", "box", box.ID, "error", err,
				"output", strings.TrimSpace(string(out)))
		}
		if err := os.Remove(box.MetadataPath); err != nil && !os.IsNotExist(err) {
			slog.Error("failed to remove the metadata file", "box", box.ID, "error", err)
		}
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
