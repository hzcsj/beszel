import type { SystemRecord } from "@/types"

function configuredOrder(system: SystemRecord): number | undefined {
	const order = system.vps?.order
	return Number.isFinite(order) ? order : undefined
}

export function compareSystemsByOrder(a: SystemRecord, b: SystemRecord): number {
	const aOrder = configuredOrder(a)
	const bOrder = configuredOrder(b)

	if (aOrder !== undefined && bOrder !== undefined && aOrder !== bOrder) {
		return aOrder - bOrder
	}
	if (aOrder !== undefined && bOrder === undefined) {
		return -1
	}
	if (aOrder === undefined && bOrder !== undefined) {
		return 1
	}
	return a.name.localeCompare(b.name)
}
