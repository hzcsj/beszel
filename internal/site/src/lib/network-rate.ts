export const NETWORK_RATE_WARNING_BYTES_PER_SECOND = 1000 * 1024
export const NETWORK_RATE_CRITICAL_BYTES_PER_SECOND = 1000 * 1024 ** 2

export type NetworkRateColorClass = "" | "text-yellow-500" | "text-red-500"

/** Return the list-page color for a raw network rate in bytes per second. */
export function getNetworkRateColorClass(bytesPerSecond: number | undefined): NetworkRateColorClass {
	if (bytesPerSecond === undefined || !Number.isFinite(bytesPerSecond) || bytesPerSecond < 0) return ""
	if (bytesPerSecond >= NETWORK_RATE_CRITICAL_BYTES_PER_SECOND) return "text-red-500"
	if (bytesPerSecond >= NETWORK_RATE_WARNING_BYTES_PER_SECOND) return "text-yellow-500"
	return ""
}

/** Resolve download and upload colors independently from their raw byte rates. */
export function getDirectionalNetworkRateColorClasses(download: number, upload: number) {
	return {
		download: getNetworkRateColorClass(download),
		upload: getNetworkRateColorClass(upload),
	}
}
