import { computed, inject, ref, type InjectionKey } from 'vue'
import SdkDialog from './sdk/SdkDialog.vue'
import SdkIoList from './sdk/SdkIoList.vue'
import SdkMenu from './sdk/SdkMenu.vue'
import SdkSegmented from './sdk/SdkSegmented.vue'
import SdkSelect from './sdk/SdkSelect.vue'
import SdkTabs from './sdk/SdkTabs.vue'
import RekaDialog from './reka/RekaDialog.vue'
import RekaIoList from './reka/RekaIoList.vue'
import RekaMenu from './reka/RekaMenu.vue'
import RekaSegmented from './reka/RekaSegmented.vue'
import RekaSelect from './reka/RekaSelect.vue'
import RekaTabs from './reka/RekaTabs.vue'

// The components the layouts are built from, once on each library behind the same props, as the
// app's own components would wrap one of them. The lab switches between them.
export const kits = {
  sdk: { name: 'UI SDK', Dialog: SdkDialog, IoList: SdkIoList, Menu: SdkMenu, Segmented: SdkSegmented, Select: SdkSelect, Tabs: SdkTabs },
  reka: { name: 'Reka UI', Dialog: RekaDialog, IoList: RekaIoList, Menu: RekaMenu, Segmented: RekaSegmented, Select: RekaSelect, Tabs: RekaTabs },
}
export type Kit = (typeof kits)['sdk']

export const library = ref<keyof typeof kits>('sdk')
export const kit = computed<Kit>(() => kits[library.value])
export const kitKey: InjectionKey<typeof kit> = Symbol('kit')
export const useKit = () => inject(kitKey, kit)
