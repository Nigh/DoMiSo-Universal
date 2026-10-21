import { act, StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { expect, it, vi } from "vitest"
import { ReleaseContext } from "./ReleaseContext"

const fixture = vi.hoisted(() => {
  const loseContext = vi.fn()
  return { loseContext, renderer: { gl: { getExtension: () => ({ loseContext }) } } }
})
vi.mock("@ryohey/webgl-react", () => ({ useRenderer: () => fixture.renderer }))

it("keeps the context alive during StrictMode replay and releases it once on exit", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true)
  const root = createRoot(document.createElement("div"))
  await act(async () => root.render(<StrictMode><ReleaseContext /></StrictMode>))
  expect(fixture.loseContext).not.toHaveBeenCalled()
  await act(async () => root.unmount())
  expect(fixture.loseContext).toHaveBeenCalledTimes(1)
  vi.unstubAllGlobals()
})
