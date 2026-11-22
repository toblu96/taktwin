// Makes every control reachable with Tab. WebKit, the engine of the app's window on a Mac, leaves
// buttons, links and selects out of the tab order unless the system's keyboard navigation is on,
// and tabs only to text fields and to elements with a tabindex. So each interactive element
// without one gets tabindex 0, which puts it where the page order has it, as other engines do.
// A disabled one stays out: no browser focuses it. Parts that manage their own tabindex, like a
// segmented control's items, keep theirs.
const interactive = 'button:not([tabindex]), a[href]:not([tabindex]), select:not([tabindex]), summary:not([tabindex])'

function reach(root: ParentNode) {
  root.querySelectorAll<HTMLElement>(interactive).forEach((el) => (el.tabIndex = 0))
}

export function keyboardReach() {
  reach(document)
  new MutationObserver((changes) => {
    for (const change of changes)
      change.addedNodes.forEach((node) => {
        if (!(node instanceof HTMLElement)) return
        if (node.matches(interactive)) node.tabIndex = 0
        reach(node)
      })
  }).observe(document.body, { childList: true, subtree: true })
}
