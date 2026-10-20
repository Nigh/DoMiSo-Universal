import { useEditorMode } from "../../hooks/useEditorMode"
import { usePlayer } from "../../hooks/usePlayer"
import styled from "@emotion/styled"
import useComponentSize from "@rehooks/component-size"
import { clamp } from "lodash"
import { FC, useCallback, useEffect, useRef, useState } from "react"
import { Layout, WHEEL_SCROLL_RATE } from "../../Constants"
import { isTouchPadEvent } from "../../helpers/touchpad"
import { useKeyScroll } from "../../hooks/useKeyScroll"
import { usePianoNotesKeyboardShortcut } from "../../hooks/usePianoNotesKeyboardShortcut"
import { usePianoRoll } from "../../hooks/usePianoRoll"
import { useTickScroll } from "../../hooks/useTickScroll"
import { useTrack } from "../../hooks/useTrack"
import ControlPane from "../ControlPane/ControlPane"
import {
  HorizontalScaleScrollBar,
  VerticalScaleScrollBar,
} from "../inputs/ScaleScrollBar"
import { PianoRollStage } from "./PianoRollStage"
import { StyledSplitPane } from "./StyledSplitPane"

const Parent = styled.div`
  flex-grow: 1;
  background: var(--color-background);
  position: relative;
`

const Alpha = styled.div`
  flex-grow: 1;
  position: relative;
  outline: none;
`

const Beta = styled.div`
  border-top: 1px solid var(--color-divider);
  height: calc(100% - 17px);
  display: flex;
  flex-direction: column;
`

const PaneTabs = styled.div`
  display: flex;
  height: 2rem;
  flex-shrink: 0;
  background: var(--color-background);
  border-bottom: 1px solid var(--color-divider);
`

const PaneTab = styled.button<{ active: boolean }>`
  border: 0;
  border-bottom: 2px solid ${({ active }) => active ? "var(--color-theme)" : "transparent"};
  background: ${({ active }) => active ? "var(--color-background-secondary)" : "transparent"};
  color: var(--color-text);
  padding: 0 1rem;
`

const PaneContent = styled.div`
  flex: 1;
  min-height: 0;
`

const PianoRollWrapper: FC = () => {
  const [playbackMode] = useEditorMode()
  const { setPosition } = usePlayer()
  const { transform, scrollBy, selectedTrackId, setActivePane } = usePianoRoll()
  const { isRhythmTrack } = useTrack(selectedTrackId)
  const {
    contentHeight,
    scrollTop,
    scaleAroundPointY,
    setScrollTopInPixels,
    setScaleY,
  } = useKeyScroll()
  const {
    scrollLeft,
    startTick,
    canvasWidth,
    contentWidth,
    scaleAroundPointX,
    setAutoScroll,
    setScrollLeftInPixels,
    setScaleX,
  } = useTickScroll()
  const keyboardShortcutProps = usePianoNotesKeyboardShortcut()
  const [bottomPane, setBottomPane] = useState<"keyboard" | "controls">("keyboard")

  const keyboardHostRef = useCallback((element: HTMLDivElement | null) => {
    window.dispatchEvent(new CustomEvent("performance-keyboard-host", { detail: element }))
  }, [])

  const ref = useRef(null)
  const size = useComponentSize(ref)

  const alphaRef = useRef(null)
  const { height: alphaHeight = 0 } = useComponentSize(alphaRef)
  const keyWidth = isRhythmTrack
    ? Layout.keyWidth + Layout.drumKeysWidth
    : Layout.keyWidth

  const onClickScaleUpHorizontal = useCallback(
    () => scaleAroundPointX(0.2, 0),
    [scaleAroundPointX],
  )
  const onClickScaleDownHorizontal = useCallback(
    () => scaleAroundPointX(-0.2, 0),
    [scaleAroundPointX],
  )
  const onClickScaleResetHorizontal = useCallback(
    () => setScaleX(1),
    [setScaleX],
  )

  const onClickScaleUpVertical = useCallback(
    () => scaleAroundPointY(0.2, 0),
    [scaleAroundPointY],
  )
  const onClickScaleDownVertical = useCallback(
    () => scaleAroundPointY(-0.2, 0),
    [scaleAroundPointY],
  )
  const onClickScaleResetVertical = useCallback(() => setScaleY(1), [setScaleY])

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      if (e.shiftKey && (e.altKey || e.ctrlKey)) {
        // vertical zoom
        let scaleYDelta = isTouchPadEvent(e.nativeEvent)
          ? 0.02 * e.deltaY
          : 0.01 * e.deltaX
        scaleYDelta = clamp(scaleYDelta, -0.15, 0.15) // prevent acceleration to zoom too fast
        scaleAroundPointY(scaleYDelta, e.nativeEvent.offsetY)
      } else if (e.altKey || e.ctrlKey) {
        // horizontal zoom
        const scaleFactor = isTouchPadEvent(e.nativeEvent) ? 0.01 : -0.01
        const scaleXDelta = clamp(e.deltaY * scaleFactor, -0.15, 0.15) // prevent acceleration to zoom too fast
        scaleAroundPointX(scaleXDelta, e.nativeEvent.offsetX)
      } else {
        // scrolling
        const scaleFactor = isTouchPadEvent(e.nativeEvent)
          ? 1
          : transform.pixelsPerKey * WHEEL_SCROLL_RATE
        const deltaY = e.deltaY * scaleFactor
        scrollBy(-e.deltaX, -deltaY)
      }
    },
    [scrollBy, transform, scaleAroundPointX, scaleAroundPointY],
  )

  const onChangeSplitPane = useCallback(() => {
    setScrollTopInPixels(scrollTop)
  }, [setScrollTopInPixels, scrollTop])

  const onFocusNotes = useCallback(
    () => setActivePane("notes"),
    [setActivePane],
  )

  const onBlurNotes = useCallback(() => setActivePane(null), [setActivePane])

  useEffect(
    () => () => {
      setActivePane(null)
    },
    [setActivePane],
  )

  return (
    <Parent ref={ref}>
      <StyledSplitPane
        split="horizontal"
        minSize={50}
        defaultSize={"60%"}
        onChange={onChangeSplitPane}
      >
        <Alpha
          onWheel={onWheel}
          ref={alphaRef}
          {...keyboardShortcutProps}
          onFocus={onFocusNotes}
          onBlur={onBlurNotes}
          tabIndex={0}
        >
          <PianoRollStage
            width={Math.max(0, size.width - keyWidth)}
            height={alphaHeight}
            keyWidth={keyWidth}
          />
          <VerticalScaleScrollBar
            scrollOffset={scrollTop}
            contentLength={contentHeight}
            onScroll={setScrollTopInPixels}
            onClickScaleUp={onClickScaleUpVertical}
            onClickScaleDown={onClickScaleDownVertical}
            onClickScaleReset={onClickScaleResetVertical}
          />
        </Alpha>
        <Beta>
          <PaneTabs>
            <PaneTab active={bottomPane === "keyboard"} onClick={() => setBottomPane("keyboard")}>PC 键盘</PaneTab>
            <PaneTab active={bottomPane === "controls"} onClick={() => setBottomPane("controls")}>Velocity 等属性</PaneTab>
          </PaneTabs>
          <PaneContent
            id="performance-keyboard"
            ref={keyboardHostRef}
            style={{ display: bottomPane === "keyboard" ? "block" : "none" }}
          />
          <PaneContent inert={playbackMode} style={{ display: bottomPane === "controls" ? "block" : "none", pointerEvents: playbackMode ? "none" : undefined }}>
            <ControlPane axisWidth={keyWidth} />
          </PaneContent>
        </Beta>
      </StyledSplitPane>
      <HorizontalScaleScrollBar
        scrollOffset={scrollLeft - transform.getX(startTick) + (playbackMode ? canvasWidth / 2 : 0)}
        contentLength={contentWidth - transform.getX(startTick) + (playbackMode ? canvasWidth / 2 : 0)}
        onScroll={useCallback(
          (v: number) => {
            if (playbackMode) setPosition(startTick + transform.getTick(v))
            else setScrollLeftInPixels(v + transform.getX(startTick))
            setAutoScroll(false)
          },
          [setScrollLeftInPixels, setAutoScroll, playbackMode, setPosition, startTick, transform],
        )}
        onClickScaleUp={onClickScaleUpHorizontal}
        onClickScaleDown={onClickScaleDownHorizontal}
        onClickScaleReset={onClickScaleResetHorizontal}
      />
    </Parent>
  )
}

export default PianoRollWrapper
