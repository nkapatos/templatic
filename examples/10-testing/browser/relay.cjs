// A TCP relay in front of the app: cut() drops every open connection and
// refuses new ones until restore(). An open EventSource only notices a
// dropped connection, which the browser's offline mode does not cause.
const net = require("node:net");

/** relay listens on a free local port and forwards to target ("host:port"). */
async function relay(target) {
	const [host, port] = target.split(":");
	const open = new Set();
	let cutOff = false;
	const server = net.createServer((client) => {
		if (cutOff) return client.destroy();
		const upstream = net.connect(Number(port), host);
		const pair = [client, upstream];
		open.add(pair);
		const drop = () => {
			open.delete(pair);
			client.destroy();
			upstream.destroy();
		};
		client.on("error", drop).on("close", drop);
		upstream.on("error", drop).on("close", drop);
		client.pipe(upstream);
		upstream.pipe(client);
	});
	await new Promise((ok) => server.listen(0, "127.0.0.1", ok));
	return {
		addr: `127.0.0.1:${server.address().port}`,
		cut() {
			cutOff = true;
			for (const [c, u] of open) {
				c.destroy();
				u.destroy();
			}
			open.clear();
		},
		restore() {
			cutOff = false;
		},
		close() {
			this.cut();
			return new Promise((ok) => server.close(ok));
		},
	};
}

module.exports = { relay };
