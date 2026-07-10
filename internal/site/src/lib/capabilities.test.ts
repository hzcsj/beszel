import { describe, test, expect, mock, beforeEach } from "bun:test"

const mockAuthStore = { record: null as { role?: string; id?: string } | null }

const pbCollectionCreate = mock(() => Promise.resolve({ settings: {} }))
const pbCollectionGetFirstListItem = mock(() => Promise.resolve({ settings: {} }))
const pbCollectionUpdate = mock(() => Promise.resolve({ settings: {} }))

const mockCollection = () => ({
	create: pbCollectionCreate,
	getFirstListItem: pbCollectionGetFirstListItem,
	update: pbCollectionUpdate,
})

mock.module("@lingui/core/macro", () => ({
	t: (strings: TemplateStringsArray) => strings.join(""),
}))

mock.module("@lingui/react/macro", () => ({
	Trans: ({ children }: { children: unknown }) => children,
	useLingui: () => ({
		t: (strings: TemplateStringsArray) => strings.join(""),
	}),
}))

mock.module("pocketbase", () => ({
	default: class {
		authStore = mockAuthStore
		collection = mockCollection
		send() {
			return Promise.resolve({})
		}
		realtime = { unsubscribe() {} }
	},
}))

mock.module("@/components/router", () => ({
	basePath: "",
	$router: { subscribe: () => () => {}, get: () => null },
	Link: () => null,
	navigate: () => {},
}))

mock.module("@/components/ui/use-toast", () => ({
	toast: () => {},
}))

mock.module("@/lib/stores", () => ({
	$alerts: { set: () => {}, get: () => ({}) },
	$allSystemsById: { set: () => {} },
	$allSystemsByName: { set: () => {} },
	$userSettings: { set: () => {} },
}))

mock.module("@/lib/utils", () => ({
	chartTimeData: {},
	cn: (...args: string[]) => args.filter(Boolean).join(" "),
	useBrowserStorage: () => [null, () => {}],
}))

mock.module("@nanostores/react", () => ({
	useStore: () => ({}),
}))

mock.module("@nanostores/router", () => ({
	getPagePath: () => "/settings/general",
	redirectPage: () => {},
}))

mock.module("@/components/ui/card", () => ({
	Card: ({ children }: { children: unknown }) => children,
	CardContent: ({ children }: { children: unknown }) => children,
	CardDescription: ({ children }: { children: unknown }) => children,
	CardHeader: ({ children }: { children: unknown }) => children,
	CardTitle: ({ children }: { children: unknown }) => children,
}))

mock.module("@/components/ui/separator", () => ({
	Separator: () => null,
}))

mock.module("@/components/ui/select", () => ({
	Select: () => null,
	SelectContent: () => null,
	SelectItem: () => null,
	SelectTrigger: () => null,
	SelectValue: () => null,
}))

mock.module("@/components/ui/button", () => ({
	Button: () => null,
	buttonVariants: () => "",
}))

const iconStub = () => null
mock.module("lucide-react", () => ({
	AlertOctagonIcon: iconStub,
	BellIcon: iconStub,
	FileSlidersIcon: iconStub,
	FingerprintIcon: iconStub,
	HeartPulseIcon: iconStub,
	SettingsIcon: iconStub,
	LoaderCircleIcon: iconStub,
	PlusIcon: iconStub,
	SaveIcon: iconStub,
	Trash2Icon: iconStub,
	SendIcon: iconStub,
	LanguagesIcon: iconStub,
	AlertCircleIcon: iconStub,
	ArrowUpDownIcon: iconStub,
	ChevronDownIcon: iconStub,
	CopyIcon: iconStub,
	EyeIcon: iconStub,
	EyeOffIcon: iconStub,
	KeyIcon: iconStub,
	RefreshCwIcon: iconStub,
	TrashIcon: iconStub,
	XIcon: iconStub,
}))

const {
	getUserCapabilities,
	isReadOnlyUser,
	isAdmin,
	runIfSensitiveDetailsAllowed,
	shouldInitializeAlertManager,
	shouldRedirectSettings,
	updateUserSettings,
} = await import("./api")
const { saveSettings } = await import("@/components/routes/settings/layout")

beforeEach(() => {
	pbCollectionCreate.mockClear()
	pbCollectionGetFirstListItem.mockClear()
	pbCollectionUpdate.mockClear()
})

describe("isReadOnlyUser", () => {
	test("returns true for readonly role", () => {
		mockAuthStore.record = { role: "readonly" }
		expect(isReadOnlyUser()).toBe(true)
	})

	test("returns false for admin role", () => {
		mockAuthStore.record = { role: "admin" }
		expect(isReadOnlyUser()).toBe(false)
	})

	test("returns false for user role", () => {
		mockAuthStore.record = { role: "user" }
		expect(isReadOnlyUser()).toBe(false)
	})

	test("returns false when no record", () => {
		mockAuthStore.record = null
		expect(isReadOnlyUser()).toBe(false)
	})

	test("returns false when record has no role", () => {
		mockAuthStore.record = {}
		expect(isReadOnlyUser()).toBe(false)
	})
})

describe("isAdmin", () => {
	test("returns true for admin role", () => {
		mockAuthStore.record = { role: "admin" }
		expect(isAdmin()).toBe(true)
	})

	test("returns false for readonly role", () => {
		mockAuthStore.record = { role: "readonly" }
		expect(isAdmin()).toBe(false)
	})

	test("returns false for user role", () => {
		mockAuthStore.record = { role: "user" }
		expect(isAdmin()).toBe(false)
	})

	test("returns false when no record", () => {
		mockAuthStore.record = null
		expect(isAdmin()).toBe(false)
	})
})

describe("getUserCapabilities", () => {
	test("readonly user cannot manage or view sensitive details", () => {
		mockAuthStore.record = { role: "readonly" }

		expect(getUserCapabilities()).toEqual({
			manageSystems: false,
			manageAlerts: false,
			manageSettings: false,
			viewSensitiveDetails: false,
		})
	})

	test.each(["user", "admin"])("%s preserves management capabilities", (role) => {
		mockAuthStore.record = { role }

		expect(getUserCapabilities()).toEqual({
			manageSystems: true,
			manageAlerts: true,
			manageSettings: true,
			viewSensitiveDetails: true,
		})
	})
})

describe("readonly navigation and startup decisions", () => {
	test("readonly settings route redirects", () => {
		mockAuthStore.record = { role: "readonly" }
		expect(shouldRedirectSettings("settings")).toBe(true)
		expect(shouldRedirectSettings("home")).toBe(false)
	})

	test("normal user settings route does not redirect", () => {
		mockAuthStore.record = { role: "user" }
		expect(shouldRedirectSettings("settings")).toBe(false)
	})

	test("alert manager is disabled only for readonly users", () => {
		mockAuthStore.record = { role: "readonly" }
		expect(shouldInitializeAlertManager()).toBe(false)
		mockAuthStore.record = { role: "user" }
		expect(shouldInitializeAlertManager()).toBe(true)
	})
})

describe("sensitive detail guard", () => {
	test("readonly user cannot invoke a container or systemd detail loader", () => {
		mockAuthStore.record = { role: "readonly" }
		const loadDetails = mock(() => {})

		expect(runIfSensitiveDetailsAllowed(loadDetails)).toBe(false)
		expect(loadDetails).not.toHaveBeenCalled()
	})

	test("normal user can invoke a sensitive detail loader", () => {
		mockAuthStore.record = { role: "user" }
		const loadDetails = mock(() => {})

		expect(runIfSensitiveDetailsAllowed(loadDetails)).toBe(true)
		expect(loadDetails).toHaveBeenCalledTimes(1)
	})
})

describe("updateUserSettings - readonly guard", () => {
	test("readonly user does not create settings when none exist", async () => {
		mockAuthStore.record = { role: "readonly" }
		pbCollectionGetFirstListItem.mockImplementation(() => Promise.reject(new Error("no record")))

		await updateUserSettings()

		expect(pbCollectionCreate).not.toHaveBeenCalled()
	})

	test("normal user creates settings when none exist", async () => {
		mockAuthStore.record = { role: "user", id: "user123" }
		pbCollectionGetFirstListItem.mockImplementation(() => Promise.reject(new Error("no record")))
		pbCollectionCreate.mockImplementation(() => Promise.resolve({ settings: { chartTime: "1h" } }))

		await updateUserSettings()

		expect(pbCollectionCreate).toHaveBeenCalledTimes(1)
	})
})

describe("saveSettings - production function", () => {
	test("readonly user: production saveSettings makes no PB calls", async () => {
		mockAuthStore.record = { role: "readonly" }

		await saveSettings({ chartTime: "24h" })

		expect(pbCollectionGetFirstListItem).not.toHaveBeenCalled()
		expect(pbCollectionUpdate).not.toHaveBeenCalled()
	})

	test("normal user: production saveSettings fetches and updates", async () => {
		mockAuthStore.record = { role: "user" }
		pbCollectionGetFirstListItem.mockImplementation(() =>
			Promise.resolve({ id: "rec1", settings: { chartTime: "1h" } })
		)
		pbCollectionUpdate.mockImplementation(() => Promise.resolve({ settings: { chartTime: "24h" } }))

		await saveSettings({ chartTime: "24h" })

		expect(pbCollectionGetFirstListItem).toHaveBeenCalledTimes(1)
		expect(pbCollectionUpdate).toHaveBeenCalledTimes(1)
	})
})
