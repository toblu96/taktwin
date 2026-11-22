import { createChange, createReorder, definePrimitive, type ChangeDetails, type ReorderDraft } from '@ui-sdk/core'

// A list whose items people put in order, by dragging or with the keyboard: Space picks one up,
// the arrows move it, Space puts it down and Escape puts it back. The order changes only when an
// item is put down, through onValueChange. Each item is a key, such as a signal's address.
type Options = {
  value?: string[]
  defaultValue?: string[]
  onValueChange?: (value: string[], details: ChangeDetails<'drag' | 'move-key' | 'imperative'>) => void
}

export const ioList = definePrimitive({
  name: 'io-list',
  options: { value: 'value', defaultValue: 'value', onValueChange: 'callback' },
  state: (options: Options) => ({ value: options.value ?? options.defaultValue ?? [], draft: null as ReorderDraft | null }),
  sync: (options) => (options.value === undefined ? {} : { value: options.value }),
  api: (context) => {
    const change = createChange(context)
    const reorder = createReorder(context, {
      slot: 'item',
      key: 'value',
      draft: 'draft',
      set: (value, details) => change('value', value, { reason: details.reason, event: details.event }),
    })
    // An item's key is its value; UI SDK keys keyed parts through api.internal.keyOf.
    return { reorder, setValue: (value: string[]) => change('value', value, { reason: 'imperative' }), internal: { keyOf: (value: unknown) => String(value) } }
  },
  slots: (slot) => ({
    // The order to show: the moving one's while an item is moved, else the value.
    root: slot({ attrs: () => ({ role: 'list' }), state: (state, { api }) => ({ order: api.reorder.order(state) }) }),
    item: slot<{ value: string }>({
      props: { value: 'key' },
      attrs: (state, { key, api }) => ({
        role: 'listitem',
        tabindex: '0',
        'data-moving': api.reorder.moving(state, key) ? '' : undefined,
        'data-dragging': api.reorder.dragging(state, key) ? '' : undefined,
      }),
      on: {
        keydown: (event, { api, key }) => {
          if (api.reorder.keydown(event, key)) event.preventDefault()
        },
        pointerdown: (event, { api, key }) => api.reorder.pointerdown(event, key),
      },
      bind: (el, part) => part.api.reorder.bind(el, part),
    }),
  }),
})
