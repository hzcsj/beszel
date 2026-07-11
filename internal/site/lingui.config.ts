import { defineConfig } from "@lingui/cli"

export default defineConfig({
	locales: ["en", "zh-CN"],
	sourceLocale: "en",
	fallbackLocales: {
		default: "en",
	},
	compileNamespace: "ts",
	formatOptions: {
		lineNumbers: false,
	},
	catalogs: [
		{
			path: "<rootDir>/src/locales/{locale}/{locale}",
			include: ["src"],
		},
	],
})
