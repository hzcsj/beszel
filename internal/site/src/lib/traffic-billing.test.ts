/** biome-ignore-all lint/style/noNonNullAssertion: test assertions after toBeNull check */
import { describe, test, expect } from "bun:test"
import { calculateCycleProgressPct, getCycleTrafficColorClass, isInProtectionPeriod } from "./traffic-billing"

/**
 * Helper: create a Date at a specific Beijing time.
 * Beijing is UTC+8, so we subtract 8 hours from the target BJ time to get UTC.
 */
function bjDate(year: number, month: number, day: number, hour = 0, min = 0, sec = 0): Date {
	return new Date(Date.UTC(year, month - 1, day, hour - 8, min, sec))
}

describe("calculateCycleProgressPct", () => {
	test("resetDay=1 normal month (August 2026, 31 days)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: bjDate(2026, 8, 16, 12, 0, 0), // mid-month
		})
		expect(result).not.toBeNull()
		// 15.5 days out of 31 days ≈ 50%
		expect(result!).toBeCloseTo(50, 0)
	})

	test("resetDay=16 mid-cycle", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-07-16",
			resetDay: 16,
			now: bjDate(2026, 7, 31, 12, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jul 16 → Aug 16 = 31 days. elapsed = 15.5 days → ~50%
		expect(result!).toBeCloseTo(50, 0)
	})

	test("resetDay=29 non-leap year February (2025-01-29 → 2025-02-28)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2025-01-29",
			resetDay: 29,
			now: bjDate(2025, 2, 14, 0, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jan 29 → Feb 28 (clamped from 29 in non-leap Feb) = 30 days
		// elapsed = 16 days → ~53.3%
		expect(result!).toBeGreaterThan(50)
		expect(result!).toBeLessThan(60)
	})

	test("resetDay=29 leap year February (2024-01-29 → 2024-02-29)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2024-01-29",
			resetDay: 29,
			now: bjDate(2024, 2, 14, 0, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jan 29 → Feb 29 = 31 days. elapsed = 16 days → ~51.6%
		expect(result!).toBeGreaterThan(50)
		expect(result!).toBeLessThan(55)
	})

	test("resetDay=30 in February (2026-01-30 → 2026-02-28)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-01-30",
			resetDay: 30,
			now: bjDate(2026, 2, 14, 0, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jan 30 → Feb 28 = 29 days. elapsed = 15 days → ~51.7%
		expect(result!).toBeGreaterThan(50)
		expect(result!).toBeLessThan(55)
	})

	test("resetDay=31 in 30-day month (2026-06-30 → 2026-07-31)", () => {
		// June has 30 days so cycle starts on 30 (clamped), next is July 31
		const result = calculateCycleProgressPct({
			cycleStart: "2026-06-30",
			resetDay: 31,
			now: bjDate(2026, 7, 15, 12, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jun 30 → Jul 31 = 31 days. elapsed = 15.5 days → ~50%
		expect(result!).toBeCloseTo(50, 0)
	})

	test("resetDay=31 in February (2026-01-31 → 2026-02-28)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-01-31",
			resetDay: 31,
			now: bjDate(2026, 2, 14, 0, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Jan 31 → Feb 28 = 28 days. elapsed = 14 days → 50%
		expect(result!).toBeCloseTo(50, 0)
	})

	test("December to January (cross-year)", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-12-01",
			resetDay: 1,
			now: bjDate(2026, 12, 16, 12, 0, 0),
		})
		expect(result).not.toBeNull()
		// cycle: Dec 1 → Jan 1 = 31 days. elapsed = 15.5 days → ~50%
		expect(result!).toBeCloseTo(50, 0)
	})

	test("result clamped to 0 when now < cycleStart", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: bjDate(2026, 7, 31, 0, 0, 0),
		})
		expect(result).toBe(0)
	})

	test("result clamped to 100 when now > nextCycleStart", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: bjDate(2026, 9, 2, 0, 0, 0),
		})
		expect(result).toBe(100)
	})

	test("invalid cycleStart returns null", () => {
		expect(calculateCycleProgressPct({ cycleStart: "", resetDay: 1, now: new Date() })).toBeNull()
		expect(calculateCycleProgressPct({ cycleStart: "invalid", resetDay: 1, now: new Date() })).toBeNull()
		expect(calculateCycleProgressPct({ cycleStart: "2026-13-01", resetDay: 1, now: new Date() })).toBeNull()
		expect(calculateCycleProgressPct({ cycleStart: "2026-02-31", resetDay: 1, now: new Date() })).toBeNull()
	})

	test("invalid resetDay returns null", () => {
		expect(calculateCycleProgressPct({ cycleStart: "2026-08-01", resetDay: 0, now: new Date() })).toBeNull()
		expect(calculateCycleProgressPct({ cycleStart: "2026-08-01", resetDay: 32, now: new Date() })).toBeNull()
		expect(calculateCycleProgressPct({ cycleStart: "2026-08-01", resetDay: NaN, now: new Date() })).toBeNull()
	})

	test("does not produce NaN or Infinity", () => {
		const result = calculateCycleProgressPct({
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: bjDate(2026, 8, 15, 0, 0, 0),
		})
		expect(result).not.toBeNull()
		expect(Number.isFinite(result!)).toBe(true)
	})
})

describe("isInProtectionPeriod", () => {
	test("at cycle start (00:00 Beijing) is in protection", () => {
		expect(isInProtectionPeriod("2026-08-01", bjDate(2026, 8, 1, 0, 0, 0))).toBe(true)
	})

	test("at 07:59:59 Beijing is still in protection", () => {
		expect(isInProtectionPeriod("2026-08-01", bjDate(2026, 8, 1, 7, 59, 59))).toBe(true)
	})

	test("at 08:00:00 Beijing is no longer protected", () => {
		expect(isInProtectionPeriod("2026-08-01", bjDate(2026, 8, 1, 8, 0, 0))).toBe(false)
	})

	test("resetDay=16 protection from 16th 00:00", () => {
		expect(isInProtectionPeriod("2026-07-16", bjDate(2026, 7, 16, 7, 59, 59))).toBe(true)
		expect(isInProtectionPeriod("2026-07-16", bjDate(2026, 7, 16, 8, 0, 0))).toBe(false)
	})

	test("invalid cycleStart returns true (safe fallback)", () => {
		expect(isInProtectionPeriod("", new Date())).toBe(true)
		expect(isInProtectionPeriod("invalid", new Date())).toBe(true)
	})
})

describe("getCycleTrafficColorClass", () => {
	const quota = 2199023255552 // 2 TiB

	test("quota missing: no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: 1000000000,
				quotaBytes: 0,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 15, 12, 0, 0),
			})
		).toBe("")
	})

	test("quota=0: no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: 500000000,
				quotaBytes: 0,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 15, 12, 0, 0),
			})
		).toBe("")
	})

	test("within 8-hour protection (hour 0): no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota, // 100% usage
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 1, 0, 0, 0),
			})
		).toBe("")
	})

	test("within 8-hour protection (hour 7:59:59): no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 1, 7, 59, 59),
			})
		).toBe("")
	})

	test("after 8-hour protection (hour 8:00:00): color starts", () => {
		// At 08:00 Beijing on day 1 of a 31-day cycle: progress ≈ 1.08%
		// Usage 100% >> progress → red
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 1, 8, 0, 0),
			})
		).toBe("text-red-500")
	})

	test("during protection, usagePct > 100%: still no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 2,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 1, 4, 0, 0),
			})
		).toBe("")
	})

	test("progress 50%, usage 30%: no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.3,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0), // ~50%
			})
		).toBe("")
	})

	test("progress 50%, usage 40%: no color (40 <= 50*0.8=40)", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.4,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0),
			})
		).toBe("")
	})

	test("progress 50%, usage 40.1%: yellow", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.401,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0),
			})
		).toBe("text-yellow-500")
	})

	test("progress 50%, usage 50%: yellow (== progress, not red)", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.5,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0),
			})
		).toBe("text-yellow-500")
	})

	test("progress 50%, usage 50.1%: red", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.501,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0),
			})
		).toBe("text-red-500")
	})

	test("progress 90%, usage 80%: yellow", () => {
		// Progress ~90%: day 1 + 27.9 days in a 31-day month
		// Use resetDay=1 with Aug: 31 days, need 90% progress
		// 90% of 31 days = 27.9 days → Aug 28 ~21:36 BJ
		const now = bjDate(2026, 8, 28, 21, 36, 0)
		const progressCheck = calculateCycleProgressPct({ cycleStart: "2026-08-01", resetDay: 1, now })
		// Verify progress is approximately 90%
		expect(progressCheck!).toBeCloseTo(90, 0)
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.8,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now,
			})
		).toBe("text-yellow-500")
	})

	test("usage > 100%: red", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 1.5,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 20, 12, 0, 0),
			})
		).toBe("text-red-500")
	})

	test("projected > quota but usagePct below progress*0.8: no color", () => {
		// progress ~50%, usage 30% (below 40% threshold)
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota * 0.3,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 1,
				now: bjDate(2026, 8, 16, 12, 0, 0),
			})
		).toBe("")
	})

	test("billingMode continues using vt.bill, not recalculated", () => {
		// Just verifying the function takes billableBytes directly
		const result = getCycleTrafficColorClass({
			billableBytes: quota * 0.6,
			quotaBytes: quota,
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: bjDate(2026, 8, 16, 12, 0, 0), // ~50% progress
		})
		expect(result).toBe("text-red-500")
	})

	test("invalid cycleStart: no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota,
				quotaBytes: quota,
				cycleStart: "",
				resetDay: 1,
				now: new Date(),
			})
		).toBe("")
	})

	test("invalid resetDay: safe fallback no color", () => {
		expect(
			getCycleTrafficColorClass({
				billableBytes: quota,
				quotaBytes: quota,
				cycleStart: "2026-08-01",
				resetDay: 0,
				now: bjDate(2026, 8, 15, 12, 0, 0),
			})
		).toBe("")
	})
})

describe("timezone independence", () => {
	test("browser in UTC still calculates based on Beijing time", () => {
		// If browser is in UTC, the calculation should still use BJ time
		// At UTC 2026-07-31 16:00:00 = BJ 2026-08-01 00:00:00 (start of protection)
		const utcTime = new Date(Date.UTC(2026, 7, 1, 0, 0, 0)) // UTC Aug 1 00:00 = BJ Aug 1 08:00
		// This should be exactly at the end of protection
		expect(isInProtectionPeriod("2026-08-01", utcTime)).toBe(false)

		// UTC Jul 31 23:59:59 = BJ Aug 1 07:59:59 (still in protection)
		const utcBefore = new Date(Date.UTC(2026, 6, 31, 23, 59, 59))
		expect(isInProtectionPeriod("2026-08-01", utcBefore)).toBe(true)
	})

	test("results do not change with different browser timezone simulations", () => {
		// The function uses explicit UTC offset arithmetic, not browser locale
		// We test by passing the same absolute moment as a Date object
		const moment = new Date(Date.UTC(2026, 7, 16, 4, 0, 0)) // = BJ Aug 16 12:00
		const result1 = getCycleTrafficColorClass({
			billableBytes: 2199023255552 * 0.6,
			quotaBytes: 2199023255552,
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: moment,
		})
		// Same moment, same result
		const result2 = getCycleTrafficColorClass({
			billableBytes: 2199023255552 * 0.6,
			quotaBytes: 2199023255552,
			cycleStart: "2026-08-01",
			resetDay: 1,
			now: new Date(moment.getTime()),
		})
		expect(result1).toBe(result2)
		expect(result1).toBe("text-red-500")
	})
})
