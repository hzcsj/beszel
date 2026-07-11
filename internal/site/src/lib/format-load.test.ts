import { describe, expect, test } from "bun:test"
import { formatLoad } from "./format-load"

describe("formatLoad", () => {
	const cases: [number, string][] = [
		[0, "0.00"],
		[0.004, "0.00"],
		[0.005, "0.01"],
		[0.01, "0.01"],
		[0.1, "0.10"],
		[1.2, "1.20"],
		[9.994, "9.99"],
		[9.995, "10.0"],
		[9.999, "10.0"],
		[10, "10.0"],
		[12.34, "12.3"],
		[99.94, "99.9"],
		[99.95, "100"],
		[123.4, "123"],
		[999.4, "999"],
		[999.5, "1.00K"],
		[1004, "1.00K"],
		[12_340, "12.3K"],
		[1e-7, "0.00"],
		[1e20, "100E"],
	]

	for (const [input, expected] of cases) {
		test(`formatLoad(${input}) = "${expected}"`, () => {
			expect(formatLoad(input)).toBe(expected)
		})
	}

	test("NaN returns 0.00", () => {
		expect(formatLoad(Number.NaN)).toBe("0.00")
	})

	test("Infinity returns 0.00", () => {
		expect(formatLoad(Number.POSITIVE_INFINITY)).toBe("0.00")
	})

	test("-Infinity returns 0.00", () => {
		expect(formatLoad(Number.NEGATIVE_INFINITY)).toBe("0.00")
	})

	test("negative values return 0.00", () => {
		expect(formatLoad(-1)).toBe("0.00")
		expect(formatLoad(-0.5)).toBe("0.00")
	})
})
