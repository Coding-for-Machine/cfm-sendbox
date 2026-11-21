// Package api implements the Piston-compatible v2 REST and WebSocket API on Fiber.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/globals"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/job"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/pkgman"
	"github.com/Coding-for-Machine/cfm-sendbox/internal/runtime"
)

// Server wires the HTTP handlers to the core components.
type Server struct {
	reg    *runtime.Registry
	pm     *pkgman.Manager
	runner *job.Runner
}

// New creates the API server.
func New(reg *runtime.Registry, pm *pkgman.Manager, runner *job.Runner) *Server {
	return &Server{reg: reg, pm: pm, runner: runner}
}

// Register mounts the /api/v2 routes on app.
func (s *Server) Register(app *fiber.App) {
	api := app.Group("/api/v2", requireJSON)

	api.Use("/connect", func(c fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	api.Get("/connect", websocket.New(s.connect))

	api.Post("/execute", s.execute)
	api.Get("/runtimes", s.runtimes)
	api.Get("/packages", s.listPackages)
	api.Post("/packages", s.installPackage)
	api.Delete("/packages", s.uninstallPackage)
}

// requireJSON rejects state-changing requests that are not JSON.
func requireJSON(c fiber.Ctx) error {
	switch c.Method() {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return c.Next()
	}
	if !strings.HasPrefix(c.Get(fiber.HeaderContentType), "application/json") {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"message": "requests must be of type application/json",
		})
	}
	return c.Next()
}

// requestError is a problem with the client's request (HTTP 400).
type requestError string

func (e requestError) Error() string { return string(e) }

func badRequest(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": msg})
}

func parseBody(raw []byte) (map[string]any, error) {
	body := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return body, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, requestError("invalid JSON body: " + err.Error())
	}
	switch t := v.(type) {
	case nil:
		return body, nil
	case map[string]any:
		return t, nil
	}
	return nil, requestError("request body must be a JSON object")
}

// isFalsy mirrors JavaScript truthiness for decoded JSON values.
func isFalsy(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case bool:
		return !t
	case float64:
		return t == 0
	case string:
		return t == ""
	}
	return false
}

// newJob validates an execute request (also used by the WebSocket "init"
// message) and builds the job.
func (s *Server) newJob(body map[string]any) (*job.Job, error) {
	language, ok := body["language"].(string)
	if !ok || language == "" {
		return nil, requestError("language is required as a string")
	}
	version, ok := body["version"].(string)
	if !ok || version == "" {
		return nil, requestError("version is required as a string")
	}
	rawFiles, ok := body["files"].([]any)
	if !ok {
		return nil, requestError("files is required as an array")
	}

	files := make([]job.File, len(rawFiles))
	for i, rf := range rawFiles {
		obj, _ := rf.(map[string]any)
		content, ok := obj["content"].(string)
		if !ok {
			return nil, requestError(fmt.Sprintf("files[%d].content is required as a string", i))
		}
		f := job.File{Content: content, Encoding: "utf8"}
		if name, ok := obj["name"].(string); ok {
			f.Name = name
		}
		if enc, ok := obj["encoding"].(string); ok {
			switch enc {
			case "base64", "hex", "utf8":
				f.Encoding = enc
			}
		}
		if f.Name == "" {
			f.Name = fmt.Sprintf("file%d.code", i)
		}
		if err := f.Validate(); err != nil {
			return nil, requestError(fmt.Sprintf("files[%d]: %v", i, err))
		}
		files[i] = f
	}

	rt := s.reg.LatestMatchingLanguageVersion(language, version)
	if rt == nil {
		return nil, requestError(fmt.Sprintf("%s-%s runtime is unknown", language, version))
	}

	hasUTF8 := false
	for _, f := range files {
		if f.Encoding == "utf8" {
			hasUTF8 = true
			break
		}
	}
	if rt.Language == "file" {
		if len(files) == 0 {
			return nil, requestError("files must include at least one file")
		}
	} else if !hasUTF8 {
		return nil, requestError("files must include at least one utf8 encoded file")
	}

	// Per-request limits: they may lower, never raise, the configured ones.
	var memoryLimits, timeouts, cpuTimes runtime.Stage
	limitSpecs := []struct {
		constraint string
		configured runtime.Stage
		out        *runtime.Stage
	}{
		{"memory_limit", rt.MemoryLimits, &memoryLimits},
		{"timeout", rt.Timeouts, &timeouts},
		{"cpu_time", rt.CPUTimes, &cpuTimes},
	}
	for _, spec := range limitSpecs {
		for _, stage := range []string{"compile", "run"} {
			configured := spec.configured.Run
			dst := &spec.out.Run
			if stage == "compile" {
				configured = spec.configured.Compile
				dst = &spec.out.Compile
			}
			v, err := pickLimit(body, stage+"_"+spec.constraint, configured)
			if err != nil {
				return nil, err
			}
			*dst = v
		}
	}

	args := []string{}
	if raw, present := body["args"]; present && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, requestError("args must be an array of strings")
		}
		for _, a := range list {
			str, ok := a.(string)
			if !ok {
				return nil, requestError("args must be an array of strings")
			}
			args = append(args, str)
		}
	}

	stdin := ""
	if raw, present := body["stdin"]; present && raw != nil {
		str, ok := raw.(string)
		if !ok {
			return nil, requestError("stdin must be a string")
		}
		stdin = str
	}

	return s.runner.New(job.Params{
		Runtime:      rt,
		Files:        files,
		Args:         args,
		Stdin:        stdin,
		Timeouts:     timeouts,
		CPUTimes:     cpuTimes,
		MemoryLimits: memoryLimits,
	}), nil
}

// pickLimit returns the limit for one stage: the requested value when valid,
// otherwise the configured default.
func pickLimit(body map[string]any, name string, configured int) (int, error) {
	raw, present := body[name]
	if !present || isFalsy(raw) {
		return configured, nil
	}
	num, ok := raw.(float64)
	if !ok {
		return 0, requestError(fmt.Sprintf("If specified, %s must be a number", name))
	}
	if configured > 0 {
		if num > float64(configured) {
			return 0, requestError(fmt.Sprintf("%s cannot exceed the configured limit of %d", name, configured))
		}
		if num < 0 {
			return 0, requestError(fmt.Sprintf("%s must be non-negative", name))
		}
	}
	return int(num), nil
}

func (s *Server) execute(c fiber.Ctx) error {
	body, err := parseBody(c.Body())
	if err != nil {
		return badRequest(c, err.Error())
	}
	j, err := s.newJob(body)
	if err != nil {
		return badRequest(c, err.Error())
	}
	defer j.Cleanup()

	box, err := j.Prime()
	if err != nil {
		slog.Error("error priming job", "job", j.UUID, "error", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	result, err := j.Execute(box, nil)
	if err != nil {
		slog.Error("error executing job", "job", j.UUID, "error", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	// Backward compatibility when the run stage is not started.
	if result.Run == nil {
		result.Run = result.Compile
	}
	return c.Status(fiber.StatusOK).JSON(result)
}

type runtimeInfo struct {
	Language string   `json:"language"`
	Version  string   `json:"version"`
	Aliases  []string `json:"aliases"`
	Runtime  string   `json:"runtime,omitempty"`
}

func (s *Server) runtimes(c fiber.Ctx) error {
	all := s.reg.All()
	out := make([]runtimeInfo, 0, len(all))
	for _, rt := range all {
		out = append(out, runtimeInfo{
			Language: rt.Language,
			Version:  rt.Version.Original(),
			Aliases:  rt.Aliases,
			Runtime:  rt.Runtime,
		})
	}
	return c.Status(fiber.StatusOK).JSON(out)
}

type packageInfo struct {
	Language        string `json:"language"`
	LanguageVersion string `json:"language_version"`
	Installed       bool   `json:"installed"`
}

func (s *Server) listPackages(c fiber.Ctx) error {
	slog.Debug("request to list packages")
	pkgs, err := s.pm.List()
	if err != nil {
		slog.Error("error listing packages", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	out := make([]packageInfo, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, packageInfo{
			Language:        p.Language,
			LanguageVersion: p.Version.Original(),
			Installed:       p.Installed(),
		})
	}
	return c.Status(fiber.StatusOK).JSON(out)
}

// findPackage handles the shared parsing/lookup of install and uninstall.
// It returns a nil package (and a nil error) once it has written a response.
func (s *Server) findPackage(c fiber.Ctx) (*pkgman.Package, error) {
	body, err := parseBody(c.Body())
	if err != nil {
		return nil, badRequest(c, err.Error())
	}
	language, _ := body["language"].(string)
	version, _ := body["version"].(string)
	if language == "" || version == "" {
		return nil, badRequest(c, "language and version are required as strings")
	}

	pkg, err := s.pm.Get(language, version)
	if err != nil {
		slog.Error("error looking up package", "error", err)
		return nil, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	if pkg == nil {
		return nil, c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": fmt.Sprintf("Requested package %s-%s does not exist", language, version),
		})
	}
	return pkg, nil
}

func (s *Server) installPackage(c fiber.Ctx) error {
	slog.Debug("request to install package")
	pkg, err := s.findPackage(c)
	if pkg == nil {
		return err
	}
	res, err := pkg.Install()
	if err != nil {
		slog.Error("error installing package", "package", pkg.Language+"-"+pkg.Version.Original(), "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func (s *Server) uninstallPackage(c fiber.Ctx) error {
	slog.Debug("request to uninstall package")
	pkg, err := s.findPackage(c)
	if pkg == nil {
		return err
	}
	res, err := pkg.Uninstall()
	if err != nil {
		slog.Error("error uninstalling package", "package", pkg.Language+"-"+pkg.Version.Original(), "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// connect serves the interactive execution WebSocket (/api/v2/connect).
//
// Client -> server: init, data (stdin only), signal.
// Server -> client: runtime, stage, data, exit, error.
func (s *Server) connect(c *websocket.Conn) {
	c.SetReadLimit(1 << 20)

	var (
		writeMu sync.Mutex // gorilla-style connections allow a single writer
		stateMu sync.Mutex
		started bool
		current *job.Job
	)

	send := func(v any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = c.WriteJSON(v)
	}
	closeWith := func(code int, text string) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = c.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
		_ = c.Close()
	}
	fail := func(msg string) {
		// The close reason is limited to 123 bytes, so notify first, then close.
		send(fiber.Map{"type": "error", "message": msg})
		closeWith(4002, "Notified Error")
	}

	bus := &job.Bus{
		OnStdout: func(d string) { send(fiber.Map{"type": "data", "stream": "stdout", "data": d}) },
		OnStderr: func(d string) { send(fiber.Map{"type": "data", "stream": "stderr", "data": d}) },
		OnStage:  func(st string) { send(fiber.Map{"type": "stage", "stage": st}) },
		OnExit: func(stage string, code *int, signal *string) {
			send(fiber.Map{"type": "exit", "stage": stage, "code": code, "signal": signal})
		},
	}

	// Terminate the socket after one second if it was not initialised.
	timer := time.AfterFunc(time.Second, func() {
		stateMu.Lock()
		ok := started
		stateMu.Unlock()
		if !ok {
			closeWith(4001, "Initialization Timeout")
		}
	})
	defer timer.Stop()

	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			bus.Kill() // client went away: stop whatever is running
			return
		}

		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			fail(err.Error())
			return
		}

		switch msgType, _ := msg["type"].(string); msgType {
		case "init":
			stateMu.Lock()
			already := started
			started = true
			stateMu.Unlock()
			if already {
				closeWith(4000, "Already Initialized")
				continue
			}

			j, err := s.newJob(msg)
			if err != nil {
				fail(err.Error())
				return
			}
			stateMu.Lock()
			current = j
			stateMu.Unlock()

			go s.runInteractive(j, bus, send, fail, closeWith)

		case "data":
			stateMu.Lock()
			j := current
			stateMu.Unlock()
			if j == nil {
				closeWith(4003, "Not yet initialized")
				continue
			}
			if stream, _ := msg["stream"].(string); stream == "stdin" {
				d, _ := msg["data"].(string)
				_ = bus.WriteStdin(d)
			} else {
				closeWith(4004, "Can only write to stdin")
			}

		case "signal":
			stateMu.Lock()
			j := current
			stateMu.Unlock()
			if j == nil {
				closeWith(4003, "Not yet initialized")
				continue
			}
			name, _ := msg["signal"].(string)
			if sig, ok := globals.SignalByName(name); ok {
				_ = bus.Signal(sig)
			} else {
				closeWith(4005, "Invalid signal")
			}
		}
	}
}

func (s *Server) runInteractive(j *job.Job, bus *job.Bus,
	send func(any), fail func(string), closeWith func(int, string)) {

	err := func() error {
		defer j.Cleanup()

		box, err := j.Prime()
		if err != nil {
			return err
		}
		send(fiber.Map{
			"type":     "runtime",
			"language": j.Runtime().Language,
			"version":  j.Runtime().Version.Original(),
		})
		_, err = j.Execute(box, bus)
		return err
	}()
	if err != nil {
		slog.Error("error running interactive job", "job", j.UUID, "error", err)
		fail(err.Error())
		return
	}
	closeWith(4999, "Job Completed")
}

// ErrorHandler renders every error as {"message": ...} JSON.
func ErrorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "Internal Server Error"
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code, msg = fe.Code, fe.Message
	}
	return c.Status(code).JSON(fiber.Map{"message": msg})
}
