import { Trans, useLingui } from "@lingui/react/macro"
import { LanguagesIcon } from "lucide-react"
import { buttonVariants } from "@/components/ui/button"
import { dynamicActivate } from "@/lib/i18n"
import { cn } from "@/lib/utils"
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip"

export function LangToggle() {
	const { i18n } = useLingui()

	const nextLocale = i18n.locale === "zh-CN" ? "en" : "zh-CN"
	const nextLabel = nextLocale === "zh-CN" ? "中文" : "EN"

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					className={cn(buttonVariants({ variant: "ghost", size: "icon" }))}
					onClick={async () => {
						await dynamicActivate(nextLocale)
						window.location.reload()
					}}
				>
					<LanguagesIcon className="absolute h-[1.2rem] w-[1.2rem] light:opacity-85" />
					<span className="sr-only">
						<Trans>Language</Trans>
					</span>
				</button>
			</TooltipTrigger>
			<TooltipContent>{nextLabel}</TooltipContent>
		</Tooltip>
	)
}
