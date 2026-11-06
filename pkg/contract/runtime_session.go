package contract

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type runtimeSessionKey struct{}

// A CLI invocation owns one lazy MDX worker. No document results are cached.
// Mermaid retains its process isolation because its parser has mutable globals.
type runtimeSession struct {
	mu        sync.Mutex
	path      string
	supported bool
	closed    bool
	failure   error
	command   *exec.Cmd
	input     io.WriteCloser
	output    io.ReadCloser
	reader    *bufio.Reader
	stderr    bytes.Buffer
}

func parserRuntimeContext(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	session := &runtimeSession{}
	stop := context.AfterFunc(ctx, session.close)
	return context.WithValue(ctx, runtimeSessionKey{}, session), func() {
		cancel()
		stop()
		session.close()
	}
}

func supportsMDXStream(script string) bool {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(script), "package.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Version int `json:"marklintMdxStream"`
	}
	return json.Unmarshal(data, &manifest) == nil && manifest.Version == 1
}

func (s *runtimeSession) start(script string) error {
	s.stderr.Reset()
	s.command = exec.Command("node", script, "--stream-mdx")
	var err error
	if s.input, err = s.command.StdinPipe(); err != nil {
		return err
	}
	if s.output, err = s.command.StdoutPipe(); err != nil {
		return err
	}
	s.command.Stderr = &s.stderr
	if err := s.command.Start(); err != nil {
		return err
	}
	s.reader = bufio.NewReader(s.output)
	return nil
}

func (s *runtimeSession) request(ctx context.Context, script string, payload []byte) (parserResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return parserResult{}, true, fmt.Errorf("analysis runtime session is closed")
	}
	if err := ctx.Err(); err != nil {
		return parserResult{}, true, err
	}
	if s.path != script {
		s.stop()
		s.path, s.supported, s.failure = script, supportsMDXStream(script), nil
	}
	if !s.supported {
		return parserResult{}, false, nil
	}
	if s.failure != nil {
		return parserResult{}, true, s.failure
	}
	if s.command == nil {
		if err := s.start(script); err != nil {
			return s.fail(err)
		}
	}
	process, input := s.command.Process, s.input
	stopCancel := context.AfterFunc(ctx, func() {
		_ = process.Kill()
		_ = input.Close()
	})
	defer stopCancel()
	if _, err := s.input.Write(append(payload, '\n')); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return s.fail(err)
	}
	result, err := s.read(ctx)
	if err != nil {
		return s.fail(err)
	}
	return result, true, nil
}

func (s *runtimeSession) read(ctx context.Context) (parserResult, error) {
	reader := s.reader
	finished := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := reader.ReadBytes('\n')
		finished <- struct {
			data []byte
			err  error
		}{data, err}
	}()
	select {
	case <-ctx.Done():
		return parserResult{}, ctx.Err()
	case response := <-finished:
		if err := ctx.Err(); err != nil {
			return parserResult{}, err
		}
		if response.err != nil {
			return parserResult{}, response.err
		}
		var result parserResult
		if err := json.Unmarshal(response.data, &result); err != nil {
			return parserResult{}, fmt.Errorf("invalid parser result: %w", err)
		}
		return result, nil
	}
}

func (s *runtimeSession) fail(err error) (parserResult, bool, error) {
	s.stop()
	s.failure = fmt.Errorf("analysis runtime: %w: %s", err, s.stderr.String())
	return parserResult{}, true, s.failure
}

// Called with the session locked, after the final read or while aborting it.
func (s *runtimeSession) stop() {
	if s.input != nil {
		_ = s.input.Close()
	}
	if s.command != nil && s.command.Process != nil {
		_ = s.command.Process.Kill()
		_ = s.command.Wait()
	}
	if s.output != nil {
		_ = s.output.Close()
	}
	s.command, s.input, s.output, s.reader = nil, nil, nil, nil
}

func (s *runtimeSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
	s.closed = true
}
