/** biome-ignore-all lint/suspicious/noAssignInExpressions: it's fine :) */
import type { PreinitializedMapStore } from "nanostores"
import { pb, verifyAuth } from "@/lib/api"
import {
	$allSystemsById,
	$allSystemsByName,
	$downSystems,
	$longestSystemNameLen,
	$pausedSystems,
	$upSystems,
} from "@/lib/stores"
import { getVisualStringWidth, updateFavicon } from "@/lib/utils"
import type { SystemRecord } from "@/types"
import { BatteryState, SystemStatus } from "./enums"

const COLLECTION = pb.collection<SystemRecord>("systems")
const FIELDS_DEFAULT = "id,name,host,port,info,status"

/** Maximum system name length for display purposes */
const MAX_SYSTEM_NAME_LENGTH = 22

let initialized = false
// biome-ignore lint/suspicious/noConfusingVoidType: typescript rocks
let unsub: (() => void) | undefined | void
let realtimeActive = false

/** Initialize the systems manager and set up listeners */
export function init() {
	if (initialized) {
		return
	}
	initialized = true

	// sync system stores on change
	$allSystemsById.listen((newSystems, oldSystems, changedKey) => {
		const oldSystem = oldSystems[changedKey]
		const newSystem = newSystems[changedKey]

		// if system is undefined (deleted), remove it from the stores
		if (oldSystem && !newSystem?.id) {
			removeFromStore(oldSystem, $upSystems)
			removeFromStore(oldSystem, $downSystems)
			removeFromStore(oldSystem, $pausedSystems)
			removeFromStore(oldSystem, $allSystemsById)
		}

		if (!newSystem) {
			onSystemsChanged(newSystems, undefined)
			return
		}

		const newStatus = newSystem.status
		if (newStatus === SystemStatus.Up) {
			$upSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $downSystems)
			removeFromStore(newSystem, $pausedSystems)
		} else if (newStatus === SystemStatus.Down) {
			$downSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $pausedSystems)
		} else if (newStatus === SystemStatus.Paused) {
			$pausedSystems.setKey(newSystem.id, newSystem)
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $downSystems)
		} else if (newStatus === SystemStatus.Pending) {
			removeFromStore(newSystem, $upSystems)
			removeFromStore(newSystem, $downSystems)
			removeFromStore(newSystem, $pausedSystems)
		}

		// run things that need to be done when systems change
		onSystemsChanged(newSystems, newSystem)
	})
}

/** Update the longest system name length and favicon based on system status */
function onSystemsChanged(_: Record<string, SystemRecord>, changedSystem: SystemRecord | undefined) {
	const downSystemsStore = $downSystems.get()
	const downSystems = Object.values(downSystemsStore)

	// Update longest system name length
	const longestName = $longestSystemNameLen.get()
	const nameLen = Math.min(MAX_SYSTEM_NAME_LENGTH, getVisualStringWidth(changedSystem?.name || ""))
	if (nameLen > longestName) {
		$longestSystemNameLen.set(nameLen)
	}

	updateFavicon(downSystems.length)
}

/** Fetch systems from collection */
async function fetchSystems(): Promise<SystemRecord[]> {
	try {
		return await COLLECTION.getFullList({ sort: "+name", fields: FIELDS_DEFAULT })
	} catch (error) {
		console.error("Failed to fetch systems:", error)
		return []
	}
}

/** Makes sure the system has valid info object and throws if not */
function validateSystemInfo(system: SystemRecord) {
	if (!("cpu" in system.info)) {
		throw new Error(`${system.name} has no CPU info`)
	}
}

/** Add system to both name and ID stores */
export function add(system: SystemRecord) {
	try {
		validateSystemInfo(system)
		$allSystemsByName.setKey(system.name, system)
		$allSystemsById.setKey(system.id, system)
	} catch (error) {
		console.error(error)
	}
}

/** Update system in stores */
export function update(system: SystemRecord) {
	try {
		validateSystemInfo(system)
		// if name changed, make sure old name is removed from the name store
		const oldName = $allSystemsById.get()[system.id]?.name
		if (oldName !== system.name) {
			$allSystemsByName.setKey(oldName, undefined as unknown as SystemRecord)
		}
		add(system)
	} catch (error) {
		console.error(error)
	}
}

/** Remove system from stores */
export function remove(system: SystemRecord) {
	removeFromStore(system, $allSystemsByName)
	removeFromStore(system, $allSystemsById)
	removeFromStore(system, $upSystems)
	removeFromStore(system, $downSystems)
	removeFromStore(system, $pausedSystems)
}

/** Remove system from specific store */
function removeFromStore(system: SystemRecord, store: PreinitializedMapStore<Record<string, SystemRecord>>) {
	const key = store === $allSystemsByName ? system.name : system.id
	store.setKey(key, undefined as unknown as SystemRecord)
}

/** Action functions for subscription */
const actionFns: Record<string, (system: SystemRecord) => void> = {
	create: add,
	update: update,
	delete: remove,
}

/** Subscribe to real-time system updates from the collection */
export async function subscribe() {
	try {
		unsub = await COLLECTION.subscribe("*", ({ action, record }) => actionFns[action]?.(record), {
			fields: FIELDS_DEFAULT,
		})
	} catch (error) {
		console.error("Failed to subscribe to systems collection:", error)
	}
}

/** Refresh all systems with latest data from the hub */
export async function refresh() {
	try {
		const records = await fetchSystems()
		if (!records.length) {
			// No systems found, verify authentication
			verifyAuth()
			return
		}
		for (const record of records) {
			add(record)
		}
	} catch (error) {
		console.error("Failed to refresh systems:", error)
	}
}

/** Unsubscribe from real-time system updates */
export const unsubscribe = () => (unsub = unsub?.())

interface SystemListSummary {
	id: string
	ts: number
	info: SystemRecord["info"]
}

const rtSubsBySystem = new Map<string, () => void>()
const rtPending = new Map<string, number>()
let rtGeneration = 0
let storeUnsub: (() => void) | undefined

/** Dynamic fields that omitempty/omitzero may omit when zero — explicitly reset before merge */
const dynamicZeroDefaults: Partial<SystemRecord["info"]> = {
	dt: 0,
	g: 0,
	b: 0,
	bat: [0, BatteryState.Unknown],
}

function handleRealtimeMessage(data: SystemListSummary) {
	const existing = $allSystemsById.get()[data.id]
	if (!existing) return
	const updated = {
		...existing,
		info: { ...existing.info, ...dynamicZeroDefaults, ...data.info },
	}
	$allSystemsById.setKey(data.id, updated)
	$allSystemsByName.setKey(updated.name, updated)
	if (updated.status === SystemStatus.Up) {
		$upSystems.setKey(updated.id, updated)
	}
}

async function subscribeSystem(systemId: string, gen: number) {
	if (rtSubsBySystem.has(systemId)) return
	const pendingGen = rtPending.get(systemId)
	if (pendingGen !== undefined && pendingGen >= gen) return
	rtPending.set(systemId, gen)
	try {
		const fn = await pb.realtime.subscribe("rt_systems", handleRealtimeMessage, {
			query: { system: systemId },
		})
		if (rtPending.get(systemId) === gen) {
			rtPending.delete(systemId)
		}
		const stillUp = !!$upSystems.get()[systemId]?.id
		if (fn && realtimeActive && gen === rtGeneration && stillUp) {
			rtSubsBySystem.set(systemId, fn)
		} else if (fn) {
			fn()
		}
	} catch {
		if (rtPending.get(systemId) === gen) {
			rtPending.delete(systemId)
		}
	}
}

function unsubscribeSystem(systemId: string) {
	const fn = rtSubsBySystem.get(systemId)
	if (fn) {
		try {
			fn()
		} catch {
			// ignore
		}
		rtSubsBySystem.delete(systemId)
	}
}

/** Subscribe to rt_systems realtime updates, tracking $upSystems changes */
export function subscribeRealtime() {
	if (realtimeActive) return
	realtimeActive = true
	rtGeneration++
	const gen = rtGeneration

	for (const sys of Object.values($upSystems.get())) {
		subscribeSystem(sys.id, gen)
	}

	storeUnsub = $upSystems.listen((newSystems, oldSystems, changedKey) => {
		if (!realtimeActive) return
		const isNowUp = !!newSystems[changedKey]?.id
		const wasUp = !!oldSystems[changedKey]?.id
		if (isNowUp && !wasUp) {
			subscribeSystem(changedKey, rtGeneration)
		} else if (!isNowUp && wasUp) {
			unsubscribeSystem(changedKey)
		}
	})
}

/** Unsubscribe from all rt_systems realtime updates */
export function unsubscribeRealtime() {
	realtimeActive = false
	rtGeneration++
	if (storeUnsub) {
		storeUnsub()
		storeUnsub = undefined
	}
	for (const [, fn] of rtSubsBySystem) {
		try {
			fn()
		} catch {
			// ignore
		}
	}
	rtSubsBySystem.clear()
}
