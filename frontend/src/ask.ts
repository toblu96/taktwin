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
