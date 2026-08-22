import { computed, ref, watch, type Ref } from 'vue'
import api, { errorMessage } from '@/utils/ky'
import signStore from '@/stores/sign'
import type { DirectorySelection, ProbeResult, SourceFile, TargetSelection, VideoBaseInfo, VideoTreeItem } from '@/types'
import { basename } from '@/utils/path'
import { stripVideoNameTags } from '@/utils/videoName'

interface EpisodeOption {
  key: string
  itemId: number
  seasonNumber: number
  episodeNumber: number
  label: string
  title: string
}

interface BatchAssignment {
  file: SourceFile
  enabled: boolean
  seasonNumber: number | null
  episodeNumber: number | null
  episodeHint: string
  selectedEpisodeKey: string
  target: TargetSelection | null
  storageType: string
  probe: ProbeResult | null
  probeLoading: boolean
  baseInfo: VideoBaseInfo | null
  baseLoading: boolean
  duplicateConfirmed: boolean
  status: 'auto' | 'manual' | 'unmatched' | 'queued' | 'error'
  error: string
}

function episodeKey(seasonNumber: number, episodeNumber: number) {
  return `${seasonNumber}:${episodeNumber}`
}

function parseEpisode(name: string) {
  const seasonEpisode = name.match(/S(\d{1,2})\s*E(\d{1,3})/i)
  if (seasonEpisode) {
    const seasonNumber = Number(seasonEpisode[1])
    const episodeNumber = Number(seasonEpisode[2])
    return {
      seasonNumber,
      episodeNumber,
      hint: `S${String(seasonNumber).padStart(2, '0')}E${String(episodeNumber).padStart(2, '0')}`,
    }
  }
  const episode = name.match(/E(\d{1,3})/i)
  if (episode) {
    const episodeNumber = Number(episode[1])
    return {
      seasonNumber: null,
      episodeNumber,
      hint: `E${String(episodeNumber).padStart(2, '0')}`,
    }
  }
  return {
    seasonNumber: null,
    episodeNumber: null,
    hint: '未发现集数',
  }
}

function targetTitle(series: VideoTreeItem, option: EpisodeOption) {
  return `${series.title} · S${String(option.seasonNumber).padStart(2, '0')}E${String(option.episodeNumber).padStart(2, '0')} ${option.title}`
}

function normalizedMatchText(value: string) {
  return stripVideoNameTags(value).toLocaleLowerCase().replace(/\s+/g, '')
}

function directoryContainsSeriesTitle(series: VideoTreeItem, directory: DirectorySelection | null) {
  const directoryText = normalizedMatchText(directory?.name || basename(directory?.path ?? ''))
  const seriesText = normalizedMatchText(series.title)
  return Boolean(directoryText && seriesText && directoryText.includes(seriesText))
}

function isPositiveInteger(value: string) {
  return /^[1-9]\d*$/.test(value)
}

export function useBatchInspector(directory: Ref<DirectorySelection | null>) {
  const sign = signStore()
  const files = ref<SourceFile[]>([])
  const directoryLoading = ref(false)
  const probeCompleted = ref(0)
  const probeTotal = ref(0)
  const mode = ref<'movie' | 'tv'>('movie')
  const searchTitle = ref('')
  const searchTodbId = ref('')
  const searchBusy = ref(false)
  const searchResults = ref<VideoTreeItem[]>([])
  const selectedMovie = ref<VideoTreeItem | null>(null)
  const selectedSeries = ref<VideoTreeItem | null>(null)
  const assignments = ref<BatchAssignment[]>([])
  const createTaskBusy = ref(false)
  const createTaskProgress = ref({ completed: 0, total: 0 })
  const storageOptions = computed(() => sign.fileStorages.map((value) => ({ value, label: value })))
  const bulkStorageType = ref('')
  const error = ref('')
  const notice = ref('')
  const showCreateConfirm = ref(false)
  let loadSequence = 0

  const folderTodbId = computed(() => {
    const match = directory.value?.path.match(/\[todbid=(\d+)\]/i)
    return match?.[1] ?? ''
  })
  const episodeOptions = computed<EpisodeOption[]>(() => {
    if (!selectedSeries.value) {
      return []
    }
    return (
      selectedSeries.value.seasons?.flatMap((season) =>
        season.episodes.map((episode) => ({
          key: episodeKey(season.season_number, episode.episode_number),
          itemId: episode.item_id,
          seasonNumber: season.season_number,
          episodeNumber: episode.episode_number,
          label: `S${String(season.season_number).padStart(2, '0')}E${String(episode.episode_number).padStart(2, '0')} · ${episode.episode_title}`,
          title: episode.episode_title,
        })),
      ) ?? []
    )
  })
  const selectedVideo = computed(() => selectedMovie.value ?? selectedSeries.value)
  const activeAssignments = computed(() => assignments.value.filter((assignment) => assignment.enabled))
  const matchedAssignments = computed(() => activeAssignments.value.filter((assignment) => assignment.target))
  const unmatchedAssignments = computed(() => activeAssignments.value.filter((assignment) => !assignment.target))
  const probingAssignments = computed(() => activeAssignments.value.filter((assignment) => assignment.probeLoading))
  const invalidAssignments = computed(() => activeAssignments.value.filter((assignment) => !assignment.probeLoading && assignment.probe !== null && !assignment.probe.valid))
  const unprobedAssignments = computed(() => activeAssignments.value.filter((assignment) => !assignment.probeLoading && assignment.probe === null))
  const missingBaseInfoAssignments = computed(() => matchedAssignments.value.filter((assignment) => assignment.baseInfo === null && !assignment.baseLoading))
  const duplicateAssignments = computed(() =>
    matchedAssignments.value.filter((assignment) => assignment.baseInfo?.video_medias.some((media) => media.media_file_size === assignment.file.size) && !assignment.duplicateConfirmed),
  )
  const canCreateTasks = computed(
    () =>
      matchedAssignments.value.length > 0 &&
      unmatchedAssignments.value.length === 0 &&
      matchedAssignments.value.every(
        (assignment) =>
          assignment.probe?.valid &&
          !assignment.probeLoading &&
          !assignment.baseLoading &&
          assignment.baseInfo !== null &&
          (!assignment.baseInfo?.video_medias.some((media) => media.media_file_size === assignment.file.size) || assignment.duplicateConfirmed),
      ) &&
      Boolean(storageOptions.value.length) &&
      !createTaskBusy.value,
  )
  const createBlocker = computed(() => {
    if (createTaskBusy.value) return ''
    if (activeAssignments.value.length === 0) return '请选择至少一个视频加入本次任务'
    if (unmatchedAssignments.value.length > 0) return `还有 ${unmatchedAssignments.value.length} 个文件未匹配目标`
    if (probingAssignments.value.length > 0) return `正在校验 ${probingAssignments.value.length} 个文件`
    if (invalidAssignments.value.length > 0) return `${invalidAssignments.value.length} 个文件校验失败，修复后才能继续`
    if (unprobedAssignments.value.length > 0) return `${unprobedAssignments.value.length} 个文件尚未完成校验`
    if (missingBaseInfoAssignments.value.length > 0) return `${missingBaseInfoAssignments.value.length} 个文件的目标信息读取失败`
    if (duplicateAssignments.value.length > 0) return `请确认 ${duplicateAssignments.value.length} 个重复资源是否仍要上传`
    if (matchedAssignments.value.some((assignment) => assignment.baseLoading)) return '正在读取目标信息'
    return ''
  })

  function clearSelection(resetAssignments = true) {
    selectedMovie.value = null
    selectedSeries.value = null
    if (resetAssignments) {
      assignments.value = []
    }
    searchResults.value = []
    notice.value = ''
  }

  function inferredTitle(nextFiles: SourceFile[]) {
    const directoryTitle = stripVideoNameTags(basename(directory.value?.path ?? ''))
    if (directoryTitle && directoryTitle.toLowerCase() !== 'video') {
      return directoryTitle
    }
    return stripVideoNameTags(nextFiles[0]?.name ?? '')
  }

  function findEpisode(seasonNumber: number | null, episodeNumber: number | null) {
    if (episodeNumber === null) {
      return null
    }
    const season = seasonNumber ?? (selectedSeries.value?.seasons?.length === 1 ? (selectedSeries.value.seasons[0]?.season_number ?? null) : null)
    if (season === null) {
      return null
    }
    return episodeOptions.value.find((option) => option.key === episodeKey(season, episodeNumber)) ?? null
  }

  function createAssignment(file: SourceFile, series: VideoTreeItem | null): BatchAssignment {
    const parsed = parseEpisode(file.name)
    const option = series ? findEpisode(parsed.seasonNumber, parsed.episodeNumber) : null
    return {
      file,
      enabled: true,
      seasonNumber: parsed.seasonNumber,
      episodeNumber: parsed.episodeNumber,
      episodeHint: parsed.hint,
      selectedEpisodeKey: option?.key ?? '',
      target:
        option && series
          ? {
              item_type: 've',
              item_id: option.itemId,
              title: targetTitle(series, option),
              video_title: series.title,
              season_number: option.seasonNumber,
              episode_number: option.episodeNumber,
            }
          : null,
      storageType: sign.fileStorages[0] ?? '',
      probe: null,
      probeLoading: false,
      baseInfo: null,
      baseLoading: false,
      duplicateConfirmed: false,
      status: option ? 'auto' : 'unmatched',
      error: '',
    }
  }

  function preserveProbe(next: BatchAssignment, previous: BatchAssignment | undefined) {
    if (!previous) return next
    next.enabled = previous.enabled
    next.probe = previous.probe
    next.probeLoading = previous.probeLoading
    next.error = previous.probe?.valid === false || previous.probe === null ? (previous.probe?.error ?? previous.error) : ''
    return next
  }

  function chooseMovie(item: VideoTreeItem) {
    const previousAssignments = new Map(assignments.value.map((assignment) => [assignment.file.id, assignment]))
    selectedMovie.value = item
    selectedSeries.value = null
    assignments.value = files.value
      .map((file) => ({
        file,
        enabled: previousAssignments.get(file.id)?.enabled ?? true,
        seasonNumber: null,
        episodeNumber: null,
        episodeHint: '电影资源',
        selectedEpisodeKey: '',
        target: {
          item_type: 'vl',
          item_id: item.item_id,
          title: item.title,
          video_title: item.title,
        },
        storageType: sign.fileStorages[0] ?? '',
        probeLoading: false,
        baseInfo: null,
        baseLoading: false,
        duplicateConfirmed: false,
        status: 'auto',
        error: '',
      }))
      .map((assignment) => preserveProbe(assignment, previousAssignments.get(assignment.file.id)))
    assignments.value.forEach((assignment) => {
      void loadBaseInfo(assignment)
    })
    error.value = ''
  }

  function chooseSeries(item: VideoTreeItem) {
    const previousAssignments = new Map(assignments.value.map((assignment) => [assignment.file.id, assignment]))
    selectedSeries.value = item
    selectedMovie.value = null
    assignments.value = files.value.map((file) => preserveProbe(createAssignment(file, item), previousAssignments.get(file.id)))
    assignments.value.forEach((assignment) => {
      if (assignment.target) void loadBaseInfo(assignment)
    })
    error.value = ''
  }

  function updateAssignmentTarget(assignment: BatchAssignment) {
    if (!selectedSeries.value) {
      return
    }
    const option = episodeOptions.value.find((candidate) => candidate.key === assignment.selectedEpisodeKey)
    if (!option) {
      assignment.target = null
      assignment.status = 'unmatched'
      return
    }
    assignment.target = {
      item_type: 've',
      item_id: option.itemId,
      title: targetTitle(selectedSeries.value, option),
      video_title: selectedSeries.value.title,
      season_number: option.seasonNumber,
      episode_number: option.episodeNumber,
    }
    void loadBaseInfo(assignment)
    assignment.seasonNumber = option.seasonNumber
    assignment.episodeNumber = option.episodeNumber
    assignment.status = 'manual'
    assignment.error = ''
  }

  async function loadBaseInfo(assignment: BatchAssignment) {
    if (!assignment.target) return
    assignment.baseLoading = true
    assignment.baseInfo = null
    try {
      assignment.baseInfo = await api
        .get('api/upload/video/base', {
          searchParams: {
            item_type: assignment.target.item_type,
            item_id: String(assignment.target.item_id),
          },
        })
        .json<VideoBaseInfo>()
      const hasExistingResource = assignment.baseInfo.video_medias.some((media) => media.media_file_size === assignment.file.size)
      if (hasExistingResource) assignment.enabled = false
    } catch (requestError) {
      assignment.error = await errorMessage(requestError)
    } finally {
      assignment.baseLoading = false
    }
  }

  async function probeAssignment(assignment: BatchAssignment) {
    assignment.probeLoading = true
    assignment.probe = null
    assignment.error = ''
    try {
      assignment.probe = await api.post('api/files/probe', { json: { path: assignment.file.path } }).json<ProbeResult>()
      if (!assignment.probe.valid) {
        assignment.error = assignment.probe.error ?? '文件未通过视频校验'
      }
    } catch (requestError) {
      assignment.error = await errorMessage(requestError)
    } finally {
      assignment.probeLoading = false
    }
  }

  async function searchTargets() {
    searchBusy.value = true
    error.value = ''
    clearSelection(false)
    try {
      const params: Record<string, string> = { type: mode.value }
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
      searchResults.value = await api
        .get('api/video/tree', {
          searchParams: params,
        })
        .json<VideoTreeItem[]>()
      const onlySeries = searchResults.value.length === 1 ? searchResults.value[0] : null
      if (mode.value === 'tv' && onlySeries?.video_type === 'tv' && directoryContainsSeriesTitle(onlySeries, directory.value)) {
        chooseSeries(onlySeries)
        searchResults.value = []
        return
      }
      if (searchResults.value.length === 0) {
        error.value = '没有找到匹配的视频条目'
      }
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      searchBusy.value = false
    }
  }

  async function saveDirectoryTodbId() {
    const path = directory.value?.path
    const value = String(searchTodbId.value ?? '').trim()
    if (!path || !isPositiveInteger(value)) return
    try {
      await api.post('api/files/directory-metadata', { json: { path, todb_id: Number(value) } })
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    }
  }

  async function inspectDirectory(nextDirectory: DirectorySelection | null) {
    const sequence = ++loadSequence
    files.value = []
    probeCompleted.value = 0
    probeTotal.value = 0
    clearSelection()
    error.value = ''
    notice.value = ''
    if (!nextDirectory) {
      return
    }
    directoryLoading.value = true
    try {
      const data = await api
        .post('api/files/scan', {
          json: {
            path: nextDirectory.path,
            recursive: false,
          },
        })
        .json<{ path: string; files: SourceFile[] }>()
      if (sequence !== loadSequence) {
        return
      }
      files.value = data.files
      if (files.value.length === 0) {
        error.value = '目录中没有可处理的视频文件'
        return
      }
      mode.value = files.value.some((file) => parseEpisode(file.name).episodeNumber !== null) ? 'tv' : 'movie'
      assignments.value = files.value.map((file) => createAssignment(file, null))
      searchTodbId.value = folderTodbId.value
      try {
        const saved = await api.get('api/files/directory-metadata', { searchParams: { path: nextDirectory.path } }).json<{ todb_id: number }>()
        if (saved.todb_id > 0) searchTodbId.value = String(saved.todb_id)
      } catch {
        /* directory metadata is optional */
      }
      searchTitle.value = ''
      probeTotal.value = assignments.value.length
      let cursor = 0
      const worker = async () => {
        while (cursor < assignments.value.length) {
          const assignment = assignments.value[cursor++]
          if (!assignment) continue
          await probeAssignment(assignment)
          probeCompleted.value += 1
        }
      }
      await Promise.all(Array.from({ length: Math.min(4, assignments.value.length) }, () => worker()))
    } catch (requestError) {
      if (sequence === loadSequence) {
        error.value = await errorMessage(requestError)
      }
    } finally {
      if (sequence === loadSequence) {
        directoryLoading.value = false
      }
    }
  }

  function requestCreateUploadTasks() {
    if (canCreateTasks.value) showCreateConfirm.value = true
  }

  function applyStorageToAll(storage: string) {
    if (!sign.fileStorages.includes(storage)) {
      return
    }
    bulkStorageType.value = storage
    assignments.value.forEach((assignment) => {
      assignment.storageType = storage
    })
  }

  function cancelCreateUploadTasks() {
    showCreateConfirm.value = false
  }

  async function createUploadTasks() {
    if (!canCreateTasks.value) {
      return
    }
    showCreateConfirm.value = false
    createTaskBusy.value = true
    notice.value = ''
    error.value = ''
    const pending = matchedAssignments.value.filter((assignment) => assignment.probe?.valid && !assignment.baseLoading)
    createTaskProgress.value = { completed: 0, total: pending.length }
    let cursor = 0
    let created = 0
    const worker = async () => {
      while (cursor < pending.length) {
        const assignment = pending[cursor]
        cursor += 1
        if (!assignment?.target) {
          continue
        }
        if (assignment.baseInfo?.video_medias.some((media) => media.media_file_size === assignment.file.size) && !assignment.duplicateConfirmed) {
          assignment.error = '存在相同大小的已有资源，请确认后再创建'
          continue
        }
        try {
          await api
            .post('api/upload/tasks', {
              json: {
                path: assignment.file.path,
                task_type: 'video',
                item_type: assignment.target.item_type,
                item_id: assignment.target.item_id,
                season_number: assignment.target.season_number,
                episode_number: assignment.target.episode_number,
                video_title: assignment.target.video_title,
                file_storage: assignment.storageType,
                generate_sprites: false,
              },
            })
            .json<{ task_id: string }>()
          assignment.status = 'queued'
          assignment.error = ''
          created += 1
        } catch (requestError) {
          assignment.status = 'error'
          assignment.error = await errorMessage(requestError)
        } finally {
          createTaskProgress.value.completed += 1
        }
      }
    }
    try {
      await Promise.all(Array.from({ length: Math.min(3, pending.length) }, () => worker()))
      notice.value = created === pending.length ? `已创建 ${created} 个上传任务` : `已创建 ${created} 个上传任务，${pending.length - created} 个失败`
    } finally {
      createTaskBusy.value = false
    }
  }

  function setMode(nextMode: 'movie' | 'tv') {
    if (mode.value === nextMode) {
      return
    }
    mode.value = nextMode
    clearSelection()
  }

  watch(
    directory,
    (nextDirectory) => {
      void inspectDirectory(nextDirectory)
    },
    { immediate: true },
  )
  watch(
    () => sign.fileStorages,
    (nextStorages) => {
      const defaultStorage = nextStorages[0] ?? ''
      if (!nextStorages.includes(bulkStorageType.value)) {
        bulkStorageType.value = defaultStorage
      }
      assignments.value.forEach((assignment) => {
        if (!nextStorages.includes(assignment.storageType)) {
          assignment.storageType = defaultStorage
        }
      })
    },
    { immediate: true },
  )

  return {
    files,
    directoryLoading,
    probeCompleted,
    probeTotal,
    mode,
    searchTitle,
    searchTodbId,
    searchBusy,
    searchResults,
    selectedVideo,
    activeAssignments,
    selectedMovie,
    selectedSeries,
    assignments,
    episodeOptions,
    unmatchedAssignments,
    matchedAssignments,
    createTaskBusy,
    createTaskProgress,
    storageOptions,
    bulkStorageType,
    folderTodbId,
    canCreateTasks,
    createBlocker,
    probingAssignments,
    invalidAssignments,
    duplicateAssignments,
    error,
    notice,
    setMode,
    searchTargets,
    chooseMovie,
    chooseSeries,
    updateAssignmentTarget,
    probeAssignment,
    applyStorageToAll,
    saveDirectoryTodbId,
    createUploadTasks,
    requestCreateUploadTasks,
    cancelCreateUploadTasks,
    showCreateConfirm,
  }
}
