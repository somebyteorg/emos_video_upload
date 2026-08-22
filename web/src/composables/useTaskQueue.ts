import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import api, { errorMessage } from '@/utils/ky'
import type { TaskPage, TaskStatus } from '@/types'

export function useTaskQueue() {
  const tasks = ref<TaskStatus[]>([])
  const taskTotal = ref(0)
  const taskPage = ref(1)
  const taskPageSize = 20
  const tasksLoading = ref(false)
  const taskError = ref('')
  const deleteCandidate = ref<TaskStatus | null>(null)
  const speeds = reactive<Record<string, number>>({})
  const samples = new Map<string, { bytes: number; at: number }>()
  const taskPageCache = new Map<string, { etag: string; data: TaskPage }>()
  let pollTimer: number | undefined
  let polling = false

  function taskPageKey(page: number) {
    return `${page}:${taskPageSize}`
  }

  async function requestTaskPage(page: number) {
    const key = taskPageKey(page)
    const cached = taskPageCache.get(key)
    const response = await api.get('api/tasks', {
      searchParams: { limit: String(taskPageSize), page: String(page) },
      headers: cached?.etag ? { 'If-None-Match': cached.etag } : undefined,
      throwHttpErrors: false,
    })
    if (response.status === 304) {
      return cached?.data ?? null
    }
    const data = await response.json<TaskPage>()
    const etag = response.headers.get('ETag')
    if (etag) {
      taskPageCache.set(key, { etag, data })
    }
    return data
  }

  async function loadTasks(nextPage = taskPage.value) {
    tasksLoading.value = true
    taskError.value = ''
    try {
      const nextTasks = await requestTaskPage(nextPage)
      if (!nextTasks) {
        return
      }
      updateSpeeds(nextTasks.tasks)
      tasks.value = nextTasks.tasks
      taskTotal.value = nextTasks.total
      taskPage.value = nextTasks.page
    } catch (error) {
      taskError.value = await errorMessage(error)
    } finally {
      tasksLoading.value = false
    }
  }

  function updateSpeeds(nextTasks: TaskStatus[]) {
    const now = Date.now()
    for (const task of nextTasks) {
      const previous = samples.get(task.task_id)
      if (previous) {
        const elapsed = (now - previous.at) / 1000
        const delta = task.uploaded_bytes - previous.bytes
        speeds[task.task_id] = elapsed > 0 && delta > 0 ? delta / elapsed : 0
      }
      samples.set(task.task_id, { bytes: task.uploaded_bytes, at: now })
      if (task.status !== 'running' && task.status !== 'queued') {
        speeds[task.task_id] = 0
      }
    }
  }

  function taskSpeed(task: TaskStatus) {
    const sample = samples.get(task.task_id)
    if (!sample || Date.now() - sample.at > 4000) {
      return 0
    }
    return speeds[task.task_id] ?? 0
  }

  function startPolling() {
    stopPolling()
    pollTimer = window.setInterval(() => {
      if (!polling) {
        void pollTaskList()
      }
    }, 1500)
  }

  function stopPolling() {
    if (pollTimer !== undefined) {
      window.clearInterval(pollTimer)
      pollTimer = undefined
    }
  }

  async function pollTaskList() {
    polling = true
    taskError.value = ''
    try {
      const nextTasks = await requestTaskPage(taskPage.value)
      if (!nextTasks) {
        return
      }
      updateSpeeds(nextTasks.tasks)
      tasks.value = nextTasks.tasks
      taskTotal.value = nextTasks.total
    } catch (error) {
      taskError.value = await errorMessage(error)
    } finally {
      polling = false
    }
  }

  async function retryTask(task: TaskStatus) {
    taskError.value = ''
    try {
      await api.post(`api/tasks/${task.task_id}/retry`).json<TaskStatus>()
      await loadTasks()
    } catch (error) {
      taskError.value = await errorMessage(error)
    }
  }

  async function deleteTask(task: TaskStatus) {
    deleteCandidate.value = task
  }

  async function deleteCompletedTasks() {
    taskError.value = ''
    try {
      await api.delete('api/tasks/completed')
      await loadTasks()
    } catch (error) {
      taskError.value = await errorMessage(error)
    }
  }

  async function confirmDeleteTask() {
    const task = deleteCandidate.value
    if (!task) return
    deleteCandidate.value = null
    taskError.value = ''
    try {
      await api.delete(`api/tasks/${task.task_id}`)
      await loadTasks()
    } catch (error) {
      taskError.value = await errorMessage(error)
    }
  }

  onMounted(() => {
    void loadTasks()
    startPolling()
  })

  onBeforeUnmount(stopPolling)

  return {
    tasks,
    taskTotal,
    taskPage,
    taskPageSize,
    loadTaskPage: loadTasks,
    tasksLoading,
    taskError,
    taskSpeed,
    loadTasks,
    retryTask,
    deleteTask,
    deleteCandidate,
    confirmDeleteTask,
    cancelDeleteTask: () => {
      deleteCandidate.value = null
    },
    deleteCompletedTasks,
  }
}
