// The order-table island: TanStack Table sorts, searches and pages every
// order in the browser. Its state lives in the table, so a bind push of new
// orders keeps each viewer's own sort, search and page.

import { html } from "lit";
import {
	TableController,
	columnFilteringFeature,
	createColumnHelper,
	createFilteredRowModel,
	createPaginatedRowModel,
	createSortedRowModel,
	filterFn_includesString,
	globalFilteringFeature,
	rowPaginationFeature,
	rowSortingFeature,
	sortFn_alphanumeric,
	sortFn_basic,
	tableFeatures,
} from "@tanstack/lit-table";
import { OrderTableBase } from "templatic/islands/order-table.base";
import type { Order } from "templatic/islands/order-table.types";

const features = tableFeatures({
	rowSortingFeature,
	sortedRowModel: createSortedRowModel(),
	sortFns: { alphanumeric: sortFn_alphanumeric, basic: sortFn_basic },
	columnFilteringFeature,
	globalFilteringFeature,
	filteredRowModel: createFilteredRowModel(),
	filterFns: { includesString: filterFn_includesString },
	rowPaginationFeature,
	paginatedRowModel: createPaginatedRowModel(),
});

const money = (cents: number): string => `$${(cents / 100).toFixed(2)}`;

const col = createColumnHelper<typeof features, Order>();
const columns = col.columns([
	col.accessor("id", { header: "#" }),
	col.accessor("customer", { header: "Customer", sortFn: "alphanumeric" }),
	col.accessor("status", { header: "Status", sortFn: "alphanumeric" }),
	col.accessor("amount", { header: "Amount", cell: (c) => money(c.getValue()) }),
	col.accessor("placed", { header: "Placed", sortFn: "alphanumeric" }),
]);

const searchable = new Set(["customer", "status"]);
const sortMark = { asc: " ▲", desc: " ▼" } as const;
const noOrders: Order[] = [];

customElements.define(
	"order-table",
	class extends OrderTableBase {
		tables = new TableController<typeof features, Order>(this);

		protected render(): unknown {
			const table = this.tables.table({
				features,
				columns,
				// The props object is new on every push; its orders array is what changed.
				data: this.props?.orders ?? noOrders,
				getRowId: (o) => String(o.id),
				globalFilterFn: "includesString",
				getColumnCanGlobalFilter: (c) => searchable.has(c.id),
				// A push must not throw a viewer back to page 1.
				autoResetPageIndex: false,
				initialState: { pagination: { pageIndex: 0, pageSize: 15 } },
			});
			const { pageIndex } = table.state.pagination;
			return html`
				<p class="tools">
					<input type="search" placeholder="Search customer or status" aria-label="Search"
						.value=${table.state.globalFilter ?? ""}
						@input=${(e: Event) => table.setGlobalFilter((e.target as HTMLInputElement).value)} />
					<span>${table.getFilteredRowModel().rows.length} of ${this.props?.orders.length ?? 0} orders</span>
				</p>
				<table class="grid">
					<thead>
						${table.getHeaderGroups().map(
							(g) => html`<tr>
								${g.headers.map((h) => {
									const sorted = h.column.getIsSorted();
									return html`<th aria-sort=${sorted ? (sorted === "asc" ? "ascending" : "descending") : "none"}>
										<button class="sort" @click=${h.column.getToggleSortingHandler()}>
											${table.FlexRender({ header: h })}${sorted ? sortMark[sorted] : ""}
										</button>
									</th>`;
								})}
							</tr>`,
						)}
					</thead>
					<tbody>
						${table.getRowModel().rows.map(
							(row) => html`<tr>
								${row.getAllCells().map(
									(c) => html`<td class=${c.column.id === "amount" ? "num" : ""}>${table.FlexRender({ cell: c })}</td>`,
								)}
							</tr>`,
						)}
					</tbody>
				</table>
				<p class="pager">
					<button class="secondary" ?disabled=${!table.getCanPreviousPage()} @click=${() => table.previousPage()}>Previous</button>
					Page ${pageIndex + 1} of ${Math.max(1, table.getPageCount())}
					<button class="secondary" ?disabled=${!table.getCanNextPage()} @click=${() => table.nextPage()}>Next</button>
				</p>
			`;
		}
	},
);
