import { defineConfig } from "vitest/config";

export default defineConfig({
	test: {
		environment: "happy-dom",
		environmentOptions: { happyDOM: { url: "http://page.local/workspace", settings: { handleDisabledFileLoadingAsSuccess: true } } },
		include: ["test/**/*.test.ts"],
		setupFiles: ["test/support/setup.ts"],
		restoreMocks: true,
		unstubGlobals: true,
	},
});
