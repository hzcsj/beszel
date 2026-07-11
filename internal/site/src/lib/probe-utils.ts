import type { VPSProbeTargetStats } from "@/types"

export type ProbeLossLevel = "normal" | "warning" | "critical" | "muted"

export function getProbeLossLevel(loss: number | undefined, isLocal: boolean, isMissing: boolean): ProbeLossLevel {
	if (isLocal || isMissing) return "muted"
	const l = loss ?? 0
	if (l >= 20) return "critical"
	if (l >= 5) return "warning"
	return "normal"
}

export interface ResolvedProbeTarget {
	id: string
	label: string
	pos: number
	stats: VPSProbeTargetStats
}

const defaultDynColors = [
	"hsl(217, 91%, 60%)",
	"hsl(142, 71%, 45%)",
	"hsl(25, 95%, 53%)",
	"hsl(271, 81%, 56%)",
] as const

export function getDynamicProbeColor(index: number): string {
	return defaultDynColors[index % defaultDynColors.length]
}

export function resolveProbeTargets(vp: Record<string, VPSProbeTargetStats> | undefined): ResolvedProbeTarget[] {
	if (!vp) return []
	const entries: ResolvedProbeTarget[] = []
	for (const [id, stats] of Object.entries(vp)) {
		const rawPos = stats.pos ?? 0
		const validPos = rawPos >= 1 && rawPos <= 4 ? rawPos : 0
		entries.push({
			id,
			label: stats.label || id.toUpperCase(),
			pos: validPos,
			stats,
		})
	}
	const posCount = new Map<number, number>()
	for (const e of entries) {
		if (e.pos > 0) {
			posCount.set(e.pos, (posCount.get(e.pos) ?? 0) + 1)
		}
	}
	for (const e of entries) {
		if (e.pos > 0 && (posCount.get(e.pos) ?? 0) > 1) {
			e.pos = 0
		}
	}
	entries.sort((a, b) => {
		if (a.pos > 0 && b.pos > 0) return a.pos - b.pos
		if (a.pos > 0) return -1
		if (b.pos > 0) return 1
		return a.id.localeCompare(b.id)
	})
	return entries
}

/** The systems table shows only the three primary targets; hover/detail keep all targets. */
export function resolveCompactProbeTargets(vp: Record<string, VPSProbeTargetStats> | undefined): ResolvedProbeTarget[] {
	return resolveProbeTargets(vp).slice(0, 3)
}

export function getListLatency(p: VPSProbeTargetStats): number | undefined {
	const lat = p.latw ?? p.lat1 ?? p.lat
	return lat != null && lat > 0 ? lat : undefined
}

export function getListLoss(p: VPSProbeTargetStats): number | undefined {
	if (p.local || !p.n || p.n <= 0) return undefined
	return Math.min(Math.max(p.loss ?? 0, 0), 100)
}

export function getDetailLat(p: VPSProbeTargetStats | undefined): number | undefined {
	if (!p || p.local) return undefined
	return p.lat1 != null && p.lat1 > 0 ? p.lat1 : undefined
}

export function getDetailLoss(p: VPSProbeTargetStats | undefined): number | undefined {
	if (!p || p.local || !p.n1 || p.n1 <= 0) return undefined
	return Math.min(Math.max(p.loss1 ?? 0, 0), 100)
}
