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
  document.body.append(feedback);
  let timer;
  let pending = false;
  let confirmed;

  function clearConfirmation() {
    clearTimeout(timer);
    if (confirmed) {
      confirmed.classList.remove('hp-copy-confirmed');
      confirmed.removeAttribute('data-hp-copy-message');
      confirmed = undefined;
    }
    status.textContent = '';
  }

  dispose(() => { clearConfirmation(); feedback.remove(); });
  window.addEventListener('beforeprint', clearConfirmation, { signal: controller.signal });
  return (value, label, button, surface = button) => {
    if (pending || value === '' || controller.signal.aborted) return;
    clearConfirmation();
    feedback.classList.remove('hp-copy-error');
    manual.hidden = true;
    manual.value = '';
    pending = true;
    button.disabled = true;

    function failed() {
      if (controller.signal.aborted) return;
      pending = false;
      button.disabled = false;
      button.after(feedback);
      feedback.classList.add('hp-copy-error');
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
        confirmed = surface;
        surface.setAttribute('data-hp-copy-message', status.textContent);
        surface.classList.add('hp-copy-confirmed');
        timer = setTimeout(clearConfirmation, 2000);
      }, failed);
    } catch { failed(); }
  };
}

function enhanceHeader(header, copyValue, events, dispose) {
  const heading = header.querySelector('[data-hp-source]');
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
    const encoded = code.getAttribute('data-hp-copy-base64');
    const value = encoded === null ? code.textContent : new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0)));
    const label = pre ? 'Code block ' + (++blockNumber) : 'Inline code ' + (++inlineNumber);
    if (value === '') continue;
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'hp-code-copy';
    button.textContent = '⧉';
    button.setAttribute('aria-label', 'Copy ' + label.toLowerCase());
    button.title = 'Copy ' + label.toLowerCase();
    let surface = code;
    button.addEventListener('click', () => { copyValue(value, label, button, surface); }, events);
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
      surface = wrapper;
    } else {
      code.after(button);
    }
    const target = { value, label, button, surface };
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
    copyValue(target.value, target.label, target.button, target.surface);
  }, events);
}

// Match details only when their section/summary identity is unambiguous on both pages.
function foldableDetails() {
  return [...document.querySelectorAll('#hp-document details, #hp-frontmatter')];
}
function detailIdentities() {
  const unique = new Map();
  for (const detail of foldableDetails()) {
    const key = detail.id || JSON.stringify([detail.closest('section[id]')?.id || '', detail.querySelector('summary')?.textContent || '']);
    unique.set(key, unique.has(key) ? null : detail);
  }
  return unique;
}

async function enhanceOutline(main, header, controller, dispose) {

  const events = { signal: controller.signal };
  // Endnotes belong to the document, outside the final foldable section.
  for (const notes of main.querySelectorAll('section.footnotes')) main.append(notes);
  const closeOwnedDrawers = root => { root.querySelectorAll('details[data-hp-org-drawer]').forEach(detail => { detail.open = false; }); };
  const owners = new WeakMap();
  const records = [];
  let lastPreset = main.dataset.hpPreset || 'showall';
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
  if (controller.signal.aborted) return;
  if (records.length === 0) closeOwnedDrawers(main);

  function refresh() {
    for (const record of records) {
      record.visible = !record.parent || (record.parent.visible && record.parent.mode !== 'folded');
      record.node.hidden = !record.visible;
      for (const part of record.node.children) {
        if (part !== record.title && part !== record.button && part !== record.more && !part.matches('section[data-hp-level]')) part.hidden = record.mode !== 'all';
      }
      const expanded = record.mode === 'all';
      record.button.setAttribute('aria-expanded', String(expanded));
      record.button.setAttribute('aria-label', (expanded ? 'Collapse ' : 'Expand ') + record.label);
      record.button.title = (expanded ? 'Collapse ' : 'Expand ') + record.label;
      record.more.hidden = expanded;
      record.more.setAttribute('aria-expanded', String(expanded));
      record.node.classList.toggle('hp-folded', !expanded);
      record.node.setAttribute('data-hp-fold-mode', record.mode);
    }
    for (const button of toolbar.querySelectorAll('[data-hp-mode]')) {
      const expected = { overview: 'folded', content: 'children', showall: 'all' }[button.dataset.hpMode];
      button.setAttribute('aria-pressed', String(records.length ? records.every(record => record.mode === expected) : button.dataset.hpMode === lastPreset));
    }
  }

  function subtree(root, mode) {
    const stack = [root];
    while (stack.length) {
      const record = stack.pop();
      record.mode = mode;
      stack.push(...record.children);
    }
  }

  const toolbar = document.createElement('nav');
  let printState;
  toolbar.className = 'hp-toolbar';
  toolbar.setAttribute('aria-label', 'Document outline');
  function globalMode(mode) {
    lastPreset = mode; main.dataset.hpPreset = mode;
    for (const record of records) record.mode = mode === 'overview' ? 'folded' : mode === 'content' ? 'children' : 'all';
    if (mode === 'showall') foldableDetails().forEach(detail => { detail.open = true; });
    refresh();
  }
  for (const [label, mode] of [['Overview', 'overview'], ['Contents', 'content'], ['Show all', 'showall']]) {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = label;
    button.dataset.hpMode = mode;
    button.addEventListener('click', () => {
      const apply = () => {
        if (document.body.classList.contains('hp-plaintext')) applyPlaintext(false);
        globalMode(mode);
      };
      const change = new CustomEvent('hp-before-outline', { cancelable: true, detail: { apply } });
      if (document.dispatchEvent(change)) apply();
    }, events);
    toolbar.append(button);
  }
  dispose(() => {
    restorePrint();
    toolbar.remove();
    for (const record of records) {
      record.node.hidden = false;
      record.node.classList.remove('hp-outline-section');
      record.node.classList.remove('hp-folded');
      for (const part of record.node.children) part.hidden = false;
      if (record.button) record.button.remove();
      if (record.more) record.more.remove();
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
    const indicator = document.createElement('span');
    indicator.className = 'hp-toggle-indicator';
    indicator.textContent = '▸';
    indicator.setAttribute('aria-hidden', 'true');
    button.append(indicator);
    const more = document.createElement('button');
    more.type = 'button';
    more.className = 'hp-show-more';
    more.textContent = '▸ Show more …';
    more.setAttribute('aria-label', 'Expand ' + record.label);
    more.hidden = true;
    const toggle = () => {
      subtree(record, record.mode === 'all' ? 'folded' : 'all');
      if (record.mode === 'all') closeOwnedDrawers(record.node);
      refresh();
    };
    button.addEventListener('click', toggle, events);
    more.addEventListener('click', toggle, events);
    record.button = button;
    record.more = more;
    record.node.classList.add('hp-outline-section');
    record.node.prepend(button);
    record.title.after(more);
    if (i % 250 === 249) await nextFrame(controller.signal);
  }
  if (controller.signal.aborted) return;
  for (const record of records) record.button.disabled = false;
  header.querySelector('#hp-header-row').append(toolbar);
  globalMode(main.getAttribute('data-hp-startup'));
  for (const record of records) {
    const initial = record.node.getAttribute('data-hp-visibility');
    const inherited = record.parent && record.parent.initialCascade;
    if (inherited) record.mode = inherited;
    if (initial) record.mode = initial;
    record.initialCascade = initial ? (initial === 'all' ? 'all' : 'folded') : inherited;
  }
  closeOwnedDrawers(main);
  const headers = main.dataset.hpFoldHeaders;
  for (const record of records) {
    if (headers === 'open' || headers === 'closed') record.mode = headers === 'open' ? 'all' : 'folded';
    const restored = record.node.dataset.hpRestoredFold;
    if (['all', 'children', 'folded'].includes(restored)) record.mode = restored;
    delete record.node.dataset.hpRestoredFold;
  }
  for (const detail of foldableDetails()) {
    const choice = detail.hasAttribute('data-hp-org-drawer') ? main.dataset.hpFoldDrawers : main.dataset.hpFoldDefault;
    if (choice === 'open' || choice === 'closed') detail.open = choice === 'open';
    if (detail.hasAttribute('data-hp-restored-open')) detail.open = detail.dataset.hpRestoredOpen === 'true';
    delete detail.dataset.hpRestoredOpen;
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
    printState = { modes: records.map(record => record.mode), drawers: foldableDetails().map(detail => [detail, detail.open]) };
    globalMode('showall');
  }, events);
  window.addEventListener('afterprint', restorePrint, events);
}

// This state survives reader refreshes, but a new document load starts afresh.
const navigationState = { depth: null, branches: new Map() };

async function enhanceNavigation(controller, dispose) {
  const nav = document.getElementById('hp-toc');
  if (!nav) return;
  const branches = [];
  for (const item of nav.querySelectorAll('li')) {
    const link = item.querySelector(':scope > a');
    const children = item.querySelector(':scope > ul');
    if (!link || !children) continue;
    let level = 1;
    for (let parent = item.parentElement; parent && parent !== nav; parent = parent.parentElement) {
      if (parent.tagName === 'LI') level += 1;
    }
    const button = document.createElement('button');
    button.type = 'button'; button.className = 'hp-nav-toggle'; button.textContent = '▸';
    const row = document.createElement('div'); row.className = 'hp-nav-row';
    item.insertBefore(row, link); row.append(button, link);
    const key = link.getAttribute('href');
    const setOpen = open => {
      children.hidden = !open;
      button.setAttribute('aria-expanded', String(open));
      button.setAttribute('aria-label', (open ? 'Collapse ' : 'Expand ') + link.textContent.trim());
    };
    button.addEventListener('click', () => {
      const open = children.hidden;
      // A reader can act before fonts settle; their choice takes precedence.
      navigationState.depth ??= 3;
      setOpen(open); navigationState.branches.set(key, open);
    }, { signal: controller.signal });
    branches.push({ key, level, children, setOpen });
    dispose(() => { row.before(link); row.remove(); children.hidden = false; });
  }

  const applyDepth = depth => { for (const branch of branches) branch.setOpen(branch.level < depth); };
  if (navigationState.depth === null) {
    applyDepth(3);
    // Embedded fonts affect wrapped labels and therefore the fitting decision.
    if (document.fonts) await document.fonts.ready;
    await nextFrame(controller.signal);
    if (controller.signal.aborted) return;
    if (navigationState.depth === null) {
      navigationState.depth = 3;
      if (nav.getClientRects().length && getComputedStyle(nav).position === 'sticky') {
        while (navigationState.depth > 1 && nav.scrollHeight > nav.clientHeight + 1) {
          navigationState.depth -= 1;
          applyDepth(navigationState.depth);
        }
      }
    }
  } else {
    for (const branch of branches) {
      branch.setOpen(navigationState.branches.get(branch.key) ?? branch.level < navigationState.depth);
    }
  }
  navigationState.branches = new Map(branches.map(branch => [branch.key, !branch.children.hidden]));
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
  const measureHeader = () => document.documentElement.style.setProperty('--hp-bar-height', header.getBoundingClientRect().height + 'px');
  if (typeof ResizeObserver === 'function') {
    const observer = new ResizeObserver(measureHeader); observer.observe(header); dispose(() => observer.disconnect());
  }
  window.addEventListener('resize', measureHeader, { signal: controller.signal }); measureHeader();
  // Outline labels are captured before inline copy controls can affect headings.
  async function enhance() {
    await enhanceOutline(main, header, controller, dispose);
    if (!controller.signal.aborted) enhancePlaintext(header, controller, dispose);
    if (!controller.signal.aborted) await enhanceCode(main, copyValue, controller, dispose);
    if (!controller.signal.aborted) await enhanceNavigation(controller, dispose);
  }
  const ready = enhance().catch(() => {
    if (controller.signal.aborted) return;
    teardown();
    const status = document.createElement('p');
    status.className = 'hp-status';
    status.setAttribute('role', 'status');
    status.textContent = 'Interactive controls unavailable. The full document remains readable.';
    header.after(status);
    window.addEventListener('pagehide', () => { status.remove(); }, { once: true });
  });
  return { teardown, ready };
}

function applyPlaintext(on) {
  const payload = document.getElementById('hp-source-data');
  const view = document.getElementById('hp-source-text');
  const button = document.getElementById('hp-plaintext-toggle');
  if (!payload || !view || !button) throw new Error('Original source is unavailable.');
  if (on) {
    const bytes = Uint8Array.from(atob(payload.content.textContent), ch => ch.charCodeAt(0));
    view.textContent = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
  }
  document.body.classList.toggle('hp-plaintext', on); view.hidden = !on;
  button.setAttribute('aria-pressed', String(on));
  for (const control of document.querySelectorAll('#hp-header [data-hp-mode]')) {
    if (on) {
      control.dataset.hpRenderedPressed ??= control.getAttribute('aria-pressed');
      control.setAttribute('aria-pressed', 'false');
    } else if (control.dataset.hpRenderedPressed !== undefined) {
      control.setAttribute('aria-pressed', control.dataset.hpRenderedPressed);
      delete control.dataset.hpRenderedPressed;
    }
  }
}

function enhancePlaintext(header, controller, dispose) {
  const payload = document.getElementById('hp-source-data');
  const view = document.getElementById('hp-source-text');
  if (!payload || !view) return;
  const button = document.createElement('button'); button.type = 'button'; button.textContent = 'Show plaintext'; button.id = 'hp-plaintext-toggle';
  button.setAttribute('aria-controls', view.id);
  header.querySelector('.hp-toolbar').append(button);
  button.addEventListener('click', async () => {
    const next = !document.body.classList.contains('hp-plaintext');
    const change = new CustomEvent('hp-before-plaintext', { cancelable: true, detail: { apply: () => applyPlaintext(next), on: next } });
    if (document.dispatchEvent(change)) {
      try {
        // Annotation-enabled pages own their guarded refresh above. Other
        // service pages still need the current source on each explicit entry.
        if (next && location.protocol === 'http:') {
          button.disabled = true;
          const html = await annotationRequest(location.href, { html: true });
          if (controller.signal.aborted) return;
          const page = new DOMParser().parseFromString(html, 'text/html');
          const fresh = page.getElementById('hp-source-data');
          if (!fresh || page.querySelector('[data-hp-source]')?.getAttribute('data-hp-source') !== header.querySelector('[data-hp-source]').getAttribute('data-hp-source')) {
            throw new Error('The refreshed document has no matching original source.');
          }
          // Validate before replacing the last readable snapshot.
          new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(Uint8Array.from(atob(fresh.content.textContent), ch => ch.charCodeAt(0)));
          document.getElementById('hp-source-data').replaceWith(document.importNode(fresh, true));
        }
        applyPlaintext(next);
      }
      catch (error) {
        if (controller.signal.aborted) return;
        const status = document.createElement('p'); status.className = 'hp-status'; status.setAttribute('role', 'status');
        status.textContent = 'Original source unavailable: ' + error.message; header.after(status);
        dispose(() => status.remove());
      } finally { button.disabled = false; }
    }
  }, { signal: controller.signal });
  applyPlaintext(document.body.classList.contains('hp-plaintext'));
  dispose(() => button.remove());
}

let previewLifecycle = enhancePreview();
export async function reinitializePreview(replaceRegions) {
  previewLifecycle.teardown();
  if (replaceRegions) replaceRegions();
  previewLifecycle = enhancePreview();
  await previewLifecycle.ready;
}
window.addEventListener('pageshow', event => {
  if (event.persisted) reinitializePreview().catch(() => {
    const status = document.createElement('p');
    status.setAttribute('role', 'status');
    status.textContent = 'Reader controls could not be restored.';
    document.getElementById('hp-header').after(status);
  });
});
