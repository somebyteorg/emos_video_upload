package api

import "encoding/json"

type DirectoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	FileCount int    `json:"file_count,omitempty"`
}

type SourceFile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

type ProbeSummary struct {
	Duration     float64 `json:"duration"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	VideoCodec   string  `json:"video_codec"`
	AudioCodec   string  `json:"audio_codec"`
	FrameRate    string  `json:"frame_rate"`
	Bitrate      int64   `json:"bitrate"`
	PixelFormat  string  `json:"pixel_format"`
	ColorSpace   string  `json:"color_space"`
	DynamicRange string  `json:"dynamic_range"`
	VideoStreams int     `json:"video_streams"`
	AudioStreams int     `json:"audio_streams"`
}

type ProbeResult struct {
	Valid    bool           `json:"valid"`
	Metadata map[string]any `json:"metadata"`
	Summary  ProbeSummary   `json:"summary"`
	Error    string         `json:"error,omitempty"`
}

type VideoTreeItem struct {
	VideoType string        `json:"video_type"`
	ItemType  string        `json:"item_type"`
	ItemID    int64         `json:"item_id"`
	TMDBID    int64         `json:"tmdb_id,omitempty"`
	TODBID    int64         `json:"todb_id"`
	Title     string        `json:"title"`
	DateAir   string        `json:"date_air,omitempty"`
	HasMedia  bool          `json:"has_media,omitempty"`
	Seasons   []VideoSeason `json:"seasons,omitempty"`
}

type VideoSeason struct {
	ItemType     string         `json:"item_type"`
	ItemID       int64          `json:"item_id"`
	SeasonTitle  string         `json:"season_title"`
	SeasonNumber int            `json:"season_number"`
	DateAir      string         `json:"date_air,omitempty"`
	Episodes     []VideoEpisode `json:"episodes"`
}

type VideoEpisode struct {
	ItemType      string `json:"item_type"`
	ItemID        int64  `json:"item_id"`
	EpisodeTitle  string `json:"episode_title"`
	EpisodeNumber int    `json:"episode_number"`
	DateAir       string `json:"date_air,omitempty"`
}

type VideoBaseInfo struct {
	VideoListName string       `json:"video_list_name"`
	SeasonNumber  string       `json:"season_number,omitempty"`
	EpisodeNumber string       `json:"episode_number,omitempty"`
	EpisodeTitle  string       `json:"episode_title,omitempty"`
	Title         string       `json:"title"`
	VideoMedias   []VideoMedia `json:"video_medias"`
}

type VideoMedia struct {
	MediaID       string `json:"media_id"`
	MediaName     string `json:"media_name"`
	MediaFileSize int64  `json:"media_file_size"`
	UserPseudonym string `json:"user_pseudonym,omitempty"`
	IsSelfUpload  bool   `json:"is_self_upload,omitempty"`
}

type UploadTokenResponse struct {
	Type   string          `json:"type"`
	FileID string          `json:"file_id"`
	Data   json.RawMessage `json:"data"`
}

type GoogleDriveTokenData struct {
	UploadURL string `json:"upload_url"`
}

type MultipartTokenData struct {
	MultipartSize struct {
		Min int64 `json:"min"`
		Max int64 `json:"max"`
	} `json:"multipart_size"`
}

type MultipartPresign struct {
	Number     int            `json:"number"`
	UploadURL  string         `json:"upload_url"`
	UploadData map[string]any `json:"upload_data"`
}

type MultipartPart struct {
	Number int    `json:"number"`
	ETag   string `json:"etag"`
}

type SaveVideoResponse struct {
	Count   int    `json:"count"`
	MediaID string `json:"media_id"`
	Carrot  int    `json:"carrot"`
}

type taskResponse struct {
	TaskID string `json:"task_id"`
}

type taskStatusResponse struct {
	TaskID        string  `json:"task_id"`
	Kind          string  `json:"kind,omitempty"`
	Path          string  `json:"path,omitempty"`
	FileName      string  `json:"file_name,omitempty"`
	ItemType      string  `json:"item_type,omitempty"`
	ItemID        int64   `json:"item_id,omitempty"`
	SeasonNumber  int     `json:"season_number,omitempty"`
	EpisodeNumber int     `json:"episode_number,omitempty"`
	VideoTitle    string  `json:"video_title,omitempty"`
	Storage       string  `json:"storage,omitempty"`
	Status        string  `json:"status"`
	Stage         string  `json:"stage"`
	Progress      float64 `json:"progress"`
	UploadedBytes int64   `json:"uploaded_bytes,omitempty"`
	TotalBytes    int64   `json:"total_bytes,omitempty"`
	Error         string  `json:"error,omitempty"`
	FileID        string  `json:"file_id,omitempty"`
	MediaID       string  `json:"media_id,omitempty"`
}
