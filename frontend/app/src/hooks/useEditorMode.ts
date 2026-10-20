import { atom, useAtom } from "jotai"

export const playbackModeAtom = atom(false)
export const useEditorMode = () => useAtom(playbackModeAtom)
