<script lang="ts" setup>
import {onMounted, ref} from 'vue'
import {useAuth} from '@/composables/useAuth'
import SourceBrowser from '@/components/SourceBrowser.vue'
import DirectoryInspector from '@/components/DirectoryInspector.vue'
import VideoInspector from '@/components/VideoInspector.vue'
import TaskList from '@/components/TaskList.vue'
import type {DirectorySelection, SourceFile} from '@/types'

const selectedFile = ref<SourceFile | null>(null)
const selectedDirectory = ref<DirectorySelection | null>(null)
const {
    authReady,
    loginBusy,
    loginError,
    loginUsername,
    loginPassword,
    isAuthenticated,
    authEnabled,
    checkAuth,
    login,
    logout,
} = useAuth()

onMounted(() => {
    void checkAuth()
})

function handleSelectFile(file: SourceFile | null) {
    selectedFile.value = file
    if (file) {
        selectedDirectory.value = null
    }
}

function handleSelectDirectory(directory: DirectorySelection | null) {
    selectedDirectory.value = directory
    if (directory) {
        selectedFile.value = null
    }
}
</script>

<template>
    <main class="min-h-screen bg-canvas">
        <section v-if="!authReady" class="grid min-h-screen place-items-center p-6 text-muted">
            <div class="flex items-center gap-3">
                <div class="size-[18px] animate-spin rounded-full border-2 border-[#c9dfdb] border-t-accent"></div>
                <p>正在连接服务</p>
            </div>
        </section>

        <section v-else-if="authEnabled && !isAuthenticated" class="grid min-h-screen place-items-center p-6">
            <form class="grid w-full max-w-[360px] gap-4 rounded-lg border border-line bg-white p-9 shadow-[0_16px_40px_rgb(35_51_61/8%)]" @submit.prevent="login">
                <div class="w-max bg-accent px-2 py-1 text-[11px] font-extrabold tracking-[0.08em] text-white">EMOS</div>
                <h1 class="m-0 text-[28px] text-ink">视频上传</h1>
                <label class="grid gap-2 text-[13px] font-semibold text-[#63717b]">
                    <span>用户名</span>
                    <input v-model="loginUsername" autocomplete="username" autofocus class="min-h-[42px] w-full rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 text-ink outline-none focus:border-accent" />
                </label>
                <label class="grid gap-2 text-[13px] font-semibold text-[#63717b]">
                    <span>密码</span>
                    <input v-model="loginPassword" autocomplete="current-password" class="min-h-[42px] w-full rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 text-ink outline-none focus:border-accent" type="password" />
                </label>
                <p v-if="loginError" class="m-0 text-xs leading-5 text-danger">{{ loginError }}</p>
                <button :disabled="loginBusy" class="min-h-10 w-full rounded-md bg-accent px-4 text-[13px] font-bold text-white shadow-[0_2px_5px_rgb(20_120_109/16%)] transition hover:bg-accent-dark hover:shadow-[0_4px_9px_rgb(20_120_109/20%)]">
                    {{ loginBusy ? '登录中…' : '登录' }}
                </button>
            </form>
        </section>

        <template v-else>
            <header class="flex min-h-[68px] items-center justify-between border-b border-line bg-white/95 px-[clamp(16px,4vw,64px)]">
                <div class="flex items-center gap-[11px]">
                    <div class="bg-accent px-2 py-[7px] text-[10px] font-extrabold tracking-[0.08em] text-white">EMOS</div>
                    <div>
                        <strong class="block text-[15px] text-ink">视频上传</strong>
                    </div>
                </div>
                <div class="flex items-center gap-5">
                    <button class="bg-transparent p-0 text-[13px] font-bold text-accent transition hover:text-accent-dark" @click="logout">退出</button>
                </div>
            </header>

            <div class="mx-auto mt-[18px] mb-16 grid w-[min(1480px,calc(100%_-_36px))] items-start gap-[18px] pb-4 lg:grid-cols-[minmax(300px,0.78fr)_minmax(0,1.6fr)] max-lg:mb-20 max-lg:w-[min(calc(100%_-_20px),1480px)] max-lg:gap-3">
                <SourceBrowser @select-file="handleSelectFile" @select-directory="handleSelectDirectory" />
                <section class="grid min-w-0 gap-[18px]">
                    <DirectoryInspector v-if="selectedDirectory" :directory="selectedDirectory" />
                    <VideoInspector v-else :file="selectedFile" />
                    <TaskList />
                </section>
            </div>
        </template>
    </main>
</template>
