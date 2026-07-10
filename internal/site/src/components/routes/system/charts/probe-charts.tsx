import { t } from "@lingui/core/macro"
import { useMemo, useState } from "react"
import type { ChartData, SystemStatsRecord, VPSProbeTargetStats } from "@/types"
import { ChartCard } from "../chart-card"
import { CartesianGrid, Line, LineChart, Tooltip, YAxis } from "recharts"
import { ChartContainer, xAxis } from "@/components/ui/chart"
import { chartMargin, cn, decimalString, formatShortDate } from "@/lib/utils"
import { useYAxisWidth } from "@/components/charts/hooks"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { useEffect } from "react"

const probeColors = {
	hub: "hsl(217, 91%, 60%)",
	ct: "hsl(142, 71%, 45%)",
	cu: "hsl(25, 95%, 53%)",
	cm: "hsl(271, 81%, 60%)",
} as const

const probeKeys = ["hub", "ct", "cu", "cm"] as const
const probeLabels = { hub: "HUB", ct: "CT", cu: "CU", cm: "CM" } as const

type ProbeKey = (typeof probeKeys)[number]
type TargetFilter = "all" | ProbeKey
type MetricFilter = "all" | "latency" | "loss"

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

	const [targetFilter, setTargetFilter] = useState<TargetFilter>("all")
	const [metricFilter, setMetricFilter] = useState<MetricFilter>("all")

	const sourceData = chartData.systemStats
	const [displayData, setDisplayData] = useState(sourceData)

	useEffect(() => {
		const shouldPrime = sourceData.length && !displayData.length
		const changed = sourceData !== displayData
		if (shouldPrime || (changed && isIntersecting)) {
			setDisplayData(sourceData)
		}
	}, [displayData, isIntersecting, sourceData])

	const showLat = metricFilter === "all" || metricFilter === "latency"
	const showLoss = metricFilter === "all" || metricFilter === "loss"

	const visibleTargets = targetFilter === "all" ? probeKeys : [targetFilter]

	const filteredIsLocalOnly =
		targetFilter !== "all" &&
		displayData.some((r) => r.stats?.vp?.[targetFilter]?.local) &&
		!displayData.some((r) => {
			const p = r.stats?.vp?.[targetFilter]
			if (!p || p.local) return false
			return (showLat && getDetailLat(p) != null) || (showLoss && getDetailLoss(p) != null)
		})

	const filteredHasNumericData =
		!filteredIsLocalOnly &&
		displayData.some((r) => {
			const vp = r.stats?.vp
			if (!vp) return false
			return visibleTargets.some((key) => {
				const p = vp[key]
				if (!p || p.local) return false
				return (showLat && getDetailLat(p) != null) || (showLoss && getDetailLoss(p) != null)
			})
		})

	const lines = useMemo(() => {
		const result: JSX.Element[] = []
		for (const key of visibleTargets) {
			const color = probeColors[key]
			const label = probeLabels[key]
			if (showLat) {
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
			}
			if (showLoss) {
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
		}
		return result
	}, [visibleTargets, showLat, showLoss])

	const handleTargetClick = (key: ProbeKey) => {
		setTargetFilter((prev) => (prev === key ? "all" : key))
	}

	const handleMetricClick = (metric: "latency" | "loss") => {
		setMetricFilter((prev) => (prev === metric ? "all" : metric))
	}

	const needLatAxis = showLat
	const needLossAxis = showLoss

	const scopedEmpty = hasProbeData && (filteredIsLocalOnly || !filteredHasNumericData)
	const scopedEmptyMessage = filteredIsLocalOnly ? t`Local` : t`No data for selected filter`

	return (
		<ChartCard
			empty={dataEmpty || !hasProbeData || scopedEmpty}
			emptyMessage={!hasProbeData ? t`No probe data` : scopedEmpty ? scopedEmptyMessage : undefined}
			grid={grid}
			title={t`Latency / Loss`}
			description={t`TCP probe latency (ms) and packet loss (%)`}
			legend={hasProbeData}
		>
			{hasProbeData && (
				<div className="flex flex-col h-full w-full absolute inset-0">
					<div className="flex-1 relative min-h-0">
						{!scopedEmpty && displayData.length > 0 && (
							<ChartContainer
								ref={ref}
								className={cn("h-full w-full absolute aspect-auto bg-card opacity-0 transition-opacity", {
									"opacity-100": needLatAxis ? leftWidth : true,
								})}
							>
								<LineChart data={displayData} margin={chartMargin}>
									<CartesianGrid vertical={false} />
									{needLatAxis && (
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
									)}
									{!needLatAxis && <YAxis yAxisId="latency" hide width={0} />}
									{needLossAxis && (
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
									)}
									{!needLossAxis && <YAxis yAxisId="loss" hide width={0} />}
									{xAxis(chartData)}
									<Tooltip
										animationEasing="ease-out"
										animationDuration={150}
										content={<ProbeTooltip visibleTargets={visibleTargets} showLat={showLat} showLoss={showLoss} />}
									/>
									{lines}
								</LineChart>
							</ChartContainer>
						)}
					</div>
					<div className="shrink-0 pb-1">
						<ProbeLegend
							targetFilter={targetFilter}
							metricFilter={metricFilter}
							onTargetClick={handleTargetClick}
							onMetricClick={handleMetricClick}
						/>
					</div>
				</div>
			)}
		</ChartCard>
	)
}

function ProbeLegend({
	targetFilter,
	metricFilter,
	onTargetClick,
	onMetricClick,
}: {
	targetFilter: TargetFilter
	metricFilter: MetricFilter
	onTargetClick: (key: ProbeKey) => void
	onMetricClick: (metric: "latency" | "loss") => void
}) {
	return (
		<div className="flex items-center justify-center gap-1.5 text-xs pt-1 flex-wrap">
			{probeKeys.map((key) => {
				const selected = targetFilter === key
				return (
					<button
						key={key}
						type="button"
						aria-pressed={selected}
						onClick={() => onTargetClick(key)}
						className={cn(
							"flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-transparent transition-colors cursor-pointer",
							"hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
							selected && "border-border bg-muted font-medium",
							!selected && targetFilter !== "all" && "opacity-40"
						)}
					>
						<span className="inline-block w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: probeColors[key] }} />
						{probeLabels[key]}
					</button>
				)
			})}
			<span className="text-muted-foreground mx-0.5">|</span>
			<button
				type="button"
				aria-pressed={metricFilter === "latency"}
				onClick={() => onMetricClick("latency")}
				className={cn(
					"flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-transparent transition-colors cursor-pointer",
					"hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
					metricFilter === "latency" && "border-border bg-muted font-medium",
					metricFilter !== "all" && metricFilter !== "latency" && "opacity-40"
				)}
			>
				<span className="inline-block w-4 border-t-2 border-current" />
				{t`Latency`}
			</button>
			<button
				type="button"
				aria-pressed={metricFilter === "loss"}
				onClick={() => onMetricClick("loss")}
				className={cn(
					"flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-transparent transition-colors cursor-pointer",
					"hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
					metricFilter === "loss" && "border-border bg-muted font-medium",
					metricFilter !== "all" && metricFilter !== "loss" && "opacity-40"
				)}
			>
				<span className="inline-block w-4 border-t-2 border-dashed border-current" />
				{t`Packet loss`}
			</button>
		</div>
	)
}

function ProbeTooltip(props: {
	active?: boolean
	// biome-ignore lint/suspicious/noExplicitAny: recharts tooltip payload type
	payload?: any[]
	visibleTargets: readonly ProbeKey[]
	showLat: boolean
	showLoss: boolean
}) {
	const { active, payload, visibleTargets, showLat, showLoss } = props
	if (!active || !payload?.length) return null
	const record = payload[0]?.payload as SystemStatsRecord | undefined
	if (!record?.stats?.vp) return null

	return (
		<div className="rounded-lg border bg-background p-2 shadow-md text-xs">
			<p className="mb-1 text-muted-foreground">{formatShortDate(record.created)}</p>
			<div className="grid gap-0.5 font-mono">
				{visibleTargets.map((key) => {
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
							{showLat && <span className="text-right w-16">{latStr}</span>}
							{showLoss && <span className="text-right w-12">{lossStr}</span>}
						</div>
					)
				})}
			</div>
		</div>
	)
}
