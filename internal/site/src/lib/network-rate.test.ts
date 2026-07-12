import { describe, expect, test } from "bun:test"
import {
	getDirectionalNetworkRateColorClasses,
	getNetworkRateColorClass,
	NETWORK_RATE_CRITICAL_BYTES_PER_SECOND,
	NETWORK_RATE_WARNING_BYTES_PER_SECOND,
} from "./network-rate"

describe("getNetworkRateColorClass", () => {
	test("uses exact formatBytes promotion thresholds", () => {
		expect(getNetworkRateColorClass(NETWORK_RATE_WARNING_BYTES_PER_SECOND - 1)).toBe("")
		expect(getNetworkRateColorClass(NETWORK_RATE_WARNING_BYTES_PER_SECOND)).toBe("text-yellow-500")
		expect(getNetworkRateColorClass(NETWORK_RATE_CRITICAL_BYTES_PER_SECOND - 1)).toBe("text-yellow-500")
		expect(getNetworkRateColorClass(NETWORK_RATE_CRITICAL_BYTES_PER_SECOND)).toBe("text-red-500")
	})

	test("keeps invalid legacy aggregate values at the default color", () => {
		expect(getNetworkRateColorClass(undefined)).toBe("")
		expect(getNetworkRateColorClass(Number.NaN)).toBe("")
		expect(getNetworkRateColorClass(-1)).toBe("")
	})
})

describe("getDirectionalNetworkRateColorClasses", () => {
	test("KB/MB colors upload only", () => {
		expect(getDirectionalNetworkRateColorClasses(832 * 1024, 12.3 * 1024 ** 2)).toEqual({
			download: "",
			upload: "text-yellow-500",
		})
	})

	test("MB/MB colors both directions yellow", () => {
		expect(getDirectionalNetworkRateColorClasses(8.4 * 1024 ** 2, 42 * 1024 ** 2)).toEqual({
			download: "text-yellow-500",
			upload: "text-yellow-500",
		})
	})

	test("MB/GB colors directions independently", () => {
		expect(getDirectionalNetworkRateColorClasses(16.3 * 1024 ** 2, 1.2 * 1024 ** 3)).toEqual({
			download: "text-yellow-500",
			upload: "text-red-500",
		})
	})
})
