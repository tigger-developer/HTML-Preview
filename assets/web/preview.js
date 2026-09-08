// ABOUTME: Adds explicit clipboard activation and accessible outline controls.
// ABOUTME: No source content runs as code and static reading requires no script.
function enhancePreview() {
  const header = document.getElementById('hp-header');
  const heading = header.firstElementChild;
  const sourcePath = heading.getAttribute('data-hp-source');
  const controller = new AbortController();
  const events = { signal: controller.signal };
  let feedbackTimer;
  let pending = false;
  let teardownOutline;
  const copy = document.createElement('button');
  copy.type = 'button';
  copy.className = 'hp-copy';
  copy.setAttribute('aria-label', 'Copy source path');
  while (heading.firstChild) copy.append(heading.firstChild);
  heading.append(copy);
  const printSource = document.createElement('span');
  printSource.className = 'hp-print-source';
  for (const child of copy.childNodes) printSource.append(child.cloneNode(true));
  heading.append(printSource);
  const status = document.createElement('span');
  status.className = 'hp-status';
  status.setAttribute('role', 'status');
  status.setAttribute('aria-live', 'polite');
  header.append(status);
  const manual = document.createElement('textarea');
  manual.className = 'hp-manual';
  manual.readOnly = true;
  manual.value = sourcePath;
  manual.setAttribute('aria-label', 'Source path for manual copying');
  manual.hidden = true;
  header.append(manual);

  function copyFailed() {
    pending = false;
    copy.disabled = false;
    status.textContent = 'Clipboard unavailable. Select and copy the path below.';
    manual.hidden = false;
  }
  copy.addEventListener('click', () => {
    if (pending) return;
    clearTimeout(feedbackTimer);
    status.textContent = '';
    manual.hidden = true;
    pending = true;
    copy.disabled = true;
    try {
      const writing = navigator.clipboard.writeText(sourcePath);
      writing.then(() => {
        if (controller.signal.aborted) return;
        pending = false;
        copy.disabled = false;
        status.textContent = 'Source path copied.';
        feedbackTimer = setTimeout(() => { status.textContent = ''; }, 2000);
      }, () => { if (!controller.signal.aborted) copyFailed(); });
    } catch { copyFailed(); }
  }, events);

  window.addEventListener('pagehide', () => {
    clearTimeout(feedbackTimer);
    if (teardownOutline) teardownOutline();
    controller.abort();
    while (copy.firstChild) heading.insertBefore(copy.firstChild, copy);
    copy.remove();
    printSource.remove();
    status.remove();
    manual.remove();
  }, { once: true });

  async function enhanceOutline() {
    const main = document.getElementById('hp-document');
    const owners = new WeakMap();
    const records = [];
    const walker = document.createTreeWalker(main, NodeFilter.SHOW_ELEMENT);
    let count = 0;
    while (walker.nextNode()) {
      if (controller.signal.aborted) return;
      const node = walker.currentNode;
      const parent = owners.get(node.parentElement) || null;
      let owner = parent;
      if (node.tagName === 'SECTION' && node.hasAttribute('data-hp-level')) {
        const title = Array.from(node.children).find(child => child.matches('h2,h3,h4,h5,h6,[role=heading]'));
        if (title) {
          owner = { node, title, parent, mode: 'all', visible: true, children: [] };
          records.push(owner);
          if (parent) parent.children.push(owner);
        }
      }
      owners.set(node, owner);
      count += 1;
      if (count % 500 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
    }
    if (controller.signal.aborted || records.length === 0) return;

    function refresh() {
      for (const record of records) {
        record.visible = !record.parent || (record.parent.visible && record.parent.mode !== 'folded');
        record.node.hidden = !record.visible;
        for (const part of record.node.children) {
          if (part !== record.title && !part.matches('section[data-hp-level]')) part.hidden = record.mode !== 'all';
        }
        record.button.textContent = record.mode === 'all' ? 'Fold' : 'Expand';
        record.button.setAttribute('aria-expanded', String(record.mode !== 'folded'));
      }
    }

    function subtree(root, mode) {
      const stack = [root];
      while (stack.length) {
        const record = stack.pop();
        record.mode = mode === 'all' ? 'all' : 'folded';
        stack.push(...record.children);
      }
      root.mode = mode;
      if (mode === 'all') root.node.querySelectorAll('details').forEach(detail => { detail.open = true; });
    }

    const bar = document.createElement('nav');
    let printState;
    bar.className = 'hp-toolbar';
    bar.setAttribute('aria-label', 'Document outline');
    function globalMode(mode) {
      for (const record of records) record.mode = mode === 'overview' ? 'folded' : mode === 'content' ? 'children' : 'all';
      if (mode === 'showall') main.querySelectorAll('details').forEach(detail => { detail.open = true; });
      refresh();
    }
    for (const [label, mode] of [['Overview', 'overview'], ['Contents', 'content'], ['Show all', 'showall']]) {
      const button = document.createElement('button');
      button.type = 'button';
      button.textContent = label;
      button.addEventListener('click', () => { globalMode(mode); }, events);
      bar.append(button);
    }
    teardownOutline = () => {
      restorePrint();
      bar.remove();
      for (const record of records) {
        record.node.hidden = false;
        for (const part of record.node.children) part.hidden = false;
        if (record.button) record.button.remove();
      }
      main.querySelectorAll('details').forEach(detail => { detail.open = true; });
    };
    let controls = 0;
    for (const record of records) {
      if (controller.signal.aborted) return;
      const button = document.createElement('button');
      button.type = 'button';
      button.disabled = true;
      button.className = 'hp-toggle';
      button.setAttribute('aria-label', 'Fold or expand ' + record.title.textContent);
      button.addEventListener('click', () => {
        const next = record.mode === 'all' ? 'folded' : record.mode === 'folded' && record.children.length ? 'children' : 'all';
        subtree(record, next);
        refresh();
      }, events);
      record.button = button;
      record.title.prepend(button);
      controls += 1;
      if (controls % 250 === 0) await new Promise(resolve => requestAnimationFrame(resolve));
    }
    if (controller.signal.aborted) return;
    for (const record of records) record.button.disabled = false;
    main.before(bar);
    globalMode(main.getAttribute('data-hp-startup'));
    for (const record of records) {
      const initial = record.node.getAttribute('data-hp-visibility');
      const inherited = record.parent && record.parent.initialCascade;
      if (inherited) record.mode = inherited;
      if (initial) record.mode = initial;
      record.initialCascade = initial ? (initial === 'all' ? 'all' : 'folded') : inherited;
    }
    refresh();

    function revealFragment() {
      let id;
      try { id = decodeURIComponent(location.hash.slice(1)); } catch { return; }
      if (!id) return;
      const target = document.getElementById(id);
      if (!target || !main.contains(target)) return;
      let record = owners.get(target);
      while (record) { record.mode = 'all'; record = record.parent; }
      for (let node = target; node && node !== main; node = node.parentElement) {
        if (node.tagName === 'DETAILS') node.open = true;
      }
      refresh();
      target.scrollIntoView();
    }
    window.addEventListener('hashchange', revealFragment, events);
    revealFragment();

    function restorePrint() {
      if (!printState) return;
      records.forEach((record, i) => { record.mode = printState.modes[i]; });
      for (const [detail, open] of printState.drawers) detail.open = open;
      printState = undefined;
      refresh();
    }
    window.addEventListener('beforeprint', () => {
      if (printState) return;
      printState = { modes: records.map(record => record.mode), drawers: Array.from(main.querySelectorAll('details'), detail => [detail, detail.open]) };
      globalMode('showall');
    }, events);
    window.addEventListener('afterprint', restorePrint, events);
  }

  enhanceOutline();
}

enhancePreview();
window.addEventListener('pageshow', event => {
  if (event.persisted) enhancePreview();
});
