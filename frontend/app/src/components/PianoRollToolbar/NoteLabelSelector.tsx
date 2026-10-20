import styled from "@emotion/styled"
import { useNoteLabels } from "../../hooks/useNoteLabels"
import { useSettings } from "../../hooks/useSettings"
import { noteNameWithOctString } from "../../helpers/noteNumberString"
import type { NoteLabelMode } from "../../helpers/pianoRollLabels"
import { Select } from "../ui/Select"

const Controls = styled.div`
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin: 0 0.5rem;
  flex-shrink: 0;
  min-height: 2.5rem;
  flex-wrap: wrap;
  select { height: 2rem; font-size: 0.75rem; padding: 0 1.5rem 0 0.5rem; }
  svg { top: 0.25rem; right: 0.125rem; }
`

export function NoteLabelSelector() {
  const { mode, setMode, tonic, setTonic } = useNoteLabels()
  const { showNoteLabels, setShowNoteLabels } = useSettings()
  return <Controls>
    <Select aria-label="音符字符显示" value={showNoteLabels ? mode : "off"} onChange={(e) => {
      setShowNoteLabels(e.target.value !== "off")
      if (e.target.value !== "off") setMode(e.target.value as NoteLabelMode)
    }}>
      <option value="off">隐藏字符</option>
      <option value="name">音名</option>
      <option value="number">简谱</option>
      <option value="key">键盘按键</option>
    </Select>
    {showNoteLabels && mode === "number" && <>
      <span title="仅改变简谱显示，不移调 MIDI">1=</span>
      <Select aria-label="简谱首调（音名与八度）" title="默认读取文本中的第一个首调；这里使用 MIDI 音名八度（C4=中央 C）" value={tonic} onChange={(e) => setTonic(Number(e.target.value))}>
        {Array.from({ length: 128 }, (_, note) => <option key={note} value={note}>{noteNameWithOctString(note)}</option>)}
      </Select>
    </>}
  </Controls>
}
