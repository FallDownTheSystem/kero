// Writes a site's storage, as read.js read it, in a page of the site's
// origin: localStorage's items over those the site has, and each database
// in place of the site's of its name, made again as it was. Returns what
// it wrote, as JSON.
(async (site) => {
	if (location.origin !== site.origin) throw new Error("the page is at " + location.origin + ", not " + site.origin);
	const res = { items: 0, databases: 0, records: 0, notes: [] };
	for (const { name, value } of site.localStorage || []) {
		try {
			localStorage.setItem(name, value);
			res.items++;
		} catch (e) {
			res.notes.push("localStorage: " + e.message);
			break;
		}
	}
	const unb64 = (s) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
	const views = ["Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array",
		"Float16Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array"];
	const dec = (e) => {
		const [t, a, b, c, d] = e;
		switch (t) {
			case "u":
				return undefined;
			case "z":
				return null;
			case "b":
			case "s":
			case "n":
				return a;
			case "N":
				return a === "-0" ? -0 : Number(a);
			case "i":
				return BigInt(a);
			case "d":
				return new Date(a);
			case "r":
				return new RegExp(a, b);
			case "a":
				return a.map(dec);
			case "o": {
				const o = {};
				for (const [k, v] of a) Object.defineProperty(o, k, { value: dec(v), writable: true, enumerable: true, configurable: true });
				return o;
			}
			case "m":
				return new Map(a.map(([k, v]) => [dec(k), dec(v)]));
			case "S":
				return new Set(a.map(dec));
			case "B":
				return unb64(a).buffer;
			case "T": {
				const buf = unb64(b).buffer;
				if (a === "DataView") return new DataView(buf);
				if (views.includes(a) && globalThis[a]) return new globalThis[a](buf);
				break;
			}
			case "L":
				return new Blob([unb64(b)], { type: a });
			case "F":
				return new File([unb64(d)], a, { type: b, lastModified: c });
		}
		throw new Error("no value of kind " + t);
	};
	const done = (r) =>
		new Promise((ok, no) => {
			r.onsuccess = () => ok(r.result);
			r.onerror = () => no(r.error);
		});
	for (const db of site.indexedDB || []) {
		try {
			// A page of the site in a pane that has the database open holds
			// it: the delete waits for the page to let go.
			const del = indexedDB.deleteDatabase(db.name);
			await Promise.race([
				done(del),
				new Promise((_, no) => {
					del.onblocked = () => setTimeout(() => no(new Error("a pane of the site has it open: close the pane, and import again")), 5000);
				}),
			]);
			const open = indexedDB.open(db.name, db.version);
			open.onupgradeneeded = () => {
				for (const s of db.stores) {
					const os = open.result.createObjectStore(s.name, { keyPath: s.keyPath, autoIncrement: s.autoIncrement });
					for (const ix of s.indexes) os.createIndex(ix.name, ix.keyPath, { unique: ix.unique, multiEntry: ix.multiEntry });
				}
			};
			const idb = await done(open);
			const names = db.stores.map((s) => s.name);
			let n = 0;
			if (names.length) {
				const tx = idb.transaction(names, "readwrite");
				for (const s of db.stores) {
					const os = tx.objectStore(s.name);
					for (const [k, v] of s.records) {
						if (s.keyPath === null) os.put(dec(v), dec(k));
						else os.put(dec(v));
						n++;
					}
				}
				await new Promise((ok, no) => {
					tx.oncomplete = ok;
					tx.onabort = () => no(tx.error);
				});
			}
			idb.close();
			res.databases++;
			res.records += n;
		} catch (e) {
			res.notes.push(`database ${db.name}: ${e && e.message}`);
		}
	}
	return JSON.stringify(res);
})
