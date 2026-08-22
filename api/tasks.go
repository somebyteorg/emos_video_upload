package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	errTaskRunning = errors.New("task is running")
	errTaskDone    = errors.New("task is already complete")
	errTaskClosed  = errors.New("task manager is closed")
)

type TaskManager struct {
	server *Server
	ctx    context.Context
	cancel context.CancelFunc
	sem    chan struct{}
	mu     sync.Mutex
	active map[string]struct{}
	wg     sync.WaitGroup
	close  sync.Once
	closed bool
}

func NewTaskManager(server *Server) *TaskManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskManager{
		server: server,
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, server.cfg.TaskConcurrency),
		active: map[string]struct{}{},
	}
}

func (m *TaskManager) Resume(ctx context.Context) error {
	tasks, err := m.server.db.ListResumableTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := m.Start(task.ID); err != nil {
			return err
		}
	}
	return nil
}

func (m *TaskManager) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errTaskClosed
	}
	if _, exists := m.active[id]; exists {
		return nil
	}
	m.active[id] = struct{}{}
	m.wg.Add(1)
	go m.run(id)
	return nil
}

func (m *TaskManager) Retry(ctx context.Context, id string) (TaskRecord, error) {
	task, err := m.server.db.GetTask(ctx, id)
	if err != nil {
		return TaskRecord{}, err
	}
	switch task.Status {
	case "running":
		return TaskRecord{}, errTaskRunning
	case "success":
		return TaskRecord{}, errTaskDone
	case "queued":
	default:
		if err := m.server.db.ResetTaskForRetry(ctx, id); err != nil {
			return TaskRecord{}, err
		}
	}
	if err := m.Start(id); err != nil {
		return TaskRecord{}, err
	}
	return m.server.db.GetTask(ctx, id)
}

func (m *TaskManager) run(id string) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		delete(m.active, id)
		m.mu.Unlock()
	}()
	select {
	case m.sem <- struct{}{}:
	case <-m.ctx.Done():
		return
	}
	defer func() { <-m.sem }()

	task, err := m.server.db.GetTask(m.ctx, id)
	if err != nil {
		return
	}
	if err := m.update(id, func(current *TaskRecord) {
		current.Status = "running"
		if current.Stage == "" || current.Stage == "失败" || current.Stage == "等待重试" {
			current.Stage = "准备恢复"
		}
		current.Error = ""
	}); err != nil {
		return
	}

	runErr := m.runTask(task)
	if runErr != nil {
		_ = m.update(id, func(current *TaskRecord) {
			current.Status = "error"
			current.Stage = "失败"
			current.Error = runErr.Error()
			current.Progress = minFloat(current.Progress, 99)
		})
		return
	}
	_ = m.update(id, func(current *TaskRecord) {
		current.Status = "success"
		current.Stage = "已完成"
		current.Progress = 100
		current.UploadedBytes = current.TotalBytes
	})
}

func (m *TaskManager) runTask(task TaskRecord) error {
	switch task.Kind {
	case "video":
		return m.server.runVideoTask(m.ctx, task)
	case "sprite":
		return m.server.runSpriteTask(m.ctx, task)
	default:
		return fmt.Errorf("unsupported task kind %q", task.Kind)
	}
}

func (m *TaskManager) update(id string, mutate func(*TaskRecord)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, err := m.server.db.GetTask(m.ctx, id)
	if err != nil {
		return err
	}
	mutate(&task)
	task.UpdatedAt = time.Now().UTC()
	return m.server.db.UpdateTask(m.ctx, task)
}

func (m *TaskManager) UpdateProgress(id, stage string, progress float64, uploadedBytes, totalBytes int64) error {
	return m.update(id, func(task *TaskRecord) {
		task.Stage = stage
		task.Progress = clampFloat(progress, 0, 100)
		if uploadedBytes >= 0 {
			task.UploadedBytes = uploadedBytes
		}
		if totalBytes > 0 {
			task.TotalBytes = totalBytes
		}
	})
}

func (m *TaskManager) SetFileResult(id, fileID, mediaID string) error {
	return m.update(id, func(task *TaskRecord) {
		task.FileID = fileID
		task.MediaID = mediaID
	})
}

func (m *TaskManager) Close() {
	m.close.Do(func() {
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		m.cancel()
		m.wg.Wait()
	})
}

func taskMetadata(task TaskRecord) (map[string]any, error) {
	if task.ProbeMetadata == "" {
		return nil, errors.New("task has no ffprobe metadata")
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(task.ProbeMetadata), &metadata); err != nil {
		return nil, fmt.Errorf("decode task ffprobe metadata: %w", err)
	}
	return metadata, nil
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func clampFloat(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}
	if value > upper {
		return upper
	}
	return value
}
