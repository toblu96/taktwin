import { Dialogs } from '@wailsio/runtime'

// ask asks before something that cannot be undone, in the system's own dialog, and says
// whether the action was chosen. A webview's confirm() does not show in the app on a Mac,
// where it answers no at once.
export async function ask(message: string, action: string, title = 'Taktwin'): Promise<boolean> {
  try {
    const answer = await Dialogs.Question({
      Title: title,
      Message: message,
      Buttons: [
        { Label: action, IsDefault: true },
        { Label: 'Cancel', IsCancel: true },
      ],
    })
    return answer === action
  } catch {
    return false
  }
}

// choose asks which of several actions to take, in the system's own dialog, and gives the one
// chosen, or null for Cancel.
export async function choose(message: string, actions: string[], title = 'Taktwin'): Promise<string | null> {
  try {
    const answer = await Dialogs.Question({
      Title: title,
      Message: message,
      Buttons: [...actions.map((Label, i) => ({ Label, IsDefault: i === 0 })), { Label: 'Cancel', IsCancel: true }],
    })
    return actions.includes(answer) ? answer : null
  } catch {
    return null
  }
}

// tell shows what went wrong in the system's own dialog, where the page has no place for it.
export async function tell(message: string, title = 'Taktwin') {
  try {
    await Dialogs.Error({ Title: title, Message: message })
  } catch {
    // Without a dialog there is nowhere else to say it.
  }
}
