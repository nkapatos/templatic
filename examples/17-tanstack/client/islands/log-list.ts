// The log-list island: every row of the log in a TanStack Virtual list. Only
// the rows in view are in the DOM; the bound cell replaces the whole log.

import { html, type PropertyValues } from "lit";
import { createRef, ref } from "lit/directives/ref.js";
import { VirtualizerController } from "@tanstack/lit-virtual";
import { LogListBase } from "templatic/islands/log-list.base";
import type { Row } from "templatic/islands/log-list.types";

const rowHeight = 28;

customElements.define(
	"log-list",
	class extends LogListBase {
		viewport = createRef<HTMLDivElement>();
		virtual = new VirtualizerController<HTMLDivElement, HTMLElement>(this, {
			getScrollElement: () => this.viewport.value ?? null,
			count: 0,
			estimateSize: () => rowHeight,
			getItemKey: (i) => this.rows()[i]?.id ?? i,
			overscan: 10,
		});

		rows(): Row[] {
			return this.props?.rows ?? [];
		}

		// Each bind push assigns a new props object, so the count follows every change.
		protected willUpdate(changed: PropertyValues<this>): void {
			super.willUpdate(changed);
			this.virtual.setOptions({ count: this.rows().length });
		}

		protected render(): unknown {
			const rows = this.rows();
			const v = this.virtual.getVirtualizer();
			return html`
				<p class="count">${rows.length.toLocaleString()} rows, newest first</p>
				<div class="viewport" ${ref(this.viewport)}>
					<ol class="log" style="height: ${v.getTotalSize()}px">
						${v.getVirtualItems().map((item) => {
							const r = rows[item.index]!;
							return html`<li class="row ${r.level}" style="transform: translateY(${item.start}px)">
								<span class="id">#${r.id}</span><span class="at">${r.at}</span><span class="level">${r.level}</span><span class="text">${r.text}</span>
							</li>`;
						})}
					</ol>
				</div>
			`;
		}
	},
);
