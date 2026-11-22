<script setup lang="ts">
import { ask } from '../ask'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { LibraryService, SettingsService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { CollectionInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { closeSettings, reloadCollections, reloadFiles, settingsTab, type SettingsTab } from '../sim'
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

// The settings by topic, one tab at a time. Esc goes back to where the app was.
const tabs: { id: SettingsTab; name: string }[] = [
  { id: 'general', name: 'General' },
  { id: 'collections', name: 'Collections' },
  { id: 'updates', name: 'Updates' },
  { id: 'about', name: 'About' },
]
const title = computed(() => tabs.find((t) => t.id === settingsTab.value)?.name ?? '')
function key(e: KeyboardEvent) {
  if (e.key === 'Escape') closeSettings()
}
onMounted(() => window.addEventListener('keydown', key))
onUnmounted(() => window.removeEventListener('keydown', key))

// --- Collections: the app's own reference models, and what was imported.

const collections = ref<CollectionInfo[]>([])
const collectionStatus = ref('')
const collectionFailed = ref('')
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
    status.value = `Read ${stations.length} stations at ${new Date().toLocaleTimeString()}.`
    await reloadFiles()
  } catch (error) {
    failed.value = `The station in the simulation did not load again: ${(error as Error).message}`
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="settings">
    <nav class="side" aria-label="Settings">
      <h2>Settings</h2>
      <button v-for="t in tabs" :key="t.id" :class="{ active: settingsTab === t.id }" :aria-current="settingsTab === t.id ? 'page' : undefined" @click="settingsTab = t.id">
        {{ t.name }}<span v-if="t.id === 'updates' && updateAvailable" class="dot" title="An update is available" />
      </button>
    </nav>
    <div class="pane">
      <header class="pane-head">
        <h3>{{ title }}</h3>
        <button class="close" title="Back (Esc)" aria-label="Close the settings" @click="closeSettings">✕</button>
      </header>
      <div class="content">
        <section v-if="settingsTab === 'general'" class="block">
          <h3 class="section-title">Your folder</h3>
          <p class="hint">
            Your projects are kept in this folder, under <span class="mono">projects</span>, and imported collections under <span class="mono">collections</span>. The app also
            reads stations of your own from it, in front of the collections, each in <span class="mono">stations/&lt;id&gt;/station.json</span>.
          </p>
          <div class="folder">
            <code class="mono path" :title="folder">{{ folder }}</code>
            <button @click="openFolder">Open folder</button>
            <button :disabled="busy" @click="reload">Reload</button>
          </div>
          <div class="folder">
            <button :disabled="busy || fixed" @click="change(true)">Change…</button>
            <button v-if="!isDefault" :disabled="busy || fixed" @click="change(false)">Use the default folder</button>
            <span v-if="fixed" class="hint">Set by TAKTWIN_HOME.</span>
            <span v-else class="hint">Stations already in the old folder stay there.</span>
          </div>
          <p v-if="status" class="hint">{{ status }}</p>
          <p v-if="failed" class="problem">{{ failed }}</p>
          <p v-for="p in problems" :key="p.name" class="problem">
            <strong>{{ p.name }}</strong> {{ p.error }}
          </p>
        </section>
        <section v-if="settingsTab === 'collections'" class="block">
          <p class="hint">
            Stations, modules and parts come in collections. The built-in one can be hidden; an imported <span class="mono">.taktwin</span> collection is kept in your folder, and
            importing a newer one replaces it.
          </p>
          <ul class="collections">
            <li v-for="c in collections" :key="c.source + c.id + c.path" :class="{ hidden: c.hidden, shadowed: c.shadowed }">
              <div class="what">
                <strong>{{ c.name }}</strong>
                <span v-if="c.version" class="mono version">{{ c.version }}</span>
                <span class="badge">{{ SOURCES[c.source] ?? c.source }}</span>
                <span v-if="c.hidden" class="badge off">Hidden</span>
                <p v-if="c.description" class="hint">{{ c.description }}</p>
                <p class="hint counts">{{ c.stations }} stations · {{ c.modules }} modules and accessories · {{ c.parts }} parts</p>
                <p v-if="c.shadowed" class="hint">Another collection with the id {{ c.id }} comes first and is used instead.</p>
                <p v-if="c.error" class="problem">{{ c.error }}</p>
              </div>
              <div class="actions">
                <button v-if="c.id" :disabled="busy" @click="toggleHidden(c)">{{ c.hidden ? 'Show' : 'Hide' }}</button>
                <button v-if="c.source === 'installed'" class="danger" :disabled="busy" @click="removeCollection(c)">Remove</button>
              </div>
            </li>
          </ul>
          <div class="folder">
            <button :disabled="busy" @click="importCollection">Import collection…</button>
            <span v-if="collectionStatus" class="hint">{{ collectionStatus }}</span>
          </div>
          <p v-if="collectionFailed" class="problem">{{ collectionFailed }}</p>
        </section>
        <section v-if="settingsTab === 'updates'" class="block">
          <p class="hint">
            You run Taktwin <span class="mono">{{ update?.version ?? '…' }}</span>.<template v-if="update && !update.installable">
              It runs from the sources, so it checks for updates but does not install them.</template
            >
          </p>
          <div v-if="update?.ready" class="folder">
            <strong>Taktwin {{ update.latest?.version }} is installed.</strong>
            <button class="primary" @click="restartForUpdate">Restart now</button>
            <span class="hint">It takes effect when Taktwin starts again.</span>
          </div>
          <div v-else-if="update?.latest" class="folder">
            <strong>Taktwin {{ update.latest.version }} is available.</strong>
            <button class="primary" :disabled="updating || !update.installable" @click="installUpdate">Download and install</button>
            <button @click="openRelease(update.latest.url)">What's new</button>
          </div>
          <div v-else class="folder">
            <button :disabled="updating" @click="checkForUpdate">{{ updating ? 'Checking…' : 'Check for updates' }}</button>
            <span v-if="update?.state === 'up-to-date'" class="hint">Taktwin is up to date.</span>
          </div>
          <div v-if="progress" class="progress" role="progressbar" :aria-valuenow="percent ?? undefined" aria-valuemin="0" aria-valuemax="100">
            <div class="bar" :style="{ width: (percent ?? 5) + '%' }"></div>
            <span class="hint">{{ percent === null ? 'Downloading…' : `Downloading… ${percent} %` }}</span>
          </div>
          <label class="check">
            <input type="checkbox" :checked="update?.autoCheck ?? true" @change="setAutoCheck(($event.target as HTMLInputElement).checked)" />
            Check for updates when Taktwin starts
          </label>
          <p v-if="updateFailed || update?.problem" class="problem">{{ updateFailed || update?.problem }}</p>
        </section>
        <section v-if="settingsTab === 'about'" class="block">
          <p class="about">
            <img src="/logo.svg" alt="" width="40" height="40" />
            <span>
              <span><strong>Taktwin</strong> <span class="mono version">{{ update?.version ?? '' }}</span></span>
              <span class="hint">A PLC training simulator with a 3D digital twin of production stations. Apache License 2.0.</span>
            </span>
          </p>
        </section>
      </div>
    </div>
  </main>
</template>

<style scoped>
.settings {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  min-height: 0;
}

.side {
  border-right: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 18px 10px;
}

h2 {
  font-size: 16px;
  margin: 0 8px 12px;
}

.side button {
  align-items: center;
  background: none;
  border: 0;
  color: var(--text-dim);
  display: flex;
  gap: 8px;
  justify-content: space-between;
  padding: 6px 8px;
  text-align: left;
}

.side button:hover {
  color: var(--text);
}

.side .active {
  background: var(--panel-raised);
  color: var(--text);
}

.side .dot {
  background: var(--accent);
  border-radius: 50%;
  height: 7px;
  width: 7px;
}

.pane {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  min-height: 0;
  min-width: 0;
}

.pane-head {
  align-items: center;
  border-bottom: 1px solid var(--line);
  display: flex;
  justify-content: space-between;
  padding: 12px 14px 12px 28px;
}

.pane-head h3 {
  font-size: 15px;
  margin: 0;
}

.content {
  overflow: auto;
  padding: 20px 28px 32px;
}

.content > section {
  max-width: 800px;
}

.close {
  background: none;
  border-color: transparent;
  color: var(--text-dim);
  font-size: 15px;
  padding: 2px 8px;
}

.close:hover {
  color: var(--text);
}

.about {
  align-items: center;
  display: flex;
  gap: 14px;
}

.about > span {
  display: grid;
  gap: 4px;
}

.about .hint {
  margin: 0;
}

.primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--bg);
  font-weight: 600;
}

.progress {
  align-items: center;
  display: flex;
  gap: 10px;
  margin: 0 0 10px;
}

.progress .bar {
  background: var(--accent);
  border-radius: 3px;
  height: 6px;
  max-width: 240px;
  transition: width 0.2s;
}

.progress .hint {
  margin: 0;
}

.check {
  align-items: center;
  display: flex;
  font-size: 13px;
  gap: 8px;
  margin: 4px 0 10px;
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.5;
  margin: 0 0 10px;
}

.folder {
  align-items: center;
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.path {
  background: var(--panel);
  border: 1px solid var(--line);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  padding: 5px 8px;
  text-overflow: ellipsis;
  user-select: all;
  white-space: nowrap;
}

.collections {
  list-style: none;
  margin: 0 0 12px;
  padding: 0;
}

.collections li {
  border: 1px solid var(--line);
  border-radius: 3px;
  display: flex;
  gap: 12px;
  margin-bottom: 8px;
  padding: 10px 12px;
}

.collections li.hidden,
.collections li.shadowed {
  opacity: 0.6;
}

.collections .what {
  flex: 1;
  min-width: 0;
}

.collections .what .hint {
  margin: 4px 0 0;
}

.version {
  color: var(--text-dim);
  font-size: 12px;
  margin-left: 6px;
}

.badge {
  border: 1px solid var(--line);
  border-radius: 8px;
  color: var(--text-dim);
  font-size: 11px;
  margin-left: 8px;
  padding: 0 7px;
}

.badge.off {
  border-color: var(--accent);
  color: var(--accent);
}

.collections .actions {
  align-items: flex-start;
  display: flex;
  gap: 6px;
}

.problem {
  border-left: 2px solid var(--danger);
  font-size: 13px;
  line-height: 1.4;
  margin: 0 0 6px;
  padding: 2px 0 2px 8px;
}
</style>
