package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func probeVideo(ctx context.Context, ffprobePath, path string) (ProbeResult, string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandCtx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return ProbeResult{Valid: false, Error: message}, "", fmt.Errorf("ffprobe failed: %s", message)
	}
	var metadata map[string]any
	if err := json.Unmarshal(output, &metadata); err != nil {
		return ProbeResult{Valid: false, Error: "ffprobe returned invalid JSON"}, "", fmt.Errorf("decode ffprobe output: %w", err)
	}
	summary := summarizeProbe(metadata)
	result := ProbeResult{Valid: true, Metadata: metadata, Summary: summary}
	switch {
	case summary.VideoStreams == 0:
		result.Valid = false
		result.Error = "file does not contain a video stream"
	case summary.Width <= 0 || summary.Height <= 0:
		result.Valid = false
		result.Error = "video stream has invalid dimensions"
	case summary.Duration <= 0:
		result.Valid = false
		result.Error = "video duration is missing or invalid"
	}
	if !result.Valid {
		return result, string(output), fmt.Errorf("%s", result.Error)
	}
	return result, string(output), nil
}

func summarizeProbe(metadata map[string]any) ProbeSummary {
	streams, _ := metadata["streams"].([]any)
	format, _ := metadata["format"].(map[string]any)
	var video, audio map[string]any
	videoStreams, audioStreams := 0, 0
	for _, item := range streams {
		stream, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch streamString(stream, "codec_type") {
		case "video":
			videoStreams++
			if video == nil {
				video = stream
			}
		case "audio":
			audioStreams++
			if audio == nil {
				audio = stream
			}
		}
	}
	frameRate := firstString(video, "avg_frame_rate", "r_frame_rate")
	if frameRate != "" {
		frameRate = formatFrameRate(frameRate)
	}
	return ProbeSummary{
		Duration:     firstFloat(format, "duration"),
		Width:        int(firstFloat(video, "width")),
		Height:       int(firstFloat(video, "height")),
		VideoCodec:   streamString(video, "codec_name"),
		AudioCodec:   streamString(audio, "codec_name"),
		FrameRate:    frameRate,
		Bitrate:      int64(firstFloat(format, "bit_rate")),
		PixelFormat:  streamString(video, "pix_fmt"),
		ColorSpace:   streamString(video, "color_space"),
		DynamicRange: dynamicRange(video),
		VideoStreams: videoStreams,
		AudioStreams: audioStreams,
	}
}

func dynamicRange(video map[string]any) string {
	transfer := strings.ToLower(streamString(video, "color_transfer"))
	codec := strings.ToLower(streamString(video, "codec_name"))
	switch {
	case strings.Contains(codec, "dovi") || strings.Contains(transfer, "dovi"):
		return "Dolby Vision"
	case strings.Contains(transfer, "smpte2084"):
		return "HDR10"
	case strings.Contains(transfer, "arib-std-b67"):
		return "HLG"
	default:
		return "SDR"
	}
}

func formatFrameRate(value string) string {
	left, right, ok := strings.Cut(value, "/")
	if !ok {
		return value
	}
	numerator, err1 := strconv.ParseFloat(left, 64)
	denominator, err2 := strconv.ParseFloat(right, 64)
	if err1 != nil || err2 != nil || denominator == 0 {
		return value
	}
	return fmt.Sprintf("%.2f fps", numerator/denominator)
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := streamString(values, key); value != "" {
			return value
		}
	}
	return ""
}

func streamString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func firstFloat(values map[string]any, key string) float64 {
	if values == nil {
		return 0
	}
	value, ok := values[key]
	if !ok {
		return 0
	}
	switch parsed := value.(type) {
	case float64:
		return parsed
	case json.Number:
		number, _ := parsed.Float64()
		return number
	case string:
		number, _ := strconv.ParseFloat(parsed, 64)
		return number
	default:
		number, _ := strconv.ParseFloat(fmt.Sprint(parsed), 64)
		return number
	}
}
