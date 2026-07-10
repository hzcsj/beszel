import { t } from "@lingui/core/macro"
import { useState } from "react"
import type { ChartData } from "@/types"
import { ChartCard } from "../chart-card"
import LineChartDefault from "@/components/charts/line-chart"
import { decimalString } from "@/lib/utils"

type ProbeMode = "latency" | "loss"

export function ProbeChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const [mode, setMode] = useState<ProbeMode>("latency")

	const hasProbeData = chartData.systemStats.some((r) => r.stats?.vp)

	const isLatency = mode === "latency"

	return (
		<ChartCard
			empty={dataEmpty || !hasProbeData}
			emptyMessage={!hasProbeData ? t`No probe data` : undefined}
			grid={grid}
			title={t`Latency / Loss`}
			description={isLatency ? t`TCP probe latency (ms)` : t`Packet loss (%)`}
			legend={true}
			cornerEl={
				hasProbeData ? (
					<div className="flex gap-1 text-xs">
						<button
							type="button"
							className={`px-2 py-0.5 rounded ${isLatency ? "bg-primary text-primary-foreground" : "bg-muted"}`}
							onClick={() => setMode("latency")}
						>
							{t`Latency`}
						</button>
						<button
							type="button"
							className={`px-2 py-0.5 rounded ${!isLatency ? "bg-primary text-primary-foreground" : "bg-muted"}`}
							onClick={() => setMode("loss")}
						>
							{t`Packet Loss`}
						</button>
					</div>
				) : null
			}
		>
			{hasProbeData && (
				<LineChartDefault
					chartData={chartData}
					contentFormatter={(item) =>
						isLatency ? `${decimalString(item.value, 1)} ms` : `${decimalString(item.value, 1)}%`
					}
					tickFormatter={(value) => (isLatency ? `${Math.round(value)}` : `${Math.round(value)}%`)}
					legend={true}
					domain={isLatency ? undefined : [0, 100]}
					dataPoints={
						isLatency
							? [
									{
										label: "HUB",
										color: "hsl(217, 91%, 60%)",
										dataKey: ({ stats }) => (stats?.vp?.hub?.ok ? stats.vp.hub.lat : undefined),
									},
									{
										label: "CT",
										color: "hsl(142, 71%, 45%)",
										dataKey: ({ stats }) => (stats?.vp?.ct?.ok ? stats.vp.ct.lat : undefined),
									},
									{
										label: "CU",
										color: "hsl(25, 95%, 53%)",
										dataKey: ({ stats }) => (stats?.vp?.cu?.ok ? stats.vp.cu.lat : undefined),
									},
									{
										label: "CM",
										color: "hsl(271, 81%, 60%)",
										dataKey: ({ stats }) => (stats?.vp?.cm?.ok ? stats.vp.cm.lat : undefined),
									},
								]
							: [
									{
										label: "HUB",
										color: "hsl(217, 91%, 60%)",
										dataKey: ({ stats }) => stats?.vp?.hub?.loss,
									},
									{
										label: "CT",
										color: "hsl(142, 71%, 45%)",
										dataKey: ({ stats }) => stats?.vp?.ct?.loss,
									},
									{
										label: "CU",
										color: "hsl(25, 95%, 53%)",
										dataKey: ({ stats }) => stats?.vp?.cu?.loss,
									},
									{
										label: "CM",
										color: "hsl(271, 81%, 60%)",
										dataKey: ({ stats }) => stats?.vp?.cm?.loss,
									},
								]
					}
				/>
			)}
		</ChartCard>
	)
}
