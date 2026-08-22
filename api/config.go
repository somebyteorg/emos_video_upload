package api

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServerHost        = "0.0.0.0"
	defaultServerPort        = 8004
	defaultFFprobePath       = "/usr/bin/ffprobe"
	defaultHTTPTimeout       = 30 * time.Second
	defaultChunkSize         = 100 << 20
	defaultUploadConcurrency = 3
	defaultUploadRetryMax    = 3
	defaultTaskConcurrency   = 2
	minUploadChunkSize       = 5 << 20
)

type Config struct {
	WorkingDir string
	DBPath     string

	Username string
	Password string

	EMOSURL         string
	EMOSToken       string
	ForgePath       string
	VideoRoot       string
	EMOSFileStorage []string

	ServerHost  string
	ServerPort  int
	FFprobePath string

	HTTPTimeout          time.Duration
	UploadChunkSizeBytes int64
	UploadConcurrency    int
	UploadRetryMax       int
	TaskConcurrency      int
}

func LoadConfig(envFile string) (Config, error) {
	workingDir, err := filepath.Abs(".")
	if err != nil {
		return Config{}, fmt.Errorf("resolve working directory: %w", err)
	}

	values := make(map[string]string)
	// The project .env intentionally takes precedence over inherited process
	// variables such as USERNAME, which may be set by the host environment.
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	if err := loadEnvFile(filepath.Join(workingDir, ".env"), values, false); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(envFile) != "" {
		if err := loadEnvFile(envFile, values, true); err != nil {
			return Config{}, err
		}
	}

	serverPort, err := parseIntValue(values, "SERVER_PORT", defaultServerPort)
	if err != nil {
		return Config{}, err
	}
	uploadChunkSize, err := parseInt64Value(values, "UPLOAD_CHUNK_SIZE_BYTES", defaultChunkSize)
	if err != nil {
		return Config{}, err
	}
	uploadConcurrency, err := parseIntValue(values, "UPLOAD_CONCURRENCY", defaultUploadConcurrency)
	if err != nil {
		return Config{}, err
	}
	uploadRetryMax, err := parseIntValue(values, "UPLOAD_RETRY_MAX", defaultUploadRetryMax)
	if err != nil {
		return Config{}, err
	}
	taskConcurrency, err := parseIntValue(values, "TASK_CONCURRENCY", defaultTaskConcurrency)
	if err != nil {
		return Config{}, err
	}
	httpTimeout, err := parseDurationValue(values, "HTTP_TIMEOUT", defaultHTTPTimeout)
	if err != nil {
		return Config{}, err
	}

	videoRoot := strings.TrimSpace(values["VIDEO_ROOT"])
	if videoRoot != "" {
		videoRoot, err = filepath.Abs(videoRoot)
		if err != nil {
			return Config{}, fmt.Errorf("resolve VIDEO_ROOT: %w", err)
		}
	}

	cfg := Config{
		WorkingDir:           workingDir,
		DBPath:               filepath.Join(workingDir, "dbdata", "emos_video_upload.db"),
		Username:             strings.TrimSpace(values["USERNAME"]),
		Password:             values["PASSWORD"],
		EMOSURL:              strings.TrimRight(strings.TrimSpace(values["EMOS_URL"]), "/"),
		EMOSToken:            strings.TrimSpace(values["EMOS_TOKEN"]),
		ForgePath:            strings.TrimSpace(values["EMOS_FORGE_PATH"]),
		VideoRoot:            videoRoot,
		EMOSFileStorage:      parseFileStorage(values["EMOS_FILE_STORAGE"]),
		ServerHost:           firstNonEmpty(values["SERVER_HOST"], defaultServerHost),
		ServerPort:           serverPort,
		FFprobePath:          firstNonEmpty(values["FFPROBE_PATH"], defaultFFprobePath),
		HTTPTimeout:          httpTimeout,
		UploadChunkSizeBytes: uploadChunkSize,
		UploadConcurrency:    uploadConcurrency,
		UploadRetryMax:       uploadRetryMax,
		TaskConcurrency:      taskConcurrency,
	}
	if err := cfg.validateValues(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validateValues() error {
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return errors.New("SERVER_PORT must be between 1 and 65535")
	}
	if c.HTTPTimeout <= 0 {
		return errors.New("HTTP_TIMEOUT must be positive")
	}
	if c.UploadChunkSizeBytes < minUploadChunkSize {
		return fmt.Errorf("UPLOAD_CHUNK_SIZE_BYTES must be at least %d", minUploadChunkSize)
	}
	if c.UploadConcurrency <= 0 || c.UploadRetryMax <= 0 || c.TaskConcurrency <= 0 {
		return errors.New("UPLOAD_CONCURRENCY, UPLOAD_RETRY_MAX and TASK_CONCURRENCY must be positive")
	}
	if (strings.TrimSpace(c.Username) == "") != (strings.TrimSpace(c.Password) == "") {
		return errors.New("USERNAME and PASSWORD must both be set or both be empty")
	}
	return nil
}

func (c Config) ValidateServer() error {
	if err := c.validateValues(); err != nil {
		return err
	}
	if strings.TrimSpace(c.VideoRoot) == "" {
		return errors.New("VIDEO_ROOT is required")
	}
	info, err := os.Stat(c.VideoRoot)
	if err != nil {
		return fmt.Errorf("stat VIDEO_ROOT: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("VIDEO_ROOT is not a directory: %s", c.VideoRoot)
	}
	if strings.TrimSpace(c.FFprobePath) == "" {
		return errors.New("FFPROBE_PATH is required")
	}
	if _, err := resolveExecutable(c.FFprobePath); err != nil {
		return fmt.Errorf("resolve FFPROBE_PATH: %w", err)
	}
	if strings.TrimSpace(c.EMOSURL) == "" {
		return errors.New("EMOS_URL is required")
	}
	parsedURL, err := url.Parse(c.EMOSURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return errors.New("EMOS_URL must be a valid HTTP URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("EMOS_URL must use http or https")
	}
	if strings.TrimSpace(c.EMOSToken) == "" {
		return errors.New("EMOS_TOKEN is required")
	}
	if len(c.EMOSFileStorage) == 0 {
		return errors.New("EMOS_FILE_STORAGE is required")
	}
	for _, storage := range c.EMOSFileStorage {
		if !supportedStorage(storage) {
			return fmt.Errorf("unsupported EMOS_FILE_STORAGE value %q", storage)
		}
	}
	return nil
}

func (c Config) SupportsFileStorage(storage string) bool {
	storage = strings.ToLower(strings.TrimSpace(storage))
	for _, configured := range c.EMOSFileStorage {
		if configured == storage {
			return true
		}
	}
	return false
}

func supportedStorage(value string) bool {
	return value == "google_drive" || value == "zn_r2_upload"
}

func resolveExecutable(value string) (string, error) {
	if filepath.IsAbs(value) || strings.ContainsRune(value, filepath.Separator) {
		info, err := os.Stat(value)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", errors.New("path points to a directory")
		}
		if info.Mode().Perm()&0o111 == 0 {
			return "", errors.New("path is not executable")
		}
		return value, nil
	}
	return exec.LookPath(value)
}

func loadEnvFile(path string, dst map[string]string, required bool) error {
	file, err := os.Open(path)
	if err != nil {
		if !required && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid env line %s:%d", path, lineNumber)
		}
		dst[strings.TrimSpace(key)] = unquoteEnvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file %s: %w", path, err)
	}
	return nil
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return value
}

func parseFileStorage(value string) []string {
	result := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, item := range strings.Split(value, ",") {
		storage := strings.ToLower(strings.TrimSpace(item))
		if storage == "" {
			continue
		}
		if _, exists := seen[storage]; exists {
			continue
		}
		seen[storage] = struct{}{}
		result = append(result, storage)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseIntValue(values map[string]string, key string, fallback int) (int, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func parseInt64Value(values map[string]string, key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func parseDurationValue(values map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return parsed, nil
}
