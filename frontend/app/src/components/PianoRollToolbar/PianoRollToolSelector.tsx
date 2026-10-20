import { usePianoRoll } from "../../hooks/usePianoRoll"
import { useEditorMode } from "../../hooks/useEditorMode"
import { ToolSelector } from "../Toolbar/ToolSelector"
import { ToolbarButtonGroup, ToolbarButtonGroupItem } from "../Toolbar/ToolbarButtonGroup"

export const PianoRollToolSelector = () => {
  const { mouseMode, setMouseMode, resetSelection } = usePianoRoll()
  const [playbackMode, setPlaybackMode] = useEditorMode()
  return <>
    <ToolbarButtonGroup role="group" aria-label="卷帘模式" style={{ marginRight: '0.5rem' }}>
      {([false, true] as const).map((playback) => <ToolbarButtonGroupItem tabIndex={0} key={String(playback)}
        selected={playbackMode === playback} aria-pressed={playbackMode === playback}
        onClick={() => { resetSelection(); setPlaybackMode(playback) }}>
        {playback ? '播放' : '编辑'}
      </ToolbarButtonGroupItem>)}
    </ToolbarButtonGroup>
    <ToolSelector mouseMode={mouseMode} onSelect={setMouseMode} disabled={playbackMode} />
  </>
}
