import { t } from "@lingui/core/macro"
import PocketBase from "pocketbase"
import { basePath } from "@/components/router"
import { toast } from "@/components/ui/use-toast"
import type { ChartTimes, UserSettings } from "@/types"
import { $alerts, $allSystemsById, $allSystemsByName, $userSettings } from "./stores"
import { chartTimeData } from "./utils"

/** PocketBase JS Client */
export const pb = new PocketBase(basePath)

export const isAdmin = () => pb.authStore.record?.role === "admin"
export const isReadOnlyUser = () => pb.authStore.record?.role === "readonly"

export interface UserCapabilities {
	manageSystems: boolean
	manageAlerts: boolean
	manageSettings: boolean
	viewSensitiveDetails: boolean
}

export function getUserCapabilities(): UserCapabilities {
	const canManage = !isReadOnlyUser()
	return {
		manageSystems: canManage,
		manageAlerts: canManage,
		manageSettings: canManage,
		viewSensitiveDetails: canManage,
	}
}

export const shouldRedirectReadOnlyRoute = (route?: string) =>
	isReadOnlyUser() && (route === "settings" || route === "containers" || route === "smart")

export const shouldInitializeAlertManager = () => getUserCapabilities().manageAlerts

export const shouldShowProcessList = () => getUserCapabilities().viewSensitiveDetails

export function runIfSensitiveDetailsAllowed(action: () => void): boolean {
	if (!getUserCapabilities().viewSensitiveDetails) return false
	action()
	return true
}

export const verifyAuth = () => {
	pb.collection("users")
		.authRefresh()
		.catch(() => {
			logOut()
			toast({
				title: t`Failed to authenticate`,
				description: t`Please log in again`,
				variant: "destructive",
			})
		})
}

/** Logs the user out by clearing the auth store and unsubscribing from realtime updates. */
export function logOut() {
	$allSystemsByName.set({})
	$allSystemsById.set({})
	$alerts.set({})
	$userSettings.set({} as UserSettings)
	sessionStorage.setItem("lo", "t") // prevent auto login on logout
	pb.authStore.clear()
	pb.realtime.unsubscribe()
}

/** Fetch or create user settings in database */
export async function updateUserSettings() {
	try {
		const req = await pb.collection("user_settings").getFirstListItem("", { fields: "settings" })
		$userSettings.set(req.settings)
		return
	} catch (_) {
		// no existing settings record
	}
	if (!getUserCapabilities().manageSettings) {
		return
	}
	try {
		const createdSettings = await pb.collection("user_settings").create({ user: pb.authStore.record?.id })
		$userSettings.set(createdSettings.settings)
	} catch (e) {
		console.error("create settings", e)
	}
}

export function getPbTimestamp(timeString: ChartTimes, d?: Date) {
	d ||= chartTimeData[timeString].getOffset(new Date())
	const year = d.getUTCFullYear()
	const month = String(d.getUTCMonth() + 1).padStart(2, "0")
	const day = String(d.getUTCDate()).padStart(2, "0")
	const hours = String(d.getUTCHours()).padStart(2, "0")
	const minutes = String(d.getUTCMinutes()).padStart(2, "0")
	const seconds = String(d.getUTCSeconds()).padStart(2, "0")

	return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}
