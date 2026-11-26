// The traffic-chart island: a Chart.js line chart inside a lit element.
// Chart.js is imported here only, so it ships in this island's bundle and
// loads on pages that render the island.

import { html, type PropertyValues } from "lit";
import { CategoryScale, Chart, LinearScale, LineController, LineElement, PointElement } from "chart.js";
import { TrafficChartBase } from "templatic/islands/traffic-chart.base";
import type { ChartProps } from "templatic/islands/traffic-chart.types";

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale);

// visible is the part of the series the window shows, labelled in seconds ago.
function visible(p: ChartProps): { labels: string[]; data: number[] } {
	const data = p.series.slice(-p.window);
	return { labels: data.map((_, i) => `${i - data.length + 1}s`), data };
}

customElements.define(
	"traffic-chart",
	class extends TrafficChartBase {
		chart: Chart<"line", number[], string> | undefined;

		protected render(): unknown {
			const p = this.props;
			const latest = p?.series.at(-1);
			return html`
				<p class="latest">${latest ?? "–"} <span>req/s now, last ${p?.window ?? 0} points</span></p>
				<div class="plot"><canvas></canvas></div>
			`;
		}

		// Each bind push assigns a new props object, so this runs per change.
		protected updated(changed: PropertyValues<this>): void {
			super.updated(changed);
			if (!this.props) return;
			const { labels, data } = visible(this.props);
			if (!this.chart) {
				this.chart = new Chart(this.querySelector("canvas")!, {
					type: "line",
					data: { labels, datasets: [{ data, borderColor: "#2f6f4f", borderWidth: 2, pointRadius: 0, tension: 0.3 }] },
					options: { animation: false, maintainAspectRatio: false, scales: { y: { beginAtZero: true } } },
				});
				return;
			}
			this.chart.data.labels = labels;
			this.chart.data.datasets[0]!.data = data;
			this.chart.update();
		}

		disconnectedCallback(): void {
			super.disconnectedCallback();
			this.chart?.destroy();
			this.chart = undefined;
		}
	},
);
