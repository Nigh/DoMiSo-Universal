import { useEditorMode } from "../../hooks/useEditorMode"
import { NoteLabelSelector } from "./NoteLabelSelector"
import styled from "@emotion/styled"
import type { FC } from "react"
import { AutoScrollButton } from "../Toolbar/AutoScrollButton"
import { QuantizeSelector } from "../Toolbar/QuantizeSelector/QuantizeSelector"
import { Toolbar } from "../Toolbar/Toolbar"
import { TrackListMenuButton } from "../TrackList/TrackListMenuButton"
import { EventListButton } from "./EventListButton"
import { InstrumentButton } from "./InstrumentButton"
import { PanSlider } from "./PanSlider"
import { PianoRollToolSelector } from "./PianoRollToolSelector"
import { TrackNameInput } from "./TrackNameInput"
import { VolumeSlider } from "./VolumeSlider"

const ScrollableToolbar = styled(Toolbar)`
  overflow-x: auto;
  flex-shrink: 0;
`

const Spacer = styled.div`
  width: 1rem;
`

const FlexibleSpacer = styled.div`
  flex-grow: 1;
`

export const PianoRollToolbar: FC = () => {
  const [playbackMode] = useEditorMode()
  return (
      <ScrollableToolbar>
        <TrackListMenuButton />

        <TrackNameInput />

        <EventListButton />

        <Spacer />

        <InstrumentButton />

        <VolumeSlider />
        <PanSlider />

        <FlexibleSpacer />

        <NoteLabelSelector />
        <PianoRollToolSelector />

        <QuantizeSelector />

        {!playbackMode && <AutoScrollButton />}
      </ScrollableToolbar>
  )
}
