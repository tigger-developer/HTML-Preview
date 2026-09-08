// ABOUTME: Adds explicit clipboard activation and accessible outline controls.
// ABOUTME: No source content runs as code and static reading requires no script.
function clipboardService(controller, dispose) {
  const feedback = document.createElement('span');
  feedback.className = 'hp-feedback';
  const status = document.createElement('span');
  status.className = 'hp-status';
  status.setAttribute('role', 'status');
  status.setAttribute('aria-live', 'polite');
  const manual = document.createElement('textarea');
  manual.className = 'hp-manual';
  manual.readOnly = true;
  manual.hidden = true;
  feedback.append(status, manual);
  let timer;
  let pending = false;

  dispose(() => { clearTimeout(timer); feedback.remove(); });
  return (value, label, button) => {
    if (pending || value === '' || controller.signal.aborted) return;
    clearTimeout(timer);
    status.textContent = '';
    manual.hidden = true;
    manual.value = '';
    button.after(feedback);
    pending = true;
    button.disabled = true;

    function failed() {
      if (controller.signal.aborted) return;
      pending = false;
      button.disabled = false;
      status.textContent = 'Clipboard unavailable. Select and copy ' + label.toLowerCase() + ' below.';
      manual.value = value;
      manual.setAttribute('aria-label', label + ' for manual copying');
      manual.hidden = false;
    }
    try {
      // Called directly within activation, before any asynchronous work.
      const writing = navigator.clipboard.writeText(value);
      writing.then(() => {
        if (controller.signal.aborted) return;
        pending = false;
        button.disabled = false;
        status.textContent = label + ' copied.';
        timer = setTimeout(() => { status.textContent = ''; }, 2000);
      }, failed);
    } catch { failed(); }
  };
}

function enhanceHeader(header, copyValue, events, dispose) {
  const heading = header.firstElementChild;
  const sourcePath = heading.getAttribute('data-hp-source');
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
  copy.addEventListener('click', () => { copyValue(sourcePath, 'Source path', copy); }, events);
  dispose(() => {
    while (copy.firstChild) heading.insertBefore(copy.firstChild, copy);
    copy.remove();
    printSource.remove();
  });
}

function hasSelection() {
  const selection = window.getSelection();
  return selection && !selection.isCollapsed;
}

function nextFrame(signal) {
  return new Promise(resolve => {
    if (signal.aborted) { resolve(); return; }
    let frame;
    function finish() {
      cancelAnimationFrame(frame);
      signal.removeEventListener('abort', finish);
      resolve();
    }
    frame = requestAnimationFrame(finish);
    signal.addEventListener('abort', finish, { once: true });
  });
}

async function enhanceCode(main, copyValue, controller, dispose) {
  const events = { signal: controller.signal };
  const targets = new WeakMap();
  const buttons = [];
  const wrappers = [];
  const linkTails = new WeakMap();
  const blockWrappers = new WeakMap();
  let inlineNumber = 0;
  let blockNumber = 0;
  let gesture;
  dispose(() => {
    for (const button of buttons) button.remove();
    for (const wrapper of wrappers) {
      while (wrapper.firstChild) wrapper.before(wrapper.firstChild);
      wrapper.remove();
    }
  });

  const codes = Array.from(main.querySelectorAll('code'));
  for (let i = 0; i < codes.length; i += 1) {
    if (controller.signal.aborted) return;
    const code = codes[i];
    const pre = code.closest('pre');
    if (code.parentElement.closest('code')) continue;
    // Capture before any controls are inserted; never derive this from a class,
    // source attribute, control label or highlighted span.
    const value = code.textContent;
    const label = pre ? 'Code block ' + (++blockNumber) : 'Inline code ' + (++inlineNumber);
    if (value === '') continue;
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'hp-code-copy';
    button.textContent = '⧉';
    button.setAttribute('aria-label', 'Copy ' + label.toLowerCase());
    button.title = 'Copy ' + label.toLowerCase();
    button.addEventListener('click', () => { copyValue(value, label, button); }, events);
    buttons.push(button);

    const link = code.closest('a');
    if (link) {
      const tail = linkTails.get(link) || link;
      tail.after(button);
      linkTails.set(link, button);
    } else if (pre) {
      let wrapper = blockWrappers.get(pre);
      if (!wrapper) {
        wrapper = document.createElement('div');
        wrapper.className = 'hp-code-block';
        wrapper.hidden = pre.hidden;
        pre.hidden = false;
        pre.before(wrapper);
        wrapper.append(pre);
        wrappers.push(wrapper);
        blockWrappers.set(pre, wrapper);
      }
      wrapper.insertBefore(button, pre);
    } else {
      code.after(button);
    }
    const target = { value, label, button };
    targets.set(code, target);
    if (pre && pre.querySelectorAll('code').length === 1) targets.set(pre, target);
    if (i % 250 === 249) await nextFrame(controller.signal);
  }
  if (controller.signal.aborted) return;

  function targetFor(event) {
    if (!(event.target instanceof Element)) return undefined;
    if (event.target.closest('a,button,summary,textarea,input,select')) return undefined;
    const code = event.target.closest('code,pre');
    if (!code || !main.contains(code)) return undefined;
    return targets.get(code);
  }
  main.addEventListener('pointerdown', event => {
    gesture = { target: targetFor(event), x: event.clientX, y: event.clientY, selected: hasSelection(), dragged: false };
  }, events);
  main.addEventListener('pointermove', event => {
    if (gesture && (Math.abs(event.clientX - gesture.x) > 4 || Math.abs(event.clientY - gesture.y) > 4)) gesture.dragged = true;
  }, events);
  main.addEventListener('pointercancel', () => { gesture = undefined; }, events);
  main.addEventListener('click', event => {
    const target = targetFor(event);
    const prior = gesture;
    gesture = undefined;
    if (!target || event.button !== 0 || event.detail > 1 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || hasSelection()) return;
    if (prior && (prior.target !== target || prior.selected || prior.dragged)) return;
    copyValue(target.value, target.label, target.button);
  }, events);
}

async function enhanceOutline(main, controller, dispose) {
  const events = { signal: controller.signal };
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
        owner = { node, title, label: title.textContent.trim(), parent, mode: 'all', visible: true, children: [], textWrappers: [] };
        records.push(owner);
        if (parent) parent.children.push(owner);
      }
    }
    owners.set(node, owner);
    count += 1;
    if (count % 500 === 0) await nextFrame(controller.signal);
  }
  if (controller.signal.aborted || records.length === 0) return;

  function refresh() {
    for (const record of records) {
      record.visible = !record.parent || (record.parent.visible && record.parent.mode !== 'folded');
      record.node.hidden = !record.visible;
      for (const part of record.node.children) {
        if (part !== record.title && part !== record.button && !part.matches('section[data-hp-level]')) part.hidden = record.mode !== 'all';
      }
      const expanded = record.mode === 'all';
      record.button.setAttribute('aria-expanded', String(expanded));
      record.button.setAttribute('aria-label', (expanded ? 'Collapse ' : 'Expand ') + record.label);
      record.button.title = (expanded ? 'Collapse ' : 'Expand ') + record.label;
    }
  }

  function subtree(root, mode) {
    const stack = [root];
    while (stack.length) {
      const record = stack.pop();
      record.mode = mode;
      stack.push(...record.children);
    }
    if (mode === 'all') root.node.querySelectorAll('details').forEach(detail => { detail.open = true; });
  }

  const toolbar = document.createElement('nav');
  let printState;
  toolbar.className = 'hp-toolbar';
  toolbar.setAttribute('aria-label', 'Document outline');
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
    toolbar.append(button);
  }
  dispose(() => {
    restorePrint();
    toolbar.remove();
    for (const record of records) {
      record.node.hidden = false;
      record.node.classList.remove('hp-outline-section');
      for (const part of record.node.children) part.hidden = false;
      if (record.button) record.button.remove();
      for (const wrapper of record.textWrappers) {
        while (wrapper.firstChild) wrapper.before(wrapper.firstChild);
        wrapper.remove();
      }
    }
    main.querySelectorAll('details').forEach(detail => { detail.open = true; });
  });
  for (let i = 0; i < records.length; i += 1) {
    if (controller.signal.aborted) return;
    const record = records[i];
    // Removal notices and admitted raw HTML can leave direct text children.
    // Give visible text an element to hide, preserving it for teardown/printing.
    for (const part of Array.from(record.node.childNodes)) {
      if (part.nodeType !== Node.TEXT_NODE || part.textContent.trim() === '') continue;
      const wrapper = document.createElement('span');
      part.before(wrapper);
      wrapper.append(part);
      record.textWrappers.push(wrapper);
    }
    const button = document.createElement('button');
    button.type = 'button';
    button.disabled = true;
    button.className = 'hp-toggle';
    const plus = document.createElement('span');
    plus.className = 'hp-toggle-plus';
    plus.textContent = '+';
    plus.setAttribute('aria-hidden', 'true');
    button.append(plus);
    button.addEventListener('click', () => {
      subtree(record, record.mode === 'all' ? 'folded' : 'all');
      refresh();
    }, events);
    record.button = button;
    record.node.classList.add('hp-outline-section');
    record.node.prepend(button);
    if (i % 250 === 249) await nextFrame(controller.signal);
  }
  if (controller.signal.aborted) return;
  for (const record of records) record.button.disabled = false;
  main.before(toolbar);
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
  // Re-activating the current fragment does not produce a hashchange event.
  document.addEventListener('click', event => {
    const link = event.target instanceof Element && event.target.closest('a');
    if (link && link.hash && link.href === location.href) revealFragment();
  }, events);
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

function enhancePreview() {
  const header = document.getElementById('hp-header');
  const main = document.getElementById('hp-document');
  const controller = new AbortController();
  const disposers = [];
  const dispose = callback => { disposers.push(callback); };
  function teardown() {
    controller.abort();
    while (disposers.length) disposers.pop()();
  }
  window.addEventListener('pagehide', teardown, { once: true, signal: controller.signal });
  const copyValue = clipboardService(controller, dispose);
  enhanceHeader(header, copyValue, { signal: controller.signal }, dispose);
  // Outline labels are captured before inline copy controls can affect headings.
  async function enhance() {
    await enhanceOutline(main, controller, dispose);
    if (!controller.signal.aborted) await enhanceCode(main, copyValue, controller, dispose);
  }
  enhance().catch(() => {
    if (controller.signal.aborted) return;
    teardown();
    const status = document.createElement('p');
    status.className = 'hp-status';
    status.setAttribute('role', 'status');
    status.textContent = 'Interactive controls unavailable. The full document remains readable.';
    header.after(status);
    window.addEventListener('pagehide', () => { status.remove(); }, { once: true });
  });
}

enhancePreview();
window.addEventListener('pageshow', event => {
  if (event.persisted) enhancePreview();
});
