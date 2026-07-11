import { describe, test, expect, mock } from "bun:test"

mock.module("@lingui/core/macro", () => ({
	plural: (...args: unknown[]) => String(args[0]),
	t: (strings: TemplateStringsArray) => strings.join(""),
}))

mock.module("@/components/ui/use-toast", () => ({
	toast: () => {},
}))

mock.module("@/lib/stores", () => ({
	$alerts: { set: () => {}, get: () => ({}) },
	$allSystemsById: { set: () => {} },
	$allSystemsByName: { set: () => {} },
	$userSettings: {
		set: () => {},
		setKey: () => {},
		get: () => ({}),
		listen: () => () => {},
	},
	$copyContent: { set: () => {} },
	defaultLayoutWidth: 1440,
}))

const { compactMetricNumber, formatCompactWithUnit, formatProbeTooltipValue } = await import("./utils")

describe("compactMetricNumber", () => {
	test.each([
		[0, "0"],
		[0.004, "0.00"],
		[0.1, "0.10"],
		[0.126, "0.13"],
		[1, "1.00"],
		[1.234, "1.23"],
		[10, "10.0"],
		[12.34, "12.3"],
		[100, "100"],
		[123.4, "123"],
		[999.4, "999"],
	])("compactMetricNumber(%s) = %s", (input, expected) => {
		expect(compactMetricNumber(input)).toBe(expected)
	})

	test("0.999 rounds to 1.00", () => {
		expect(compactMetricNumber(0.999)).toBe("1.00")
	})

	test("9.999 rounds to 10.0", () => {
		expect(compactMetricNumber(9.999)).toBe("10.0")
	})

	test("99.99 rounds to 100", () => {
		expect(compactMetricNumber(99.99)).toBe("100")
	})

	test("999.5 rounds to 1000 (caller handles unit promotion)", () => {
		expect(compactMetricNumber(999.5)).toBe("1000")
	})

	test("NaN returns 0", () => {
		expect(compactMetricNumber(NaN)).toBe("0")
	})

	test("Infinity returns 0", () => {
		expect(compactMetricNumber(Infinity)).toBe("0")
	})

	test("-Infinity returns 0", () => {
		expect(compactMetricNumber(-Infinity)).toBe("0")
	})

	test("negative values preserve sign", () => {
		expect(compactMetricNumber(-5.67)).toBe("-5.67")
		expect(compactMetricNumber(-123.4)).toBe("-123")
	})
})

describe("formatCompactWithUnit", () => {
	test("zero value keeps unit", () => {
		expect(formatCompactWithUnit(0, "KB/s")).toBe("0KB/s")
	})

	test("normal values format correctly", () => {
		expect(formatCompactWithUnit(12.34, "MB/s")).toBe("12.3MB/s")
		expect(formatCompactWithUnit(1.234, "GB")).toBe("1.23GB")
		expect(formatCompactWithUnit(123.4, "KB")).toBe("123KB")
	})

	test("promotes byte unit when rounding exceeds 999", () => {
		const result = formatCompactWithUnit(999.5, "KB/s")
		expect(result).toContain("MB/s")
		expect(result).not.toContain("1000")
	})

	test("promotes bit unit when rounding exceeds 999", () => {
		const result = formatCompactWithUnit(999.5, "Kbps")
		expect(result).toContain("Mbps")
		expect(result).not.toContain("1000")
	})

	test("promotes plain byte unit", () => {
		const result = formatCompactWithUnit(999.5, "KB")
		expect(result).toContain("MB")
		expect(result).not.toContain("1000")
	})

	test("1004 KB promotes to 0.98MB", () => {
		expect(formatCompactWithUnit(1004, "KB")).toBe("0.98MB")
	})

	test("repeatedly promotes very large values without a four-digit mantissa", () => {
		expect(formatCompactWithUnit(1024 ** 2, "TB")).toBe("1.00EB")
		expect(formatCompactWithUnit(1000 ** 2, "Tbps")).toBe("1.00Ebps")
	})

	test("NaN keeps unit", () => {
		expect(formatCompactWithUnit(NaN, "MB/s")).toBe("0MB/s")
	})

	test("unknown unit falls through without promotion", () => {
		expect(formatCompactWithUnit(999.5, "??")).toBe("1000??")
	})
})

describe("formatProbeTooltipValue", () => {
	test("formats latency and loss with correct spacing", () => {
		expect(formatProbeTooltipValue("123", "0.0")).toBe("123ms 0.0%")
	})

	test("no comma in output", () => {
		expect(formatProbeTooltipValue("123", "0.0")).not.toContain(",")
	})

	test("contains exact format: CN: 123ms 0.0%", () => {
		const value = formatProbeTooltipValue("123", "0.0")
		expect(`CN: ${value}`).toBe("CN: 123ms 0.0%")
	})

	test("missing latency shows -- without ms", () => {
		const result = formatProbeTooltipValue("--", "2.5")
		expect(result).toBe("-- 2.5%")
		expect(result).not.toContain("--ms")
	})

	test("missing loss shows -- without %", () => {
		const result = formatProbeTooltipValue("45", "--")
		expect(result).toBe("45ms --")
		expect(result).not.toContain("--%")
	})

	test("both missing", () => {
		expect(formatProbeTooltipValue("--", "--")).toBe("-- --")
	})

	test("uses single space between latency and loss", () => {
		const result = formatProbeTooltipValue("123", "0.0")
		expect(result).not.toMatch(/ {2}/)
		expect(result).not.toContain("\u00a0")
	})
})
