export interface AuthStatus {
  authenticated: boolean
  enabled: boolean
  file_storages: string[]
}

export interface DirectoryEntry {
  name: string
  path: string
  kind: string
  file_count?: number
}

export interface DirectorySelection {
  name: string
  path: string
  file_count?: number
}

export interface SourceFile {
  id: string
  name: string
  path: string
  size: number
  modified_at: string
}

export interface ProbeSummary {
  duration: number
  width: number
  height: number
  video_codec: string
  audio_codec: string
  frame_rate: string
  bitrate: number
  pixel_format: string
  color_space: string
  dynamic_range: string
  video_streams: number
  audio_streams: number
}

export interface ProbeResult {
  valid: boolean
  metadata: Record<string, unknown>
  summary: ProbeSummary
  error?: string
}

export interface VideoEpisode {
  item_type: string
  item_id: number
  episode_title: string
  episode_number: number
  date_air?: string
}

export interface VideoSeason {
  item_type: string
  item_id: number
  season_title: string
  season_number: number
  date_air?: string
  episodes: VideoEpisode[]
}

export interface VideoTreeItem {
  video_type: 'tv' | 'movie'
  item_type: 'vl' | 've'
  item_id: number
  tmdb_id?: number
  todb_id: number
  title: string
  date_air?: string
  has_media?: boolean
  seasons?: VideoSeason[]
}

export interface VideoMedia {
  media_id: string
  media_name: string
  media_file_size: number
  user_pseudonym?: string
  is_self_upload?: boolean
}

export interface VideoBaseInfo {
  video_list_name: string
  season_number?: string
  episode_number?: string
  episode_title?: string
  title: string
  video_medias: VideoMedia[]
}

export interface TaskStatus {
  task_id: string
  kind?: string
  path?: string
  file_name?: string
  item_type?: string
  item_id?: number
  season_number?: number
  episode_number?: number
  video_title?: string
  storage?: string
  status: 'queued' | 'running' | 'success' | 'error' | string
  stage: string
  progress: number
  uploaded_bytes: number
  total_bytes: number
  error?: string
  file_id?: string
  media_id?: string
}

export interface TaskPage {
  tasks: TaskStatus[]
  total: number
  page: number
  limit: number
}

export interface TargetSelection {
  item_type: 'vl' | 've'
  item_id: number
  title: string
  video_title?: string
  season_number?: number
  episode_number?: number
}
