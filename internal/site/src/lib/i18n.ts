import type { Messages } from "@lingui/core"
import { i18n } from "@lingui/core"
import { t } from "@lingui/core/macro"
import { detect, fromNavigator, fromStorage } from "@lingui/detect-locale"
import { messages as enMessages } from "@/locales/en/en"
import { BatteryState } from "./enums"
import { $direction } from "./stores"

function activateLocale(locale: string, messages: Messages = enMessages) {
	i18n.load(locale, messages)
	i18n.activate(locale)
	document.documentElement.lang = locale
	localStorage.setItem("lang", locale)
	$direction.set("ltr")
}

export async function dynamicActivate(locale: string) {
	const resolved = normalizeLocale(locale)
	if (resolved === "en") {
		activateLocale(resolved)
		return
	}
	try {
		const { messages }: { messages: Messages } = await import("../locales/zh-CN/zh-CN.ts")
		const merged: Messages = { ...enMessages, ...messages }
		activateLocale(resolved, merged)
	} catch (error) {
		console.error("Error loading zh-CN", error)
		activateLocale("en")
	}
}

function normalizeLocale(raw: string | null | undefined): "en" | "zh-CN" {
	if (!raw) return "en"
	const lower = raw.toLowerCase()
	if (lower === "zh" || lower.startsWith("zh-")) return "zh-CN"
	return "en"
}

export function getLocale(): "en" | "zh-CN" {
	const detected = detect(fromStorage("lang"), fromNavigator(), "en")
	if (import.meta.env.DEV) {
		console.log("detected locale", detected)
	}
	return normalizeLocale(detected)
}

////////////////////////////////////////////////////////

export const batteryStateTranslations = {
	[BatteryState.Unknown]: () => t({ message: "Unknown", comment: "Context: Battery state" }),
	[BatteryState.Empty]: () => t({ message: "Empty", comment: "Context: Battery state" }),
	[BatteryState.Full]: () => t({ message: "Full", comment: "Context: Battery state" }),
	[BatteryState.Charging]: () => t({ message: "Charging", comment: "Context: Battery state" }),
	[BatteryState.Discharging]: () => t({ message: "Discharging", comment: "Context: Battery state" }),
	[BatteryState.Idle]: () => t({ message: "Idle", comment: "Context: Battery state" }),
} as const
