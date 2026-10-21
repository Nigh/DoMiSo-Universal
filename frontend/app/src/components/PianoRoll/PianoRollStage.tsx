import styled from "@emotion/styled"
import { FC } from "react"
import { Layout } from "../../Constants"
import { useKeyScroll } from "../../hooks/useKeyScroll"
import { Positioned } from "../ui/Positioned"
import CanvasPianoRuler from "./CanvasPianoRuler"
import { InstrumentLane } from "./InstrumentLane"
import { PianoKeys } from "./PianoKeys"
import { PianoRollCanvas } from "./PianoRollCanvas/PianoRollCanvas"
import { useEditorMode } from "../../hooks/useEditorMode"
import { PlaybackViewport } from "./playback/PlaybackViewport"
import { PlaybackRoll } from "./playback/PlaybackRoll"

export interface PianoRollStageProps {
  width: number
  height: number
  keyWidth: number
}

const Container = styled.div``

const RulerPosition = styled(Positioned)`
  height: var(--size-ruler-height);
  background: var(--color-background);
  border-bottom: 1px solid var(--color-divider);
`

const LeftTopSpace = styled(RulerPosition)``

export const PianoRollStage: FC<PianoRollStageProps> = ({
  width, height, keyWidth,
}) => {
  const [playback] = useEditorMode()
  const stage = <StageContent width={width} height={height} keyWidth={keyWidth} playback={playback} />
  return playback ? <PlaybackViewport width={width}>{stage}</PlaybackViewport> : stage
}

const StageContent = ({
  width,
  height,
  keyWidth,
  playback,
}: PianoRollStageProps & { playback: boolean }) => {
  const { scrollTop } = useKeyScroll()

  return (
    <Container>
      <Positioned top={Layout.rulerHeight} left={keyWidth}>
        {playback ? <PlaybackRoll width={width} height={height - Layout.rulerHeight} /> : <PianoRollCanvas width={width} height={height - Layout.rulerHeight} />}
      </Positioned>
      <Positioned top={-scrollTop + Layout.rulerHeight}>
        <PianoKeys width={keyWidth} />
      </Positioned>
      <LeftTopSpace width={keyWidth} />
      <RulerPosition left={keyWidth}>
        <CanvasPianoRuler />
      </RulerPosition>
      <Positioned top={Layout.rulerHeight} left={keyWidth}>
        <InstrumentLane width={width} />
      </Positioned>
    </Container>
  )
}
