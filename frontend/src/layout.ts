import { stored } from './ui/stored'

// The sizes of the parts of a page that can be dragged, shared by the pages that have them, such
// as an open project and the library, and kept between starts.
export const treeWidth = stored('layout.tree', 240)
export const detailsWidth = stored('layout.details', 320)
export const panelHeight = stored('layout.panel', 240)
