<script lang="ts" setup>
import {computed, ref, toRef, watch} from 'vue'
import {useVideoInspector} from '@/composables/useVideoInspector'
import {formatBytes, formatDuration} from '@/utils/format'
import SelectMenu from '@/components/SelectMenu.vue'
import type {SourceFile, VideoSeason, VideoTreeItem} from '@/types'

const props = defineProps<{
    file: SourceFile | null
}>()

const resultFilter = ref('')
const expandedSeasons = ref<Set<string>>(new Set())

const {
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
} = useVideoInspector(toRef(props, 'file'))

const displayedResults = computed(() => {
    const query = resultFilter.value.trim().toLowerCase()
    if (!query) {
        return searchResults.value
    }

    return searchResults.value.flatMap((item) => {
        const videoText = `${item.title} ${item.todb_id}`.toLowerCase()
        if (targetType.value === 'movie') {
            return videoText.includes(query) ? [item] : []
        }
        if (videoText.includes(query)) {
            return [item]
        }

        const seasons = (item.seasons ?? []).flatMap((season) => {
            const seasonText = `s${String(season.season_number).padStart(2, '0')} ${season.season_number} ${season.season_title}`.toLowerCase()
            if (seasonText.includes(query)) {
                return [season]
            }
            const episodes = season.episodes.filter((episode) => (
                `e${String(episode.episode_number).padStart(2, '0')} ${episode.episode_number} ${episode.episode_title}`
                    .toLowerCase()
                    .includes(query)
            ))
            return episodes.length ? [{...season, episodes}] : []
        })

        return seasons.length ? [{...item, seasons}] : []
    })
})

function seasonKey(item: VideoTreeItem, season: VideoSeason) {
    return `${item.item_id}:${season.item_id}`
}

function isSeasonExpanded(item: VideoTreeItem, season: VideoSeason) {
    return expandedSeasons.value.has(seasonKey(item, season))
}

function toggleSeason(item: VideoTreeItem, season: VideoSeason) {
    const key = seasonKey(item, season)
    const next = new Set(expandedSeasons.value)
    if (next.has(key)) {
        next.delete(key)
    } else {
        next.add(key)
    }
    expandedSeasons.value = next
}

function resetResultView(items: VideoTreeItem[] = []) {
    resultFilter.value = ''
    expandedSeasons.value = new Set(
        items.flatMap((item) => {
            const season = item.seasons?.[0]
            return season ? [seasonKey(item, season)] : []
        }),
    )
}

watch(searchResults, (items) => resetResultView(items))
</script>

<template>
    <section class="rounded-lg border border-line bg-white p-6 shadow-[0_5px_18px_rgb(28_52_60/3.5%)] max-sm:p-[17px]">
        <div class="mb-[19px] flex min-w-0 items-center justify-between gap-3.5">
            <div class="min-w-0">
                <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">识别</span>
                <h2 class="mt-1 truncate text-xl leading-tight text-ink max-sm:text-lg">{{ file ? file.name : '选择文件后开始识别' }}</h2>
            </div>
            <span v-if="file" class="inline-flex w-max shrink-0 items-center rounded-[3px] bg-accent-soft px-[7px] py-1 text-[11px] font-bold text-[#356d61]">{{ formatBytes(file.size) }}</span>
        </div>

        <div v-if="!file" class="grid min-h-[190px] place-items-center p-7 text-center text-[#728089]">
            <div class="grid">
                <div class="mx-auto grid size-11 place-items-center rounded-full border border-[#c3ded2] bg-[#e6f2ed] text-xs font-extrabold text-[#167858]">—</div>
                <strong class="mt-3.5 text-[15px] text-[#44545e]">从左侧选择一个视频文件</strong>
                <p class="mt-2 max-w-[360px] text-[13px] leading-[1.65]">系统会先执行 ffprobe 校验，再匹配 EMOS 中的电影或剧集。</p>
            </div>
        </div>

        <template v-else>
            <div v-if="probeLoading" class="flex min-h-[148px] items-center gap-[11px] text-[13px] text-[#718089]">
                <div class="size-[18px] animate-spin rounded-full border-2 border-[#c9dfdb] border-t-accent"></div>
                <span>正在分析视频文件…</span>
            </div>
            <div v-else-if="probe" class="grid gap-[17px]">
                <div class="flex items-start gap-2.5">
                    <span :class="{ 'bg-[#c45e50]': !probe.valid }" class="mt-[5px] size-[9px] shrink-0 rounded-full bg-[#1d936a]"></span>
                    <div>
                        <strong class="block text-[13px] text-[#32434c]">{{ probe.valid ? '视频校验通过' : '视频校验失败' }}</strong>
                        <span class="mt-1 block text-xs text-[#829099]">{{ probe.valid ? `${formatDuration(probe.summary.duration)} · ${probe.summary.dynamic_range || 'SDR'}` : probe.error }}</span>
                    </div>
                </div>
                <div class="grid grid-cols-3 gap-x-5 gap-y-[13px] border-t border-[#edf0f1] pt-[15px] pb-[3px] max-sm:grid-cols-2">
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">分辨率</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.width }} × {{ probe.summary.height }}</strong></div>
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">视频编码</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.video_codec || '—' }}</strong></div>
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">音频</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.audio_codec || '无音频' }} · {{ probe.summary.audio_streams }} 路</strong></div>
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">帧率</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.frame_rate || '—' }}</strong></div>
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">像素格式</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.pixel_format || '—' }}</strong></div>
                    <div class="grid min-w-0 gap-[5px]"><span class="text-xs text-[#7c8991]">码率</span><strong class="truncate text-[13px] text-[#40515b]">{{ probe.summary.bitrate ? formatBytes(probe.summary.bitrate / 8) + '/s' : '—' }}</strong></div>
                </div>
            </div>

            <p v-if="error" class="mt-[14px] text-xs leading-5 text-danger">{{ error }}</p>
            <p v-if="notice" class="mt-[14px] border border-[#c2ded7] bg-accent-soft px-3.5 py-[11px] text-[13px] text-[#327164]">{{ notice }}</p>

            <div class="my-6 h-px bg-[#e9edef]"></div>

            <div class="grid gap-[15px]">
                <div class="flex items-start gap-3.5 max-sm:flex-wrap">
                    <div class="min-w-0">
                        <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">匹配目标</span>
                        <h3 class="mt-1 text-base text-ink">这个文件对应哪个视频？</h3>
                    </div>
                    <div aria-label="视频类型" class="ml-auto inline-flex w-max shrink-0 rounded-[5px] border border-[#e2e7e8] bg-[#f1f4f4] p-[3px] max-sm:ml-0">
                        <button :class="{ 'bg-white text-[#205c49] shadow-[0_1px_4px_rgb(50_74_67/12%)]': targetType === 'movie' }" class="min-w-[84px] rounded-[3px] bg-transparent px-3.5 py-[7px] text-xs font-bold text-[#7a878e]" @click="setTargetType('movie')">电影</button>
                        <button :class="{ 'bg-white text-[#205c49] shadow-[0_1px_4px_rgb(50_74_67/12%)]': targetType === 'tv' }" class="min-w-[84px] rounded-[3px] bg-transparent px-3.5 py-[7px] text-xs font-bold text-[#7a878e]" @click="setTargetType('tv')">电视剧</button>
                    </div>
                </div>
                <div class="grid grid-cols-[minmax(260px,1fr)_116px_auto] gap-2 max-sm:grid-cols-1">
                    <input v-model="searchTitle" class="min-h-[42px] w-full rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 text-ink outline-none focus:border-accent" placeholder="按标题搜索，例如：西部世界" @keydown.enter="searchTargets" />
                    <input v-model="searchTodbId" class="min-h-[42px] w-[116px] rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-3 text-ink outline-none focus:border-accent max-sm:w-full" inputmode="numeric" pattern="[0-9]*" placeholder="todb_id" @keydown.enter="searchTargets" />
                    <button :disabled="searchBusy || (!searchTitle.trim() && !searchTodbId.trim() && !folderTodbId)" class="min-h-10 rounded-md border border-[#c8ddda] bg-[#f7fbfa] px-4 text-[13px] font-bold text-[#315b58] transition hover:border-[#acd0c9] hover:bg-accent-soft hover:text-accent-dark" @click="searchTargets">
                        {{ searchBusy ? '搜索中…' : '搜索' }}
                    </button>
                </div>
                <p v-if="folderTodbId" class="mt-[-4px] text-xs text-[#7c8991]">当前文件夹识别到 `todb_id={{ folderTodbId }}`，可直接填入右侧查询。</p>

                <div v-if="selectedTarget" class="flex items-center justify-between gap-3 border border-[#cce3d8] bg-[#edf7f2] px-3.5 py-3">
                    <div class="grid min-w-0 gap-[5px]">
                        <span class="text-xs text-[#7c8991]">已选择上传目标</span>
                        <strong class="truncate text-[13px] text-[#245744]">{{ selectedTarget.title }}</strong>
                    </div>
                    <button class="shrink-0 bg-transparent p-0 text-[13px] font-bold text-accent transition hover:text-accent-dark" @click="clearTarget">更换</button>
                </div>

                <div v-if="searchResults.length && !selectedTarget" class="grid gap-2">
                    <div v-if="targetType === 'tv' || searchResults.length > 1" class="flex items-center gap-2">
                        <input v-model="resultFilter" class="min-h-9 min-w-0 flex-1 rounded-md border border-[#d2dfe2] bg-[#fbfdfd] px-2.5 text-xs text-ink outline-none focus:border-accent" placeholder="筛选季、集或标题" />
                        <span class="shrink-0 text-[11px] text-muted">{{ displayedResults.length }}/{{ searchResults.length }}</span>
                    </div>
                    <div class="max-h-[360px] overflow-auto border-y border-[#e8edee]">
                        <template v-for="item in displayedResults" :key="`${item.item_id}-${item.title}`">
                        <button v-if="targetType === 'movie'" class="flex w-full items-center justify-between gap-4 border-b border-[#eef1f2] bg-transparent px-1 py-3 text-left text-[#33434d] transition hover:bg-[#f7faf8] hover:text-[#167858]" @click="chooseMovie(item)">
                            <span class="grid min-w-0 gap-1">
                                <strong class="truncate text-[13px]">{{ item.title }}</strong>
                                <small class="truncate text-[11px] text-[#91a0a7]">todb_id {{ item.todb_id }}<span v-if="item.date_air"> · {{ item.date_air }}</span></small>
                            </span>
                            <span class="shrink-0 text-xs font-bold text-[#167858]">选择</span>
                        </button>
                        <div v-else class="border-b border-[#e8edee] px-1 py-3.5">
                            <div class="mb-2 flex min-w-0 items-baseline gap-2">
                                <strong class="truncate text-[13px] text-[#33434d]">{{ item.title }}</strong>
                                <small class="shrink-0 text-[11px] text-[#91a0a7]">todb_id {{ item.todb_id }} · {{ item.seasons?.length ?? 0 }} 季</small>
                            </div>
                            <div v-for="season in item.seasons ?? []" :key="season.item_id" class="mt-1.5 border-t border-[#eef1f2] first:mt-0">
                                <button class="flex w-full items-center justify-between gap-2 bg-transparent py-2 text-left text-[11px] font-bold text-[#74828a] hover:text-accent-dark" type="button" @click="toggleSeason(item, season)">
                                    <span class="truncate">S{{ String(season.season_number).padStart(2, '0') }} · {{ season.season_title }}</span>
                                    <span class="flex shrink-0 items-center gap-1.5 text-[10px] font-medium text-muted-light">
                                        {{ season.episodes.length }} 集
                                        <span :class="{ 'rotate-180': isSeasonExpanded(item, season) }" class="text-xs transition-transform">⌄</span>
                                    </span>
                                </button>
                                <div v-if="isSeasonExpanded(item, season)" class="grid grid-cols-[repeat(auto-fill,minmax(132px,1fr))] gap-1.5 pb-2 max-sm:grid-cols-[repeat(auto-fill,minmax(108px,1fr))]">
                                    <button
                                        v-for="episode in season.episodes"
                                        :key="episode.item_id"
                                        class="flex min-w-0 items-center gap-1 rounded-[3px] border border-[#d6e7df] bg-[#f1f7f4] px-2 py-1.5 text-left text-[11px] text-[#3d5a4f] transition hover:bg-[#e4f1eb] hover:text-[#167858]"
                                        @click="chooseEpisode(item, season.season_number, episode.episode_number, episode.item_id, episode.episode_title)"
                                    >
                                        <span class="shrink-0">E{{ String(episode.episode_number).padStart(2, '0') }}</span>
                                        <span class="min-w-0 truncate text-[#7d8a91]">{{ episode.episode_title }}</span>
                                    </button>
                                </div>
                            </div>
                        </div>
                        </template>
                        <div v-if="displayedResults.length === 0" class="px-1 py-6 text-center text-xs text-muted">没有符合条件的结果</div>
                    </div>
                </div>
            </div>

            <div v-if="selectedTarget">
                <div class="my-6 h-px bg-[#e9edef]"></div>
                <div class="flex items-start gap-3.5">
                    <div>
                        <span class="text-[10px] font-extrabold uppercase tracking-[0.11em] text-accent">上传设置</span>
                        <h3 class="mt-1 text-base text-ink">确认上传设置</h3>
                    </div>
                    <span v-if="baseLoading" class="ml-auto text-[13px] text-muted">读取目标信息…</span>
                </div>
                <div v-if="matchingMedia.length" class="mt-[15px] grid gap-1 border-l-[3px] border-[#db9c51] bg-[#fff7eb] px-[13px] py-[11px] text-xs text-[#8f5f32]">
                    <strong>发现相同大小的已有资源</strong>
                    <span class="text-[#a1774c]">{{ matchingMedia.length }} 个资源与当前文件大小相同，仍可继续上传。</span>
                    <ul class="mt-[5px] grid list-none gap-[7px] p-0">
                        <li v-for="media in matchingMedia" :key="media.media_id">
                            <div class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2.5 rounded border border-[rgb(219_156_81/28%)] bg-white/50 px-2.5 py-2 text-[11px] text-[#8f6b48]">
                                <span class="truncate text-[#80552d]">{{ media.media_name }}</span>
                                <span>{{ formatBytes(media.media_file_size) }}</span>
                            </div>
                        </li>
                    </ul>
                </div>
                <div v-else-if="baseInfo" class="mt-[15px] grid gap-1 border-l-[3px] border-[#7bb99f] bg-[#f0f8f4] px-[13px] py-[11px] text-xs text-[#477260]">未发现相同大小的已有资源</div>
                <div class="mt-[17px] flex items-end justify-between gap-4 rounded-md border border-[#e1ece8] bg-[#f8fbfa] p-3.5 max-sm:flex-col max-sm:items-stretch">
                    <div class="w-full max-w-[250px] max-sm:max-w-none">
                        <span class="mb-[7px] block text-[13px] font-semibold text-[#63717b]">储存位置</span>
                        <SelectMenu v-model="storageType" :options="storageOptions" aria-label="文件储存位置" />
                    </div>
                    <button :disabled="!canUpload" class="min-h-10 min-w-[140px] rounded-md bg-accent px-4 text-[13px] font-bold text-white shadow-[0_2px_5px_rgb(20_120_109/16%)] transition hover:bg-accent-dark hover:shadow-[0_4px_9px_rgb(20_120_109/20%)] max-sm:w-full" @click="createUploadTask">
                        {{ createTaskBusy ? '创建任务中…' : '开始上传' }}
                    </button>
                </div>
            </div>
        </template>
    </section>
</template>
