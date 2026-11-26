import { beforeEach, expect, test, vi } from "vitest";

import { applyDev, swapCSS } from "../ts/dev";

function link(href: string, integrity?: string): HTMLLinkElement {
	const l = document.createElement("link");
	l.setAttribute("rel", "stylesheet");
	l.setAttribute("href", href);
	if (integrity) l.setAttribute("integrity", integrity);
	return l;
}

beforeEach(() => {
	vi.spyOn(window.location, "reload").mockImplementation(() => {});
});

test("dev css frame swaps matching stylesheets in place", () => {
	const app = link("/_templatic/app-aaa.css", "sha384-old");
	const isl = link("/_templatic/islands/x-card-111.css?v=1");
	const other = link("/static/app.css");
	document.head.append(app, isl, other);
	const links = document.querySelectorAll('link[rel="stylesheet"]');
	const n = swapCSS(links, { "app-aaa.css": "app-bbb.css", "islands/x-card-111.css": "islands/x-card-222.css" });
	expect(n).toBe(2);
	expect(app.getAttribute("href")).toBe("/_templatic/app-bbb.css");
	expect(app.getAttribute("integrity")).toBeNull();
	expect(isl.getAttribute("href")).toBe("/_templatic/islands/x-card-222.css?v=1");
	expect(other.getAttribute("href")).toBe("/static/app.css");
	expect([...document.querySelectorAll("link")]).toEqual([app, isl, other]);
});

test("dev css frame does not match a suffix inside another name", () => {
	expect(swapCSS([link("/_templatic/myapp-aaa.css")], { "app-aaa.css": "app-bbb.css" })).toBe(0);
});

test("dev reload frame reloads the page", () => {
	applyDev({ what: "reload" });
	expect(window.location.reload).toHaveBeenCalledTimes(1);
});
