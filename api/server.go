package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (s *Server) probeCached(ctx context.Context, path string, info os.FileInfo) (ProbeResult, string, error) {
	raw, err := s.db.FindProbe(ctx, path, info.Size(), info.ModTime().UnixNano())
	if err == nil {
		var metadata map[string]any
		if json.Unmarshal([]byte(raw), &metadata) == nil {
			result := ProbeResult{Valid: true, Metadata: metadata, Summary: summarizeProbe(metadata)}
			if result.Summary.VideoStreams == 0 || result.Summary.Width <= 0 || result.Summary.Height <= 0 || result.Summary.Duration <= 0 {
				result.Valid = false
				result.Error = "cached probe result is invalid"
			}
			return result, raw, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ProbeResult{Valid: false, Error: "read probe cache failed"}, "", fmt.Errorf("read probe cache: %w", err)
	}
	result, raw, err := probeVideo(ctx, s.cfg.FFprobePath, path)
	if raw != "" {
		if saveErr := s.db.SaveProbe(ctx, path, info.Size(), info.ModTime().UnixNano(), raw); saveErr != nil {
			return result, raw, fmt.Errorf("save probe cache: %w", saveErr)
		}
	}
	return result, raw, err
}

type Server struct {
	cfg        Config
	db         *Database
	emos       *EMOSClient
	tasks      *TaskManager
	static     fs.FS
	httpServer *http.Server
	auth       *Auth
	httpClient *http.Client
	closeOnce  sync.Once
}

func NewServer(cfg Config, static fs.FS) (*Server, error) {
	if err := cfg.ValidateServer(); err != nil {
		return nil, err
	}
	db, err := OpenDatabase(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	emos, err := NewEMOSClient(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	server := &Server{
		cfg:        cfg,
		db:         db,
		emos:       emos,
		static:     static,
		auth:       NewAuth(cfg.Username, cfg.Password),
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
	server.tasks = NewTaskManager(server)
	server.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:           server.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return server, nil
}

func (s *Server) Serve(ctx context.Context) error {
	if err := s.db.ResetRunningTasks(ctx); err != nil {
		return err
	}
	if err := s.tasks.Resume(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	fmt.Printf("server started successfully: http://%s/\n", s.httpServer.Addr)

	serverErr := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		s.Close()
		return nil
	case err := <-serverErr:
		s.Close()
		return err
	}
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.tasks != nil {
			s.tasks.Close()
		}
		if s.db != nil {
			_ = s.db.Close()
		}
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/status", s.handleAuthStatus)
	mux.Handle("/api/", s.authMiddleware(http.HandlerFunc(s.handleAPI)))
	mux.HandleFunc("/", s.handleStatic)
	return mux
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/files/tree":
		s.handleFilesTree(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/files/scan":
		s.handleFilesScan(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/files/probe":
		s.handleFilesProbe(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/files/directory-metadata":
		s.handleDirectoryMetadata(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/files/directory-metadata":
		s.handleSaveDirectoryMetadata(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/video/tree":
		s.handleVideoTree(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/upload/video/base":
		s.handleVideoBase(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/upload/tasks":
		s.handleCreateTask(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/upload/tasks/batch":
		s.handleCreateBatchTasks(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/tasks":
		s.handleTaskList(w, r)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/tasks/completed":
		s.handleDeleteCompletedTasks(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/tasks/") && strings.HasSuffix(r.URL.Path, "/retry"):
		s.handleTaskRetry(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/tasks/"):
		s.handleTaskDelete(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/tasks/"):
		s.handleTaskStatus(w, r)
	default:
		writeError(w, http.StatusNotFound, "API endpoint not found")
	}
}

func (s *Server) handleDeleteCompletedTasks(w http.ResponseWriter, r *http.Request) {
	count, err := s.db.DeleteCompletedTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": count})
}

func (s *Server) handleDirectoryMetadata(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.db.GetDirectoryTODBID(r.Context(), path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"todb_id": 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todb_id": id})
}

func (s *Server) handleSaveDirectoryMetadata(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path   string `json:"path"`
		TODBID int64  `json:"todb_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	path, err := s.resolvePath(input.Path)
	if err != nil || input.TODBID <= 0 {
		writeError(w, http.StatusBadRequest, "path and positive todb_id are required")
		return
	}
	if err := s.db.SaveDirectoryTODBID(r.Context(), path, input.TODBID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"todb_id": input.TODBID})
}

func (s *Server) handleCreateBatchTasks(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tasks []struct {
			Path          string `json:"path"`
			ItemType      string `json:"item_type"`
			ItemID        int64  `json:"item_id"`
			SeasonNumber  int    `json:"season_number"`
			EpisodeNumber int    `json:"episode_number"`
			VideoTitle    string `json:"video_title"`
			FileStorage   string `json:"file_storage"`
		} `json:"tasks"`
	}
	if !decodeJSON(w, r, &input) || len(input.Tasks) == 0 || len(input.Tasks) > 200 {
		writeError(w, http.StatusBadRequest, "tasks must contain 1 to 200 items")
		return
	}
	result := make([]taskResponse, 0, len(input.Tasks))
	for _, item := range input.Tasks {
		path, info, err := s.resolveVideoFile(item.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if item.ItemType != "vl" && item.ItemType != "ve" || item.ItemID <= 0 || !s.cfg.SupportsFileStorage(item.FileStorage) {
			writeError(w, http.StatusBadRequest, "invalid batch task target")
			return
		}
		if item.ItemType == "ve" && (item.SeasonNumber <= 0 || item.EpisodeNumber <= 0) {
			writeError(w, http.StatusBadRequest, "season_number and episode_number must be positive for TV tasks")
			return
		}
		probe, metadata, err := s.probeCached(r.Context(), path, info)
		if err != nil || !probe.Valid {
			writeError(w, http.StatusUnprocessableEntity, probe.Error)
			return
		}
		now := time.Now().UTC()
		task := TaskRecord{ID: newID(), Kind: "video", Path: path, FileName: info.Name(), FileSize: info.Size(), FileMtimeNS: info.ModTime().UnixNano(), ItemType: item.ItemType, ItemID: item.ItemID, SeasonNumber: item.SeasonNumber, EpisodeNumber: item.EpisodeNumber, VideoTitle: strings.TrimSpace(item.VideoTitle), Storage: item.FileStorage, Status: "queued", Stage: "等待处理", TotalBytes: info.Size(), ProbeMetadata: metadata, CreatedAt: now, UpdatedAt: now}
		created, inserted, err := s.db.CreateTask(r.Context(), task)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if inserted {
			_ = s.tasks.Start(created.ID)
		}
		result = append(result, taskResponse{TaskID: created.ID})
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleFilesTree(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]DirectoryEntry, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(path, entry.Name())
		result = append(result, DirectoryEntry{
			Name:      entry.Name(),
			Path:      child,
			Kind:      "directory",
			FileCount: countVideoFiles(child),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "entries": result})
}

func (s *Server) handleFilesScan(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	path, err := s.resolvePath(input.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	files, err := scanVideoFiles(path, input.Recursive)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "files": files})
}

func (s *Server) handleFilesProbe(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	path, info, err := s.resolveVideoFile(input.Path)
	if err != nil {
		writeJSON(w, http.StatusOK, ProbeResult{Valid: false, Error: err.Error()})
		return
	}
	result, _, probeErr := s.probeCached(r.Context(), path, info)
	if probeErr != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleVideoTree(w http.ResponseWriter, r *http.Request) {
	result, err := s.emos.GetVideoTree(r.Context(), r.URL.Query().Get("type"), r.URL.Query().Get("title"), r.URL.Query().Get("todb_id"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleVideoBase(w http.ResponseWriter, r *http.Request) {
	itemID, err := parseInt64(r.URL.Query().Get("item_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "item_id must be an integer")
		return
	}
	itemType := strings.TrimSpace(r.URL.Query().Get("item_type"))
	if itemType != "vl" && itemType != "ve" {
		writeError(w, http.StatusBadRequest, "item_type must be vl or ve")
		return
	}
	result, err := s.emos.GetVideoBase(r.Context(), itemType, itemID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path            string `json:"path"`
		TaskType        string `json:"task_type"`
		ItemType        string `json:"item_type"`
		ItemID          int64  `json:"item_id"`
		SeasonNumber    int    `json:"season_number"`
		EpisodeNumber   int    `json:"episode_number"`
		VideoTitle      string `json:"video_title"`
		FileStorage     string `json:"file_storage"`
		GenerateSprites bool   `json:"generate_sprites"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.TaskType != "video" && input.TaskType != "sprite" {
		writeError(w, http.StatusBadRequest, "task_type must be video or sprite")
		return
	}
	if input.ItemType != "vl" && input.ItemType != "ve" {
		writeError(w, http.StatusBadRequest, "item_type must be vl or ve")
		return
	}
	if input.ItemID <= 0 {
		writeError(w, http.StatusBadRequest, "item_id must be positive")
		return
	}
	if input.ItemType == "ve" && (input.SeasonNumber <= 0 || input.EpisodeNumber <= 0) {
		writeError(w, http.StatusBadRequest, "season_number and episode_number must be positive for TV tasks")
		return
	}
	if !s.cfg.SupportsFileStorage(input.FileStorage) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("file_storage must be one of %s", strings.Join(s.cfg.EMOSFileStorage, ", ")))
		return
	}
	path, info, err := s.resolveVideoFile(input.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	probe, metadata, err := s.probeCached(r.Context(), path, info)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, probe.Error)
		return
	}
	metadataJSON := metadata
	now := time.Now().UTC()
	task := TaskRecord{
		ID:              newID(),
		Kind:            input.TaskType,
		Path:            path,
		FileName:        info.Name(),
		FileSize:        info.Size(),
		FileMtimeNS:     info.ModTime().UnixNano(),
		ItemType:        input.ItemType,
		ItemID:          input.ItemID,
		SeasonNumber:    input.SeasonNumber,
		EpisodeNumber:   input.EpisodeNumber,
		VideoTitle:      strings.TrimSpace(input.VideoTitle),
		Storage:         input.FileStorage,
		GenerateSprites: input.GenerateSprites,
		Status:          "queued",
		Stage:           "等待处理",
		TotalBytes:      info.Size(),
		ProbeMetadata:   metadataJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if !probe.Valid {
		writeError(w, http.StatusUnprocessableEntity, probe.Error)
		return
	}
	created, inserted, err := s.db.CreateTask(r.Context(), task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if inserted {
		if err := s.tasks.Start(created.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusAccepted, taskResponse{TaskID: created.ID})
}

func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		if parsed, err := parseInt64(value); err == nil {
			limit = int(parsed)
		}
	}
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	page := 1
	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		if parsed, err := parseInt64(value); err == nil && parsed > 0 {
			page = int(parsed)
		}
	}
	offset := (page - 1) * limit
	tasks, total, err := s.db.ListTasksPage(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]taskStatusResponse, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, taskStatus(task))
	}
	response := map[string]any{"tasks": result, "total": total, "page": page, "limit": limit}
	etag := taskListETag(response)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func taskListETag(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func (s *Server) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := s.db.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, taskStatus(task))
}

func (s *Server) handleTaskRetry(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/retry")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := s.tasks.Retry(r.Context(), id)
	if err != nil {
		if errors.Is(err, errTaskRunning) || errors.Is(err, errTaskDone) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, taskStatus(task))
}

func (s *Server) handleTaskDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := s.db.GetTask(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if task.Status == "running" {
		writeError(w, http.StatusConflict, "running task cannot be deleted")
		return
	}
	if err := s.db.DeleteTask(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func taskStatus(task TaskRecord) taskStatusResponse {
	videoTitle := task.VideoTitle
	if videoTitle == "" {
		videoTitle = fmt.Sprintf("%s %d", task.ItemType, task.ItemID)
	}
	return taskStatusResponse{
		TaskID:        task.ID,
		Kind:          task.Kind,
		Path:          task.Path,
		FileName:      task.FileName,
		ItemType:      task.ItemType,
		ItemID:        task.ItemID,
		SeasonNumber:  task.SeasonNumber,
		EpisodeNumber: task.EpisodeNumber,
		VideoTitle:    videoTitle,
		Storage:       task.Storage,
		Status:        task.Status,
		Stage:         task.Stage,
		Progress:      task.Progress,
		UploadedBytes: task.UploadedBytes,
		TotalBytes:    task.TotalBytes,
		Error:         task.Error,
		FileID:        task.FileID,
		MediaID:       task.MediaID,
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.static == nil {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(filepath.ToSlash(r.URL.Path), "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(s.static, path); err != nil {
		path = "index.html"
	}
	request := r.Clone(r.Context())
	if path == "index.html" {
		// FileServer redirects /index.html to ./; request / instead so the
		// embedded index is served without creating a redirect loop.
		request.URL.Path = "/"
	} else {
		request.URL.Path = "/" + path
	}
	http.FileServer(http.FS(s.static)).ServeHTTP(w, request)
}

func countVideoFiles(path string) int {
	count := 0
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir() || !isVideoExtension(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			count++
		}
	}
	return count
}

func scanVideoFiles(path string, recursive bool) ([]SourceFile, error) {
	files := make([]SourceFile, 0)
	appendFile := func(filePath string, entry os.DirEntry) error {
		if entry.IsDir() || !isVideoExtension(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			return nil
		}
		files = append(files, SourceFile{
			ID:         sourceFileID(filePath, info),
			Name:       info.Name(),
			Path:       filePath,
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	}

	if recursive {
		err := filepath.WalkDir(path, func(currentPath string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			return appendFile(currentPath, entry)
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := appendFile(filepath.Join(path, entry.Name()), entry); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(files, func(left, right int) bool {
		return strings.ToLower(files[left].Path) < strings.ToLower(files[right].Path)
	})
	return files, nil
}

func parseInt64(value string) (int64, error) {
	result, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || result <= 0 {
		return 0, errors.New("invalid integer")
	}
	return result, nil
}

func newID() string {
	var bytesValue [16]byte
	if _, err := rand.Read(bytesValue[:]); err != nil {
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", time.Now().UnixNano()%1_000_000_000_000)
	}
	bytesValue[6] = (bytesValue[6] & 0x0f) | 0x40
	bytesValue[8] = (bytesValue[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(bytesValue[0:4]),
		hex.EncodeToString(bytesValue[4:6]),
		hex.EncodeToString(bytesValue[6:8]),
		hex.EncodeToString(bytesValue[8:10]),
		hex.EncodeToString(bytesValue[10:16]),
	)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "request body must contain one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
