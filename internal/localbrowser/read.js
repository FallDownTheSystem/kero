// Reads the site's localStorage and IndexedDB, in a world of its own in a
// tab of the site in the user's browser: a function of the most bytes its
// databases may take, which returns them as JSON. Values go as tagged
// arrays (enc), which keep what JSON would lose: dates, binary data, maps,
// sets, blobs, undefined. What a page can't copy, as a CryptoKey, is left
// out, and counted.
(async (limit) => {
	const out = { origin: location.origin, localStorage: [], indexedDB: [], notes: [] };
	try {
		for (let i = 0; i < localStorage.length; i++) {
			const name = localStorage.key(i);
			out.localStorage.push({ name, value: localStorage.getItem(name) });
		}
	} catch (e) {
		out.notes.push("localStorage: " + e.message);
	}
	const b64 = (u8) => {
		let s = "";
		for (let i = 0; i < u8.length; i += 0x8000) s += String.fromCharCode.apply(null, u8.subarray(i, i + 0x8000));
		return btoa(s);
	};
	const bytes = (v) => new Uint8Array(v.buffer, v.byteOffset, v.byteLength);
	const enc = async (v, seen) => {
		switch (typeof v) {
			case "undefined":
				return ["u"];
			case "boolean":
				return ["b", v];
			case "string":
				return ["s", v];
			case "bigint":
				return ["i", String(v)];
			case "number":
				return Number.isFinite(v) && !Object.is(v, -0) ? ["n", v] : ["N", Object.is(v, -0) ? "-0" : String(v)];
		}
		if (v === null) return ["z"];
		if (typeof v !== "object") throw new Error("a " + typeof v);
		if (seen.has(v)) throw new Error("a value that holds itself");
		seen.add(v);
		try {
			const tag = Object.prototype.toString.call(v).slice(8, -1);
			if (Array.isArray(v)) {
				const a = [];
				for (const x of v) a.push(await enc(x, seen));
				return ["a", a];
			}
			if (ArrayBuffer.isView(v)) return ["T", tag, b64(bytes(v))];
			switch (tag) {
				case "Date":
					return ["d", v.getTime()];
				case "RegExp":
					return ["r", v.source, v.flags];
				case "ArrayBuffer":
					return ["B", b64(new Uint8Array(v))];
				case "Blob":
					return ["L", v.type, b64(new Uint8Array(await v.arrayBuffer()))];
				case "File":
					return ["F", v.name, v.type, v.lastModified, b64(new Uint8Array(await v.arrayBuffer()))];
				case "Map": {
					const a = [];
					for (const [k, x] of v) a.push([await enc(k, seen), await enc(x, seen)]);
					return ["m", a];
				}
				case "Set": {
					const a = [];
					for (const x of v) a.push(await enc(x, seen));
					return ["S", a];
				}
				case "Object": {
					const o = [];
					for (const k of Object.keys(v)) o.push([k, await enc(v[k], seen)]);
					return ["o", o];
				}
			}
			throw new Error("a " + tag);
		} finally {
			seen.delete(v);
		}
	};
	const done = (r) =>
		new Promise((ok, no) => {
			r.onsuccess = () => ok(r.result);
			r.onerror = () => no(r.error);
		});
	let dbs = [];
	try {
		dbs = await indexedDB.databases();
	} catch (e) {
		out.notes.push("IndexedDB: " + e.message);
	}
	let total = 0;
	for (const { name, version } of dbs) {
		try {
			const open = indexedDB.open(name);
			// Gone since it was listed: opened, it would be made.
			open.onupgradeneeded = () => open.transaction.abort();
			const db = await done(open);
			const stores = [];
			const names = [...db.objectStoreNames];
			if (names.length) {
				const tx = db.transaction(names, "readonly");
				for (const sn of names) {
					const s = tx.objectStore(sn);
					const st = {
						name: sn,
						keyPath: s.keyPath,
						autoIncrement: s.autoIncrement,
						indexes: [...s.indexNames].map((n) => {
							const ix = s.index(n);
							return { name: n, keyPath: ix.keyPath, unique: ix.unique, multiEntry: ix.multiEntry };
						}),
						records: [],
					};
					stores.push(st);
					s.openCursor().onsuccess = (e) => {
						const c = e.target.result;
						if (c) {
							st.records.push([c.primaryKey, c.value]);
							c.continue();
						}
					};
				}
				await new Promise((ok, no) => {
					tx.oncomplete = ok;
					tx.onabort = () => no(tx.error);
				});
			}
			db.close();
			// Encoded once the transaction ended: reading a blob would end it.
			let skipped = 0;
			for (const st of stores) {
				const recs = [];
				for (const [k, v] of st.records) {
					try {
						recs.push([await enc(k, new Set()), await enc(v, new Set())]);
					} catch (e) {
						skipped++;
					}
				}
				st.records = recs;
			}
			const entry = { name, version: version || db.version, stores };
			const size = JSON.stringify(entry).length;
			if (total + size > limit) {
				out.notes.push(`database ${name}: left out, as it takes ${Math.ceil(size / 1048576)} MB`);
				continue;
			}
			total += size;
			out.indexedDB.push(entry);
			if (skipped) out.notes.push(`database ${name}: left out ${skipped} records of values a page can't copy, such as keys of the browser's own`);
		} catch (e) {
			out.notes.push(`database ${name}: ${e && e.message}`);
		}
	}
	return JSON.stringify(out);
})
