import { t } from "@lingui/core/macro"
import { useMemo } from "react"
import type { ChartData, SystemStatsRecord, VPSProbeTargetStats } from "@/types"
import { ChartCard } from "../chart-card"
import { CartesianGrid, Line, LineChart, Tooltip, YAxis } from "recharts"
import { ChartContainer, ChartLegend, xAxis } from "@/components/ui/chart"
import { chartMargin, cn, decimalString, formatShortDate } from "@/lib/utils"
import { useYAxisWidth } from "@/components/charts/hooks"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { useEffect, useState } from "react"

const probeColors = {
	hub: "hsl(217, 91%, 60%)",
	ct: "hsl(142, 71%, 45%)",
	cu: "hsl(25, 95%, 53%)",
	cm: "hsl(271, 81%, 60%)",
} as const

const probeKeys = ["hub", "ct", "cu", "cm"] as const
const probeLabels = { hub: "HUB", ct: "CT", cu: "CU", cm: "CM" } as const

function getDetailLat(p: VPSProbeTargetStats | undefined): number | undefined {
	if (!p || p.local) return undefined
	return p.lat1 != null && p.lat1 > 0 ? p.lat1 : undefined
}

function getDetailLoss(p: VPSProbeTargetStats | undefined): number | undefined {
	if (!p || p.local || !p.n1 || p.n1 <= 0) return undefined
	return Math.min(Math.max(p.loss1 ?? 0, 0), 100)
}

export function ProbeChart({
	chartData,
	grid,
	dataEmpty,
}: {
	chartData: ChartData
	grid: boolean
	dataEmpty: boolean
}) {
	const hasProbeData = chartData.systemStats.some((r) => {
		const vp = r.stats?.vp
		if (!vp) return false
		return Object.values(vp).some((p) => p.n1 && p.n1 > 0)
	})
	const { yAxisWidth: leftWidth, updateYAxisWidth: updateLeft } = useYAxisWidth()
	const { isIntersecting, ref } = useIntersectionObserver({ freeze: false })

	const sourceData = chartData.systemStats
	const [displayData, setDisplayData] = useState(sourceData)

	useEffect(() => {
		const shouldPrime = sourceData.length && !displayData.length
		const changed = sourceData !== displayData
		if (shouldPrime || (changed && isIntersecting)) {
			setDisplayData(sourceData)
		}
	}, [displayData, isIntersecting, sourceData])

	const lines = useMemo(() => {
		const result: JSX.Element[] = []
		for (const key of probeKeys) {
			const color = probeColors[key]
			const label = probeLabels[key]
			result.push(
				<Line
					key={`${label}-lat`}
					yAxisId="latency"
					dataKey={(r: SystemStatsRecord) => getDetailLat(r.stats?.vp?.[key])}
					name={`${label}-lat`}
					type="monotoneX"
					dot={false}
					strokeWidth={1.5}
					stroke={color}
					isAnimationActive={false}
					connectNulls={false}
					activeDot={false}
				/>
			)
			result.push(
				<Line
					key={`${label}-loss`}
					yAxisId="loss"
					dataKey={(r: SystemStatsRecord) => getDetailLoss(r.stats?.vp?.[key])}
					name={`${label}-loss`}
					type="monotoneX"
					dot={false}
					strokeWidth={1.5}
					stroke={color}
					strokeDasharray="4 3"
					isAnimationActive={false}
					connectNulls={false}
					activeDot={false}
				/>
			)
		}
		return result
	}, [])

	return (
		<ChartCard
			empty={dataEmpty || !hasProbeData}
			emptyMessage={!hasProbeData ? t`No probe data` : undefined}
			grid={grid}
			title={t`Latency / Loss`}
			description={t`TCP probe latency (ms) and packet loss (%)`}
			legend={true}
		>
			{hasProbeData && displayData.length > 0 && (
				<ChartContainer
					ref={ref}
					className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
						"opacity-100": leftWidth,
					})}
				>
					<LineChart data={displayData} margin={chartMargin}>
						<CartesianGrid vertical={false} />
						<YAxis
							yAxisId="latency"
							orientation={chartData.orientation}
							className="tracking-tighter"
							width={leftWidth}
							domain={[0, "auto"]}
							tickFormatter={(value) => updateLeft(`${Math.round(value)}ms`)}
							tickLine={false}
							axisLine={false}
						/>
						<YAxis
							yAxisId="loss"
							orientation={chartData.orientation === "left" ? "right" : "left"}
							className="tracking-tighter"
							width={50}
							domain={[0, 100]}
							tickFormatter={(value) => `${Math.round(value)}%`}
							tickLine={false}
							axisLine={false}
						/>
						{xAxis(chartData)}
						<Tooltip animationEasing="ease-out" animationDuration={150} content={<ProbeTooltip />} />
						{lines}
						<ChartLegend content={<ProbeLegend />} />
					</LineChart>
				</ChartContainer>
			)}
		</ChartCard>
	)
}

function ProbeLegend() {
	return (
		<div className="flex items-center justify-center gap-3 text-xs pt-1 flex-wrap">
			{probeKeys.map((key) => (
				<span key={key} className="flex items-center gap-1">
					<span className="inline-block w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: probeColors[key] }} />
					{probeLabels[key]}
				</span>
			))}
			<span className="flex items-center gap-1 text-muted-foreground">
				<span className="inline-block w-4 border-t-2 border-current" />
				{t`Latency`}
			</span>
			<span className="flex items-center gap-1 text-muted-foreground">
				<span className="inline-block w-4 border-t-2 border-dashed border-current" />
				{t`Packet loss`}
			</span>
		</div>
	)
}

// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip payload type
function ProbeTooltip({ active, payload }: any) {
	if (!active || !payload?.length) return null
	const record = payload[0]?.payload as SystemStatsRecord | undefined
	if (!record?.stats?.vp) return null

	return (
		<div className="rounded-lg border bg-background p-2 shadow-md text-xs">
			<p className="mb-1 text-muted-foreground">{formatShortDate(record.created)}</p>
			<div className="grid gap-0.5 font-mono">
				{probeKeys.map((key) => {
					const p = record.stats?.vp?.[key]
					if (!p) return null
					if (p.local) {
						return (
							<div key={key} className="flex items-center gap-2">
								<span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: probeColors[key] }} />
								<span className="w-7">{probeLabels[key]}</span>
								<span className="text-right w-28 text-muted-foreground">{t`Local`}</span>
							</div>
						)
					}
					const lat = getDetailLat(p)
					const loss = getDetailLoss(p)
					const latStr = lat != null ? `${decimalString(lat, 1)} ms` : "--"
					const lossStr = loss != null ? `${decimalString(loss, 1)}%` : "--"
					return (
						<div key={key} className="flex items-center gap-2">
							<span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: probeColors[key] }} />
							<span className="w-7">{probeLabels[key]}</span>
							<span className="text-right w-16">{latStr}</span>
							<span className="text-right w-12">{lossStr}</span>
						</div>
					)
				})}
			</div>
		</div>
	)
}
