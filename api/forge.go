package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var forgeProgressPattern = regexp.MustCompile(`sprites_generate\s+\|\s+(?:running|succeeded)\s+\|\s+([0-9]+(?:\.[0-9]+)?)%`)

func (s *Server) runSpriteTask(ctx context.Context, task TaskRecord) error {
	_, info, err := s.resolveVideoFile(task.Path)
	if err != nil {
		return err
	}
	if info.Size() != task.FileSize || info.ModTime().UnixNano() != task.FileMtimeNS {
		return errors.New("video file changed after ffprobe; analyze it again")
	}
	return s.runForge(ctx, task)
}

func (s *Server) runForge(ctx context.Context, task TaskRecord) error {
	if strings.TrimSpace(s.cfg.ForgePath) == "" {
		return errors.New("EMOS_FORGE_PATH is required for sprite generation")
	}
	outputRoot := filepath.Join(s.cfg.WorkingDir, "dbdata", "forge", task.ID)
	if err := os.MkdirAll(outputRoot, 0o700); err != nil {
		return fmt.Errorf("create forge output directory: %w", err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, forgeTimeout())
	defer cancel()
	command := exec.Command(s.cfg.ForgePath,
		"local",
		"--input", task.Path,
		"--uuid", task.ID,
		"--output", outputRoot,
		"--video=false",
		"--audio=false",
		"--subtitles=false",
		"--sprites=true",
		"--sprite-sizes", "1280x720,640x360,320x180",
		"--encrypt=false",
	)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Forge: %w", err)
	}
	processDone := make(chan struct{})
	go stopProcessGroupOnCancel(commandCtx, command.Process.Pid, processDone)

	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	var outputTail lineTail
	var errorTail lineTail
	go scanForgeOutput(stdout, &outputTail, func(line string) {
		s.updateForgeProgress(task.ID, line)
	}, stdoutDone)
	go scanForgeOutput(stderr, &errorTail, nil, stderrDone)
	waitErr := command.Wait()
	close(processDone)
	<-stdoutDone
	<-stderrDone
	if waitErr != nil {
		message := errorTail.String()
		if message == "" {
			message = outputTail.String()
		}
		if message == "" {
			message = waitErr.Error()
		}
		return fmt.Errorf("Forge failed: %s", message)
	}
	manifestPath := filepath.Join(outputRoot, task.ID, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		return fmt.Errorf("Forge completed without manifest.json: %w", err)
	}
	if err := validateSpriteManifest(manifestPath); err != nil {
		return err
	}
	return s.tasks.UpdateProgress(task.ID, "雪碧图已生成（等待上传接口）", 100, task.FileSize, task.FileSize)
}

func stopProcessGroupOnCancel(ctx context.Context, pid int, done <-chan struct{}) {
	select {
	case <-done:
		return
	case <-ctx.Done():
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

func (s *Server) updateForgeProgress(taskID, line string) {
	match := forgeProgressPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return
	}
	progress, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return
	}
	_ = s.tasks.UpdateProgress(taskID, "采集雪碧图", 10+progress*.85, -1, -1)
}

func validateSpriteManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Forge manifest: %w", err)
	}
	var manifest struct {
		Status  string `json:"status"`
		Sprites []struct {
			Images     []string  `json:"images"`
			FrameTimes []float64 `json:"frame_times"`
		} `json:"sprites"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode Forge manifest: %w", err)
	}
	if manifest.Status != "" && manifest.Status != "succeeded" {
		return fmt.Errorf("Forge manifest status is %q", manifest.Status)
	}
	if len(manifest.Sprites) == 0 {
		return errors.New("Forge manifest contains no sprites")
	}
	for _, sprite := range manifest.Sprites {
		if len(sprite.Images) == 0 || len(sprite.FrameTimes) == 0 {
			return errors.New("Forge manifest contains an incomplete sprite")
		}
	}
	return nil
}

func scanForgeOutput(reader io.Reader, tail *lineTail, onLine func(string), done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		tail.Add(line)
		if onLine != nil {
			onLine(line)
		}
	}
}

type lineTail struct {
	lines []string
}

func (t *lineTail) Add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > 30 {
		t.lines = t.lines[len(t.lines)-30:]
	}
}

func (t *lineTail) String() string {
	return strings.Join(t.lines, "\n")
}

func forgeTimeout() time.Duration {
	return 24 * time.Hour
}
