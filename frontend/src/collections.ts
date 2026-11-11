import { ref, watch } from 'vue'
import { LibraryService } from '../bindings/github.com/toblu96/taktwin/internal/simapp'
import type { CollectionInfo } from '../bindings/github.com/toblu96/taktwin/internal/simapp/models'
import { filesRead } from './sim'

// The collections the library is made of, in its order, read again whenever the files are.
export const collections = ref<CollectionInfo[]>([])

export async function refreshCollections() {
  collections.value = (await LibraryService.Collections()) ?? []
}
watch(filesRead, () => void refreshCollections())
void refreshCollections()

// OWN is what the user's own stations are called, which belong to no collection.
export const OWN = 'Your own'

// collectionName is how a collection is shown, by its id.
export function collectionName(id: string | undefined) {
  if (!id) return OWN
  return collections.value.find((c) => c.id === id && !c.shadowed)?.name ?? id
}

// grouped sorts things by the collection they come from: the user's own first, then the
// collections in the library's order.
export function grouped<T extends { collection?: string }>(items: T[]): { id: string; name: string; items: T[] }[] {
  const order = ['', ...collections.value.filter((c) => !c.shadowed).map((c) => c.id)]
  const rank = (id: string) => (order.indexOf(id) + 1 || 99) - 1
  const ids = [...new Set(items.map((i) => i.collection ?? ''))].sort((a, b) => rank(a) - rank(b))
  return ids.map((id) => ({ id, name: collectionName(id), items: items.filter((i) => (i.collection ?? '') === id) }))
}
