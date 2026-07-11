/**
 * Pure functions for VPS traffic billing cycle progress and color determination.
 * All time calculations use Asia/Shanghai (UTC+08:00) explicitly.
 */

const TIMEZONE = "Asia/Shanghai"
const PROTECTION_HOURS = 8

interface CycleProgressInput {
	/** Cycle start date string YYYY-MM-DD */
	cycleStart: string
	/** Reset day (1–31) */
	resetDay: number
	/** Current time (injectable for testing) */
	now: Date
}

interface CycleTrafficColorInput {
	/** Billable bytes (vt.bill) */
	billableBytes: number
	/** Quota bytes (vt.quota) */
	quotaBytes: number
	/** Cycle start date string YYYY-MM-DD */
	cycleStart: string
	/** Reset day (1–31) */
	resetDay: number
	/** Current time (injectable for testing) */
	now: Date
}

export type TrafficColorClass = "" | "text-red-500" | "text-yellow-500"

/**
 * Convert a YYYY-MM-DD string to a Date representing that date at 00:00 Beijing time.
 * Returns null if the input is invalid.
 */
function parseCycleStartAsBeijing(dateStr: string): Date | null {
	if (!dateStr || !/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) return null
	const [y, m, d] = dateStr.split("-").map(Number)
	if (!y || !m || !d || m < 1 || m > 12 || d < 1 || d > 31) return null
	// Beijing time is UTC+8, so 00:00 Beijing = previous day 16:00 UTC
	const utcMs = Date.UTC(y, m - 1, d, 0, 0, 0) - 8 * 3600_000
	const date = new Date(utcMs)
	if (Number.isNaN(date.getTime())) return null
	const normalized = new Intl.DateTimeFormat("en-CA", {
		timeZone: TIMEZONE,
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
	}).format(date)
	if (normalized !== dateStr) return null
	return date
}

/**
 * Get the last day of a given month (1-indexed month).
 */
function getLastDayOfMonth(year: number, month: number): number {
	return new Date(year, month, 0).getDate()
}

/**
 * Compute the next cycle start date given the current cycle start and reset day.
 * Handles short months, leap year February, and year boundaries.
 * Returns a Date at 00:00 Beijing time.
 */
function computeNextCycleStart(cycleStartDate: Date, resetDay: number): Date | null {
	if (!Number.isFinite(resetDay) || resetDay < 1 || resetDay > 31) return null

	// Get current cycle start in Beijing time components
	const bjFormatter = new Intl.DateTimeFormat("en-CA", {
		timeZone: TIMEZONE,
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
	})
	const parts = bjFormatter.formatToParts(cycleStartDate)
	let year = 0
	let month = 0
	for (const p of parts) {
		if (p.type === "year") year = Number(p.value)
		else if (p.type === "month") month = Number(p.value)
	}
	if (!year || !month) return null

	// Next cycle is in the following month
	let nextMonth = month + 1
	let nextYear = year
	if (nextMonth > 12) {
		nextMonth = 1
		nextYear++
	}

	// Clamp reset day to the last day of the next month
	const lastDay = getLastDayOfMonth(nextYear, nextMonth)
	const clampedDay = Math.min(resetDay, lastDay)

	// Convert to UTC (Beijing 00:00 = UTC -8h)
	const utcMs = Date.UTC(nextYear, nextMonth - 1, clampedDay, 0, 0, 0) - 8 * 3600_000
	return new Date(utcMs)
}

/**
 * Calculate the billing cycle progress percentage.
 * Returns a value clamped to [0, 100], or null if inputs are invalid.
 */
export function calculateCycleProgressPct(input: CycleProgressInput): number | null {
	const { cycleStart, resetDay, now } = input

	const start = parseCycleStartAsBeijing(cycleStart)
	if (!start) return null

	const end = computeNextCycleStart(start, resetDay)
	if (!end) return null

	const totalMs = end.getTime() - start.getTime()
	if (totalMs <= 0) return null

	const elapsedMs = now.getTime() - start.getTime()
	const pct = (elapsedMs / totalMs) * 100

	return Math.max(0, Math.min(100, pct))
}

/**
 * Determine whether the 8-hour protection period is still active.
 * Protection period = first 8 hours after cycleStart (Beijing 00:00).
 */
export function isInProtectionPeriod(cycleStart: string, now: Date): boolean {
	const start = parseCycleStartAsBeijing(cycleStart)
	if (!start) return true // invalid → treat as protected (safe fallback = no color)
	const colorEligibleAt = new Date(start.getTime() + PROTECTION_HOURS * 3600_000)
	return now.getTime() < colorEligibleAt.getTime()
}

/**
 * Get the CSS class for cycle traffic color.
 *
 * Rules:
 * - No quota or quota <= 0 → no color
 * - Within 8-hour protection after cycle start → no color
 * - usagePct > cycleProgressPct → red
 * - usagePct > cycleProgressPct * 0.8 → yellow
 * - Otherwise → no color
 *
 * Boundary: strict greater-than (== progress → yellow, == progress*0.8 → no color)
 */
export function getCycleTrafficColorClass(input: CycleTrafficColorInput): TrafficColorClass {
	const { billableBytes, quotaBytes, cycleStart, resetDay, now } = input

	if (!quotaBytes || quotaBytes <= 0) return ""
	if (!cycleStart) return ""

	if (isInProtectionPeriod(cycleStart, now)) return ""

	const cycleProgressPct = calculateCycleProgressPct({ cycleStart, resetDay, now })
	if (cycleProgressPct === null) return ""

	const usagePct = (billableBytes / quotaBytes) * 100

	if (!Number.isFinite(usagePct)) return ""

	if (usagePct > cycleProgressPct) return "text-red-500"
	if (usagePct > cycleProgressPct * 0.8) return "text-yellow-500"
	return ""
}
