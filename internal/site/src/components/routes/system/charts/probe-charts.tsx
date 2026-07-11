import { t } from "@lingui/core/macro"
import { useMemo, useState } from "react"
import type { ChartData, SystemStatsRecord } from "@/types"
import { ChartCard } from "../chart-card"
import { CartesianGrid, Line, LineChart, Tooltip, YAxis } from "recharts"
import { ChartContainer, xAxis } from "@/components/ui/chart"
import { chartMargin, cn, decimalString, formatShortDate } from "@/lib/utils"
import { useYAxisWidth } from "@/components/charts/hooks"
import { useIntersectionObserver } from "@/lib/use-intersection-observer"
import { useEffect } from "react"
import { resolveProbeTargets, getDynamicProbeColor, getDetailLat, getDetailLoss } from "@/lib/probe-utils"

type TargetFilter = "all" | string
type MetricFilter = "all" | "latency" | "loss"

interface DynTarget {
	id: string
	label: string
	color: string
}

function discoverTargets(records: SystemStatsRecord[]): DynTarget[] {
	if (!records.length) return []
	const latest = records[records.length - 1]
	const currentTargets = resolveProbeTargets(latest.stats?.vp)
	if (!currentTargets.length) return []

	const targets: DynTarget[] = []
	for (const ct of currentTargets) {
		let label = ct.label
		for (let i = records.length - 1; i >= 0; i--) {
			const p = records[i].stats?.vp?.[ct.id]
			if (p?.label) {
				label = p.label
				break
			}
		}
		targets.push({
			id: ct.id,
			label,
			color: getDynamicProbeColor(targets.length),
		})
	}
	return targets
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
	const sourceData = chartData.systemStats

	const allTargets = useMemo(() => discoverTargets(sourceData), [sourceData])

	const hasProbeData = allTargets.length > 0
	const { yAxisWidth: leftWidth, updateYAxisWidth: updateLeft } = useYAxisWidth()
	const { isIntersecting, ref } = useIntersectionObserver({ freeze: false })

	const [targetFilter, setTargetFilter] = useState<TargetFilter>("all")
	const [metricFilter, setMetricFilter] = useState<MetricFilter>("all")

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

	const visibleTargets = useMemo(() => {
		if (targetFilter === "all") return allTargets
		return allTargets.filter((t) => t.id === targetFilter)
	}, [targetFilter, allTargets])

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
			return visibleTargets.some((t) => {
				const p = vp[t.id]
				if (!p || p.local) return false
				return (showLat && getDetailLat(p) != null) || (showLoss && getDetailLoss(p) != null)
			})
		})

	const lines = useMemo(() => {
		const result: JSX.Element[] = []
		for (const t of visibleTargets) {
			if (showLat) {
				result.push(
					<Line
						key={`${t.id}-lat`}
						yAxisId="latency"
						dataKey={(r: SystemStatsRecord) => getDetailLat(r.stats?.vp?.[t.id])}
						name={`${t.label}-lat`}
						type="monotoneX"
						dot={false}
						strokeWidth={1.5}
						stroke={t.color}
						isAnimationActive={false}
						connectNulls={false}
						activeDot={false}
					/>
				)
			}
			if (showLoss) {
				result.push(
					<Line
						key={`${t.id}-loss`}
						yAxisId="loss"
						dataKey={(r: SystemStatsRecord) => getDetailLoss(r.stats?.vp?.[t.id])}
						name={`${t.label}-loss`}
						type="monotoneX"
						dot={false}
						strokeWidth={1.5}
						stroke={t.color}
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

	const handleTargetClick = (id: string) => {
		setTargetFilter((prev) => (prev === id ? "all" : id))
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
							allTargets={allTargets}
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
	allTargets,
	targetFilter,
	metricFilter,
	onTargetClick,
	onMetricClick,
}: {
	allTargets: DynTarget[]
	targetFilter: TargetFilter
	metricFilter: MetricFilter
	onTargetClick: (id: string) => void
	onMetricClick: (metric: "latency" | "loss") => void
}) {
	return (
		<div className="flex items-center justify-center gap-1.5 text-xs pt-1 flex-wrap">
			{allTargets.map((t) => {
				const selected = targetFilter === t.id
				return (
					<button
						key={t.id}
						type="button"
						aria-pressed={selected}
						onClick={() => onTargetClick(t.id)}
						className={cn(
							"flex items-center gap-1 px-1.5 py-0.5 rounded-sm border border-transparent transition-colors cursor-pointer",
							"hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
							selected && "border-border bg-muted font-medium",
							!selected && targetFilter !== "all" && "opacity-40"
						)}
					>
						<span className="inline-block w-2.5 h-2.5 rounded-sm" style={{ backgroundColor: t.color }} />
						{t.label}
					</button>
				)
			})}
			{allTargets.length > 0 && <span className="text-muted-foreground mx-0.5">|</span>}
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
	visibleTargets: DynTarget[]
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
				{visibleTargets.map((t) => {
					const p = record.stats?.vp?.[t.id]
					if (!p) return null
					if (p.local) {
						return (
							<div key={t.id} className="flex items-center gap-2">
								<span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: t.color }} />
								<span className="w-12 truncate">{t.label}</span>
								<span className="text-right w-28 text-muted-foreground">{t`Local`}</span>
							</div>
						)
					}
					const lat = getDetailLat(p)
					const loss = getDetailLoss(p)
					const latStr = lat != null ? `${decimalString(lat, 1)} ms` : "--"
					const lossStr = loss != null ? `${decimalString(loss, 1)}%` : "--"
					return (
						<div key={t.id} className="flex items-center gap-2">
							<span className="inline-block w-2 h-2 rounded-sm" style={{ backgroundColor: t.color }} />
							<span className="w-12 truncate">{t.label}</span>
							<span className="text-right w-16">{latStr}</span>
							<span className="text-right w-12">{lossStr}</span>
						</div>
					)
				})}
			</div>
		</div>
	)
}
