import { computed, ref } from 'vue'
import api, { errorMessage } from '@/utils/ky'
import type { DirectoryEntry, SourceFile } from '@/types'
import { parentPath } from '@/utils/path'

export function useSourceBrowser() {
  const currentPath = ref('')
  const rootPath = ref('')
  const directories = ref<DirectoryEntry[]>([])
  const files = ref<SourceFile[]>([])
  const selectedFile = ref<SourceFile | null>(null)
  const directoryLoading = ref(false)
  const scanLoading = ref(false)
  const error = ref('')

  const folderTodbId = computed(() => {
    const match = currentPath.value.match(/\[todbid=(\d+)\]/i)
    return match?.[1] ?? ''
  })
  const canGoUp = computed(() => Boolean(parentPath(currentPath.value, rootPath.value)))

  async function loadDirectory(path: string) {
    directoryLoading.value = true
    error.value = ''
    try {
      const data = await api
        .get('api/files/tree', {
          searchParams: path ? { path } : {},
        })
        .json<{ path: string; entries: DirectoryEntry[] }>()
      currentPath.value = data.path
      if (!rootPath.value) {
        rootPath.value = data.path
      }
      directories.value = data.entries.sort((left, right) => left.name.localeCompare(right.name))
      files.value = []
      selectedFile.value = null
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      directoryLoading.value = false
    }
  }

  async function scanCurrentDirectory() {
    if (!currentPath.value) {
      return
    }
    scanLoading.value = true
    error.value = ''
    try {
      const data = await api
        .post('api/files/scan', {
          json: { path: currentPath.value },
        })
        .json<{ path: string; files: SourceFile[] }>()
      files.value = data.files.sort((left, right) => left.name.localeCompare(right.name, undefined, { numeric: true }))
      selectedFile.value = null
    } catch (requestError) {
      error.value = await errorMessage(requestError)
    } finally {
      scanLoading.value = false
    }
  }

  async function openDirectory(path: string) {
    await loadDirectory(path)
    await scanCurrentDirectory()
  }

  async function goUp() {
    const target = parentPath(currentPath.value, rootPath.value)
    if (target) {
      await openDirectory(target)
    }
  }

  function selectFile(file: SourceFile | null) {
    selectedFile.value = file
    error.value = ''
  }

  async function initialize() {
    await loadDirectory('')
  }

  return {
    currentPath,
    rootPath,
    directories,
    files,
    selectedFile,
    directoryLoading,
    scanLoading,
    folderTodbId,
    canGoUp,
    error,
    initialize,
    openDirectory,
    goUp,
    scanCurrentDirectory,
    selectFile,
  }
}
