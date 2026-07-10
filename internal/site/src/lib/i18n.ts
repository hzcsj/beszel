import type { Messages } from "@lingui/core"
import { i18n } from "@lingui/core"
import { t } from "@lingui/core/macro"
import { detect, fromNavigator, fromStorage } from "@lingui/detect-locale"
import languages from "@/lib/languages"
import { messages as enMessages } from "@/locales/en/en"
import { BatteryState } from "./enums"
import { $direction } from "./stores"

const rtlLanguages = new Set(["ar", "fa", "he"])

// activates locale
function activateLocale(locale: string, messages: Messages = enMessages) {
	i18n.load(locale, messages)
	i18n.activate(locale)
	document.documentElement.lang = locale
	localStorage.setItem("lang", locale)
	$direction.set(rtlLanguages.has(locale) ? "rtl" : "ltr")
}

// dynamically loads translations for the given locale
export async function dynamicActivate(locale: string) {
	if (locale === "en") {
		activateLocale(locale)
		return
	}
	try {
		if (locale === "zh-HK") {
			const [localeModule, zhCNModule] = await Promise.all([
				import("../locales/zh-HK/zh-HK.ts"),
				import("../locales/zh-CN/zh-CN.ts"),
			])
			const merged: Messages = { ...enMessages, ...zhCNModule.messages, ...localeModule.messages }
			activateLocale(locale, merged)
		} else {
			const { messages }: { messages: Messages } = await import(`../locales/${locale}/${locale}.ts`)
			const merged: Messages = { ...enMessages, ...messages }
			activateLocale(locale, merged)
		}
	} catch (error) {
		console.error(`Error loading ${locale}`, error)
		activateLocale("en")
	}
}

const legacyZhMap: Record<string, string> = {
	zh: "zh-HK",
	"zh-TW": "zh-HK",
	"zh-MO": "zh-HK",
	"zh-Hant": "zh-HK",
	"zh-Hans": "zh-CN",
}

export function getLocale() {
	let locale = detect(fromStorage("lang"), fromNavigator(), "en")
	if (import.meta.env.DEV) {
		console.log("detected locale", locale)
	}
	if (locale) {
		const mapped = legacyZhMap[locale]
		if (mapped) {
			localStorage.setItem("lang", mapped)
			return mapped
		}
		if (locale.startsWith("zh-")) {
			if (locale === "zh-HK") return "zh-HK"
			return "zh-CN"
		}
	}
	locale = (locale || "en").split("-")[0]
	if (!languages.some((l) => l[0] === locale)) {
		locale = "en"
	}
	return locale
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
