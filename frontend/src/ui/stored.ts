import { ref, watch, type Ref } from 'vue'

// A value that the app keeps between its starts, such as the width of a side bar, in the
// window's local storage. Storage can be blocked or empty; then it starts from the fallback.
export function stored<T>(key: string, fallback: T): Ref<T> {
  let start = fallback
  try {
    const text = localStorage.getItem(`taktwin.${key}`)
    if (text !== null) start = JSON.parse(text) as T
  } catch {
    // Not kept: the fallback it is.
  }
  const value = ref(start) as Ref<T>
  watch(value, (v) => {
    try {
      localStorage.setItem(`taktwin.${key}`, JSON.stringify(v))
    } catch {
      // Only means the next start begins from the fallback.
    }
  })
  return value
}
