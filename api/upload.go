package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	tokenLifetime        = 6 * time.Hour
	googleChunkMultiple  = int64(256 << 10)
	defaultMultipartPart = int64(100 << 20)
)

type videoUploader interface {
	Upload(context.Context, TaskRecord, *os.File, int64, UploadTokenResponse) error
}

type googleDriveUploader struct{ server *Server }

func (u googleDriveUploader) Upload(ctx context.Context, task TaskRecord, file *os.File, total int64, token UploadTokenResponse) error {
	return u.server.uploadGoogleDrive(ctx, task, file, total, token)
}

type r2MultipartUploader struct{ server *Server }

func (u r2MultipartUploader) Upload(ctx context.Context, task TaskRecord, file *os.File, total int64, token UploadTokenResponse) error {
	return u.server.uploadR2Multipart(ctx, task, file, total, token)
}

func (s *Server) runVideoTask(ctx context.Context, task TaskRecord) error {
	path, info, err := s.resolveVideoFile(task.Path)
	if err != nil {
		return err
	}
	if info.Size() != task.FileSize || info.ModTime().UnixNano() != task.FileMtimeNS {
		return errors.New("video file changed after ffprobe; analyze it again")
	}
	metadata, err := taskMetadata(task)
	if err != nil {
		return err
	}
	if err := s.tasks.UpdateProgress(task.ID, "准备恢复", maxFloat(task.Progress, 3), task.UploadedBytes, info.Size()); err != nil {
		return err
	}
	fileType, err := detectMIME(path)
	if err != nil {
		return err
	}
	if err := s.tasks.UpdateProgress(task.ID, "获取上传凭证", maxFloat(task.Progress, 8), task.UploadedBytes, info.Size()); err != nil {
		return err
	}
	token, cacheKey, err := s.getUploadToken(ctx, task, fileType)
	if err != nil {
		return err
	}
	if err := s.tasks.UpdateProgress(task.ID, "上传文件", maxFloat(task.Progress, 10), task.UploadedBytes, info.Size()); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open video file: %w", err)
	}
	defer file.Close()

	var uploader videoUploader
	switch task.Storage {
	case "google_drive":
		uploader = googleDriveUploader{s}
	case "zn_r2_upload":
		uploader = r2MultipartUploader{s}
	default:
		return fmt.Errorf("unsupported file storage %q", task.Storage)
	}
	err = uploader.Upload(ctx, task, file, info.Size(), token)
	if err != nil {
		return err
	}
	if err := s.tasks.UpdateProgress(task.ID, "保存上传结果", 97, info.Size(), info.Size()); err != nil {
		return err
	}
	saveResult, err := s.emos.SaveVideo(ctx, task.ItemType, task.ItemID, token.FileID, metadata)
	if err != nil {
		return err
	}
	if err := s.db.CompleteUploadToken(ctx, cacheKey); err != nil {
		return err
	}
	if err := s.tasks.SetFileResult(task.ID, token.FileID, saveResult.MediaID); err != nil {
		return err
	}
	return nil
}

func (s *Server) getUploadToken(ctx context.Context, task TaskRecord, fileType string) (UploadTokenResponse, string, error) {
	cacheKey := uploadCacheKey(
		task.Path, task.FileSize, task.FileMtimeNS,
		task.ItemType, task.ItemID, task.SeasonNumber, task.EpisodeNumber, task.Storage, task.ID,
	)
	if cached, err := s.db.GetUploadToken(ctx, cacheKey); err == nil {
		var response UploadTokenResponse
		if json.Unmarshal([]byte(cached.RawJSON), &response) == nil &&
			strings.TrimSpace(response.FileID) != "" &&
			(cached.Completed || time.Now().Before(cached.ExpiresAt)) {
			return response, cacheKey, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return UploadTokenResponse{}, cacheKey, err
	}
	response, err := s.emos.GetUploadToken(ctx, task.FileName, fileType, task.FileSize, task.Storage)
	if err != nil {
		return UploadTokenResponse{}, cacheKey, err
	}
	raw, err := encodeJSON(response)
	if err != nil {
		return UploadTokenResponse{}, cacheKey, err
	}
	uploadURL := ""
	switch response.Type {
	case "google_drive":
		var data GoogleDriveTokenData
		if err := json.Unmarshal(response.Data, &data); err != nil || strings.TrimSpace(data.UploadURL) == "" {
			return UploadTokenResponse{}, cacheKey, errors.New("google_drive upload_url is missing")
		}
		uploadURL = data.UploadURL
	case "multipart":
		var data MultipartTokenData
		if err := json.Unmarshal(response.Data, &data); err != nil {
			return UploadTokenResponse{}, cacheKey, fmt.Errorf("decode multipart token: %w", err)
		}
	default:
		return UploadTokenResponse{}, cacheKey, fmt.Errorf("unsupported upload token type %q", response.Type)
	}
	if err := s.db.SaveUploadToken(ctx, UploadTokenRecord{
		CacheKey: cacheKey, Path: task.Path, FileSize: task.FileSize, Storage: task.Storage,
		FileMtimeNS: task.FileMtimeNS,
		TokenType:   response.Type, FileID: response.FileID, UploadURL: uploadURL, RawJSON: raw,
		ExpiresAt: time.Now().Add(tokenLifetime),
	}); err != nil {
		return UploadTokenResponse{}, cacheKey, err
	}
	return response, cacheKey, nil
}

func (s *Server) uploadGoogleDrive(ctx context.Context, task TaskRecord, file *os.File, total int64, token UploadTokenResponse) error {
	var data GoogleDriveTokenData
	if err := json.Unmarshal(token.Data, &data); err != nil {
		return fmt.Errorf("decode google_drive token: %w", err)
	}
	chunkSize := s.cfg.UploadChunkSizeBytes - (s.cfg.UploadChunkSizeBytes % googleChunkMultiple)
	if chunkSize < 5<<20 {
		chunkSize = 5 << 20
	}
	offset, completed, err := queryGoogleOffset(ctx, s.httpClient, data.UploadURL, total)
	if err != nil {
		return err
	}
	if completed {
		return s.tasks.UpdateProgress(task.ID, "上传文件", 95, total, total)
	}
	for offset < total {
		length := chunkSize
		if remaining := total - offset; remaining < length {
			length = remaining
		}
		end := offset + length - 1
		var lastErr error
		for attempt := 1; attempt <= s.cfg.UploadRetryMax; attempt++ {
			if err := s.tasks.UpdateProgress(task.ID, "上传文件", uploadProgress(offset, total), offset, total); err != nil {
				return err
			}
			reader := newProgressReader(io.NewSectionReader(file, offset, length), func(done int64) {
				absolute := offset + done
				_ = s.tasks.UpdateProgress(task.ID, "上传文件", uploadProgress(absolute, total), absolute, total)
			})
			request, err := http.NewRequestWithContext(ctx, http.MethodPut, data.UploadURL, reader)
			if err != nil {
				return err
			}
			request.ContentLength = length
			request.Header.Set("Content-Type", "application/octet-stream")
			request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, total))
			response, err := s.httpClient.Do(request)
			if err != nil {
				lastErr = err
			} else {
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				switch response.StatusCode {
				case http.StatusPermanentRedirect:
					acknowledged := end
					if rangeValue := response.Header.Get("Range"); rangeValue != "" {
						if parsed, parseErr := parseGoogleRange(rangeValue); parseErr == nil {
							acknowledged = parsed
						}
					}
					offset = acknowledged + 1
					lastErr = nil
					attempt = s.cfg.UploadRetryMax
				case http.StatusOK, http.StatusCreated:
					offset = end + 1
					lastErr = nil
					attempt = s.cfg.UploadRetryMax
				default:
					lastErr = fmt.Errorf("google upload returned HTTP %d", response.StatusCode)
				}
			}
			if lastErr == nil {
				break
			}
			if attempt < s.cfg.UploadRetryMax {
				if err := waitRetry(ctx, attempt); err != nil {
					return err
				}
			}
		}
		if lastErr != nil {
			return lastErr
		}
	}
	return s.tasks.UpdateProgress(task.ID, "上传文件", 95, total, total)
}

func queryGoogleOffset(ctx context.Context, client *http.Client, uploadURL string, total int64) (int64, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, nil)
	if err != nil {
		return 0, false, err
	}
	request.ContentLength = 0
	request.Header.Set("Content-Length", "0")
	request.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", total))
	response, err := client.Do(request)
	if err != nil {
		return 0, false, err
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	switch response.StatusCode {
	case http.StatusPermanentRedirect:
		rangeValue := response.Header.Get("Range")
		if rangeValue == "" {
			return 0, false, nil
		}
		offset, err := parseGoogleRange(rangeValue)
		return offset + 1, false, err
	case http.StatusOK, http.StatusCreated:
		return total, true, nil
	default:
		return 0, false, fmt.Errorf("query google upload offset returned HTTP %d", response.StatusCode)
	}
}

func parseGoogleRange(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "bytes=") {
		return 0, errors.New("invalid google upload Range")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return 0, errors.New("invalid google upload Range")
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

func (s *Server) uploadR2Multipart(ctx context.Context, task TaskRecord, file *os.File, total int64, token UploadTokenResponse) error {
	var data MultipartTokenData
	if err := json.Unmarshal(token.Data, &data); err != nil {
		return fmt.Errorf("decode multipart token: %w", err)
	}
	partSize := s.cfg.UploadChunkSizeBytes
	if partSize <= 0 {
		partSize = defaultMultipartPart
	}
	if data.MultipartSize.Min > 0 && partSize < data.MultipartSize.Min {
		partSize = data.MultipartSize.Min
	}
	if data.MultipartSize.Max > 0 && partSize > data.MultipartSize.Max {
		partSize = data.MultipartSize.Max
	}
	if partSize <= 0 {
		partSize = defaultMultipartPart
	}
	partCount := int((total + partSize - 1) / partSize)
	if partCount < 1 {
		partCount = 1
	}
	if partCount > 1000 {
		return fmt.Errorf("multipart file requires %d parts; maximum is 1000", partCount)
	}
	cacheKey := uploadCacheKey(
		task.Path, task.FileSize, task.FileMtimeNS,
		task.ItemType, task.ItemID, task.SeasonNumber, task.EpisodeNumber, task.Storage, task.ID,
	)
	cached, err := s.db.GetUploadToken(ctx, cacheKey)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		cached = UploadTokenRecord{}
	}
	var presigns []MultipartPresign
	if cached.PresignsJSON != "" {
		_ = json.Unmarshal([]byte(cached.PresignsJSON), &presigns)
	}
	if len(presigns) != partCount {
		presigns, err = s.emos.GetMultipartPresigns(ctx, token.FileID, partCount)
		if err != nil {
			return err
		}
		raw, marshalErr := encodeJSON(presigns)
		if marshalErr != nil {
			return marshalErr
		}
		if err := s.db.SaveMultipartPresigns(ctx, cacheKey, raw); err != nil {
			return err
		}
	}
	parts := make([]MultipartPart, partCount)
	jobs := make(chan int)
	errCh := make(chan error, 1)
	var uploaded atomic.Int64
	var wg sync.WaitGroup
	workers := s.cfg.UploadConcurrency
	if workers > partCount {
		workers = partCount
	}
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				part := presigns[index]
				offset := int64(index) * partSize
				length := partSize
				if remaining := total - offset; remaining < length {
					length = remaining
				}
				etag, uploadErr := uploadMultipartPart(ctx, s.httpClient, part.UploadURL, file, offset, length, s.cfg.UploadRetryMax, func(delta int64) {
					current := uploaded.Add(delta)
					_ = s.tasks.UpdateProgress(task.ID, "上传分片", uploadProgress(current, total), current, total)
				})
				if uploadErr != nil {
					select {
					case errCh <- fmt.Errorf("multipart part %d: %w", part.Number, uploadErr):
					default:
					}
					continue
				}
				parts[index] = MultipartPart{Number: part.Number, ETag: etag}
			}
		}()
	}
	for index := range presigns {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case err := <-errCh:
			close(jobs)
			wg.Wait()
			return err
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	if err := s.tasks.UpdateProgress(task.ID, "合并分片", 94, total, total); err != nil {
		return err
	}
	if cached.MultipartCompleted {
		return nil
	}
	if err := s.emos.CompleteMultipart(ctx, token.FileID, parts); err != nil {
		return err
	}
	return s.db.MarkMultipartCompleted(ctx, cacheKey)
}

func uploadMultipartPart(ctx context.Context, client *http.Client, uploadURL string, file *os.File, offset, length int64, retries int, onProgress func(int64)) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		var reported int64
		reader := newProgressReader(io.NewSectionReader(file, offset, length), func(done int64) {
			if onProgress != nil {
				delta := done - reported
				reported = done
				if delta > 0 {
					onProgress(delta)
				}
			}
		})
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
		if err != nil {
			return "", err
		}
		request.ContentLength = length
		response, err := client.Do(request)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				etag := strings.TrimSpace(response.Header.Get("ETag"))
				if etag == "" {
					etag = strings.TrimSpace(response.Header.Get("Etag"))
				}
				if etag == "" {
					return "", errors.New("multipart response did not contain ETag")
				}
				return etag, nil
			}
			lastErr = fmt.Errorf("multipart upload returned HTTP %d", response.StatusCode)
		} else {
			lastErr = err
		}
		if lastErr != nil && reported > 0 && onProgress != nil {
			onProgress(-reported)
		}
		if attempt < retries {
			if err := waitRetry(ctx, attempt); err != nil {
				return "", err
			}
		}
	}
	return "", lastErr
}

type progressReader struct {
	reader    io.Reader
	callback  func(int64)
	read      int64
	lastEvent time.Time
	mu        sync.Mutex
}

func newProgressReader(reader io.Reader, callback func(int64)) *progressReader {
	return &progressReader{reader: reader, callback: callback}
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	if count > 0 {
		r.mu.Lock()
		r.read += int64(count)
		shouldNotify := time.Since(r.lastEvent) >= 250*time.Millisecond || err == io.EOF
		read := r.read
		if shouldNotify {
			r.lastEvent = time.Now()
		}
		r.mu.Unlock()
		if shouldNotify && r.callback != nil {
			r.callback(read)
		}
	}
	return count, err
}

func detectMIME(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	buffer := make([]byte, 512)
	count, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	detected := strings.TrimSpace(http.DetectContentType(buffer[:count]))
	if detected != "" && detected != "application/octet-stream" {
		if mediaType, _, ok := strings.Cut(detected, ";"); ok {
			detected = strings.TrimSpace(mediaType)
		}
		return detected, nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv":
		return "video/x-matroska", nil
	default:
		if mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); strings.HasPrefix(mediaType, "video/") {
			return mediaType, nil
		}
		return "video/octet-stream", nil
	}
}

func uploadProgress(uploaded, total int64) float64 {
	if total <= 0 {
		return 10
	}
	return 10 + 85*(float64(uploaded)/float64(total))
}

func waitRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(retryDelay(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
