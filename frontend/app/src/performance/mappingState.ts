import { atom } from "jotai"
import { genshinWindsongLyre } from "../game-mapping"

export const activeProfileAtom = atom(genshinWindsongLyre)
export const mappingStateAtom = atom(genshinWindsongLyre.initialState)
