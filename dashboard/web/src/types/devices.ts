import type { CameraSrcKind, PrinterKind } from './ws'

// CameraDetail mirrors GET /api/cameras/{name}. Credentials are never returned:
// srcInput has its userinfo stripped and the password is only flagged.
export interface CameraDetail {
  name: string
  srcKind: CameraSrcKind
  srcInput: string
  username: string
  hasPassword: boolean
  managed: boolean // dashboard owns this camera (UI-added)
  promoted: boolean // written into the Frigate config (NVR)
  configStream: boolean // defined in the hand-authored Frigate config
  editable: boolean // the source of a config-defined camera may be edited
  rotate?: number
  flipH?: boolean
  flipV?: boolean
}

// PrinterDetail mirrors GET /api/printers/{id} (secrets redacted to has* flags).
export interface PrinterDetail {
  id: string
  kind: PrinterKind
  name: string
  baseUrl?: string
  ip?: string
  serial?: string
  model?: string
  camera?: string
  uiUrl?: string
  macros?: string[]
  hasApiKey: boolean
  hasAccessCode: boolean
  managed: boolean
}
