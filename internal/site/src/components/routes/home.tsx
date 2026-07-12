import { useLingui } from "@lingui/react/macro"
import { memo, Suspense, useCallback, useEffect, useMemo } from "react"
import SystemsTable from "@/components/systems-table/systems-table"
import { ActiveAlerts } from "@/components/active-alerts"
import { FooterRepoLink } from "@/components/footer-repo-link"
import { subscribeRealtime, unsubscribeRealtime } from "@/lib/systemsManager"

export default memo(() => {
	const { t } = useLingui()

	useEffect(() => {
		document.title = `${t`All Nodes`} / Beszel`
	}, [t])

	const handleVisibilityChange = useCallback(() => {
		if (document.visibilityState === "visible") {
			subscribeRealtime()
		} else {
			unsubscribeRealtime()
		}
	}, [])

	useEffect(() => {
		subscribeRealtime()
		document.addEventListener("visibilitychange", handleVisibilityChange)
		return () => {
			document.removeEventListener("visibilitychange", handleVisibilityChange)
			unsubscribeRealtime()
		}
	}, [handleVisibilityChange])

	return useMemo(
		() => (
			<>
				<div className="flex flex-col gap-4">
					<ActiveAlerts />
					<Suspense>
						<SystemsTable />
					</Suspense>
				</div>
				<FooterRepoLink />
			</>
		),
		[]
	)
})
