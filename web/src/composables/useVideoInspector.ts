import { computed, ref, watch, type Ref } from 'vue'
import api, { errorMessage } from '@/utils/ky'
import signStore from '@/stores/sign'
import type { ProbeResult, SourceFile, TargetSelection, VideoBaseInfo, VideoTreeItem } from '@/types'
import { basename } from '@/utils/path'
import { stripVideoNameTags } from '@/utils/videoName'

function isPositiveInteger(value: string) {
  return /^[1-9]\d*$/.test(value)
}

function parseEpisode(name: string) {
  const seasonEpisode = name.match(/S(\d{1,2})\s*E(\d{1,3})/i)
  if (seasonEpisode) {
    return {
      seasonNumber: Number(seasonEpisode[1]),
      episodeNumber: Number(seasonEpisode[2]),
    }
  }
  const episode = name.match(/E(\d{1,3})/i)
  return {
    seasonNumber: null,
    episodeNumber: episode ? Number(episode[1]) : null,
  }
}

export function useVideoInspector(file: Ref<SourceFile | null>) {
  const sign = signStore()
  const probe = ref<ProbeResult | null>(null)
  const probeLoading = ref(false)
  const targetType = ref<'movie' | 'tv'>('movie')
  const searchTitle = ref('')
  const searchTodbId = ref('')
  const searchBusy = ref(false)
  const searchResults = ref<VideoTreeItem[]>([])
  const selectedTarget = ref<TargetSelection | null>(null)
  const baseInfo = ref<VideoBaseInfo | null>(null)
  const baseLoading = ref(false)
  const storageOptions = computed(() => sign.fileStorages.map((value) => ({ value, label: value })))
  const storageType = ref('')
  const createTaskBusy = ref(false)
  const error = ref('')
  const notice = ref('')

  const folderTodbId = computed(() => {
    const match = file.value?.path.match(/\[todbid=(\d+)\]/i)
    return match?.[1] ?? ''
  })
  const guessedTitle = computed(() => stripVideoNameTags(basename(file.value?.name ?? '')))
  const matchingMedia = computed(() => {
    if (!file.value || !baseInfo.value) {
      return []
    }
    return baseInfo.value.video_medias.filter((media) => media.media_file_size === file.value?.size)
  })
  const episodeNumbers = computed(() => parseEpisode(file.value?.name ?? ''))
  const canUpload = computed(() => Boolean(file.value && probe.value?.valid && selectedTarget.value && !baseLoading.value && !createTaskBusy.value && storageType.value))

  function resetMatch() {
    searchResults.value = []
    selectedTarget.value = null
    baseInfo.value = null
    error.value = ''
  }

  async function inspectFile(nextFile: SourceFile | null) {
    resetMatch()
    probe.value = null
    notice.value = ''
    if (!nextFile) {
      return
    }
    searchTodbId.value = folderTodbId.value
    searchTitle.value = guessedTitle.value
    targetType.value = episodeNumbers.value.episodeNumber !== null ? 'tv' : 'movie'
    probeLoading.value = true
    error.value = ''
    try {
      probe.value = await api
        .post('api/files/probe', {
          json: { path: nextFile.path },
        })
        .json<ProbeResult>()
      if (!probe.value.valid) {
        error.value = probe.value.error ?? '文件未通过视频校验'
      }
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      probeLoading.value = false
    }
  }

  function setTargetType(type: 'movie' | 'tv') {
    if (targetType.value === type) {
      return
    }
    targetType.value = type
    resetMatch()
  }

  async function searchTargets() {
    searchBusy.value = true
    error.value = ''
    try {
      const params: Record<string, string> = { type: targetType.value }
      const todbId = String(searchTodbId.value ?? '').trim() || folderTodbId.value
      const title = String(searchTitle.value ?? '').trim()
      if (todbId) {
        if (!isPositiveInteger(todbId)) {
          error.value = 'todb_id 必须是正整数'
          return
        }
        params.todb_id = todbId
      } else if (title) {
        params.title = title
      } else {
        error.value = '请输入标题或 todb_id'
        return
      }
      const results = await api
        .get('api/video/tree', {
          searchParams: params,
        })
        .json<VideoTreeItem[]>()
      searchResults.value = results
      const autoMatchedEpisode = results.length === 1 ? findAutoMatchedEpisode(results[0]) : null
      if (autoMatchedEpisode) {
        await chooseTarget({
          item_type: 've',
          item_id: autoMatchedEpisode.episode.item_id,
          title: `${autoMatchedEpisode.video.title} · S${String(autoMatchedEpisode.season.season_number).padStart(2, '0')}E${String(autoMatchedEpisode.episode.episode_number).padStart(2, '0')} ${autoMatchedEpisode.episode.episode_title}`,
          video_title: autoMatchedEpisode.video.title,
          season_number: autoMatchedEpisode.season.season_number,
          episode_number: autoMatchedEpisode.episode.episode_number,
        })
      } else if (searchResults.value.length === 0) {
        error.value = '没有找到匹配的视频条目'
      }
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      searchBusy.value = false
    }
  }

  function findAutoMatchedEpisode(video: VideoTreeItem | undefined) {
    if (!video || targetType.value !== 'tv') {
      return null
    }
    const { seasonNumber, episodeNumber } = episodeNumbers.value
    if (seasonNumber === null || episodeNumber === null) {
      return null
    }
    const season = video.seasons?.find((candidate) => candidate.season_number === seasonNumber)
    const episode = season?.episodes.filter((candidate) => candidate.episode_number === episodeNumber)
    if (!season || episode?.length !== 1 || !episode[0]) {
      return null
    }
    return {
      video,
      season,
      episode: episode[0],
    }
  }

  async function chooseTarget(target: TargetSelection) {
    selectedTarget.value = target
    baseInfo.value = null
    baseLoading.value = true
    error.value = ''
    try {
      baseInfo.value = await api
        .get('api/upload/video/base', {
          searchParams: {
            item_type: target.item_type,
            item_id: String(target.item_id),
          },
        })
        .json<VideoBaseInfo>()
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      baseLoading.value = false
    }
  }

  function chooseMovie(item: VideoTreeItem) {
    void chooseTarget({
      item_type: 'vl',
      item_id: item.item_id,
      title: item.title,
      video_title: item.title,
    })
  }

  function chooseEpisode(parent: VideoTreeItem, seasonNumber: number, episodeNumber: number, itemId: number, episodeTitle: string) {
    void chooseTarget({
      item_type: 've',
      item_id: itemId,
      title: `${parent.title} · S${String(seasonNumber).padStart(2, '0')}E${String(episodeNumber).padStart(2, '0')} ${episodeTitle}`,
      video_title: parent.title,
      season_number: seasonNumber,
      episode_number: episodeNumber,
    })
  }

  function clearTarget() {
    selectedTarget.value = null
    baseInfo.value = null
  }

  async function createUploadTask() {
    if (!file.value || !selectedTarget.value) {
      return
    }
    createTaskBusy.value = true
    notice.value = ''
    error.value = ''
    try {
      await api
        .post('api/upload/tasks', {
          json: {
            path: file.value.path,
            task_type: 'video',
            item_type: selectedTarget.value.item_type,
            item_id: selectedTarget.value.item_id,
            season_number: selectedTarget.value.season_number,
            episode_number: selectedTarget.value.episode_number,
            video_title: selectedTarget.value.video_title,
            file_storage: storageType.value,
            generate_sprites: false,
          },
        })
        .json<{ task_id: string }>()
      notice.value = '上传任务已创建'
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      createTaskBusy.value = false
    }
  }

  watch(
    file,
    (nextFile) => {
      void inspectFile(nextFile)
    },
    { immediate: true },
  )
  watch(
    () => sign.fileStorages,
    (nextStorages) => {
      if (!nextStorages.includes(storageType.value)) {
        storageType.value = nextStorages[0] ?? ''
      }
    },
    { immediate: true },
  )

  return {
    probe,
    probeLoading,
    targetType,
    searchTitle,
    searchTodbId,
    searchBusy,
    searchResults,
    selectedTarget,
    baseInfo,
    baseLoading,
    storageOptions,
    storageType,
    createTaskBusy,
    folderTodbId,
    matchingMedia,
    canUpload,
    error,
    notice,
    setTargetType,
    searchTargets,
    chooseMovie,
    chooseEpisode,
    clearTarget,
    createUploadTask,
  }
}
