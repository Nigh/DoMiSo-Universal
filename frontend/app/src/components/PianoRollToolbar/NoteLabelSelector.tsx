import styled from "@emotion/styled"
import { useNoteLabels } from "../../hooks/useNoteLabels"
import { useSettings } from "../../hooks/useSettings"
import { noteNameWithOctString } from "../../helpers/noteNumberString"
import { ToolbarButtonGroup, ToolbarButtonGroupItem } from "../Toolbar/ToolbarButtonGroup"
import { Select } from "../ui/Select"

const Controls = styled.div`
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin: 0 0.5rem;
  flex-shrink: 0;
  min-height: 2rem;
  flex-wrap: nowrap;
  select { height: 2rem; font-size: 0.75rem; padding: 0 1.5rem 0 0.5rem; }
  svg { top: 0.25rem; right: 0.125rem; }
`

export function NoteLabelSelector() {
  const { mode, setMode, tonic, setTonic } = useNoteLabels()
  const { showNoteLabels, setShowNoteLabels } = useSettings()
  return <Controls>
    <ToolbarButtonGroup role="group" aria-label="音符字符显示">
      {([['off', '隐藏'], ['name', '音名'], ['number', '简谱'], ['key', '按键']] as const).map(([value, label]) => (
        <ToolbarButtonGroupItem tabIndex={0} key={value} aria-pressed={(showNoteLabels ? mode : 'off') === value}
          selected={(showNoteLabels ? mode : 'off') === value} onClick={() => {
            setShowNoteLabels(value !== 'off')
            if (value !== 'off') setMode(value)
          }}>{label}</ToolbarButtonGroupItem>
      ))}
    </ToolbarButtonGroup>
    {showNoteLabels && mode === "number" && <>
      <span title="仅改变简谱显示，不移调 MIDI">1=</span>
      <Select aria-label="简谱首调（音名与八度）" title="默认读取文本中的第一个首调；这里使用 MIDI 音名八度（C4=中央 C）" value={tonic} onChange={(e) => setTonic(Number(e.target.value))}>
        {Array.from({ length: 128 }, (_, note) => <option key={note} value={note}>{noteNameWithOctString(note)}</option>)}
      </Select>
    </>}
  </Controls>
}
