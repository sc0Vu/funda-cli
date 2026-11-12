(() => {
  'use strict';
  const term = document.getElementById('demo-terminal');
  const section = document.getElementById('demo');
  const out = document.getElementById('demo-scrollback');
  const form = document.getElementById('demo-form');
  const input = document.getElementById('demo-input');
  if (!term || !out || !input) return;

  const PROMPT = '➜ funda';
  const history = [];
  let histPos = 0;
  let state = 'idle'; // idle | loading | ready | failed
  const queue = [];

  function print(text, cls) {
    const d = document.createElement('div');
    if (cls) d.className = cls;
    d.textContent = text;
    out.appendChild(d);
    out.scrollTop = out.scrollHeight;
    return d;
  }

  function echo(line) {
    const d = document.createElement('div');
    d.className = 'demo-line-cmd';
    const p = document.createElement('span');
    p.className = 'demo-p';
    p.textContent = PROMPT + ' ';
    d.appendChild(p);
    d.appendChild(document.createTextNode(line));
    out.appendChild(d);
    out.scrollTop = out.scrollHeight;
  }

  function execute(line) {
    let res;
    try {
      res = globalThis.fundaExec(line);
    } catch (e) {
      print('Error: ' + (e && e.message ? e.message : e), 'demo-line-err');
      return;
    }
    if (res) print(String(res).replace(/\n+$/, ''));
  }

  function run(line) {
    const trimmed = line.trim();
    if (trimmed.toLowerCase() === 'clear') {
      out.textContent = '';
      return;
    }
    echo(line);
    if (!trimmed) return;
    if (state === 'ready') execute(line);
    else if (state === 'failed') print("Demo unavailable. Build it with 'make wasm'.", 'demo-line-err');
    else {
      queue.push(line);
      load();
    }
  }

  function injectScript(src) {
    return new Promise((resolve, reject) => {
      const s = document.createElement('script');
      s.src = src;
      s.onload = resolve;
      s.onerror = () => reject(new Error('could not load ' + src));
      document.head.appendChild(s);
    });
  }

  async function instantiate(go) {
    if (WebAssembly.instantiateStreaming) {
      try {
        return await WebAssembly.instantiateStreaming(fetch('./funda.wasm'), go.importObject);
      } catch (e) {
        // wrong MIME type or no streaming support: fall back below
      }
    }
    const resp = await fetch('./funda.wasm');
    if (!resp.ok) throw new Error('could not load funda.wasm (HTTP ' + resp.status + ')');
    return WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
  }

  async function waitReady() {
    for (let i = 0; i < 100; i++) {
      if (globalThis.fundaReady === true) return;
      await new Promise((r) => setTimeout(r, 50));
    }
    throw new Error('module did not become ready in time');
  }

  async function load() {
    if (state !== 'idle') return;
    state = 'loading';
    print('Loading WebAssembly module…', 'demo-line-sys');
    try {
      if (typeof WebAssembly === 'undefined') throw new Error('WebAssembly is not supported');
      await injectScript('./wasm_exec.js');
      if (typeof Go === 'undefined') throw new Error('wasm_exec.js did not define Go');
      const go = new Go();
      const { instance } = await instantiate(go);
      go.run(instance); // never resolves; do not await
      await waitReady();
      state = 'ready';
      print("Ready. Type 'help'.", 'demo-line-sys');
      while (queue.length) execute(queue.shift());
    } catch (e) {
      state = 'failed';
      queue.length = 0;
      const reason = e && e.message ? e.message : String(e);
      print("Demo unavailable: " + reason + ". Build it with 'make wasm'.", 'demo-line-err');
    }
  }

  function submit(line) {
    if (line.trim()) {
      if (history[history.length - 1] !== line) history.push(line);
    }
    histPos = history.length;
    run(line);
  }

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    const line = input.value;
    input.value = '';
    submit(line);
  });

  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (histPos > 0) input.value = history[--histPos];
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (histPos < history.length - 1) input.value = history[++histPos];
      else {
        histPos = history.length;
        input.value = '';
      }
    } else if (e.key.toLowerCase() === 'l' && e.ctrlKey) {
      e.preventDefault();
      out.textContent = '';
    }
  });

  term.addEventListener('click', () => {
    if (!String(window.getSelection())) input.focus();
    load();
  });
  input.addEventListener('focus', load);

  document.querySelectorAll('.demo-chip').forEach((b) => {
    b.addEventListener('click', () => {
      submit(b.dataset.cmd);
      input.focus({ preventScroll: true });
    });
  });

  if ('IntersectionObserver' in window && section) {
    const io = new IntersectionObserver((entries) => {
      if (entries.some((en) => en.isIntersecting)) {
        io.disconnect();
        load();
      }
    });
    io.observe(section);
  }
})();
