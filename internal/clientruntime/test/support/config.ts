/** TOKEN is a page token whose payload carries the sid "test-sid". */
export const TOKEN = "eyJzIjoidGVzdC1zaWQiLCJpIjoxfQ.c2ln";

/** configScript is the page's tc-config script; extra overrides or adds fields. */
export function configScript(extra: Record<string, unknown> = {}): string {
	const cfg = { stream: "/live/stream", commands: "/live/commands", assets: "/_templatic", token: TOKEN, ...extra };
	return `<script type="application/tc-config">${JSON.stringify(cfg)}</script>`;
}
