import { describe, expect, test } from "bun:test"
import type { SystemRecord } from "@/types"
import { compareSystemsByOrder } from "./system-order"

function system(name: string, order?: number): SystemRecord {
	return { name, vps: order === undefined ? undefined : { order } } as SystemRecord
}

describe("compareSystemsByOrder", () => {
	test("sorts configured systems by ascending order", () => {
		const systems = [system("Japan", 6), system("Hangzhou", 1), system("Bandwagon", 2)]
		expect(systems.sort(compareSystemsByOrder).map(({ name }) => name)).toEqual(["Hangzhou", "Bandwagon", "Japan"])
	})

	test("places configured systems before unconfigured systems", () => {
		const systems = [system("Alpha"), system("Configured", 10), system("Beta")]
		expect(systems.sort(compareSystemsByOrder).map(({ name }) => name)).toEqual(["Configured", "Alpha", "Beta"])
	})

	test("falls back to name for equal or missing order", () => {
		const systems = [system("Zulu", 2), system("Alpha", 2), system("Delta"), system("Beta")]
		expect(systems.sort(compareSystemsByOrder).map(({ name }) => name)).toEqual(["Alpha", "Zulu", "Beta", "Delta"])
	})

	test("ignores non-finite order values", () => {
		const systems = [system("Zulu", Number.NaN), system("Alpha", Number.POSITIVE_INFINITY), system("Middle", 1)]
		expect(systems.sort(compareSystemsByOrder).map(({ name }) => name)).toEqual(["Middle", "Alpha", "Zulu"])
	})
})
