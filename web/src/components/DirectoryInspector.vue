<script lang="ts" setup>
import {computed, ref, toRef} from 'vue'
import {useBatchInspector} from '@/composables/useBatchInspector'
import {formatBytes, formatDate} from '@/utils/format'
import SelectMenu from '@/components/SelectMenu.vue'
import type {DirectorySelection, SourceFile, VideoBaseInfo} from '@/types'

const props = defineProps<{
    directory: DirectorySelection | null
}>()

const {
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
    selectedSeries,
    assignments,
    episodeOptions,
    unmatchedAssignments,
    matchedAssignments,
    invalidAssignments,
    probingAssignments,
    createTaskBusy,
    createTaskProgress,
    storageOptions,
    bulkStorageType,
    folderTodbId,
    canCreateTasks,
    createBlocker,
    error,
    notice,
    setMode,
    searchTargets,
    chooseMovie,
    chooseSeries,
    updateAssignmentTarget,
    probeAssignment,
    saveDirectoryTodbId,
    createUploadTasks,
    requestCreateUploadTasks,
    cancelCreateUploadTasks,
    showCreateConfirm,
} = useBatchInspector(toRef(props, 'directory'))

const allEnabled = computed(() => assignments.value.length > 0 && assignments.value.every((assignment) => assignment.enabled))
const mediaAssignment = ref<{file: SourceFile; baseInfo: VideoBaseInfo | null} | null>(null)
function toggleAll(enabled: boolean) {
    assignments.value.forEach((assignment) => { assignment.enabled = enabled })
}

function statusLabel(status: string) {
    switch (status) {
        case 'auto':
            return '自动匹配'
        case 'manual':
            return '手动匹配'
        case 'queued':
            return '已创建'
        case 'error':
            return '创建失败'
        default:
            return '待匹配'
    }
}

function probeLabel(assignment: {probeLoading: boolean; probe: {valid: boolean} | null; error: string}) {
    if (assignment.probeLoading) return '校验中'
    if (assignment.probe?.valid) return '校验通过'
    if (assignment.probe || assignment.error) return '校验失败'
    return '待校验'
}
</script>

<template>
    <section class="min-w-0 max-w-full overflow-hidden rounded-lg border border-line bg-white p-5 shadow-[0_5px_18px_rgb(28_52_60/3.5%)] max-sm:p-[15px]">
        <div class="mb-[19px] flex min-w-0 items-center justify-between gap-3.5">
            <div class="min-w-0">
                <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">批量上传</span>
                <h2 class="mt-1 max-w-full truncate text-xl leading-tight text-ink max-sm:text-lg">{{ directory?.name || '选择视频目录' }}</h2>
            </div>
            <span class="inline-flex w-max shrink-0 items-center rounded-[3px] bg-accent-soft px-[7px] py-1 text-[11px] font-bold text-[#356d61]">{{ files.length }} 个视频</span>
        </div>

        <div class="grid gap-1.5 border-l-[3px] border-[#76b3a6] bg-[#f6f9f9] px-3 py-[11px]">
            <span class="text-xs text-muted">上传目录</span>
            <code class="truncate font-mono text-[11px] text-[#41565e]">{{ directory?.path || '读取中…' }}</code>
        </div>

        <div v-if="directoryLoading" class="flex min-h-[180px] items-center gap-[11px] text-[13px] text-[#718089]">
            <div class="size-[18px] animate-spin rounded-full border-2 border-[#c9dfdb] border-t-accent"></div>
            <span>正在读取视频并校验（{{ probeCompleted }}/{{ probeTotal }}）…</span>
            <strong v-if="probeTotal" class="ml-auto">{{ Math.round(probeCompleted / probeTotal * 100) }}%</strong>
        </div>

        <template v-else>
            <p v-if="error" class="mt-[14px] text-xs leading-5 text-danger">{{ error }}</p>
            <p v-if="notice" class="mt-[14px] border border-[#c2ded7] bg-accent-soft px-3.5 py-[11px] text-[13px] text-[#327164]">{{ notice }}</p>

            <div class="my-6 h-px bg-[#e9edef]"></div>

            <div class="grid gap-2.5">
                <div class="flex items-start gap-3.5 max-sm:flex-wrap">
                    <div class="min-w-0">
                        <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">匹配目标</span>
                    </div>
                    <div aria-label="视频类型" class="ml-auto inline-flex w-max shrink-0 rounded-[5px] border border-[#e2e7e8] bg-[#f1f4f4] p-[3px] max-sm:ml-0">
                        <button :class="{ 'bg-white text-[#205c49] shadow-[0_1px_4px_rgb(50_74_67/12%)]': mode === 'movie' }" class="min-w-[84px] rounded-[3px] bg-transparent px-3.5 py-[7px] text-xs font-bold text-[#7a878e]" type="button" @click="setMode('movie')">电影</button>
                        <button :class="{ 'bg-white text-[#205c49] shadow-[0_1px_4px_rgb(50_74_67/12%)]': mode === 'tv' }" class="min-w-[84px] rounded-[3px] bg-transparent px-3.5 py-[7px] text-xs font-bold text-[#7a878e]" type="button" @click="setMode('tv')">电视剧</button>
                    </div>
                </div>

                <div class="grid grid-cols-[minmax(220px,1fr)_140px_auto] gap-2 max-sm:grid-cols-1">
                    <div class="relative min-w-0">
                        <input v-model="searchTitle" class="min-h-[42px] w-full rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 pr-8 text-ink outline-none focus:border-accent" placeholder="视频标题" @keydown.enter="searchTargets" />
                        <button v-if="searchTitle" aria-label="清除视频标题" class="absolute right-2 top-1/2 grid size-5 -translate-y-1/2 place-items-center rounded-full bg-transparent text-[17px] leading-none text-muted hover:bg-accent-soft hover:text-accent-dark" type="button" @click="searchTitle = ''">×</button>
                    </div>
                    <input v-model="searchTodbId" class="min-h-[42px] w-[140px] rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 text-ink outline-none focus:border-accent max-sm:w-full" inputmode="numeric" pattern="[0-9]*" placeholder="目录 todb_id" @blur="saveDirectoryTodbId" @keydown.enter="saveDirectoryTodbId" />
                    <button :disabled="searchBusy || (!searchTitle.trim() && !searchTodbId.trim() && !folderTodbId)" class="min-h-10 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-4 text-[13px] font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" @click="searchTargets">
                        {{ searchBusy ? '搜索中…' : '搜索' }}
                    </button>
                </div>

                <div v-if="searchResults.length" class="max-h-[260px] overflow-auto border-y border-[#e8edee]">
                    <template v-for="item in searchResults" :key="`${item.item_id}-${item.title}`">
                        <button v-if="mode === 'movie'" class="flex w-full min-w-0 items-center justify-between gap-3 border-b border-[#eef1f2] bg-transparent px-1 py-3 text-left text-[#33434d] transition hover:bg-[#f7faf8] hover:text-[#167858]" type="button" @click="chooseMovie(item)">
                            <span class="grid min-w-0 gap-1">
                                <strong class="truncate text-[13px]">{{ item.title }}</strong>
                                <small class="truncate text-[11px] text-[#91a0a7]">todb_id {{ item.todb_id }}<span v-if="item.date_air"> · {{ item.date_air }}</span></small>
                            </span>
                            <span class="max-w-[42%] shrink-0 text-right text-xs font-bold text-[#167858]">匹配到这部电影</span>
                        </button>
                        <div v-else class="border-b border-[#e8edee] px-1 py-3.5">
                            <div class="mb-[11px] flex items-baseline gap-[9px]">
                                <strong class="text-[13px] text-[#33434d]">{{ item.title }}</strong>
                                <small class="text-[11px] text-[#91a0a7]">todb_id {{ item.todb_id }} · {{ item.seasons?.length ?? 0 }} 季</small>
                            </div>
                            <div class="flex items-center justify-between gap-3 text-xs text-muted">
                                <span>{{ item.seasons?.reduce((total, season) => total + season.episodes.length, 0) ?? 0 }} 集</span>
                                <button class="min-h-8 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-2.5 text-xs font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" type="button" @click="chooseSeries(item)">匹配到这部剧</button>
                            </div>
                        </div>
                    </template>
                </div>
            </div>

            <div v-if="selectedVideo">
                <div class="my-6 h-px bg-[#e9edef]"></div>
                <div class="mb-3.5 flex min-w-0 items-center justify-start gap-4 rounded-md border border-[#e1ece9] bg-[#f7faf9] px-[13px] py-[11px] max-[760px]:items-stretch max-[760px]:flex-col">
                    <div class="flex min-w-0 items-baseline gap-[9px]">
                        <span class="shrink-0 text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">任务设置</span>
                        <strong class="truncate text-xs text-[#35554f]">{{ selectedVideo.title }}</strong>
                        <span class="shrink-0 text-[13px] text-muted">{{ assignments.length }} 个视频</span>
                    </div>
                    <span class="ml-auto whitespace-nowrap text-xs text-muted max-[760px]:ml-0">统一储存位置</span>
                    <div class="min-w-0 flex-1 max-[760px]:w-full">
                        <SelectMenu v-model="bulkStorageType" :options="storageOptions" aria-label="批量储存位置" @update:model-value="applyStorageToAll" />
                    </div>
                </div>

                <div class="w-full min-w-0 max-w-full overflow-x-auto border-y border-[#e4ecee]">
                    <div class="min-w-[760px]">
                        <div class="grid min-w-[760px] grid-cols-[42px_minmax(135px,0.7fr)_minmax(220px,1.2fr)_minmax(150px,0.75fr)_minmax(250px,1.3fr)] items-center gap-3.5 px-1 py-[9px] text-[11px] font-bold text-[#87959b]">
                            <label aria-label="选择全部视频" class="inline-flex items-center gap-[5px] text-[11px] text-muted"><input :checked="allEnabled" class="accent-accent" type="checkbox" @change="toggleAll(($event.target as HTMLInputElement).checked)" /></label>
                            <span>视频文件</span>
                            <span>上传目标</span>
                            <span>储存位置</span>
                            <span>校验与状态</span>
                        </div>
                        <div v-for="assignment in assignments" :key="assignment.file.id" :class="{ 'bg-[#fafcfc]': !assignment.enabled }" class="grid min-h-[74px] min-w-[760px] grid-cols-[42px_minmax(135px,0.7fr)_minmax(220px,1.2fr)_minmax(150px,0.75fr)_minmax(250px,1.3fr)] items-center gap-3.5 border-t border-[#eef2f3] px-1 py-[11px]">
                            <label class="grid place-items-center"><input v-model="assignment.enabled" class="size-4 accent-accent" type="checkbox" /></label>
                            <div :class="{ 'opacity-55': !assignment.enabled }" class="grid min-w-0 gap-[5px]">
                                <strong class="truncate text-xs text-[#354a52]">{{ assignment.file.name }}</strong>
                                <small class="truncate text-[10px] text-muted">{{ formatBytes(assignment.file.size) }} · {{ formatDate(assignment.file.modified_at) }}</small>
                            </div>
                            <div :class="{ 'opacity-55': !assignment.enabled }" class="grid min-w-0 gap-[5px]">
                                <span v-if="!selectedSeries" class="text-[11px] font-bold text-[#47756b]">{{ assignment.episodeHint }}</span>
                                <SelectMenu
                                    v-if="selectedSeries"
                                    v-model="assignment.selectedEpisodeKey"
                                    :options="episodeOptions.map((episode) => ({value: episode.key, label: episode.label}))"
                                    aria-label="选择剧集"
                                    placeholder="选择集数"
                                    @update:model-value="updateAssignmentTarget(assignment)"
                                />
                                <span v-else class="truncate text-xs font-bold text-[#354a52]">{{ assignment.target?.title || '未匹配' }}</span>
                            </div>
                            <div :class="{ 'opacity-55': !assignment.enabled }">
                                <SelectMenu v-model="assignment.storageType" :options="storageOptions" aria-label="文件储存位置" />
                            </div>
                            <div :class="[{ 'text-danger': assignment.probe && !assignment.probe.valid }, { 'opacity-55': !assignment.enabled }]" class="grid min-w-0 gap-[3px] text-[11px] text-[#47756b]">
                                <span>{{ probeLabel(assignment) }}</span>
                                <small class="truncate text-[10px] text-inherit">{{ statusLabel(assignment.status) }}</small>
                                <button v-if="assignment.baseInfo?.video_medias.length" class="w-max bg-transparent p-0 text-[10px] font-bold text-accent-dark hover:underline" type="button" @click="mediaAssignment = assignment">查看已有资源（{{ assignment.baseInfo.video_medias.length }}）</button>
                                <button v-if="assignment.probe && !assignment.probe.valid || assignment.error && !assignment.probe" :disabled="assignment.probeLoading" class="w-max bg-transparent p-0 text-[10px] font-bold text-accent-dark hover:text-accent hover:underline" type="button" @click="probeAssignment(assignment)">重新校验</button>
                                <label v-if="assignment.baseInfo?.video_medias.some((media) => media.media_file_size === assignment.file.size)" class="inline-flex w-max items-center gap-[5px] text-[10px] text-warning">
                                    <input v-model="assignment.duplicateConfirmed" class="accent-accent" type="checkbox" @change="assignment.duplicateConfirmed && (assignment.enabled = true)" /> 确认重复仍上传
                                </label>
                                <small v-if="assignment.error" class="max-w-[190px] text-danger">{{ assignment.error }}</small>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="mt-[17px] grid grid-cols-[minmax(0,1fr)_auto] items-end gap-4 max-[760px]:grid-cols-1 max-[760px]:items-stretch">
                    <div class="grid min-w-0 gap-1">
                        <strong class="text-[13px] text-[#40545c]">{{ matchedAssignments.length }} / {{ activeAssignments.length }} 个文件已匹配</strong>
                        <span v-if="probingAssignments.length" class="text-xs text-muted">正在校验 {{ probingAssignments.length }} 个文件</span>
                        <span v-else-if="createBlocker" class="text-xs text-danger">{{ createBlocker }}</span>
                    </div>
                    <button :disabled="!canCreateTasks" class="min-h-10 min-w-[170px] rounded-md bg-accent px-4 text-[13px] font-bold text-white shadow-[0_2px_5px_rgb(20_120_109/16%)] transition hover:bg-accent-dark hover:shadow-[0_4px_9px_rgb(20_120_109/20%)] max-[760px]:w-full" type="button" @click="requestCreateUploadTasks">
                        {{ createTaskBusy ? `创建中 ${createTaskProgress.completed}/${createTaskProgress.total}` : `创建 ${matchedAssignments.length} 个任务` }}
                    </button>
                </div>
            </div>
        </template>
    </section>

    <div v-if="showCreateConfirm" class="fixed inset-0 z-20 grid place-items-center bg-slate-900/40 p-6" @click.self="cancelCreateUploadTasks">
        <section aria-labelledby="batch-create-title" aria-modal="true" class="w-full max-w-[420px] rounded-lg border border-line bg-white p-6 shadow-[0_18px_48px_rgb(15_23_42/20%)]" role="dialog">
            <h3 id="batch-create-title" class="m-0 mb-2 text-base text-ink">确认创建上传任务</h3>
            <p class="m-0 text-[13px] text-muted">将为 {{ matchedAssignments.length }} 个文件创建上传任务。确认继续吗？</p>
            <div class="mt-6 flex justify-end gap-2.5">
                <button class="min-h-10 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-4 text-[13px] font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" type="button" @click="cancelCreateUploadTasks">取消</button>
                <button class="min-h-10 rounded-md bg-accent px-4 text-[13px] font-bold text-white transition hover:bg-accent-dark" type="button" @click="createUploadTasks">确认创建</button>
            </div>
        </section>
    </div>

    <div v-if="mediaAssignment" class="fixed inset-0 z-20 grid place-items-center bg-slate-900/40 p-6" @click.self="mediaAssignment = null">
        <section aria-modal="true" class="w-full max-w-[420px] rounded-lg border border-line bg-white p-6 shadow-[0_18px_48px_rgb(15_23_42/20%)]" role="dialog">
            <h3 class="m-0 mb-2 text-base text-ink">已有资源</h3>
            <p class="m-0 text-[13px] text-muted">{{ mediaAssignment.file.name }}</p>
            <ul class="mt-3.5 grid list-none gap-2 p-0 text-xs text-muted">
                <li v-for="media in mediaAssignment.baseInfo?.video_medias" :key="media.media_id">
                    <div class="flex justify-between gap-3 border-b border-line-soft pb-2">
                        <strong class="truncate text-[#40545c]">{{ media.media_name }}</strong><span class="shrink-0 whitespace-nowrap">{{ formatBytes(media.media_file_size) }}</span>
                    </div>
                </li>
            </ul>
            <div class="mt-6 flex justify-end gap-2.5"><button class="min-h-10 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-4 text-[13px] font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" type="button" @click="mediaAssignment = null">关闭</button></div>
        </section>
    </div>
</template>
