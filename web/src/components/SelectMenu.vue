<script lang="ts" setup>
import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'

export interface SelectMenuOption {
    value: string
    label: string
    description?: string
}

const props = withDefaults(defineProps<{
    modelValue: string
    options: SelectMenuOption[]
    placeholder?: string
    ariaLabel?: string
    disabled?: boolean
}>(), {
    placeholder: '请选择',
    ariaLabel: '选择选项',
    disabled: false,
})

const emit = defineEmits<{
    'update:modelValue': [value: string]
}>()

const root = ref<HTMLElement | null>(null)
const triggerElement = ref<HTMLButtonElement | null>(null)
const optionsElement = ref<HTMLElement | null>(null)
const open = ref(false)
const query = ref('')
const menuStyle = ref<Record<string, string>>({})
const filteredOptions = computed(() => {
    const needle = query.value.trim().toLowerCase()
    return needle ? props.options.filter((option) => `${option.label} ${option.description ?? ''}`.toLowerCase().includes(needle)) : props.options
})

const selectedOption = () => props.options.find((option) => option.value === props.modelValue)

function toggle() {
    if (!props.disabled) {
        open.value = !open.value
        if (!open.value) query.value = ''
    }
}

function updateMenuPosition() {
    if (!triggerElement.value) {
        return
    }
    const triggerRect = triggerElement.value.getBoundingClientRect()
    const width = Math.max(triggerRect.width, 190)
    const edgePadding = 8
    const left = Math.min(
        Math.max(edgePadding, triggerRect.left),
        Math.max(edgePadding, window.innerWidth - width - edgePadding),
    )
    const menuHeight = Math.min(optionsElement.value?.scrollHeight ?? 280, 280)
    const openAbove = window.innerHeight - triggerRect.bottom < menuHeight + edgePadding
        && triggerRect.top > menuHeight + edgePadding

    menuStyle.value = {
        left: `${left}px`,
        top: `${openAbove ? triggerRect.top - menuHeight - 6 : triggerRect.bottom + 6}px`,
        width: `${width}px`,
    }
}

function choose(option: SelectMenuOption) {
    emit('update:modelValue', option.value)
    open.value = false
}

function closeOnOutside(event: MouseEvent) {
    const target = event.target as Node
    if (root.value?.contains(target) || optionsElement.value?.contains(target)) {
        return
    }
    open.value = false
}

watch(open, async (isOpen) => {
    if (isOpen) {
        await nextTick()
        updateMenuPosition()
        window.addEventListener('resize', updateMenuPosition)
        window.addEventListener('scroll', updateMenuPosition, true)
    } else {
        window.removeEventListener('resize', updateMenuPosition)
        window.removeEventListener('scroll', updateMenuPosition, true)
    }
})

watch(query, async () => {
    if (open.value) {
        await nextTick()
        updateMenuPosition()
    }
})

onMounted(() => document.addEventListener('click', closeOnOutside))
onBeforeUnmount(() => {
    document.removeEventListener('click', closeOnOutside)
    window.removeEventListener('resize', updateMenuPosition)
    window.removeEventListener('scroll', updateMenuPosition, true)
})
</script>

<template>
    <div ref="root" :class="{ 'opacity-60': disabled }" class="relative min-w-0">
        <button
            ref="triggerElement"
            :aria-expanded="open"
            :aria-label="ariaLabel"
            :class="{ 'border-[#8fc4bb] shadow-[0_0_0_3px_rgb(20_120_109/8%)]': open }"
            :disabled="disabled"
            class="flex min-h-[42px] w-full items-center justify-between gap-3 rounded-md border border-[#d2dfe2] bg-white px-[11px] py-[7px] text-left text-ink transition hover:border-[#8fc4bb] hover:shadow-[0_0_0_3px_rgb(20_120_109/8%)] disabled:cursor-not-allowed"
            type="button"
            @click.stop="toggle"
        >
            <span class="grid min-w-0 gap-0.5">
                <strong class="truncate text-xs font-bold text-[#30434b]">{{ selectedOption()?.label || placeholder }}</strong>
                <small v-if="selectedOption()?.description" class="truncate text-[10px] text-muted">{{ selectedOption()?.description }}</small>
            </span>
            <span
                :class="open ? '-translate-x-px -translate-y-px rotate-[225deg]' : '-translate-y-0.5'"
                aria-hidden="true"
                class="size-2 shrink-0 rotate-45 border-r-[1.5px] border-b-[1.5px] border-[#6d8087] transition-transform"
            ></span>
        </button>
        <Teleport to="body">
            <div
                v-if="open"
                ref="optionsElement"
                :aria-label="ariaLabel"
                :style="menuStyle"
                class="fixed z-[100] grid max-h-[280px] min-w-[190px] gap-[3px] overflow-auto rounded-[7px] border border-line bg-white p-[5px] shadow-[0_12px_28px_rgb(25_48_56/16%)]"
                role="listbox"
            >
                <input v-if="options.length > 8" v-model="query" class="mx-1.5 my-1.5 w-[calc(100%_-_12px)] rounded border border-line px-2 py-[7px] text-xs outline-none focus:border-accent" placeholder="快速搜索" @click.stop />
                <button
                    v-for="option in filteredOptions"
                    :key="option.value"
                    :aria-selected="option.value === modelValue"
                    :class="{ 'bg-accent-soft': option.value === modelValue }"
                    class="flex w-full items-center justify-between gap-2.5 rounded px-[9px] py-2 text-left text-ink transition hover:bg-accent-soft"
                    role="option"
                    type="button"
                    @click="choose(option)"
                >
                    <span class="grid min-w-0 gap-0.5">
                        <strong :class="option.value === modelValue ? 'text-accent-dark' : 'text-[#30434b]'" class="truncate text-xs font-bold">{{ option.label }}</strong>
                        <small v-if="option.description" class="truncate text-[10px] text-muted">{{ option.description }}</small>
                    </span>
                    <span v-if="option.value === modelValue" aria-hidden="true" class="shrink-0 text-sm font-extrabold text-accent">✓</span>
                </button>
            </div>
        </Teleport>
    </div>
</template>
