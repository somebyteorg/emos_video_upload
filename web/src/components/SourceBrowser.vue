<script lang="ts" setup>
import {onMounted, watch} from 'vue'
import {useSourceBrowser} from '@/composables/useSourceBrowser'
import {formatBytes, formatDate} from '@/utils/format'
import type {DirectoryEntry, DirectorySelection, SourceFile} from '@/types'

const emit = defineEmits<{
    'select-file': [file: SourceFile | null]
    'select-directory': [directory: DirectorySelection | null]
}>()

const {
    currentPath,
    rootPath,
    directories,
    files,
    selectedFile,
    directoryLoading,
    folderTodbId,
    canGoUp,
    error,
    initialize,
    openDirectory,
    goUp,
    scanCurrentDirectory,
    selectFile,
} = useSourceBrowser()

function handleSelectFile(file: SourceFile) {
    selectFile(file)
}

function handleSelectDirectory(directory: DirectoryEntry | DirectorySelection) {
    selectFile(null)
    emit('select-directory', directory)
}

watch(selectedFile, (file) => {
    emit('select-file', file)
})

onMounted(() => {
    void initialize()
})
</script>

<template>
    <aside class="min-w-0 lg:sticky lg:top-[18px]">
        <section class="min-w-0 overflow-hidden rounded-lg border border-line bg-white p-5 shadow-[0_5px_18px_rgb(28_52_60/3.5%)] max-sm:p-[15px]">
            <div class="mb-[19px] flex min-w-0 items-center justify-between gap-3.5">
                <div class="min-w-0">
                    <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">视频文件</span>
                    <h2 class="mt-1 truncate text-xl leading-tight text-ink max-sm:text-lg">选择要上传的视频</h2>
                </div>
                <button :disabled="directoryLoading" class="shrink-0 min-h-[34px] rounded-md border border-line bg-[#f7f9fa] px-[11px] text-xs font-bold text-[#54666d] transition hover:border-[#afd0ca] hover:bg-accent-soft hover:text-accent-dark" @click="openDirectory(currentPath)">
                    刷新
                </button>
            </div>
            <div class="grid min-w-0 gap-[7px] border-l-[3px] border-[#76b3a6] bg-[#f6f9f9] px-3 py-[11px]">
                <span class="text-xs text-muted">当前目录</span>
                <code class="block min-w-0 max-w-full truncate font-mono text-[11px] text-[#41565e]">{{ currentPath || rootPath || '读取中…' }}</code>
            </div>
            <div class="my-[13px] grid grid-cols-[max-content_max-content_minmax(0,1fr)] gap-2.5 max-sm:grid-cols-2">
                <button :disabled="!canGoUp" class="min-h-10 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-4 text-[13px] font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" @click="goUp">
                    上一级
                </button>
                <span v-if="folderTodbId" class="inline-flex w-max items-center rounded-[3px] bg-accent-soft px-[7px] py-1 text-[11px] font-bold text-[#356d61]">todb_id {{ folderTodbId }}</span>
            </div>
            <p v-if="error" class="mb-[14px] mt-[-7px] text-xs leading-5 text-danger">{{ error }}</p>
            <div class="grid gap-0">
                <div class="flex justify-between border-b border-line-soft pb-2 text-xs font-bold text-muted">子目录 <span class="font-medium text-muted-light">{{ directories.length }}</span></div>
                <div v-if="directoryLoading" class="px-[3px] py-[18px] text-xs text-[#9aa5ab]">读取目录中…</div>
                <div v-else-if="directories.length === 0" class="px-[3px] py-[18px] text-xs text-[#9aa5ab]">没有子目录</div>
                <div
                    v-for="directory in directories"
                    :key="directory.path"
                    class="flex min-h-[47px] min-w-0 items-stretch border-b border-[#eef2f3] focus-within:bg-[#f1f8f6] max-sm:flex-col max-sm:py-1"
                >
                    <button class="flex min-w-0 flex-1 items-center gap-[9px] bg-transparent px-1 py-[7px] text-left text-[#34474f] hover:bg-[#f1f8f6] max-sm:w-full" @click="openDirectory(directory.path)">
                        <span class="grid size-[21px] shrink-0 place-items-center rounded-[3px] bg-[#e2f1ee] text-[17px] font-extrabold leading-none text-accent">/</span>
                        <span class="min-w-0 flex-1 truncate text-[13px]">{{ directory.name }}</span>
                        <small class="shrink-0 text-[11px] text-muted-light">{{ directory.file_count ?? 0 }} 个视频</small>
                    </button>
                    <button
                        class="my-auto mr-1 ml-2 shrink-0 rounded-[3px] border border-[#c2ded7] bg-accent-soft px-[7px] py-[5px] text-[11px] font-bold text-accent-dark hover:bg-[#d8ede8] max-sm:mb-1 max-sm:ml-auto max-sm:mr-1 max-sm:mt-0"
                        @click="handleSelectDirectory(directory)"
                    >
                        批量匹配
                    </button>
                </div>
            </div>
            <div class="mt-[22px] grid gap-0">
                <div class="flex justify-between border-b border-line-soft pb-2 text-xs font-bold text-muted">视频文件 <span class="font-medium text-muted-light">{{ files.length }}</span></div>
                <div v-if="files.length === 0" class="px-[3px] py-[18px] text-xs text-[#9aa5ab]">扫描目录后显示视频文件</div>
                <button
                    v-for="file in files"
                    :key="file.id"
                    :class="{ 'bg-[#f1f8f6]': selectedFile?.id === file.id }"
                    class="flex min-h-[47px] w-full items-center gap-[9px] border-b border-[#eef2f3] bg-transparent px-1 py-[7px] text-left text-[#34474f] hover:bg-[#f1f8f6]"
                    @click="handleSelectFile(file)"
                >
                    <span class="shrink-0 text-[9px] font-extrabold tracking-[0.05em] text-[#8b675c]">{{ file.name.split('.').pop()?.toUpperCase() || 'VIDEO' }}</span>
                    <span class="grid min-w-0 flex-1 gap-[3px]">
                        <strong class="truncate text-xs text-[#34474f]">{{ file.name }}</strong>
                        <small class="text-[10px] text-muted-light">{{ formatBytes(file.size) }} · {{ formatDate(file.modified_at) }}</small>
                    </span>
                </button>
            </div>
        </section>
    </aside>
</template>
