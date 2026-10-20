import { noteNameWithOctString } from "../helpers/noteNumberString"
import { useAtom } from "jotai"
import { activeProfileAtom, mappingStateAtom } from "./mappingState"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Dialogs, Events } from "@wailsio/runtime"
import { keyToPitch, loadProfiles, parseProfile, pitchToKey, saveCustomProfiles } from "../game-mapping"
import { usePianoRoll } from "../hooks/usePianoRoll"
import { useMobxSelector } from "../hooks/useMobxSelector"
import { useStores } from "../hooks/useStores"
import { useTrackMute } from "../hooks/useTrackMute"
import { TrackMute } from "../trackMute/TrackMute"
import { AppService } from "../../../bindings/domiso-universal"
import { buildKeySequence } from "./planner"
import { matchesFreshChord } from "./judgement"
import { createPortal } from "react-dom"
import styled from "@emotion/styled"
import { Button } from "../components/ui/Button"
import { Checkbox } from "../components/ui/Checkbox"
import { Select } from "../components/ui/Select"

type PracticeGroup = { tick: number; notes: number[] }

const Panel = styled.aside`
  width: 300px;
  padding: 0.75rem;
  overflow: auto;
  border-left: 1px solid var(--color-divider);
  background: var(--color-background);
  box-sizing: border-box;
  h3 { margin: 0 0 0.75rem; }
  hr { border: 0; border-top: 1px solid var(--color-divider); }
`

const Row = styled.div`
  display: flex;
  gap: 0.4rem;
  margin: 0.5rem 0;
  align-items: center;
`

const Keyboard = styled.div`
  container-type: inline-size;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  overflow: auto;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
  box-sizing: border-box;
  background: var(--color-editor-background);
`

const KeyRow = styled.div<{ row: number }>`
  display: flex;
  --key-unit: clamp(1.5rem, 10cqw, 3.25rem);
  gap: 0.25rem;
  padding-left: ${({ row }) => ["0", "calc(var(--key-unit) * 0.25)", "calc(var(--key-unit) * 0.75)"][row]};
  padding-right: ${({ row }) => ["calc(var(--key-unit) * 0.75)", "calc(var(--key-unit) * 0.5)", "0"][row]};
`

const Key = styled("kbd", { shouldForwardProp: (prop) => prop !== "active" && prop !== "target" })<{ active: boolean; target: boolean }>`
  position: relative;
  overflow: hidden;
  box-sizing: border-box;
  width: var(--key-unit);
  flex-shrink: 0;
  padding: 0.5rem 0.25rem;
  box-shadow: 0 3px 0 var(--color-divider);
  transform: ${({ active }) => active ? "translateY(2px)" : "none"};
  text-align: center;
  border: 1px solid ${({ target }) => target ? "var(--color-theme)" : "var(--color-divider)"};
  border-radius: var(--radius-selector);
  color: ${({ active }) => active ? "var(--color-on-surface)" : "var(--color-text)"};
  background: ${({ active }) => active ? "var(--color-theme)" : "var(--color-background-secondary)"};
  font: 600 0.9rem var(--font-mono);
  small { display: block; margin-top: 0.25rem; font-size: 0.65rem; font-weight: 400; }
  .press-flash {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: var(--color-text);
    animation: key-press-flash 220ms ease-out both;
  }
  @keyframes key-press-flash { from { opacity: 0.65; } to { opacity: 0; } }
  @media (prefers-reduced-motion: reduce) {
    .press-flash { animation-timing-function: steps(1, end); }
  }
`

const ProfileEditor = styled.textarea`
  width: 100%;
  height: 180px;
  box-sizing: border-box;
  resize: vertical;
  color: var(--color-text);
  background: var(--color-background-dark);
  border: 1px solid var(--color-divider);
  border-radius: var(--radius-field);
  padding: 0.5rem;
  font-family: var(--font-mono);
  &:focus { border-color: var(--color-theme); outline: none; }
`

const keyboardRows = [
  ["KeyQ", "KeyW", "KeyE", "KeyR", "KeyT", "KeyY", "KeyU"],
  ["KeyA", "KeyS", "KeyD", "KeyF", "KeyG", "KeyH", "KeyJ"],
  ["KeyZ", "KeyX", "KeyC", "KeyV", "KeyB", "KeyN", "KeyM"],
]

function isTypingTarget(target: EventTarget | null) {
  return target instanceof HTMLElement &&
    (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))
}

export function PerformancePanel() {
  const { midiInput, player, synthGroup } = useStores()
  const { songStore } = useStores()
  const { trackMute } = useTrackMute()
  const { selectedTrack } = usePianoRoll()
  const position = useMobxSelector(() => player.position, [player])
  const [enabled, setEnabled] = useState(false)
  const [environment, setEnvironment] = useState<"editor" | "game">("editor")
  const [wait, setWait] = useState(true)
  const [profiles, setProfiles] = useState(loadProfiles)
  const [profile, setProfile] = useAtom(activeProfileAtom)
  const [mappingState, setMappingState] = useAtom(mappingStateAtom)
  const [profileJSON, setProfileJSON] = useState(() => JSON.stringify(profile, null, 2))
  const [pressedKeys, setPressedKeys] = useState<Set<string>>(new Set())
  const [pressCounts, setPressCounts] = useState<Record<number, number>>({})
  const [playbackNotes, setPlaybackNotes] = useState<Set<number>>(new Set())
  const [pressedNotes, setPressedNotes] = useState<Set<number>>(new Set())
  const [targetIndex, setTargetIndex] = useState(0)
  const [autoPlayStatus, setAutoPlayStatus] = useState("")
  const waitingTickRef = useRef<number | null>(null)
  const passedTickRef = useRef<number | null>(null)
  const pressedSinceTargetRef = useRef<Set<number>>(new Set())
  const [keyboardHost, setKeyboardHost] = useState<HTMLElement | null>(null)

  useEffect(() => {
    const syncHost = (event?: Event) => setKeyboardHost(
      event instanceof CustomEvent ? event.detail : document.getElementById("performance-keyboard"),
    )
    syncHost()
    window.addEventListener("performance-keyboard-host", syncHost)
    return () => window.removeEventListener("performance-keyboard-host", syncHost)
  }, [])

  useEffect(() => {
    if (enabled) return
    waitingTickRef.current = null
    pressedSinceTargetRef.current.clear()
    setPressedKeys(new Set())
    setPressedNotes(new Set())
  }, [enabled])

  useEffect(() => {
    pressedSinceTargetRef.current.clear()
    setPressedKeys(new Set())
    setPressedNotes(new Set())
  }, [environment, profile])

  const groups = useMemo<PracticeGroup[]>(() => {
    const grouped = new Map<number, number[]>()
    for (const event of selectedTrack?.events ?? []) {
      if (event.type !== "channel" || event.subtype !== "note") continue
      const notes = grouped.get(event.tick) ?? []
      notes.push(event.noteNumber)
      grouped.set(event.tick, notes)
    }
    return [...grouped].map(([tick, notes]) => ({ tick, notes })).sort((a, b) => a.tick - b.tick)
  }, [selectedTrack?.events])

  useEffect(() => {
    const next = groups.findIndex((group) => group.tick >= player.position)
    setTargetIndex(next < 0 ? Math.max(groups.length - 1, 0) : next)
    pressedSinceTargetRef.current.clear()
    setPressedKeys(new Set())
    setPressedNotes(new Set())
  }, [groups, player])

  useEffect(() => {
    const resetPressed = () => {
      pressedSinceTargetRef.current.clear()
      setPressedKeys(new Set())
      setPressedNotes(new Set())
    }
    window.addEventListener("blur", resetPressed)
    return () => window.removeEventListener("blur", resetPressed)
  }, [])

  const target = groups[targetIndex]
  const targetRef = useRef(target)
  const pressedRef = useRef(pressedNotes)
  targetRef.current = target
  pressedRef.current = pressedNotes

  useEffect(() => {
    if (!enabled) return
    const next = groups.findIndex((group) => group.tick >= position)
    if (next >= 0) setTargetIndex(next)
    if (waitingTickRef.current !== position) waitingTickRef.current = null
  }, [enabled, groups, position])

  const acceptNote = useCallback((noteNumber: number, down: boolean) => {
    if (down) setPressCounts((counts) => ({ ...counts, [noteNumber]: (counts[noteNumber] ?? 0) + 1 }))
    setPressedNotes((previous) => {
      const next = new Set(previous)
      if (down) {
        if (!next.has(noteNumber)) pressedSinceTargetRef.current.add(noteNumber)
        next.add(noteNumber)
      } else {
        next.delete(noteNumber)
      }
      return next
    })
  }, [])

  useEffect(() => {
    AppService.RegisterPerformanceHotkeys().catch(() => {})
    const offKey = Events.On("performance:key", (event) => {
      const data = event.data as { scanCode: number; down: boolean }
      const keyCode = Object.entries(profile.scanCodes).find(([, scan]) => scan === data.scanCode)?.[0]
      if (!keyCode) return
      const mapped = keyToPitch(profile, mappingState, keyCode)
      if (mapped.type === "note") {
        setPressedKeys((previous) => {
          const next = new Set(previous)
          data.down ? next.add(keyCode) : next.delete(keyCode)
          return next
        })
        acceptNote(mapped.noteNumber, data.down)
      } else if (mapped.type === "stateChange" && data.down) {
        setMappingState(mapped.state)
      }
    })
    const offStopped = Events.On("performance:stopped", (event) => setAutoPlayStatus(`自动演奏已停止：${String(event.data)}`))
    const offError = Events.On("performance:error", (event) => setAutoPlayStatus(`自动演奏错误：${String(event.data)}`))
    return () => { offKey(); offStopped(); offError() }
  }, [acceptNote, mappingState, profile])

  useEffect(() => {
    const monitor = enabled && environment === "game"
    AppService.SetGamePracticeMonitoring(monitor, profile.processNames).catch((error) => {
      if (monitor) setAutoPlayStatus(String(error))
    })
    return () => { if (monitor) AppService.SetGamePracticeMonitoring(false, profile.processNames) }
  }, [enabled, environment, profile])

  useEffect(() => midiInput.on("midiMessage", ({ data }) => {
    const command = data[0] & 0xf0
    if (command === 0x90 && data[2] > 0) acceptNote(data[1], true)
    if (command === 0x80 || (command === 0x90 && data[2] === 0)) acceptNote(data[1], false)
  }), [acceptNote, midiInput])

  useEffect(() => {
    const timers = new Map<ReturnType<typeof setTimeout>, number>()
    const sounding = new Map<string, number>()
    const publish = () => setPlaybackNotes(new Set(sounding.values()))
    const off = synthGroup.onPlayback((event, delay, timestamp, trackId) => {
      if (event.type !== "channel") return
      const channel = event.channel
      const clearChannel = () => {
        if (delay <= 0) for (const [timer, ch] of timers) if (ch === channel) { clearTimeout(timer); timers.delete(timer) }
        for (const key of sounding.keys()) if (key.startsWith(`${channel}:`)) sounding.delete(key)
        publish()
      }
      if (event.subtype === "controller" && [120, 123].includes(event.controllerType)) {
        if (delay <= 0) clearChannel()
        else {
          const timer = setTimeout(() => { timers.delete(timer); clearChannel() }, Math.max(0, timestamp + delay * 1000 - performance.now()))
          timers.set(timer, channel)
        }
        return
      }
      // Monitoring/preview already comes through MIDIInput, and must never advance practice.
      if (trackId === undefined || (event.subtype !== "noteOn" && event.subtype !== "noteOff")) return
      const timer = setTimeout(() => {
        timers.delete(timer)
        if (!player.isPlaying) return
        const key = `${channel}:${trackId}:${event.noteNumber}`
        if (event.subtype === "noteOn" && event.velocity > 0) {
          sounding.set(key, event.noteNumber)
          setPressCounts((counts) => ({ ...counts, [event.noteNumber]: (counts[event.noteNumber] ?? 0) + 1 }))
        } else sounding.delete(key)
        publish()
      }, Math.max(0, timestamp + delay * 1000 - performance.now()))
      timers.set(timer, channel)
    })
    return () => { off(); timers.forEach((_, timer) => clearTimeout(timer)) }
  }, [synthGroup, player])

  useEffect(() => {
    pressedSinceTargetRef.current.clear()
  }, [target?.tick])

  useEffect(() => {
    if (!enabled || !target || (!player.isPlaying && waitingTickRef.current !== target.tick)) return
    if (matchesFreshChord(target.notes, pressedNotes, pressedSinceTargetRef.current)) {
      passedTickRef.current = target.tick
      setTargetIndex((index) => Math.min(index + 1, groups.length - 1))
      if (wait && waitingTickRef.current === target.tick) {
        waitingTickRef.current = null
        player.play()
      }
    }
  }, [enabled, groups.length, player, pressedNotes, target, wait])

  useEffect(() => {
    if (enabled && wait && target && target.tick !== passedTickRef.current && player.isPlaying && position >= target.tick) {
      waitingTickRef.current = target.tick
      player.stop()
      player.position = target.tick
    }
  }, [enabled, player, position, target, wait])

  useEffect(() => {
    if (!enabled) return
    const onKey = (event: KeyboardEvent) => {
      if (isTypingTarget(event.target) || event.repeat) return
      const result = keyToPitch(profile, mappingState, event.code)
      if (result.type === "unmapped") return
      event.preventDefault()
      if (result.type === "stateChange") {
        if (event.type === "keydown") setMappingState(result.state)
        return
      }
      const down = event.type === "keydown"
      setPressedKeys((previous) => {
        const next = new Set(previous)
        down ? next.add(event.code) : next.delete(event.code)
        return next
      })
      if (environment === "editor") {
        midiInput.onMidiMessage({
          data: new Uint8Array([down ? 0x90 : 0x80, result.noteNumber, down ? 100 : 0]),
        })
      } else {
        acceptNote(result.noteNumber, down)
      }
    }
    window.addEventListener("keydown", onKey)
    window.addEventListener("keyup", onKey)
    return () => {
      window.removeEventListener("keydown", onKey)
      window.removeEventListener("keyup", onKey)
    }
  }, [acceptNote, enabled, environment, mappingState, midiInput, profile])

  const upcoming = groups.slice(targetIndex, targetIndex + 4)
  const prepareAutoPlay = useCallback(async () => {
    try {
      const actions = buildKeySequence(
        songStore.song,
        profile,
        player.position,
        (track) => TrackMute.shouldPlayTrack(track.id)(trackMute),
      )
      await AppService.PrepareKeySequence(actions, profile.processNames)
      setAutoPlayStatus(`已准备 ${actions.length / 2} 个按键`)
    } catch (error) {
      setAutoPlayStatus(String(error))
    }
  }, [player.position, profile, songStore.song, trackMute])

  const saveProfile = useCallback(() => {
    try {
      const parsed = parseProfile(profileJSON)
      const copy = parsed.id === profiles[0].id ? { ...parsed, id: `${parsed.id}-custom`, name: `${parsed.name}（自定义）` } : parsed
      const next = [...profiles.filter((item) => item.id !== copy.id), copy]
      setProfiles(next)
      setProfile(copy)
      setMappingState(copy.initialState)
      saveCustomProfiles(next)
      setAutoPlayStatus("映射配置已保存")
    } catch (error) {
      setAutoPlayStatus(`配置无效：${String(error)}`)
    }
  }, [profileJSON, profiles])

  const importProfile = useCallback(async () => {
    const path = await Dialogs.OpenFile({ Title: "导入游戏映射", Filters: [{ DisplayName: "JSON", Pattern: "*.json" }] })
    if (!path) return
    try {
      const json = await AppService.ImportGameProfile(path)
      setProfileJSON(json)
      setAutoPlayStatus("配置已载入，检查后点击保存配置")
    } catch (error) {
      setAutoPlayStatus(String(error))
    }
  }, [])

  const exportProfile = useCallback(async () => {
    const path = await Dialogs.SaveFile({ Title: "导出游戏映射", Filename: `${profile.id}.json`, Filters: [{ DisplayName: "JSON", Pattern: "*.json" }] })
    if (path) await AppService.ExportGameProfile(path, JSON.stringify(profile, null, 2))
  }, [profile])

  const targetKeys = new Set((target?.notes ?? []).flatMap((note) => {
    const mapped = pitchToKey(profile, mappingState, note)
    return mapped.type === "key" ? [mapped.keyCode] : []
  }))
  const keyboard = keyboardHost && createPortal(
    <Keyboard>
      {keyboardRows.map((row, rowIndex) => <KeyRow row={rowIndex} key={row[0]}>{row.map((code) => {
        const mapped = keyToPitch(profile, mappingState, code)
        const note = mapped.type === "note" ? mapped.noteNumber : undefined
        return (
        <Key key={code} active={pressedKeys.has(code) || (note !== undefined && (pressedNotes.has(note) || playbackNotes.has(note)))} target={enabled && targetKeys.has(code)}>
          {code.replace("Key", "")}
          <small>{note === undefined ? "—" : noteNameWithOctString(note)}</small>
          {note !== undefined && pressCounts[note] > 0 && <span key={pressCounts[note]} className="press-flash" onAnimationEnd={() => setPressCounts((counts) => ({ ...counts, [note]: 0 }))} />}
        </Key>
      )})}</KeyRow>)}
      <div>
        {upcoming.map((group, index) => (
          <span key={group.tick} style={{ opacity: index ? 0.55 : 1, marginRight: "1rem" }}>
            {index ? `后续 ${index}` : "当前"}：{group.notes.map((note) => {
              const mapped = pitchToKey(profile, mappingState, note)
              return mapped.type === "key" ? mapped.keyCode.replace("Key", "") : `?(${note})`
            }).join("+")}
          </span>
        ))}
      </div>
    </Keyboard>,
    keyboardHost,
  )

  return (
    <>
    {keyboard}
    <Panel>
      <h3 style={{ margin: "0 0 8px" }}>演奏与跟弹</h3>
      <Checkbox checked={enabled} onCheckedChange={(value) => setEnabled(value === true)} label="启用映射键盘与跟弹" />
      <Row>
        <Select value={environment} onChange={(event) => setEnvironment(event.target.value as "editor" | "game")}>
          <option value="editor">编辑器内（发声）</option>
          <option value="game">游戏内（静音监听）</option>
        </Select>
        <Checkbox checked={wait} onCheckedChange={(value) => setWait(value === true)} label="等待" />
      </Row>
      <p>配置：</p><Select value={profile.id} onChange={(event) => {
        const selected = profiles.find((item) => item.id === event.target.value) ?? profiles[0]
        setProfile(selected)
        setMappingState(selected.initialState)
        setProfileJSON(JSON.stringify(selected, null, 2))
      }}>{profiles.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</Select>
      <p>状态：{mappingState}</p>
      <hr />
      <Row>
        <Button onClick={prepareAutoPlay}>准备</Button>
        <Button onClick={() => AppService.StartAutoPlay().catch((error) => setAutoPlayStatus(String(error)))}>开始</Button>
        <Button onClick={() => AppService.StopAutoPlay()}>停止</Button>
      </Row>
      <small>{autoPlayStatus}</small>
      <details>
        <summary>编辑映射配置</summary>
        <ProfileEditor value={profileJSON} onChange={(event) => setProfileJSON(event.target.value)} />
        <Row><Button onClick={saveProfile}>保存配置</Button><Button onClick={importProfile}>导入</Button><Button onClick={exportProfile}>导出</Button></Row>
      </details>
    </Panel>
    </>
  )
}
