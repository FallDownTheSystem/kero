async (o) => {
  // Pick mode of a browser pane (browserpick.go): the user clicks
  // elements of the page, writes a note on each, and sends them. The
  // promise ends with what the agent needs to find each element in the
  // source, or null when the user stops picking. A new pick ends the old.
  if (window.__keroPick) window.__keroPick.stop();
  const ui = document.createElement("kero-picker");
  const root = ui.attachShadow({ mode: "open" });
  const picks = []; // { el, note }
  let hover = null, open = null, shots = o.screenshots !== false, whole = o.page !== false, done;
  const finished = new Promise((r) => (done = r));
  const accent = "#2f80ed";
  const css = `:host { all: initial; position: fixed; inset: 0; z-index: 2147483647; pointer-events: none;
  font: 12.5px/1.4 -apple-system, "Segoe UI", system-ui, sans-serif; color: #1d1d1f; }
.box { position: fixed; box-sizing: border-box; border: 2px solid ${accent}; background: ${accent}1a; border-radius: 3px; pointer-events: none; }
.box.picked { background: ${accent}0d; border-style: solid; }
.box.preview { border-style: dashed; background: ${accent}12; }
.tag { position: absolute; left: -2px; bottom: 100%; margin-bottom: 3px; padding: 1px 6px; border-radius: 4px; background: ${accent};
  color: #fff; font-size: 11px; white-space: nowrap; max-width: 420px; overflow: hidden; text-overflow: ellipsis; }
.tag.below { bottom: auto; top: 100%; margin: 3px 0 0; }
.num { position: absolute; right: -9px; top: -9px; min-width: 18px; height: 18px; border-radius: 9px; background: ${accent}; color: #fff;
  font-size: 11px; font-weight: 600; display: flex; align-items: center; justify-content: center; padding: 0 4px; box-sizing: border-box; }
.note, .tray { position: fixed; pointer-events: auto; background: #fff; border-radius: 10px;
  box-shadow: 0 0 0 1px #0000001f, 0 8px 28px #00000033; box-sizing: border-box; }
.note { width: 300px; padding: 10px; display: flex; flex-direction: column; gap: 8px; }
.note .what { font-size: 11.5px; color: #6e6e73; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
textarea { font: inherit; resize: vertical; min-height: 54px; border: 1px solid #d2d2d7; border-radius: 6px; padding: 6px 8px; outline: none; color: inherit; background: #fff; }
textarea:focus { border-color: ${accent}; box-shadow: 0 0 0 3px ${accent}33; }
.row { display: flex; gap: 6px; align-items: center; }
.grow { flex: 1; }
button { font: inherit; border: 0; border-radius: 6px; padding: 4px 10px; background: #f0f0f2; color: inherit; cursor: pointer; }
button:hover { background: #e4e4e8; }
button.primary { background: ${accent}; color: #fff; }
button.primary:hover { background: #1f6fd8; }
button:disabled { opacity: .45; cursor: default; }
.tray { left: 50%; bottom: 16px; transform: translateX(-50%); padding: 6px 6px 6px 12px; display: flex; gap: 10px; align-items: center; white-space: nowrap; }
.tray .hint { color: #6e6e73; }
label { display: flex; gap: 4px; align-items: center; color: #6e6e73; cursor: pointer; }
@media (prefers-color-scheme: dark) {
  :host { color: #f5f5f7; }
  .note, .tray { background: #2c2c2e; box-shadow: 0 0 0 1px #ffffff26, 0 8px 28px #00000080; }
  textarea { background: #1c1c1e; border-color: #48484a; }
  button { background: #3a3a3c; } button:hover { background: #48484a; }
  .note .what, .tray .hint, label { color: #a1a1a6; }
}
`;
  // A sheet of its own, which a page's Content Security Policy leaves be,
  // as it would not a <style>.
  try {
    const sheet = new CSSStyleSheet();
    sheet.replaceSync(css);
    root.adoptedStyleSheets = [sheet];
  } catch {
    const st = document.createElement("style");
    st.textContent = css;
    root.append(st);
  }
  root.innerHTML += `<div class="layer"><div class="boxes"></div></div>
<div class="tray">
  <span class="hint"></span>
  <span class="hint">Screenshots:</span>
  <label title="A picture of each element, boxed"><input type="checkbox" class="shots"> Elements</label>
  <label title="A picture of the page as you see it, the elements boxed and numbered"><input type="checkbox" class="whole"> Page</label>
  <button class="cancel">Cancel</button>
  <button class="primary send">Send</button>
</div>`;
  const $ = (s) => root.querySelector(s);
  const layer = $(".layer"), boxes = $(".boxes"), hint = $(".hint"), send = $(".send");
  $(".shots").checked = shots;
  $(".shots").onchange = (e) => (shots = e.target.checked);
  $(".whole").checked = whole;
  $(".whole").onchange = (e) => (whole = e.target.checked);
  const mine = (e) => e.composedPath().includes(ui);
  // What the page shows under the pointer: inside its shadow roots too.
  const under = (e) => {
    const t = e.composedPath()[0];
    return t instanceof Element && t !== document.documentElement ? t : null;
  };
  const describe = (el) => {
    let s = el.localName;
    if (el.id) s += "#" + el.id;
    const cls = (el.getAttribute("class") || "").trim().split(/\s+/).filter(Boolean);
    if (cls.length) s += "." + cls.slice(0, 3).join(".");
    const c = components(el)[0];
    const r = el.getBoundingClientRect();
    return (c ? c + " · " : "") + s + " · " + Math.round(r.width) + "×" + Math.round(r.height);
  };
  const place = (box, el) => {
    const r = el.getBoundingClientRect();
    Object.assign(box.style, { left: r.left + "px", top: r.top + "px", width: Math.max(r.width, 2) + "px", height: Math.max(r.height, 2) + "px" });
    const tag = box.querySelector(".tag");
    if (tag) tag.classList.toggle("below", r.top < 22);
  };
  const hoverBox = document.createElement("div");
  hoverBox.className = "box";
  hoverBox.innerHTML = `<div class="tag"></div>`;
  layer.append(hoverBox);
  // preview is what the open note's Parent button would pick, shown in
  // place of what the pointer is on while the pointer is on the button.
  let preview = null, shown = null;
  const target = () => preview || hover;
  const showHover = () => {
    const el = target();
    hoverBox.classList.toggle("preview", !!preview);
    hoverBox.style.display = el && (preview || !picks.some((p) => p.el === el)) ? "" : "none";
    if (el) {
      hoverBox.firstChild.textContent = (preview ? "Parent: " : "") + describe(el);
      place(hoverBox, el);
    }
  };
  // The note's card stays in the page while it is open: taken out and put
  // back, its field would lose the keyboard.
  const render = () => {
    boxes.textContent = "";
    picks.forEach((p, i) => {
      const b = document.createElement("div");
      b.className = "box picked";
      b.innerHTML = `<div class="num">${i + 1}</div>`;
      p.box = b;
      boxes.append(b);
    });
    const card = open && open.card;
    if (shown !== card) {
      if (shown) shown.remove();
      if (card) layer.append(card);
      shown = card;
    }
    showHover();
    const n = picks.length;
    hint.textContent = n ? n + (n === 1 ? " element picked" : " elements picked") + " · Ctrl+Enter sends"
      : "Click an element to pick it · Esc stops";
    send.disabled = !n;
    follow();
  };
  // The boxes follow their elements as the page scrolls and changes.
  let frame = 0;
  const follow = () => {
    for (const p of picks) if (p.box) place(p.box, p.el);
    if (target()) place(hoverBox, target());
    if (open) placeCard(open);
  };
  const tick = () => { follow(); frame = requestAnimationFrame(tick); };
  // A note stays by the element first clicked, as Parent widens the pick:
  // its button stays under the pointer, to click again.
  const placeCard = (p) => {
    const at = p.at && p.at.isConnected ? p.at : p.el;
    const r = at.getBoundingClientRect(), c = p.card, w = 300, h = c.offsetHeight || 140;
    let top = r.bottom + 8;
    if (top + h > innerHeight - 60) top = r.top - h - 8;
    top = Math.max(8, Math.min(top, innerHeight - h - 60));
    c.style.left = Math.max(8, Math.min(r.left, innerWidth - w - 8)) + "px";
    c.style.top = top + "px";
  };
  const edit = (p) => {
    if (open && open !== p) open.note = open.card.querySelector("textarea").value;
    open = p;
    if (p && !p.card) {
      const c = document.createElement("div");
      c.className = "note";
      c.innerHTML = `<div class="what"></div><textarea placeholder="What should change here? (optional)"></textarea>
<div class="row"><button class="up" title="Pick the element around it instead">Parent</button><button class="remove">Remove</button>
<span class="grow"></span><button class="primary ok">Done</button></div>`;
      const ta = c.querySelector("textarea");
      ta.value = p.note;
      ta.oninput = () => (p.note = ta.value);
      c.querySelector(".ok").onclick = () => { preview = null; edit(null); };
      c.querySelector(".remove").onclick = () => { picks.splice(picks.indexOf(p), 1); preview = null; edit(null); };
      const parentOf = (el) => {
        const up = el.parentElement;
        return up && up !== document.documentElement ? up : null;
      };
      const upButton = c.querySelector(".up");
      upButton.onmouseenter = () => { preview = parentOf(p.el); showHover(); };
      upButton.onmouseleave = () => { preview = null; showHover(); };
      upButton.onclick = () => {
        const up = parentOf(p.el);
        if (!up) return;
        p.el = up;
        c.querySelector(".what").textContent = (picks.indexOf(p) + 1) + ". " + describe(up);
        // The pointer stays on the button: it now shows the next parent.
        preview = parentOf(up);
        render();
        c.querySelector("textarea").focus({ preventScroll: true });
      };
      p.card = c;
    }
    render();
    if (p) {
      p.card.querySelector(".what").textContent = (picks.indexOf(p) + 1) + ". " + describe(p.el);
      const ta = p.card.querySelector("textarea");
      setTimeout(() => ta.focus({ preventScroll: true }), 0);
    }
  };
  const block = (e) => {
    if (mine(e)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
  };
  const onClick = (e) => {
    if (mine(e)) return;
    block(e);
    const el = under(e);
    if (!el) return;
    let p = picks.find((p) => p.el === el);
    if (!p) picks.push((p = { el, at: el, note: "" }));
    edit(p);
  };
  // Moving the pointer only shows what a click would pick: the note being
  // written keeps the keyboard. On the picker's own controls, it shows
  // nothing of the page, but what the Parent button would pick.
  const onMove = (e) => {
    const el = mine(e) ? null : under(e);
    if (el !== hover) { hover = el; showHover(); }
  };
  const onKey = (e) => {
    const enter = e.key === "Enter" && (e.ctrlKey || e.metaKey);
    if (enter && picks.length) { block(e); finish(true); return; }
    if (e.key === "Escape") {
      block(e);
      if (open) { preview = null; edit(null); } else finish(false);
      return;
    }
    // What the user types in a note is the note's, not the page's.
    if (mine(e)) e.stopImmediatePropagation();
  };
  const onKeyUp = (e) => { if (mine(e)) e.stopImmediatePropagation(); };
  const types = ["pointerdown", "mousedown", "pointerup", "mouseup", "dblclick", "contextmenu", "auxclick", "touchstart", "touchend"];
  const listen = (on) => {
    const f = on ? addEventListener : removeEventListener;
    for (const t of types) f(t, block, true);
    f("click", onClick, true);
    f("mousemove", onMove, true);
    f("keydown", onKey, true);
    f("keyup", onKeyUp, true);
  };
  const finish = (sending) => {
    if (open) open.note = open.card.querySelector("textarea").value;
    listen(false);
    cancelAnimationFrame(frame);
    ui.remove();
    delete window.__keroPick;
    // The picked elements stay known to the page, for Kero to scroll
    // one out of sight into it for its picture; Kero lets them go after.
    if (sending) window.__keroPicked = picks.map((p) => p.el);
    done(sending ? collect() : null);
  };
  $(".cancel").onclick = () => finish(false);
  send.onclick = () => finish(true);

  // ---- What each picked element is, strongest locator first ----

  const norm = (s) => (s || "").replace(/\s+/g, " ").trim();
  const cut = (s, n) => (s.length > n ? s.slice(0, n - 1) + "…" : s);
  const all = () => document.querySelectorAll("*");
  const count = (fn) => { let n = 0; for (const el of all()) if (fn(el)) n++; return n; };

  // The components a framework rendered the element in, innermost first.
  function components(el) {
    return frameworks(el).flatMap((f) => (f.chain || []).map((c) => c.name)).filter(Boolean);
  }
  const fileName = (f) => (f || "").split(/[\\/]/).pop().replace(/\.\w+$/, "");
  function vue(el) {
    for (let e = el; e; e = e.parentElement) {
      let c = e.__vueParentComponent;
      if (c) {
        const chain = [];
        for (; c; c = c.parent) {
          const t = c.type || {};
          const name = t.name || t.__name || fileName(t.__file);
          if (name || t.__file) chain.push({ name, file: t.__file || "" });
        }
        return chain.length ? { framework: "vue", chain } : null;
      }
      let vm = e.__vue__; // Vue 2
      if (vm) {
        const chain = [];
        for (; vm; vm = vm.$parent) {
          const op = vm.$options || {};
          if (op.name || op.__file) chain.push({ name: op.name || fileName(op.__file), file: op.__file || "" });
        }
        return chain.length ? { framework: "vue", chain } : null;
      }
    }
    return null;
  }
  function svelte(el) {
    for (let e = el; e; e = e.parentElement) {
      const m = e.__svelte_meta;
      if (m && m.loc) {
        // Svelte 4 counts lines and columns from 0 (and tells char);
        // Svelte 5 lines from 1, columns from 0.
        const old = "char" in m.loc;
        return { framework: "svelte", file: m.loc.file, line: m.loc.line + (old ? 1 : 0), column: m.loc.column + 1, of: e === el ? "" : e.localName };
      }
    }
    return null;
  }
  function react(el) {
    let fiber, e = el;
    for (; e && !fiber; e = e.parentElement) {
      const k = Object.keys(e).find((k) => k.startsWith("__reactFiber$") || k.startsWith("__reactInternalInstance$"));
      if (k) fiber = e[k];
    }
    if (!fiber) return null;
    const chain = [];
    const nameOf = (t) => {
      if (!t || typeof t === "string") return "";
      if (typeof t === "function") return t.displayName || t.name || "";
      return t.displayName || nameOf(t.render) || nameOf(t.type) || "";
    };
    for (let f = fiber.return; f && chain.length < 12; f = f.return) {
      const n = nameOf(f.type);
      // A component's _debugSource is where it was used, not written.
      if (n) chain.push({ name: n, file: "" });
    }
    const r = { framework: "react", chain };
    const src = fiber._debugSource; // React 18 and older, in development
    if (src && src.fileName) Object.assign(r, { file: src.fileName, line: src.lineNumber, column: src.columnNumber || 0 });
    else if (fiber._debugStack) r.stack = String(fiber._debugStack.stack || fiber._debugStack); // React 19
    return r;
  }
  function frameworks(el) {
    return [vue(el), svelte(el), react(el)].filter(Boolean);
  }
  // Attributes build plugins add to tell an element's source, as
  // vite-plugin-vue-inspector's data-v-inspector.
  const sourceAttr = /^data-(v-inspector|inspector-.+|source(-.+)?|loc|locator(-.+)?|component-(file|path|line)|file|line)$/i;
  function attrs(el) {
    for (let e = el, up = 0; e && up < 6; e = e.parentElement, up++) {
      const found = [...e.attributes].filter((a) => sourceAttr.test(a.name)).map((a) => ({ name: a.name, value: a.value }));
      if (found.length) return found.map((a) => ({ ...a, of: e === el ? "" : e.localName }));
    }
    return [];
  }

  // React 19 tells where an element was written only as the stack of
  // its JSX's call: the first frame of the app's own code, which the
  // module's source map turns back into the file's line.
  const maps = new Map();
  async function fromStack(stack) {
    const frames = [...stack.matchAll(/((?:https?|file):\/\/[^\s()]+?):(\d+):(\d+)/g)]
      .map((m) => ({ url: m[1], line: +m[2], column: +m[3] }))
      .filter((f) => !/\/node_modules\/|\/\.vite\/deps\/|react-dom|react_jsx|jsx-dev-runtime|\/@react-refresh/.test(f.url));
    const f = frames[0];
    if (!f) return null;
    const plain = { file: new URL(f.url).pathname, line: f.line, column: f.column, mapped: false };
    try {
      if (!maps.has(f.url)) maps.set(f.url, loadMap(f.url));
      const map = await maps.get(f.url);
      const at = map && lookup(map, f.line - 1, f.column - 1);
      if (!at) return plain;
      let file = at.source;
      try {
        const u = new URL(file, f.url);
        if (u.origin === location.origin) file = u.pathname;
      } catch {}
      return { file, line: at.line + 1, column: at.column + 1, mapped: true };
    } catch {
      return plain;
    }
  }
  async function loadMap(url) {
    const ctl = new AbortController();
    setTimeout(() => ctl.abort(), 3000);
    const code = await (await fetch(url, { signal: ctl.signal })).text();
    const m = /\/\/[#@] sourceMappingURL=(\S+)\s*$/.exec(code);
    if (!m) return null;
    let json;
    if (m[1].startsWith("data:")) {
      const data = m[1].slice(m[1].indexOf(",") + 1);
      json = /;base64/.test(m[1].slice(0, m[1].indexOf(","))) ? new TextDecoder().decode(Uint8Array.from(atob(data), (c) => c.charCodeAt(0))) : decodeURIComponent(data);
    } else {
      json = await (await fetch(new URL(m[1], url), { signal: ctl.signal })).text();
    }
    const map = JSON.parse(json);
    const root = map.sourceRoot ? map.sourceRoot.replace(/\/?$/, "/") : "";
    return { sources: (map.sources || []).map((s) => root + s), mappings: map.mappings || "" };
  }
  // lookup finds the original place of a generated one (both from 0): the
  // last mapping of its line at or before its column.
  function lookup(map, line, column) {
    const digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let src = 0, oline = 0, ocol = 0, best = null;
    const lines = map.mappings.split(";");
    for (let l = 0; l < lines.length && l <= line; l++) {
      let gcol = 0;
      for (const seg of lines[l].split(",")) {
        if (!seg) continue;
        const v = [];
        for (let i = 0, n = 0, shift = 0; i < seg.length; i++) {
          const d = digits.indexOf(seg[i]);
          n += (d & 31) << shift;
          if (d & 32) { shift += 5; continue; }
          v.push(n & 1 ? -(n >>> 1) : n >>> 1);
          n = 0; shift = 0;
        }
        gcol += v[0];
        if (v.length < 4) continue;
        src += v[1]; oline += v[2]; ocol += v[3];
        if (l === line && gcol <= column) best = { source: map.sources[src], line: oline, column: ocol };
        if (l === line && gcol > column && !best) best = { source: map.sources[src], line: oline, column: ocol };
      }
    }
    return best;
  }

  // The element's own words: its text nodes', else, for a small element,
  // all of its text.
  function ownText(el) {
    let t = norm([...el.childNodes].filter((n) => n.nodeType === 3).map((n) => n.data).join(" "));
    if (!t && el.children.length <= 3) t = norm(el.innerText);
    return t.slice(0, 2000);
  }
  // What of a text an agent can grep for: the source wraps long text over
  // lines, and may write some characters as entities (&rsquo;, &mdash;),
  // so a phrase is a run of 4 to 8 words without them, ending at a word;
  // a short text is itself. One phrase, and a second of a longer text.
  const plainWord = /^[^\u2018\u2019\u201c\u201d\u2014\u2013\u2026&<>\u00a9\u00ae\u2122\u00a0]+$/;
  function phrases(t) {
    const words = t.split(" ").filter(Boolean);
    if (words.length <= 8 && words.every((w) => plainWord.test(w))) return t ? [t] : [];
    const runs = [];
    let run = [];
    const end = () => { if (run.length) runs.push(run); run = []; };
    for (const w of words) {
      if (!plainWord.test(w)) { end(); continue; }
      run.push(w);
      if (/[.!?;:]$/.test(w)) end(); // a sentence's end
    }
    end();
    const chunks = runs.map((r) => (r.length <= 8 ? r : r.slice(0, 6)).join(" "));
    const long = chunks.filter((c) => c.split(" ").length >= 4);
    if (!long.length) {
      const best = chunks.sort((a, b) => b.split(" ").length - a.split(" ").length)[0];
      return best ? [best] : [];
    }
    return words.length > 12 && long.length > 1 ? [long[0], long[long.length - 1]] : [long[0]];
  }
  function hints(el) {
    const out = [];
    const cls = el.getAttribute("class");
    if (cls && cls.trim()) out.push({ kind: "class", value: cls.trim(), count: count((e) => (e.getAttribute("class") || "").trim() === cls.trim()) });
    for (const name of ["id", "data-testid", "data-test", "data-cy", "aria-label", "name", "alt", "title", "href", "src", "placeholder", "for", "role", "type"]) {
      const v = el.getAttribute(name);
      if (v == null || v === "" || (name === "type" && el.localName !== "input") || (name === "role" && !v)) continue;
      // A long value's start, which greps as the whole does: no "…".
      out.push({ kind: name, value: v.slice(0, 200), count: count((e) => e.getAttribute(name) === v) });
    }
    const body = norm(document.body ? document.body.innerText : "");
    for (const t of phrases(ownText(el))) {
      let n = 0;
      for (let i = body.indexOf(t); i >= 0 && n < 99; i = body.indexOf(t, i + t.length)) n++;
      out.push({ kind: "text", value: t, count: n || 1 });
    }
    return out;
  }

  const ordinal = (n) => n + (n % 10 === 1 && n % 100 !== 11 ? "st" : n % 10 === 2 && n % 100 !== 12 ? "nd" : n % 10 === 3 && n % 100 !== 13 ? "rd" : "th");
  const short = (el) => {
    let s = el.localName;
    if (el.id) return s + "#" + el.id;
    const c = (el.getAttribute("class") || "").trim().split(/\s+/)[0];
    return c ? s + "." + c : s;
  };
  const kin = (el) => {
    const c = (el.getAttribute("class") || "").trim().split(/\s+/)[0];
    return [...(el.parentElement ? el.parentElement.children : [])].filter((s) => s.localName === el.localName && (!c || s.classList.contains(c)));
  };
  // Where it is, as a person would tell it: the landmark it is in, the
  // heading above it, and which of its like it is.
  function context(el) {
    const parts = [];
    const lm = el.parentElement && el.parentElement.closest("main, nav, header, footer, aside, form, dialog, article, [role=main], [role=navigation], [role=dialog], [role=banner], [role=contentinfo], [role=complementary]");
    if (lm) {
      const label = lm.getAttribute("aria-label");
      parts.push("in <" + lm.localName + (label ? ' aria-label="' + cut(label, 40) + '"' : "") + ">");
    }
    const hs = [...document.querySelectorAll("h1, h2, h3, h4, h5, h6, [role=heading]")]
      .filter((h) => h !== el && !el.contains(h) && h.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING);
    const h = hs.pop();
    if (h && norm(h.innerText)) {
      let sec = h.parentElement;
      while (sec && !sec.contains(el)) sec = sec.parentElement;
      // The heading heads what holds them both, unless that is as wide
      // as the landmark: then it is only above.
      const what = sec && sec !== document.body && sec !== lm && sec !== document.documentElement ? short(sec) : "";
      const said = JSON.stringify(cut(norm(h.innerText), 60));
      parts.push(what ? "in <" + what + "> headed " + said : "below the heading " + said);
    }
    for (let e = el, up = 0; e && e.parentElement && up < 5; e = e.parentElement, up++) {
      const k = kin(e);
      if (k.length > 1) {
        parts.push((e === el ? "the " : "in the ") + ordinal(k.indexOf(e) + 1) + " of " + k.length + " " + short(e));
        break;
      }
    }
    return parts.join(", ");
  }
  const openTag = (el, n) => {
    let s = "<" + el.localName;
    for (const a of el.attributes) s += " " + a.name + (a.value === "" ? "" : '="' + cut(a.value, n).replace(/"/g, "&quot;") + '"');
    return s + ">";
  };
  // The element's HTML, cut short: its first children, two deep.
  function excerpt(el) {
    const walk = (e, depth, pad) => {
      const kids = [...e.childNodes].filter((n) => n.nodeType === 1 || (n.nodeType === 3 && norm(n.data)));
      const text = kids.length === 1 && kids[0].nodeType === 3;
      if (text) return pad + openTag(e, 120) + cut(norm(kids[0].data), 100) + "</" + e.localName + ">";
      if (!kids.length) return pad + openTag(e, 120) + (/^(img|input|br|hr|meta|link|source)$/.test(e.localName) ? "" : "</" + e.localName + ">");
      if (depth >= 2) return pad + openTag(e, 80) + "…</" + e.localName + ">";
      const lines = [pad + openTag(e, 120)];
      kids.slice(0, 8).forEach((k) => lines.push(k.nodeType === 3 ? pad + "  " + cut(norm(k.data), 100) : walk(k, depth + 1, pad + "  ")));
      if (kids.length > 8) lines.push(pad + "  … " + (kids.length - 8) + " more");
      lines.push(pad + "</" + e.localName + ">");
      return lines.join("\n");
    };
    const s = walk(el, 0, "");
    return s.length > 1500 ? s.slice(0, 1499) + "…" : s;
  }
  // The element among its siblings, in its parent.
  function skeleton(el) {
    const up = el.parentElement;
    if (!up || up === document.documentElement) return "";
    const line = (e) => {
      const t = e.children.length ? "…" : cut(norm(e.textContent), 40);
      return openTag(e, 50) + t + "</" + e.localName + ">";
    };
    const sibs = [...up.children].filter((e) => e !== ui);
    const at = sibs.indexOf(el);
    const lines = [openTag(up, 60)];
    sibs.forEach((s, i) => {
      if (Math.abs(i - at) > 3 && i !== 0 && i !== sibs.length - 1) {
        if (Math.abs(i - at) === 4) lines.push("  …");
        return;
      }
      lines.push("  " + line(s) + (s === el ? "   ← this one" : ""));
    });
    lines.push("</" + up.localName + ">");
    return lines.join("\n");
  }
  const esc = (s) => (window.CSS && CSS.escape ? CSS.escape(s) : s.replace(/[^\w-]/g, "\\$&"));
  const unique = (sel, el) => {
    try { const m = document.querySelectorAll(sel); return m.length === 1 && m[0] === el; } catch { return false; }
  };
  const step = (e) => {
    let s = e.localName;
    const c = (e.getAttribute("class") || "").trim().split(/\s+/).filter((c) => c && /^[a-zA-Z_-][\w-]*$/.test(c)).slice(0, 2);
    if (c.length) s += "." + c.map(esc).join(".");
    const same = e.parentElement ? [...e.parentElement.children].filter((x) => x.localName === e.localName) : [];
    if (same.length > 1) s += ":nth-of-type(" + (same.indexOf(e) + 1) + ")";
    return s;
  };
  // A selector that finds it alone: by an id or test id, else by the
  // steps up to an ancestor that has one, as short as stays unique.
  function selector(el) {
    for (const a of ["id", "data-testid", "data-test", "data-cy"]) {
      const v = el.getAttribute(a);
      if (!v) continue;
      const s = a === "id" ? "#" + esc(v) : el.localName + "[" + a + '="' + v.replace(/"/g, '\\"') + '"]';
      if (unique(s, el)) return s;
    }
    const steps = [];
    for (let e = el; e && e !== document.documentElement; e = e.parentElement) {
      if (e !== el && e.id && unique("#" + esc(e.id), e)) { steps.unshift("#" + esc(e.id)); break; }
      steps.unshift(step(e));
    }
    for (let n = 1; n <= steps.length; n++) {
      const s = steps.slice(-n).join(" > ");
      if (unique(s, el)) return s;
    }
    return steps.join(" > ");
  }
  function path(el) {
    const out = [];
    for (let e = el; e; e = e.parentElement) {
      let s = e.localName;
      if (e.id) s += "#" + e.id;
      const c = (e.getAttribute("class") || "").trim().split(/\s+/).filter(Boolean).slice(0, 2);
      if (c.length) s += "." + c.join(".");
      out.unshift(s);
    }
    return out.join(" > ");
  }
  function styles(el) {
    const cs = getComputedStyle(el), out = {};
    const keep = (k, v, skip) => { if (v && !(skip || []).includes(v)) out[k] = v; };
    keep("display", cs.display);
    keep("position", cs.position, ["static"]);
    keep("color", cs.color);
    keep("background-color", cs.backgroundColor, ["rgba(0, 0, 0, 0)", "transparent"]);
    keep("background-image", cs.backgroundImage, ["none"]);
    keep("font", [cs.fontWeight, cs.fontSize + "/" + cs.lineHeight, cs.fontFamily.split(",")[0]].join(" "));
    keep("margin", cs.margin, ["0px"]);
    keep("padding", cs.padding, ["0px"]);
    keep("border", cs.borderWidth === "0px" ? "" : cs.borderWidth + " " + cs.borderStyle + " " + cs.borderColor);
    keep("border-radius", cs.borderRadius, ["0px"]);
    keep("box-shadow", cs.boxShadow, ["none"]);
    if (/flex|grid/.test(cs.display)) {
      keep("flex-direction", cs.flexDirection, ["row"]);
      keep("gap", cs.gap, ["normal", "0px"]);
      keep("align-items", cs.alignItems, ["normal"]);
      keep("justify-content", cs.justifyContent, ["normal"]);
      if (/grid/.test(cs.display)) keep("grid-template-columns", cs.gridTemplateColumns, ["none"]);
    }
    keep("opacity", cs.opacity, ["1"]);
    keep("z-index", cs.zIndex, ["auto"]);
    return out;
  }
  function collect() {
    return Promise.all(picks.map(async (p) => {
      const el = p.el, r = el.getBoundingClientRect();
      const fw = frameworks(el);
      for (const f of fw) if (f.stack) {
        const at = await fromStack(f.stack);
        delete f.stack;
        if (at) Object.assign(f, at);
      }
      const tags = document.getElementsByTagName(el.localName);
      return {
        note: norm(p.note) ? p.note.trim() : "",
        tag: el.localName,
        text: cut(ownText(el), 60),
        frameworks: fw,
        attrs: attrs(el),
        // Which of its tag it is, for a page of a file, to find in it.
        index: [...tags].indexOf(el),
        count: tags.length,
        id: el.id || "",
        class: (el.getAttribute("class") || "").trim(),
        hints: hints(el),
        context: context(el),
        skeleton: skeleton(el),
        html: excerpt(el),
        selector: selector(el),
        path: path(el),
        rect: { x: r.left, y: r.top, w: r.width, h: r.height },
        styles: styles(el),
      };
    })).then((list) => ({
      url: location.href,
      title: document.title,
      viewport: { w: innerWidth, h: innerHeight, dpr: devicePixelRatio },
      screenshots: shots,
      pageShot: whole,
      scroll: { x: scrollX, y: scrollY },
      picks: list,
    }));
  }
  window.__keroPick = { stop: () => finish(false) };
  document.documentElement.append(ui);
  listen(true);
  render();
  tick();
  return finished;
}
