import { useAtom } from "jotai"
import { atomWithStorage } from "jotai/utils"
import { useStores } from "./useStores"
import { useMobxSelector } from "./useMobxSelector"
import type { NoteLabelMode } from "../helpers/pianoRollLabels"

const labelModeAtom = atomWithStorage<NoteLabelMode>("pianoRollNoteLabelMode", "name")
export function useNoteLabels() {
  const [mode, setMode] = useAtom(labelModeAtom)
  const { songStore } = useStores()
  const tonic = useMobxSelector(() => songStore.labelTonic, [songStore])
  return { mode, setMode, tonic, setTonic: (value: number) => songStore.setLabelTonic(value) }
}
