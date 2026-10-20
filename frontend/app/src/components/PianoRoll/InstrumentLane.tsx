import { useEditorMode } from "../../hooks/useEditorMode"
import styled from "@emotion/styled"
import { isProgramChangeEvent } from "@signal-app/core"
import { type FC, useMemo } from "react"
import { useEventView } from "../../hooks/useEventView"
import { useTickScroll } from "../../hooks/useTickScroll"
import { Positioned } from "../ui/Positioned"
import { InstrumentMark } from "./InstrumentMark"

export interface InstrumentLaneProps {
  width: number
}

const Container = styled.div`
  height: 24px;
  width: 100%;
  padding: 0.25rem 0;
  position: relative;
`

export const InstrumentLane: FC<InstrumentLaneProps> = ({ width }) => {
  const [playbackMode] = useEditorMode()
  const events = useEventView()
  const { scrollLeft, transform } = useTickScroll()

  const programChangeEvents = events.filter(isProgramChangeEvent)

  const style = useMemo(
    () => ({
      width,
    }),
    [width],
  )

  return (
    <Container style={{ ...style, pointerEvents: playbackMode ? "none" : undefined }}>
      <Positioned left={-scrollLeft}>
        {programChangeEvents.map((e) => (
          <InstrumentMark key={e.id} event={e} transform={transform} />
        ))}
      </Positioned>
    </Container>
  )
}
