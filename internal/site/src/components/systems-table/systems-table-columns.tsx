/** biome-ignore-all lint/correctness/useHookAtTopLevel: Hooks live inside memoized column definitions */
import { t } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import type { CellContext, ColumnDef, HeaderContext } from "@tanstack/react-table"
import type { ClassValue } from "clsx"
import {
	ArrowDownUpIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CopyIcon,
	CpuIcon,
	HardDriveIcon,
	MemoryStickIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	ServerIcon,
	TerminalSquareIcon,
	Trash2Icon,
	WifiIcon,
} from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { isReadOnlyUser, pb } from "@/lib/api"
import { BatteryState, ConnectionType, connectionTypeLabels, MeterState, SystemStatus } from "@/lib/enums"
import { $userSettings } from "@/lib/stores"
import {
	cn,
	copyToClipboard,
	decimalString,
	formatBytes,
	formatCompactWithUnit,
	formatDirectionalTraffic,
	formatProbeTooltipValue,
	formatTemperature,
	getHostDisplayValue,
	parseSemVer,
	secondsToUptimeString,
} from "@/lib/utils"
import { getCycleTrafficColorClass, calculateCycleProgressPct } from "@/lib/traffic-billing"
import { formatLoad } from "@/lib/format-load"
import { batteryStateTranslations } from "@/lib/i18n"
import type { SystemRecord } from "@/types"
import { compareSystemsByOrder } from "@/lib/system-order"
import {
	resolveCompactProbeTargets,
	resolveProbeTargets,
	getProbeLossLevel,
	getListLatency,
	getListLoss,
} from "@/lib/probe-utils"
import { SystemDialog } from "../add-system"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button, buttonVariants } from "../ui/button"
import { Dialog } from "../ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import {
	BatteryMediumIcon,
	EthernetIcon,
	GpuIcon,
	HourglassIcon,
	ThermometerIcon,
	WebSocketIcon,
	BatteryHighIcon,
	BatteryLowIcon,
	PlugChargingIcon,
	BatteryFullIcon,
} from "../ui/icons"

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

function getMeterStateByThresholds(value: number, warn = 65, crit = 90): MeterState {
	return value >= crit ? MeterState.Crit : value >= warn ? MeterState.Warn : MeterState.Good
}

function billingModeLabel(mode: string): string {
	switch (mode) {
		case "max_rx_tx":
			return t`Max of download/upload`
		case "sum_rx_tx":
			return t`Download + upload`
		case "tx_only":
			return t`Upload only`
		case "rx_only":
			return t`Download only`
		default:
			return mode
	}
}

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	const columns = [
		{
			size: 0,
			minSize: 0,
			accessorKey: "name",
			id: "system",
			name: () => t`System`,
			sortingFn: (a, b) => compareSystemsByOrder(a.original, b.original),
			filterFn: (() => {
				let filterInput = ""
				let filterInputLower = ""
				const nameCache = new Map<string, string>()
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`.toLowerCase(),
					[SystemStatus.Down]: t`Down`.toLowerCase(),
					[SystemStatus.Paused]: t`Paused`.toLowerCase(),
				} as const

				// match filter value against name or translated status
				return (row, _, newFilterInput) => {
					const sys = row.original
					if (sys.host?.includes(newFilterInput) || sys.info.v?.includes(newFilterInput)) {
						return true
					}
					if (newFilterInput !== filterInput) {
						filterInput = newFilterInput
						filterInputLower = newFilterInput.toLowerCase()
					}
					let nameLower = nameCache.get(sys.name)
					if (nameLower === undefined) {
						nameLower = sys.name.toLowerCase()
						nameCache.set(sys.name, nameLower)
					}
					if (nameLower.includes(filterInputLower)) {
						return true
					}
					const statusLower = statusTranslations[sys.status as keyof typeof statusTranslations]
					return statusLower?.includes(filterInputLower) || false
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			Icon: ServerIcon,
			cell: (info) => {
				const system = info.row.original
				const { name, id } = system
				const linkUrl = getPagePath($router, "system", { id })
				const hostDisplay = getHostDisplayValue(system)
				const readonly = isReadOnlyUser()
				const nameLink = (
					<Link
						href={linkUrl}
						className="truncate z-10 relative"
						onMouseEnter={(e) => {
							const a = e.currentTarget
							if (a.scrollWidth > a.clientWidth) {
								a.title = name
							} else {
								a.removeAttribute("title")
							}
						}}
					>
						{name}
					</Link>
				)

				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot system={system} />
							{readonly ? (
								nameLink
							) : (
								<Tooltip>
									<TooltipTrigger asChild>{nameLink}</TooltipTrigger>
									<TooltipContent side="bottom" className="font-mono">
										{hostDisplay}
									</TooltipContent>
								</Tooltip>
							)}
						</span>
						<Link href={linkUrl} tabIndex={-1} className="inset-0 absolute size-full" aria-label={name} />
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.cpu || undefined,
			id: "cpu",
			name: () => t`CPU`,
			size: 120,
			cell: viewMode === "table" ? TableCellEmbeddedMeter : TableCellWithMeter,
			Icon: CpuIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.mp || undefined,
			id: "memory",
			name: () => t`Memory`,
			size: 120,
			cell: viewMode === "table" ? TableCellEmbeddedMeter : TableCellWithMeter,
			Icon: MemoryStickIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.dp || undefined,
			id: "disk",
			name: () => t`Disk`,
			size: 120,
			cell: (info: CellContext<SystemRecord, unknown>) =>
				info.row.original.info.efs
					? DiskCellWithMultiple(info, viewMode)
					: viewMode === "table"
						? TableCellEmbeddedMeter(info)
						: TableCellWithMeter(info),
			Icon: HardDriveIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.g || undefined,
			id: "gpu",
			name: () => "GPU",
			cell: TableCellWithMeter,
			Icon: GpuIcon,
			header: sortableHeader,
		},
		{
			id: "loadAverage",
			accessorFn: ({ info }) => info.la?.[0],
			name: () => t({ message: "Load", comment: "Short label for load average in All Systems list" }),
			size: 0,
			Icon: HourglassIcon,
			header: sortableHeader,
			cell(info: CellContext<SystemRecord, unknown>) {
				const { info: sysInfo, status } = info.row.original
				const { major, minor } = parseSemVer(sysInfo.v)
				const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
				const load1 = sysInfo.la?.[0]

				if ((load1 === undefined || load1 === 0) && (status === SystemStatus.Paused || (major < 1 && minor < 13))) {
					return null
				}
				if (load1 === undefined) return null

				const normalizedLoad = load1 / (sysInfo.t ?? 1)
				const threshold = getMeterStateByThresholds(normalizedLoad * 100, colorWarn, colorCrit)

				return (
					<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
						<span
							className={cn("inline-block size-2 rounded-full me-0.5", {
								[STATUS_COLORS[SystemStatus.Up]]: threshold === MeterState.Good,
								[STATUS_COLORS[SystemStatus.Pending]]: threshold === MeterState.Warn,
								[STATUS_COLORS[SystemStatus.Down]]: threshold === MeterState.Crit,
								[STATUS_COLORS[SystemStatus.Paused]]: status !== SystemStatus.Up,
							})}
						/>
						<span>{formatLoad(load1)}</span>
					</div>
				)
			},
		},
		{
			accessorFn: ({ info, status }) => {
				if (status !== SystemStatus.Up) return undefined
				if (info.nb) return info.nb[0] + info.nb[1]
				return info.bb
			},
			id: "net",
			name: () => t`Net`,
			size: 0,
			Icon: EthernetIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const sysInfo = info.row.original.info
				const status = info.row.original.status
				if (status !== SystemStatus.Up) return null

				const userSettings = useStore($userSettings, { keys: ["unitNet"] })

				if (sysInfo.nb) {
					const dl = formatBytes(sysInfo.nb[1], true, userSettings.unitNet, false)
					const ul = formatBytes(sysInfo.nb[0], true, userSettings.unitNet, false)
					return (
						<span className="tabular-nums whitespace-nowrap">
							{formatDirectionalTraffic(
								formatCompactWithUnit(dl.value, dl.unit),
								formatCompactWithUnit(ul.value, ul.unit)
							)}
						</span>
					)
				}

				const bb = sysInfo.bb
				if (bb === undefined) return null
				const { value, unit } = formatBytes(bb, true, userSettings.unitNet, false)
				return (
					<span className="tabular-nums whitespace-nowrap text-muted-foreground">
						{formatCompactWithUnit(value, unit)}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => (info.vt ? (info.vt.bill ?? 0) : undefined),
			id: "cycleTraffic",
			name: () => t`Cycle Traffic`,
			size: 0,
			Icon: ArrowDownUpIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const vt = info.row.original.info.vt
				if (!vt) {
					return null
				}
				const rx = formatBytes(vt.crx ?? 0)
				const tx = formatBytes(vt.ctx ?? 0)

				const now = new Date()
				const warnClass = getCycleTrafficColorClass({
					billableBytes: vt.bill ?? 0,
					quotaBytes: vt.quota ?? 0,
					cycleStart: vt.cs ?? "",
					resetDay: vt.rd ?? 1,
					now,
				})

				const tooltipLines: string[] = []
				if (vt.quota) {
					const quotaPct = ((vt.bill ?? 0) / vt.quota) * 100
					const q = formatBytes(vt.quota)
					const quotaStr = `${decimalString(q.value, 2)} ${q.unit}`
					const pctStr = decimalString(quotaPct, 1)
					tooltipLines.push(t`Quota: ${quotaStr} (${pctStr}%)`)
					const cycleProgress = calculateCycleProgressPct({
						cycleStart: vt.cs ?? "",
						resetDay: vt.rd ?? 1,
						now,
					})
					if (cycleProgress !== null) {
						const progressStr = decimalString(cycleProgress, 1)
						const usageStr = decimalString(quotaPct, 1)
						tooltipLines.push(t`Cycle progress: ${progressStr}%`)
						tooltipLines.push(t`Usage: ${usageStr}%`)
					}
				}
				if (vt.proj) {
					const p = formatBytes(vt.proj)
					const projStr = `${decimalString(p.value, 2)} ${p.unit}`
					tooltipLines.push(t`Projected: ${projStr}`)
				}
				if (vt.dl !== undefined) tooltipLines.push(t`Days left: ${vt.dl}`)
				if (vt.mode) {
					const modeLabel = billingModeLabel(vt.mode)
					tooltipLines.push(t`Billing mode: ${modeLabel}`)
				}

				const content = (
					<span className={cn("tabular-nums whitespace-nowrap", warnClass)}>
						{formatDirectionalTraffic(
							formatCompactWithUnit(rx.value, rx.unit),
							formatCompactWithUnit(tx.value, tx.unit)
						)}
					</span>
				)

				if (tooltipLines.length === 0) {
					return content
				}

				return (
					<Tooltip>
						<TooltipTrigger asChild>{content}</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-xs">
							<div className="grid gap-0.5">
								{tooltipLines.map((line, i) => (
									<span key={i}>{line}</span>
								))}
							</div>
						</TooltipContent>
					</Tooltip>
				)
			},
		},
		{
			accessorFn: ({ info }) => (info.vt ? (info.vt.trx ?? 0) + (info.vt.ttx ?? 0) : undefined),
			id: "totalTraffic",
			name: () => t`Total Traffic`,
			size: 0,
			Icon: ArrowDownUpIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const vt = info.row.original.info.vt
				if (!vt) {
					return null
				}
				const rx = formatBytes(vt.trx ?? 0)
				const tx = formatBytes(vt.ttx ?? 0)

				const tooltipLines: string[] = []
				if (vt.cs) tooltipLines.push(t`Cycle start: ${vt.cs}`)
				if (vt.rd) tooltipLines.push(t`Reset day: ${vt.rd}`)
				if (vt.dl !== undefined) tooltipLines.push(t`Days left: ${vt.dl}`)
				const crx = formatBytes(vt.crx ?? 0)
				const ctx = formatBytes(vt.ctx ?? 0)
				const crxStr = `${decimalString(crx.value, 2)} ${crx.unit}`
				const ctxStr = `${decimalString(ctx.value, 2)} ${ctx.unit}`
				tooltipLines.push(t`Cycle traffic: ↓ ${crxStr} / ↑ ${ctxStr}`)
				if (vt.quota) {
					const quotaPct = ((vt.bill ?? 0) / vt.quota) * 100
					const q = formatBytes(vt.quota)
					const quotaStr = `${decimalString(q.value, 2)} ${q.unit}`
					const pctStr = decimalString(quotaPct, 1)
					tooltipLines.push(t`Quota: ${quotaStr} (${pctStr}%)`)
				}
				if (vt.proj) {
					const p = formatBytes(vt.proj)
					const projStr = `${decimalString(p.value, 2)} ${p.unit}`
					tooltipLines.push(t`Projected: ${projStr}`)
				}
				if (vt.mode) {
					const modeLabel = billingModeLabel(vt.mode)
					tooltipLines.push(t`Billing mode: ${modeLabel}`)
				}

				return (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="tabular-nums whitespace-nowrap">
								{formatDirectionalTraffic(
									formatCompactWithUnit(rx.value, rx.unit),
									formatCompactWithUnit(tx.value, tx.unit)
								)}
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-xs">
							<div className="grid gap-0.5">
								{tooltipLines.map((line, i) => (
									<span key={i}>{line}</span>
								))}
							</div>
						</TooltipContent>
					</Tooltip>
				)
			},
		},
		{
			accessorFn: ({ info }) => {
				const resolved = resolveCompactProbeTargets(info.vp)
				if (resolved.length === 0) return undefined
				let worstLoss = 0
				let worstLat = 0
				for (const t of resolved) {
					if (t.stats.local) continue
					if ((t.stats.loss ?? 0) > worstLoss) worstLoss = t.stats.loss ?? 0
					const lat = getListLatency(t.stats) ?? 0
					if (lat > worstLat) worstLat = lat
				}
				return worstLoss * 10000 + worstLat
			},
			id: "probeLatency",
			name: () => t`Probe Latency`,
			size: 0,
			Icon: WifiIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const vp = info.row.original.info.vp
				if (!vp) return null
				const resolved = resolveProbeTargets(vp)
				if (resolved.length === 0) return null

				// The compact list remains bounded to the three primary probes. The
				// tooltip below intentionally uses the full resolved set (up to four).
				const parts = resolveCompactProbeTargets(vp).map((rt) => {
					const p = rt.stats
					if (p.local) return { id: rt.id, latStr: t`Local`, level: "muted" as const }
					const lat = getListLatency(p)
					const latStr = lat != null ? `${Math.round(lat)}` : "--"
					const level = getProbeLossLevel(p.loss, false, lat == null && p.loss == null)
					return { id: rt.id, latStr, level }
				})

				return (
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								className="relative z-10 tabular-nums whitespace-nowrap text-sm font-normal focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring rounded-sm cursor-default"
							>
								{parts.map((p, i) => (
									<span key={p.id}>
										{i > 0 && <span className="text-muted-foreground"> | </span>}
										<span
											className={cn(
												p.level === "critical"
													? "text-red-500"
													: p.level === "warning"
														? "text-yellow-500"
														: p.level === "muted"
															? "text-muted-foreground"
															: ""
											)}
										>
											{p.latStr}
										</span>
									</span>
								))}
								<span className="text-muted-foreground"> ms</span>
							</button>
						</TooltipTrigger>
						<TooltipContent side="bottom" className="max-w-sm">
							<div className="grid gap-1 font-mono">
								{resolved.map((rt) => {
									const p = rt.stats
									if (p.local) {
										return (
											<div key={rt.id}>
												<span className="font-semibold">{rt.label}</span>
												<span className="text-muted-foreground">: {t`Local`}</span>
											</div>
										)
									}
									const lat = getListLatency(p)
									const latStr = lat != null ? `${Math.round(lat)}` : "--"
									const loss = getListLoss(p)
									const lossStr = loss != null ? decimalString(loss, 1) : "--"
									const level = getProbeLossLevel(p.loss, false, lat == null && p.loss == null)
									return (
										<div key={rt.id}>
											<span className="font-semibold">{rt.label}</span>
											<span>: </span>
											<span
												className={cn(
													level === "critical"
														? "text-red-500"
														: level === "warning"
															? "text-yellow-500"
															: level === "muted"
																? "text-muted-foreground"
																: ""
												)}
											>
												{formatProbeTooltipValue(latStr, lossStr)}
											</span>
										</div>
									)
								})}
							</div>
						</TooltipContent>
					</Tooltip>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.dt,
			id: "temp",
			name: () => t({ message: "Temp", comment: "Temperature label in systems table" }),
			size: 50,
			Icon: ThermometerIcon,
			header: sortableHeader,
			cell(info) {
				const val = info.getValue() as number
				const userSettings = useStore($userSettings, { keys: ["unitTemp"] })
				if (!val) {
					return null
				}
				const { value, unit } = formatTemperature(val, userSettings.unitTemp)
				return (
					<span className={cn("tabular-nums whitespace-nowrap", viewMode === "table" && "ps-0.5")}>
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.bat?.[0],
			id: "battery",
			name: () => t({ message: "Bat", comment: "Battery label in systems table header" }),
			size: 70,
			Icon: BatteryMediumIcon,
			header: sortableHeader,
			cell(info) {
				const [pct, state] = info.row.original.info.bat ?? []
				if (pct === undefined) {
					return null
				}

				let Icon = PlugChargingIcon
				let iconColor = "text-muted-foreground"

				if (state !== BatteryState.Charging) {
					if (pct < 25) {
						iconColor = pct < 11 ? "text-red-500" : "text-yellow-500"
						Icon = BatteryLowIcon
					} else if (pct < 75) {
						Icon = BatteryMediumIcon
					} else if (pct < 95) {
						Icon = BatteryHighIcon
					} else {
						Icon = BatteryFullIcon
					}
				}

				const stateLabel =
					state !== undefined ? (batteryStateTranslations[state as BatteryState]?.() ?? undefined) : undefined

				return (
					<Link
						tabIndex={-1}
						href={getPagePath($router, "system", { id: info.row.original.id })}
						className="flex items-center gap-1 tabular-nums tracking-tight relative z-10"
						title={stateLabel}
					>
						<Icon className={cn("size-3.5", iconColor)} />
						<span className="min-w-10">{pct}%</span>
					</Link>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.sv?.[0],
			id: "services",
			name: () => t`Services`,
			size: 50,
			Icon: TerminalSquareIcon,
			header: sortableHeader,
			sortingFn: (a, b) => {
				// sort priorities: 1) failed services, 2) total services
				const [totalCountA, numFailedA] = a.original.info.sv ?? [0, 0]
				const [totalCountB, numFailedB] = b.original.info.sv ?? [0, 0]
				if (numFailedA !== numFailedB) {
					return numFailedA - numFailedB
				}
				return totalCountA - totalCountB
			},
			cell(info) {
				const sys = info.row.original
				const [totalCount, numFailed] = sys.info.sv ?? [0, 0]
				if (sys.status !== SystemStatus.Up || totalCount === 0) {
					return null
				}
				return (
					<span className="tabular-nums whitespace-nowrap flex gap-1.5 items-center">
						<span
							className={cn("block size-2 rounded-full", {
								[STATUS_COLORS[SystemStatus.Down]]: numFailed > 0,
								[STATUS_COLORS[SystemStatus.Up]]: numFailed === 0,
							})}
						/>
						{totalCount}{" "}
						<span className="text-muted-foreground text-sm -ms-0.5">
							({t`Failed`.toLowerCase()}: {numFailed})
						</span>
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.u || undefined,
			id: "uptime",
			name: () => t`Uptime`,
			size: 40,
			Icon: ClockArrowUp,
			header: sortableHeader,
			cell(info) {
				const uptime = info.getValue() as number
				if (!uptime) {
					return null
				}
				return <span className="tabular-nums whitespace-nowrap">{secondsToUptimeString(uptime)}</span>
			},
		},
		{
			accessorFn: ({ info }) => info.v,
			id: "agent",
			name: () => t`Agent`,
			size: 50,
			Icon: WifiIcon,
			header: sortableHeader,
			cell(info) {
				const version = info.getValue() as string
				if (!version) {
					return null
				}
				const system = info.row.original
				const color = {
					"text-green-500": version === globalThis.BESZEL.HUB_VERSION,
					"text-yellow-500": version !== globalThis.BESZEL.HUB_VERSION,
					"text-red-500": system.status !== SystemStatus.Up,
				}
				return (
					<Link
						href={getPagePath($router, "system", { id: system.id })}
						className={cn(
							"flex gap-1.5 items-center md:pe-5 tabular-nums relative z-10",
							viewMode === "table" && "ps-0.5"
						)}
						tabIndex={-1}
						title={connectionTypeLabels[system.info.ct as ConnectionType]}
						role="none"
					>
						{system.info.ct === ConnectionType.WebSocket && (
							<WebSocketIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{system.info.ct === ConnectionType.SSH && (
							<ChevronRightSquareIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{!system.info.ct && <IndicatorDot system={system} className={cn(color, "bg-current mx-0.5")} />}
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</Link>
				)
			},
		},
		{
			id: "actions",
			// @ts-expect-error
			name: () => t({ message: "Actions", comment: "Table column" }),
			size: 50,
			cell: ({ row }) => (
				<div className="relative z-10 flex justify-center items-center gap-1">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]

	return isReadOnlyUser() ? columns.filter((column) => column.id !== "actions") : columns
}

function sortableHeader(context: HeaderContext<SystemRecord, unknown>) {
	const { column } = context
	// @ts-expect-error
	const { Icon, name }: { Icon: React.ElementType; name: () => string } = column.columnDef
	const isSorted = column.getIsSorted()
	return (
		<Button
			variant="ghost"
			className={cn(
				"h-9 px-3 flex w-full justify-center duration-50",
				isSorted && "bg-accent/70 light:bg-accent text-accent-foreground/90"
			)}
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="me-2 size-4" />}
			{name()}
		</Button>
	)
}

function TableCellWithMeter(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const val = Number(info.getValue()) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const meterClass = cn(
		"h-full",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full">
			<span className="min-w-8 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<span className="flex-1 min-w-8 grid bg-muted h-[1em] rounded-sm overflow-hidden">
				<span className={meterClass} style={{ width: `${val}%` }}></span>
			</span>
		</div>
	)
}

function TableCellEmbeddedMeter(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const val = Number(info.getValue()) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const fillClass = cn(
		"absolute inset-0",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	const label = `${decimalString(val, val >= 10 ? 1 : 2)}%`
	const clampedWidth = Math.min(Math.max(val, 0), 100)

	return (
		<div
			className="relative w-full bg-muted h-6 rounded-sm overflow-hidden tabular-nums tracking-tight"
			role="progressbar"
			aria-valuenow={Math.round(clampedWidth)}
			aria-valuemin={0}
			aria-valuemax={100}
			aria-label={label}
		>
			<span className={fillClass} style={{ width: `${clampedWidth}%` }} />
			<span className="absolute inset-0 flex items-center justify-center text-sm font-normal pointer-events-none">
				{label}
			</span>
			<span
				aria-hidden="true"
				className="absolute inset-0 flex items-center justify-center text-sm font-normal text-black pointer-events-none"
				style={{ clipPath: `inset(0 ${100 - clampedWidth}% 0 0)` }}
			>
				{label}
			</span>
		</div>
	)
}

function DiskCellWithMultiple(info: CellContext<SystemRecord, unknown>, viewMode: "table" | "grid") {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const { info: sysInfo, status, id } = info.row.original
	const extraFs = Object.entries(sysInfo.efs ?? {})
	const rootDiskPct = sysInfo.dp

	// sort extra disks by percentage descending
	extraFs.sort((a, b) => b[1] - a[1])

	function getIndicatorColor(pct: number) {
		const threshold = getMeterStateByThresholds(pct, colorWarn, colorCrit)
		return (
			(status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
		)
	}

	function getMeterClass(pct: number) {
		return cn("h-full", getIndicatorColor(pct))
	}

	// Extra disk indicators (max 3 dots - one per state if any disk exists in range)
	const stateColors = [STATUS_COLORS.up, STATUS_COLORS.pending, STATUS_COLORS.down]
	const extraDiskIndicators =
		status !== SystemStatus.Up
			? []
			: [...new Set(extraFs.map(([, pct]) => getMeterStateByThresholds(pct, colorWarn, colorCrit)))]
					.sort()
					.map((state) => stateColors[state])

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					href={getPagePath($router, "system", { id })}
					tabIndex={-1}
					className="flex flex-col gap-0.5 w-full relative z-10"
				>
					{viewMode === "table" ? (
						<div
							className="relative w-full bg-muted h-6 rounded-sm overflow-hidden tabular-nums tracking-tight"
							role="progressbar"
							aria-valuenow={Math.round(Math.min(Math.max(rootDiskPct, 0), 100))}
							aria-valuemin={0}
							aria-valuemax={100}
							aria-label={`${decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%`}
						>
							<span
								className={cn("absolute inset-0", getMeterClass(rootDiskPct))}
								style={{ width: `${Math.min(Math.max(rootDiskPct, 0), 100)}%` }}
							/>
							<span className="absolute inset-0 flex items-center justify-center text-sm font-normal pointer-events-none">
								{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%
							</span>
							<span
								aria-hidden="true"
								className="absolute inset-0 flex items-center justify-center text-sm font-normal text-black pointer-events-none"
								style={{ clipPath: `inset(0 ${100 - Math.min(Math.max(rootDiskPct, 0), 100)}% 0 0)` }}
							>
								{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%
							</span>
							{extraDiskIndicators.length > 0 && (
								<span className="absolute inset-y-0 end-1 flex items-center gap-0.5">
									{extraDiskIndicators.map((color) => (
										<span
											key={color}
											className={cn("size-1.5 rounded-full shrink-0 outline-[0.5px] outline-muted", color)}
										/>
									))}
								</span>
							)}
						</div>
					) : (
						<div className="flex gap-2 items-center tabular-nums tracking-tight">
							<span className="min-w-8 shrink-0">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
							<span className="flex-1 min-w-8 flex items-center gap-0.5 px-1 justify-end bg-muted h-[1em] rounded-sm overflow-hidden relative">
								<span
									className={cn("absolute inset-0", getMeterClass(rootDiskPct))}
									style={{ width: `${rootDiskPct}%` }}
								/>
								{extraDiskIndicators.map((color) => (
									<span
										key={color}
										className={cn("size-1.5 rounded-full shrink-0 outline-[0.5px] outline-muted", color)}
									/>
								))}
							</span>
						</div>
					)}
				</Link>
			</TooltipTrigger>
			<TooltipContent side="right" className="max-w-xs pb-2">
				<div className="grid gap-1">
					<div className="grid gap-0.5">
						<div className="text-muted-foreground uppercase tracking-wide tabular-nums">
							<Trans context="Root disk label">Root</Trans>
						</div>
						<div className="flex gap-2 items-center tabular-nums">
							<span className="min-w-7">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
							<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
								<span className={getMeterClass(rootDiskPct)} style={{ width: `${rootDiskPct}%` }}></span>
							</span>
						</div>
					</div>
					{extraFs.map(([name, pct]) => {
						return (
							<div key={name} className="grid gap-0.5">
								<div className="max-w-40 text-muted-foreground uppercase tracking-wide truncate">{name}</div>
								<div className="flex gap-2 items-center tabular-nums">
									<span className="min-w-7">{decimalString(pct, pct >= 10 ? 1 : 2)}%</span>
									<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
										<span className={getMeterClass(pct)} style={{ width: `${pct}%` }}></span>
									</span>
								</div>
							</div>
						)
					})}
				</div>
			</TooltipContent>
		</Tooltip>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= STATUS_COLORS[system.status as keyof typeof STATUS_COLORS] || ""
	return (
		<span
			className={cn("shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

export const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	const editOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size={"icon"}>
							<span className="sr-only">
								<Trans>Open menu</Trans>
							</span>
							<MoreHorizontalIcon className="w-5" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						{!isReadOnlyUser() && (
							<DropdownMenuItem
								onSelect={() => {
									editOpened.current = true
									setEditOpen(true)
								}}
							>
								<PenBoxIcon className="me-2.5 size-4" />
								<Trans>Edit</Trans>
							</DropdownMenuItem>
						)}
						<DropdownMenuItem
							className={cn(isReadOnlyUser() && "hidden")}
							onClick={() => {
								pb.collection("systems").update(id, {
									status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused,
								})
							}}
						>
							{status === SystemStatus.Paused ? (
								<>
									<PlayCircleIcon className="me-2.5 size-4" />
									<Trans>Resume</Trans>
								</>
							) : (
								<>
									<PauseCircleIcon className="me-2.5 size-4" />
									<Trans>Pause</Trans>
								</>
							)}
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(name)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy name</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(host)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy host</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				{/* edit dialog */}
				<Dialog open={editOpen} onOpenChange={setEditOpen}>
					{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
				</Dialog>
				{/* deletion dialog */}
				<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Are you sure you want to delete {name}?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									This action cannot be undone. This will permanently delete all current records for {name} from the
									database.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction
								className={cn(buttonVariants({ variant: "destructive" }))}
								onClick={() => pb.collection("systems").delete(id)}
							>
								<Trans>Continue</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</>
		)
	}, [id, status, host, name, system, t, deleteOpen, editOpen])
})
