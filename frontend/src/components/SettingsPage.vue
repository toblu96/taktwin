<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ask } from '../ask'
import { LibraryService, SettingsService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { CollectionInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { closeSettings, lastSettingsTab, type SettingsTab } from '../router'
import { reloadCollections, reloadFiles } from '../sim'
import UiSwitch from '../ui/UiSwitch.vue'
import IconGeneral from '~icons/fluent/options-20-regular'
import IconCollections from '~icons/fluent/box-multiple-20-regular'
import IconUpdates from '~icons/fluent/arrow-sync-20-regular'
import IconAbout from '~icons/fluent/info-20-regular'
import IconClose from '~icons/fluent/dismiss-20-regular'
import IconOpen from '~icons/fluent/folder-open-16-regular'
import IconReload from '~icons/fluent/arrow-clockwise-16-regular'
import IconChange from '~icons/fluent/folder-swap-16-regular'
import IconImport from '~icons/fluent/arrow-import-16-regular'
import IconDownload from '~icons/fluent/arrow-download-16-regular'
import IconWarning from '~icons/fluent/warning-16-regular'
import { checkForUpdate, installUpdate, openRelease, progress, restartForUpdate, setAutoCheck, update, updateAvailable, updateFailed, updating } from '../update'

// How far the download is, in percent, or null before its size is known.
const percent = computed(() => (progress.value && progress.value.total > 0 ? Math.round((progress.value.written / progress.value.total) * 100) : null))

const folder = ref('')
const isDefault = ref(true)
const fixed = ref(false)
const status = ref('')
const problems = ref<{ name: string; error: string }[]>([])
const failed = ref('')
const busy = ref(false)

function show(info: { path: string; default: boolean; fixed: boolean }) {
  folder.value = info.path
  isDefault.value = info.default
  fixed.value = info.fixed
}

onMounted(async () => {
  show(await SettingsService.Folder())
  await listCollections()
})

// The settings by topic, one tab at a time, which the address names. Without one they open on
// the tab they showed last. Esc goes back to where the app was.
const tabs: { id: SettingsTab; name: string; icon: Component }[] = [
  { id: 'general', name: 'General', icon: IconGeneral },
  { id: 'collections', name: 'Collections', icon: IconCollections },
  { id: 'updates', name: 'Updates', icon: IconUpdates },
  { id: 'about', name: 'About', icon: IconAbout },
]
const route = useRoute()
const router = useRouter()
const tab = computed<SettingsTab>(() => (route.params.tab as SettingsTab) || lastSettingsTab.value)
const showTab = (id: SettingsTab) => void router.replace({ name: 'settings', params: { tab: id } })
const title = computed(() => tabs.find((t) => t.id === tab.value)?.name ?? '')
function key(e: KeyboardEvent) {
  if (e.key === 'Escape') closeSettings()
}
onMounted(() => window.addEventListener('keydown', key))
onUnmounted(() => window.removeEventListener('keydown', key))

// --- Collections: the app's own reference models, and what was imported.

const collections = ref<CollectionInfo[]>([])
const collectionStatus = ref('')
const collectionFailed = ref('')
const count = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
const SOURCES: Record<string, string> = { 'built-in': 'Built in', installed: 'Imported', development: 'Development' }

async function listCollections() {
  collections.value = (await LibraryService.Collections()) ?? []
}

// changed reads everything again after a collection came, went, or was hidden or shown.
async function changed(what: string) {
  collectionStatus.value = what
  await listCollections()
  await reloadCollections()
}

async function collectionAction(action: () => Promise<unknown>, done: string) {
  busy.value = true
  collectionFailed.value = ''
  try {
    await action()
    await changed(done)
  } catch (error) {
    collectionFailed.value = (error as Error).message ?? String(error)
  } finally {
    busy.value = false
  }
}

// Cancelling the file dialog changes nothing.
async function importCollection() {
  busy.value = true
  collectionFailed.value = ''
  try {
    const id = await LibraryService.ImportCollection()
    if (id) await changed(`Imported ${id}: its stations are in the lists now.`)
  } catch (error) {
    collectionFailed.value = (error as Error).message ?? String(error)
  } finally {
    busy.value = false
  }
}
const toggleHidden = (c: CollectionInfo) =>
  collectionAction(() => LibraryService.SetCollectionHidden(c.id, !c.hidden), c.hidden ? `${c.name} is shown again.` : `${c.name} is hidden.`)
async function removeCollection(c: CollectionInfo) {
  if (!(await ask(`Remove the collection ${c.name} from your folder? Stations of your own that use its modules will not load without it.`, 'Remove'))) return
  void collectionAction(() => LibraryService.RemoveCollection(c.id), `${c.name} is removed.`)
}

// Another folder for your stations: the lists and the station in the simulation read it at once.
async function change(choose: boolean) {
  failed.value = ''
  try {
    const before = folder.value
    show(choose ? await SettingsService.ChooseUserFolder() : await SettingsService.SetUserFolder(''))
    if (folder.value !== before) await reload()
  } catch (error) {
    failed.value = `Could not use that folder: ${(error as Error).message}`
  }
}

async function openFolder() {
  failed.value = ''
  try {
    await SettingsService.OpenUserFolder()
  } catch (error) {
    failed.value = `Could not open the folder: ${(error as Error).message}`
  }
}

// Reads every station again and says which ones do not load.
async function reload() {
  busy.value = true
  failed.value = ''
  try {
    const stations = ((await LibraryService.Entries()) ?? []).filter((e) => e.level === 'station')
    problems.value = stations.filter((e) => e.error).map((e) => ({ name: e.name, error: e.error ?? '' }))
    status.value = `Read ${count(stations.length, 'station')} at ${new Date().toLocaleTimeString()}.`
    await reloadFiles()
  } catch (error) {
    failed.value = `The station in the simulation did not load again: ${(error as Error).message}`
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="flex min-h-0 bg-s0">
    <nav class="flex w-52 shrink-0 flex-col gap-0.5 border-r border-line bg-s1 p-2" aria-label="Settings">
      <h1 class="m-0 px-2 pt-1.5 pb-3 text-[15px] font-semibold">Settings</h1>
      <button
        v-for="t in tabs"
        :key="t.id"
        class="flex h-8 cursor-default items-center gap-2.5 rounded-control border-0 px-2 text-left focus-visible:-outline-offset-2"
        :class="tab === t.id ? 'bg-highlight text-fg' : 'bg-transparent text-muted hover:bg-hover hover:text-fg'"
        :aria-current="tab === t.id ? 'page' : undefined"
        @click="showTab(t.id)"
      >
        <component :is="t.icon" class="size-4.5 shrink-0" />
        <span class="flex-1">{{ t.name }}</span>
        <span v-if="t.id === 'updates' && updateAvailable" class="size-2 rounded-full bg-accent" title="An update is available" />
      </button>
    </nav>

    <div class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-12 shrink-0 items-center gap-3 border-b border-line px-8">
        <h2 class="m-0 flex-1 text-[15px] font-semibold">{{ title }}</h2>
        <button v-if="tab === 'collections'" class="t-btn t-btn-primary" :disabled="busy" @click="importCollection"><IconImport class="size-4" />Import collection…</button>
        <button class="t-icon-btn" title="Close (Esc)" aria-label="Close the settings" @click="closeSettings"><IconClose class="size-5" /></button>
      </header>

      <div class="min-h-0 flex-1 overflow-auto">
        <div class="max-w-3xl px-8 py-5">
          <template v-if="tab === 'general'">
            <section class="flex flex-col gap-3">
              <div>
                <h3 class="m-0 text-[13px] font-semibold">Your folder</h3>
                <p class="m-0 mt-1 text-muted">
                  Your projects are kept in this folder, under <code class="font-mono text-xs text-fg">projects</code>, and imported collections under
                  <code class="font-mono text-xs text-fg">collections</code>. The app also reads stations of your own from it, in front of the collections, each in
                  <code class="font-mono text-xs text-fg">stations/&lt;id&gt;/station.json</code>.
                </p>
              </div>
              <code class="block truncate rounded-control border border-line bg-s1 px-2.5 py-1.5 font-mono text-xs" :title="folder">{{ folder }}</code>
              <div class="flex flex-wrap items-center gap-2">
                <button class="t-btn" @click="openFolder"><IconOpen class="size-4" />Open folder</button>
                <button class="t-btn" :disabled="busy" title="Read the stations in the folder again" @click="reload"><IconReload class="size-4" />Reload</button>
                <button class="t-btn" :disabled="busy || fixed" @click="change(true)"><IconChange class="size-4" />Change…</button>
                <button v-if="!isDefault" class="t-btn" :disabled="busy || fixed" @click="change(false)">Use the default folder</button>
                <span class="text-xs text-muted">{{ fixed ? 'Set by TAKTWIN_HOME.' : 'Stations already in the old folder stay there.' }}</span>
              </div>
              <p v-if="status" class="m-0 text-xs text-muted" role="status">{{ status }}</p>
              <p v-if="failed" class="m-0 text-danger" role="alert">{{ failed }}</p>
              <ul v-if="problems.length" class="m-0 flex list-none flex-col gap-1 p-0">
                <li v-for="p in problems" :key="p.name" class="flex items-start gap-2 text-danger">
                  <IconWarning class="mt-0.5 size-4 shrink-0" /><span><strong>{{ p.name }}</strong> {{ p.error }}</span>
                </li>
              </ul>
            </section>
          </template>

          <template v-if="tab === 'collections'">
            <p class="m-0 mb-4 text-muted">
              Stations, modules and parts come in collections. The built-in one can be hidden; an imported <code class="font-mono text-xs text-fg">.taktwin</code> collection is
              kept in your folder, and importing a newer one replaces it.
            </p>
            <p v-if="collectionStatus" class="m-0 mb-3 text-xs text-muted" role="status">{{ collectionStatus }}</p>
            <p v-if="collectionFailed" class="m-0 mb-3 text-danger" role="alert">{{ collectionFailed }}</p>
            <ul class="m-0 flex list-none flex-col gap-2 p-0">
              <li v-for="c in collections" :key="c.source + c.id + c.path" class="flex items-start gap-4 rounded-card border border-line bg-s1 px-4 py-3">
                <div class="min-w-0 flex-1" :class="c.hidden || c.shadowed ? 'opacity-60' : ''">
                  <div class="flex flex-wrap items-center gap-2">
                    <strong class="font-semibold">{{ c.name }}</strong>
                    <span v-if="c.version" class="font-mono text-xs text-muted">{{ c.version }}</span>
                    <span class="rounded-sm bg-s3 px-1.5 text-[11px] leading-4.5 text-muted">{{ SOURCES[c.source] ?? c.source }}</span>
                    <span v-if="c.hidden" class="rounded-sm border border-control px-1.5 text-[11px] leading-4 text-muted">Hidden</span>
                  </div>
                  <p v-if="c.description" class="m-0 mt-1 text-muted">{{ c.description }}</p>
                  <p class="m-0 mt-1 text-xs text-faint">
                    {{ count(c.stations, 'station') }} · {{ count(c.modules, 'module or accessory', 'modules and accessories') }} · {{ count(c.parts, 'part') }}
                  </p>
                  <p v-if="c.shadowed" class="m-0 mt-1 text-xs text-muted">Another collection with the id {{ c.id }} comes first and is used instead.</p>
                  <p v-if="c.error" class="m-0 mt-1 text-danger">{{ c.error }}</p>
                </div>
                <div class="flex shrink-0 gap-2">
                  <button v-if="c.id" class="t-btn" :disabled="busy" @click="toggleHidden(c)">{{ c.hidden ? 'Show' : 'Hide' }}</button>
                  <button v-if="c.source === 'installed'" class="t-btn text-danger" :disabled="busy" @click="removeCollection(c)">Remove…</button>
                </div>
              </li>
            </ul>
          </template>

          <template v-if="tab === 'updates'">
            <section class="flex flex-col gap-4">
              <p class="m-0 text-muted">
                You run Taktwin <span class="font-mono text-xs text-fg">{{ update?.version ?? '…' }}</span>.<template v-if="update && !update.installable">
                  It runs from the sources, so it checks for updates but does not install them.</template
                >
              </p>
              <div class="flex flex-wrap items-center gap-3 rounded-card border border-line bg-s1 px-4 py-3">
                <template v-if="update?.ready">
                  <strong class="flex-1 font-semibold">Taktwin {{ update.latest?.version }} is installed. It takes effect when Taktwin starts again.</strong>
                  <button class="t-btn t-btn-primary" @click="restartForUpdate">Restart now</button>
                </template>
                <template v-else-if="update?.latest">
                  <strong class="flex-1 font-semibold">Taktwin {{ update.latest.version }} is available.</strong>
                  <button class="t-btn" @click="openRelease(update.latest.url)">What's new</button>
                  <button class="t-btn t-btn-primary" :disabled="updating || !update.installable" @click="installUpdate">
                    <IconDownload class="size-4" />Download and install
                  </button>
                </template>
                <template v-else>
                  <span class="flex-1" :class="update?.state === 'up-to-date' ? 'text-fg' : 'text-muted'">
                    {{ update?.state === 'up-to-date' ? 'Taktwin is up to date.' : 'Look for a newer version of Taktwin.' }}
                  </span>
                  <button class="t-btn" :disabled="updating" @click="checkForUpdate">{{ updating ? 'Checking…' : 'Check for updates' }}</button>
                </template>
                <div v-if="progress" class="flex w-full items-center gap-3">
                  <div
                    class="h-1.5 flex-1 overflow-hidden rounded-full bg-s3"
                    role="progressbar"
                    aria-label="Download"
                    :aria-valuenow="percent ?? undefined"
                    aria-valuemin="0"
                    aria-valuemax="100"
                  >
                    <div class="h-full rounded-full bg-focus" :style="{ width: `${percent ?? 5}%` }" />
                  </div>
                  <span class="text-xs text-muted">{{ percent === null ? 'Downloading…' : `Downloading… ${percent} %` }}</span>
                </div>
              </div>
              <UiSwitch :model-value="update?.autoCheck ?? true" label="Check for updates when Taktwin starts" @update:model-value="setAutoCheck" />
              <p v-if="updateFailed || update?.problem" class="m-0 text-danger" role="alert">{{ updateFailed || update?.problem }}</p>
            </section>
          </template>

          <template v-if="tab === 'about'">
            <section class="flex items-start gap-4">
              <img src="/logo.svg" alt="" class="size-12 shrink-0" />
              <div>
                <p class="m-0 text-[15px] font-semibold">Taktwin <span class="font-mono text-xs font-normal text-muted">{{ update?.version ?? '' }}</span></p>
                <p class="m-0 mt-1 text-muted">A PLC training simulator with a 3D digital twin of production stations.</p>
                <p class="m-0 mt-3 text-xs text-faint">Apache License 2.0. Icons: Fluent UI System Icons, MIT.</p>
              </div>
            </section>
          </template>
        </div>
      </div>
    </div>
  </main>
</template>
