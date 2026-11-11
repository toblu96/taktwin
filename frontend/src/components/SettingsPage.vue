<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { LibraryService, SettingsService } from '../../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { CollectionInfo } from '../../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { reloadCollections, reloadFiles } from '../sim'

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
function removeCollection(c: CollectionInfo) {
  if (!confirm(`Remove the collection ${c.name} from your folder? Stations of your own that use its modules will not load without it.`)) return
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
    <h2>Settings</h2>
    <section class="block">
      <h3 class="section-title">Collections</h3>
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
    <section class="block">
      <h3 class="section-title">Your stations</h3>
      <p class="hint">
        The app reads your own stations from this folder, in front of the collections. Put each in
        <span class="mono">stations/&lt;id&gt;/station.json</span>. Imported collections are kept in its <span class="mono">collections</span> folder.
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
  </main>
</template>

<style scoped>
.settings {
  max-width: 760px;
  overflow: auto;
  padding: 20px 24px;
}

h2 {
  font-size: 18px;
  margin: 0 0 18px;
}

.block {
  border-top: 1px solid var(--line);
  padding-top: 14px;
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
