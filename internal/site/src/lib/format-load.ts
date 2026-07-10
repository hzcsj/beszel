/** IEEE 754-safe rounding using exponential notation to avoid fp multiplication drift */
function roundHalfUp(num: number, decimals: number): number {
	const shifted = Number(`${num}e${decimals}`)
	if (!Number.isFinite(shifted)) return num
	return Number(`${Math.round(shifted)}e-${decimals}`)
}

/**
 * Magnitude-aware compact load formatter for All Systems list.
 * Rounds first (half-up), then picks format bracket based on the rounded value.
 *
 * 0 -> 0.00, 0.004 -> 0.00, 9.995 -> 10.0, 99.95 -> 100, 123.4 -> 123
 */
export function formatLoad(num: number): string {
	if (!Number.isFinite(num) || num < 0) return "0.00"
	const r2 = roundHalfUp(num, 2)
	if (r2 < 10) return r2.toFixed(2)
	const r1 = roundHalfUp(num, 1)
	if (r1 < 100) return r1.toFixed(1)
	return Math.round(num).toString()
}
