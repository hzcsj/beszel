import { describe, expect, test } from "bun:test"
import {
	getProbeLossLevel,
	resolveCompactProbeTargets,
	resolveProbeTargets,
	getListLatency,
	getListLoss,
	getDetailLat,
	getDetailLoss,
	getDynamicProbeColor,
} from "./probe-utils"
import type { VPSProbeTargetStats } from "@/types"

describe("getProbeLossLevel", () => {
	test("normal when loss < 5", () => {
		expect(getProbeLossLevel(0, false, false)).toBe("normal")
		expect(getProbeLossLevel(4.99, false, false)).toBe("normal")
	})
	test("warning when loss >= 5 and < 20", () => {
		expect(getProbeLossLevel(5, false, false)).toBe("warning")
		expect(getProbeLossLevel(19.99, false, false)).toBe("warning")
	})
	test("critical when loss >= 20", () => {
		expect(getProbeLossLevel(20, false, false)).toBe("critical")
		expect(getProbeLossLevel(100, false, false)).toBe("critical")
	})
	test("muted for local", () => {
		expect(getProbeLossLevel(50, true, false)).toBe("muted")
	})
	test("muted for missing", () => {
		expect(getProbeLossLevel(undefined, false, true)).toBe("muted")
	})
	test("undefined loss treated as 0 (normal) when not missing", () => {
		expect(getProbeLossLevel(undefined, false, false)).toBe("normal")
	})
	test("100% loss with ok=false is critical, not muted", () => {
		expect(getProbeLossLevel(100, false, false)).toBe("critical")
	})
})

describe("resolveCompactProbeTargets", () => {
	test("returns only the first three ordered targets while full resolution keeps four", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			cross: { ok: true, pos: 4, label: "HK" },
			cm: { ok: true, pos: 3, label: "CM" },
			ct: { ok: true, pos: 1, label: "CT" },
			cu: { ok: true, pos: 2, label: "CU" },
		}
		expect(resolveProbeTargets(vp).map((r) => r.id)).toEqual(["ct", "cu", "cm", "cross"])
		expect(resolveCompactProbeTargets(vp).map((r) => r.id)).toEqual(["ct", "cu", "cm"])
	})
})

describe("resolveProbeTargets", () => {
	test("returns empty for undefined", () => {
		expect(resolveProbeTargets(undefined)).toEqual([])
	})

	test("returns empty for empty map", () => {
		expect(resolveProbeTargets({})).toEqual([])
	})

	test("sorts by pos ascending", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			cm: { ok: true, pos: 3, label: "CM" },
			ct: { ok: true, pos: 1, label: "CT" },
			cu: { ok: true, pos: 2, label: "CU" },
		}
		const result = resolveProbeTargets(vp)
		expect(result.map((r) => r.id)).toEqual(["ct", "cu", "cm"])
	})

	test("pos 0 falls back to ID sort", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			hub: { ok: true },
			ct: { ok: true },
		}
		const result = resolveProbeTargets(vp)
		expect(result.map((r) => r.id)).toEqual(["ct", "hub"])
	})

	test("pos targets before no-pos targets", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			legacy: { ok: true },
			cn: { ok: true, pos: 1, label: "CN" },
		}
		const result = resolveProbeTargets(vp)
		expect(result[0].id).toBe("cn")
		expect(result[1].id).toBe("legacy")
	})

	test("uses label from stats, falls back to uppercase ID", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			cn: { ok: true, pos: 1, label: "中国" },
			hk: { ok: true, pos: 2 },
		}
		const result = resolveProbeTargets(vp)
		expect(result[0].label).toBe("中国")
		expect(result[1].label).toBe("HK")
	})

	test("duplicate pos demotes all to 0 and sorts by ID", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			bbb: { ok: true, pos: 1 },
			aaa: { ok: true, pos: 1 },
		}
		const result = resolveProbeTargets(vp)
		expect(result[0].id).toBe("aaa")
		expect(result[0].pos).toBe(0)
		expect(result[1].id).toBe("bbb")
		expect(result[1].pos).toBe(0)
	})

	test("pos 4 is valid and values above 4 fall back to 0", () => {
		const vp: Record<string, VPSProbeTargetStats> = {
			x: { ok: true, pos: 4 },
			y: { ok: true, pos: 1 },
			z: { ok: true, pos: 5 },
		}
		const result = resolveProbeTargets(vp)
		expect(result[0].id).toBe("y")
		expect(result[0].pos).toBe(1)
		expect(result[1].id).toBe("x")
		expect(result[1].pos).toBe(4)
		expect(result[2].id).toBe("z")
		expect(result[2].pos).toBe(0)
	})

	test("0/1/2/3/4 targets", () => {
		expect(resolveProbeTargets({})).toHaveLength(0)
		expect(resolveProbeTargets({ a: { ok: true, pos: 1 } })).toHaveLength(1)
		expect(resolveProbeTargets({ a: { ok: true, pos: 1 }, b: { ok: true, pos: 2 } })).toHaveLength(2)
		expect(
			resolveProbeTargets({ a: { ok: true, pos: 1 }, b: { ok: true, pos: 2 }, c: { ok: true, pos: 3 } })
		).toHaveLength(3)
		expect(
			resolveProbeTargets({
				a: { ok: true, pos: 1 },
				b: { ok: true, pos: 2 },
				c: { ok: true, pos: 3 },
				d: { ok: true, pos: 4 },
			})
		).toHaveLength(4)
	})
})

describe("getListLatency", () => {
	test("prefers latw", () => {
		expect(getListLatency({ ok: true, latw: 50, lat1: 40, lat: 30 })).toBe(50)
	})
	test("falls back to lat1", () => {
		expect(getListLatency({ ok: true, lat1: 40, lat: 30 })).toBe(40)
	})
	test("falls back to lat", () => {
		expect(getListLatency({ ok: true, lat: 30 })).toBe(30)
	})
	test("returns undefined for zero", () => {
		expect(getListLatency({ ok: true, lat: 0 })).toBeUndefined()
	})
	test("returns undefined for missing", () => {
		expect(getListLatency({ ok: false })).toBeUndefined()
	})
})

describe("getListLoss", () => {
	test("infers zero loss when samples exist and loss is omitted", () => {
		expect(getListLoss({ ok: true, n: 33 })).toBe(0)
	})
	test("returns and clamps explicit loss when samples exist", () => {
		expect(getListLoss({ ok: true, n: 33, loss: 5.5 })).toBe(5.5)
		expect(getListLoss({ ok: true, n: 33, loss: -5 })).toBe(0)
		expect(getListLoss({ ok: true, n: 33, loss: 150 })).toBe(100)
	})
	test("returns undefined without samples or for local targets", () => {
		expect(getListLoss({ ok: false })).toBeUndefined()
		expect(getListLoss({ ok: true, n: 0, loss: 0 })).toBeUndefined()
		expect(getListLoss({ ok: true, n: 33, local: true })).toBeUndefined()
	})
})

describe("getDetailLat", () => {
	test("returns lat1 when positive", () => {
		expect(getDetailLat({ ok: true, lat1: 42 })).toBe(42)
	})
	test("returns undefined for local", () => {
		expect(getDetailLat({ ok: true, lat1: 42, local: true })).toBeUndefined()
	})
	test("returns undefined when lat1 is 0", () => {
		expect(getDetailLat({ ok: false, lat1: 0 })).toBeUndefined()
	})
	test("returns undefined for undefined input", () => {
		expect(getDetailLat(undefined)).toBeUndefined()
	})
})

describe("getDetailLoss", () => {
	test("returns clamped loss1 when n1 > 0", () => {
		expect(getDetailLoss({ ok: true, loss1: 5.5, n1: 12 })).toBe(5.5)
	})
	test("clamps to 0-100", () => {
		expect(getDetailLoss({ ok: true, loss1: -5, n1: 12 })).toBe(0)
		expect(getDetailLoss({ ok: true, loss1: 150, n1: 12 })).toBe(100)
	})
	test("returns undefined for local", () => {
		expect(getDetailLoss({ ok: true, loss1: 5, n1: 12, local: true })).toBeUndefined()
	})
	test("returns undefined when n1 is 0", () => {
		expect(getDetailLoss({ ok: true, loss1: 5, n1: 0 })).toBeUndefined()
	})
	test("returns undefined for undefined input", () => {
		expect(getDetailLoss(undefined)).toBeUndefined()
	})
})

describe("getDynamicProbeColor", () => {
	test("returns different colors for 0,1,2,3", () => {
		const c0 = getDynamicProbeColor(0)
		const c1 = getDynamicProbeColor(1)
		const c2 = getDynamicProbeColor(2)
		const c3 = getDynamicProbeColor(3)
		expect(c0).not.toBe(c1)
		expect(c1).not.toBe(c2)
		expect(c2).not.toBe(c3)
		expect(c3).not.toBe(c0)
	})
	test("wraps around", () => {
		expect(getDynamicProbeColor(4)).toBe(getDynamicProbeColor(0))
	})
})
