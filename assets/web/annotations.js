// ABOUTME: Presents service annotations and refreshes owned document regions.
// ABOUTME: Keeps the active composer outside refreshed content and guards navigation.
async function annotationRequest(url, options = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(url, { ...options, signal: controller.signal, credentials: 'omit', cache: 'no-store' });
    const limit = options.html ? 50 * 1024 * 1024 : 11 * 1024 * 1024;
    const reader = response.body.getReader(); const chunks = []; let size = 0;
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > limit) { await reader.cancel(); throw new Error('The service response exceeds its limit.'); }
      chunks.push(value);
    }
    const bytes = new Uint8Array(size); let offset = 0;
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
    const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    const type = response.headers.get('Content-Type') || '';
    const data = type.startsWith('application/json') ? JSON.parse(text) : null;
    if (!response.ok) {
      const error = new Error(annotationFailureMessage(data?.error, response.status));
      error.status = response.status; error.code = data?.error; throw error;
    }
    if (options.html && type.startsWith('text/html')) return text;
    if (!options.html && data) return data;
    throw new Error('The service returned an unexpected response type.');
  } finally { clearTimeout(timer); }
}

function annotationFailureMessage(code, status) {
  if (['composer_capacity', 'grant_capacity'].includes(code)) return 'Annotation capacity reached. Close another comment or restart the service, then retry.';
  if (code === 'annotation_upgrade_required') return 'This preview uses an older annotation protocol. Reload the page before annotating.';
  if (code === 'invalid_label') return 'Use 1–64 letters, digits, underscores or hyphens, beginning with a letter.';
  if (code === 'label_conflict') return 'This footnote ID is already in use. Choose another ID.';
  if (code === 'point_unmappable') return 'This insertion point cannot be matched safely to the source. Your draft is retained.';
  if (code === 'render_unavailable') return 'The document preview could not be rendered. Retry the preview update.';
  if (code === 'stale_source' || code === 'stale_body') return 'The source changed. Waiting for a refreshed preview.';
  if (code?.startsWith('target_')) return 'The insertion point is unavailable or no longer unique. Your draft is retained.';
  if (['source_replaced', 'source_changed', 'unsafe_source', 'unsafe_sidecar'].includes(code)) return 'The file identity changed or is unsafe to update. Copy the draft and reopen after checking the file.';
  if (['corrupt_store', 'store_changed', 'foreign_sidecar', 'unsupported_store'].includes(code)) return 'Stored annotation data needs inspection. Earlier records and this draft are retained; no repair is automatic.';
  if (['closed_comment', 'composer_conflict', 'sequence_conflict'].includes(code)) return 'This comment cannot accept another revision. Copy the draft and create a new comment.';
  if (code === 'footnote_conflict') return 'This footnote changed on disk. Your draft is retained; copy it before reopening the current note.';
  if (code === 'invalid_footnote') return 'This text would break out of the native footnote definition. Adjust its markup and retry.';
  if (code === 'read_only_footnote') return 'The original footnote is read-only. Its source has not been changed.';
  if (code === 'operation_conflict') return 'The service found conflicting saved data. Copy the draft before retrying.';
  if (status === 413) return 'The comment, insertion context or annotation store exceeds its storage limit.';
  if (status === 403 || status === 404) return 'The preview no longer has access. Copy the draft and reopen the document through htmlpreview.';
  if (status === 429 || status === 503) return 'Saving is temporarily unavailable. The draft is retained for retry.';
  return 'The annotation request failed. Copy the draft before leaving and retry after checking the service.';
}

function annotationElement(tag, text, className) {
  const node = document.createElement(tag);
  if (text) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function annotationButton(text) {
  const node = annotationElement('button', text); node.type = 'button'; return node;
}

function annotationRevisions(data) {
  return Object.fromEntries(['revision', 'source_revision', 'body_revision'].map(key => [key, data[key]]));
}

function validateAnnotationState(state) {
  const revisions = ['revision', 'source_revision', 'body_revision'];
  if (!state || state.protocol !== 2 || !revisions.every(key => typeof state[key] === 'string' && /^[a-f0-9]{64}$/.test(state[key])) ||
      !Array.isArray(state.comments) || state.comments.length > 10000 || !Array.isArray(state.footnote_labels) || typeof state.writable !== 'boolean' ||
      typeof state.reason !== 'string' || (state.writable && (!['embedded', 'sidecar'].includes(state.storage) || typeof state.write_token !== 'string' || !state.write_token))) {
    throw new Error('Unsupported annotation state.');
  }
    if (!Array.isArray(state.footnotes) || state.footnotes.length > 10000 || !state.footnotes.every(note =>
    note && typeof note.label === 'string' && typeof note.text === 'string' && typeof note.owned === 'boolean' &&
    /^[a-f0-9]{64}$/.test(note.revision || '') && ['embedded', 'sidecar'].includes(note.storage))) throw new Error('Unsupported native footnote state.');
  return state;
}

export class
 AnnotationPanel {
  constructor(data, options = {}) {
    this.data = data; this.request = options.request || annotationRequest;
    this.clock = options.clock || { now: () => performance.now(), setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: id => clearTimeout(id) };
    this.reinitialize = options.reinitialize || reinitializePreview;
    this.controller = new AbortController(); this.events = { signal: this.controller.signal };
    this.disposed = false; this.composer = null; this.stream = null; this.pendingRefresh = null; this.failures = 0;
    this.refreshRequested = false; this.refreshTimer = null; this.typingUntil = 0;
    this.panel = annotationElement('aside', '', 'hp-annotations'); this.panel.id = 'hp-annotations'; this.panel.hidden = true;
    this.panel.setAttribute('aria-label', 'Annotations & Footnotes');
    this.toggle = annotationButton('Annotations'); this.toggle.setAttribute('aria-controls', this.panel.id); this.toggle.setAttribute('aria-expanded', 'false'); this.toggle.setAttribute('aria-pressed', 'false');
    this.connection = annotationElement('p', 'Loading annotations…', 'hp-annotation-status hp-connection-status');
    this.connection.setAttribute('role', 'status'); this.connection.setAttribute('aria-live', 'polite');
    this.list = annotationElement('div'); this.editor = annotationElement('div');
    this.connectionFaults = new Map();
    this.panel.append(annotationElement('h2', 'Annotations & Footnotes'), this.connection, this.editor, this.list);
    document.body.append(this.panel); this.attachToggle();
    this.createRecoveryDialog();
    this.listen();
    if (typeof ResizeObserver === 'function') {
      this.markerObserver = new ResizeObserver(() => this.positionCaret());
      this.markerObserver.observe(document.body);
    }
    this.ready = this.refresh().catch(error => this.showFailure(error));
  }

  attachToggle() { (document.querySelector('#hp-header .hp-toolbar') || document.getElementById('hp-header-row')).append(this.toggle); }

  listen() {
    document.addEventListener('hp-before-outline', event => {
      event.preventDefault();
      this.transition(() => { event.detail.apply(); this.toggle.disabled = false; }).catch(error => this.showComposerFailure(error));
    }, this.events);
    document.addEventListener('hp-before-plaintext', event => {
      event.preventDefault();
      this.transition(async current => {
        if (event.detail.on) {
          await this.refresh();
          if (!current()) return;
          await this.replaceSource(this.state);
        }
        if (!current()) return;
        event.detail.apply(); this.applyMode(false); this.toggle.disabled = event.detail.on;
      }).catch(error => this.showComposerFailure(error));
    }, this.events);
    this.toggle.addEventListener('click', () => this.setMode(!(this.modeRequest?.on ?? !this.panel.hidden)).catch(error => this.showComposerFailure(error)), this.events);
    document.addEventListener('click', event => {
      if (this.panel.hidden || !this.state?.writable || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
      if (window.getSelection()?.isCollapsed === false) return;
      const main = document.getElementById('hp-document');
      if (!(event.target instanceof Element) || !main.contains(event.target) || event.target.closest('a, button, summary, code, pre')) return;
      let range;
      if (document.caretPositionFromPoint) {
        const point = document.caretPositionFromPoint(event.clientX, event.clientY);
        if (point) { range = document.createRange(); range.setStart(point.offsetNode, point.offset); range.collapse(true); }
      } else if (document.caretRangeFromPoint) range = document.caretRangeFromPoint(event.clientX, event.clientY);
      if (range) this.annotateRange(range);
    }, this.events);
    document.addEventListener('keydown', event => this.placeWithKeyboard(event), this.events);
    document.addEventListener('scroll', () => this.positionCaret(), { ...this.events, capture: true, passive: true });
    window.addEventListener('resize', () => this.positionCaret(), this.events);
    window.addEventListener('beforeprint', () => this.placeEndnotes(false), this.events);
    window.addEventListener('afterprint', () => this.placeEndnotes(!this.panel.hidden), this.events);
    document.addEventListener('visibilitychange', () => {
      this.stopEvents();
      if (!document.hidden) { this.connectEvents(); this.queueRefresh(); }
      if (document.hidden && this.composer && !this.dialog.open) this.composer.flush().catch(error => this.showComposerFailure(error));
    }, this.events);
    for (const event of ['focus', 'pageshow']) window.addEventListener(event, () => {
      if (!document.hidden) this.queueRefresh();
      if (this.composer && !this.dialog.open) this.composer.schedule();
    }, this.events);
    window.addEventListener('beforeunload', event => {
      if (this.composer?.dirty) { event.preventDefault(); event.returnValue = ''; }
    }, this.events);
    document.addEventListener('keydown', event => {
      if (this.dialog.open || event.key !== 'Escape' || !this.composer || this.composer.composing) return;
      event.preventDefault(); this.closeComposer().catch(error => this.showComposerFailure(error));
    }, this.events);
    document.addEventListener('click', event => this.guardNavigation(event), { ...this.events, capture: true });
    document.addEventListener('click', event => {
      if (this.dialog.open || event.defaultPrevented || event.button !== 0 || !this.composer || !(event.target instanceof Element) || this.editor.contains(event.target) || event.target.closest('#hp-header')) return;
      // Share the same pending close with a clicked insertion point or link.
      // Failure keeps the editor visible instead of discarding the draft.
      this.closeComposer().catch(error => this.showComposerFailure(error));
    }, { ...this.events, capture: true });
    this.editor.addEventListener('focusout', event => {
      if (this.dialog.open || !event.relatedTarget || this.editor.contains(event.relatedTarget) || !this.composer) return;
      this.closeComposer().catch(error => this.showComposerFailure(error));
    }, this.events);
    for (const name of ['keydown', 'input', 'focusin']) this.editor.addEventListener(name, () => this.holdReader(), this.events);
  }

  annotateRange(range, link = null) {
    const main = document.getElementById('hp-document');
    const target = canonicalMap(main).point(range, this.state.body_revision, this.data.explicit_ids || {}, link);
    if (target) this.transition(() => {
      const result = resolveAnnotationTarget(target, canonicalMap(document.getElementById('hp-document')).text, this.state.body_revision, this.headingSpans());
      if (result.status === 'resolved') this.openComposer(result.target);
      else this.connection.textContent = 'This insertion point changed. Choose a point in the refreshed document.';
    }).catch(error => this.showComposerFailure(error));
  }

  guardNavigation(event) {
    const link = event.target instanceof Element && event.target.closest('a[href]');
    if (link && !this.panel.hidden && document.getElementById('hp-document').contains(link) && !link.matches('.footnote-ref,.footnote-back') && !link.closest('.footnotes,nav') && event.button === 0) {
      event.preventDefault(); event.stopImmediatePropagation();
      if (this.state?.writable && window.getSelection()?.isCollapsed !== false) { const range = document.createRange(); range.selectNodeContents(link); range.collapse(false); this.annotateRange(range, link); }
      return;
    }
    if (!link || !this.composer || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.target === '_blank' || link.hasAttribute('download')) return;
    const url = new URL(link.href);
    if (url.origin === location.origin && url.pathname === location.pathname && url.search === location.search) return;
    event.preventDefault(); event.stopImmediatePropagation();
    this.transition(() => location.assign(url.href)).catch(error => this.showComposerFailure(error));
  }

  async transition(action) {
    const version = this.transitionVersion = (this.transitionVersion || 0) + 1;
    await this.closeComposer();
    const current = () => !this.disposed && version === this.transitionVersion;
    if (current()) await action(current);
  }

  async setMode(on) {
    const request = { on }; this.modeRequest = request;
    try { await this.transition(() => this.applyMode(on)); }
    finally { if (this.modeRequest === request) this.modeRequest = undefined; }
  }

  applyMode(on) {
    if (on && this.connectionPaused) {
      this.connectionPaused = false;
      this.refresh().catch(error => this.showFailure(error));
    }
    this.panel.hidden = !on;
    this.toggle.setAttribute('aria-expanded', String(on));
    this.toggle.setAttribute('aria-pressed', String(on));
    document.body.classList.toggle('hp-annotating', on);
    this.prepareKeyboard(on);
    this.placeEndnotes(on);
  }

  openComposer(target, note = null) {
    if (!this.state?.writable || this.composer || !['point', 'footnote'].includes(target?.type) || document.body.classList.contains('hp-plaintext')) return;
    this.clearCaret();
    this.displayedSequence = 0;
    this.editor.replaceChildren();
    this.markInsertionPoint(target);
    this.textarea = annotationElement('textarea'); this.textarea.id = 'hp-annotation-text'; this.textarea.rows = 6;
    this.textarea.value = note?.text || '';
    const label = annotationElement('label', note ? 'Edit footnote' : 'Your comment'); label.htmlFor = this.textarea.id;
    this.status = annotationElement('p', 'Not saved', 'hp-annotation-status'); this.status.setAttribute('role', 'status'); this.status.setAttribute('aria-live', 'polite');
    this.idInput = annotationElement('input'); this.idInput.id = 'hp-annotation-id'; this.idInput.type = 'text'; this.idInput.maxLength = 64;
    this.idInput.value = note?.label || defaultFootnoteID(this.data.display_name || '', this.state.footnote_labels);
    this.idInput.readOnly = Boolean(note);
    const idLabel = annotationElement('label', 'Footnote ID', 'hp-annotation-id-label'); idLabel.htmlFor = this.idInput.id;
    this.idError = annotationElement('small'); this.idError.id = 'hp-annotation-id-error'; this.idInput.setAttribute('aria-describedby', this.idError.id);
    this.editor.append(label, this.textarea, this.status, idLabel, this.idInput, this.idError);

    this.composer = new AnnotationComposer({ clock: this.clock, revisions: annotationRevisions(this.state), target, note, label: this.idInput.value,
      send: (request, secret) => this.request(this.data.endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-HTMLPreview-Annotation-Token': this.state.write_token, 'X-HTMLPreview-Composer-Token': secret }, body: JSON.stringify(request) }),
      onState: state => {
        const saved = !state.error && !state.dirty && (state.status === 'Autosaved draft' || state.status === 'Saved');
        this.status.textContent = saved ? 'Auto saved' : state.status;
        this.status.classList.toggle('hp-autosaved', saved);
        this.editor.dataset.saveState = state.error ? 'error' : state.dirty ? 'pending' : saved ? 'saved' : 'empty';
        const fieldError = ['invalid_label', 'label_conflict'].includes(state.error?.code);
        this.idInput.setAttribute('aria-invalid', String(fieldError)); this.idError.textContent = fieldError ? state.error.message : '';
        this.updateRecoveryButtons();
        if (state.error && (this.composer?.failed || this.composer?.paused)) this.showComposerFailure(state.error);

        if (this.composer?.sequence && this.displayedSequence !== this.composer.sequence) { this.displayedSequence = this.composer.sequence; this.queueRefresh(); }
        else this.scheduleRefresh();
      },
      onStale: async current => {
        await this.refresh({ reconcile: true });
        if (this.closing && this.state.body_revision !== this.data.body_revision) this.state = await this.replaceSource(this.state, true);
        return this.rebaseTarget(current);
      },
    });
    this.idInput.addEventListener('input', () => this.composer.setLabel(this.idInput.value), this.events);
    this.textarea.addEventListener('input', () => this.composer.input(this.textarea.value), this.events);
    this.textarea.addEventListener('compositionstart', () => this.composer.composition(true), this.events);
    this.textarea.addEventListener('compositionend', () => { this.holdReader(); this.composer.composition(false); }, this.events);
    if (note) this.markInsertionPoint(target);
    this.syncFootnoteCards();
    this.textarea.focus();
  }

  async retryComposer() {
    // Retry a lost acknowledgement with its original operation identity first.
    if (this.composer.failed) await this.composer.retry();
    await this.refresh({ reconcile: true });
    if (this.state.body_revision !== this.data.body_revision) {
      this.state = await this.replaceSource(this.state, true);
      const replacement = this.rebaseTarget(this.composer.target);
      if (replacement) this.composer.rebase(replacement.target, replacement.revisions);
    }
    if (this.composer.paused) throw this.composer.error;
    await this.composer.retry();
  }

  closeComposer() {
    if (this.dialog.open) return Promise.reject(new Error('Resolve the recovery dialog before leaving the editor.'));
    if (!this.composer || this.closing) return this.closing || Promise.resolve();
    this.closing = this.finishComposer();
    return this.closing;
  }

  async finishComposer() {
    this.textarea.readOnly = true; this.idInput.readOnly = true;
    try {
      // External body changes need a current DOM before a paused target can rebase.
      if (this.state.body_revision !== this.data.body_revision) {
        const state = await this.replaceSource(this.state, true);
        this.state = state;
        const replacement = this.rebaseTarget(this.composer.target);
        if (replacement) this.composer.rebase(replacement.target, replacement.revisions);
      }
      await this.composer.close();
      const restoreFocus = this.editor.contains(document.activeElement);
      this.composer.dispose(); this.composer = null; this.editor.replaceChildren(); this.clearInsertionPoint();
      this.syncFootnoteCards();
      if (restoreFocus) this.toggle.focus();
      // A state fetch started during the final save may have deferred rendering.
      if (this.pendingRefresh) await this.pendingRefresh;
      await this.refresh();
    } finally {
      this.closing = null;
      if (this.composer) { this.textarea.readOnly = false; this.idInput.readOnly = this.composer.editing; }
      this.scheduleRefresh();
    }
  }

  showComposerFailure(error) {
    if (this.disposed) return;
    if (!this.composer) { this.showFailure(error); return; }
    this.status.classList.remove('hp-autosaved'); this.editor.dataset.saveState = 'error';
    this.status.textContent = 'Not saved';
    this.composer.cancelTimer();
    this.showRecovery(error, 'save');
  }

  showFailure(error) {
    if (this.disposed) return;
    // A refresh started before typing resumed may fail while the editor is held.
    // Keep the pending update; a failed write has its separate recovery path.
    if (this.deferDocumentRefresh() && ![403, 404].includes(error.status)) { this.queueRefresh(); return; }
    this.connectionFailed('refresh', error);
    if (this.composer && [403, 404].includes(error.status)) this.composer.suspend('The document is unavailable. Copy your draft before leaving.');
  }

  createRecoveryDialog() {
    this.dialog = annotationElement('dialog', '', 'hp-annotation-recovery');
    const title = annotationElement('h2', 'Annotation recovery'); title.id = 'hp-recovery-title';
    this.dialog.setAttribute('aria-labelledby', title.id);
    this.recoveryMessage = annotationElement('p'); this.recoveryMessage.id = 'hp-recovery-message';
    this.dialog.setAttribute('aria-describedby', this.recoveryMessage.id);
    this.recoveryDraft = annotationElement('textarea'); this.recoveryDraft.readOnly = true;
    this.recoveryDraft.setAttribute('aria-label', 'Retained annotation draft'); this.recoveryDraft.rows = 6;
    this.recoveryStatus = annotationElement('p', '', 'hp-annotation-status'); this.recoveryStatus.setAttribute('role', 'status');
    this.copyDraft = annotationButton('Copy'); this.revertDraft = annotationButton('Revert'); this.retryDraft = annotationButton('Try again');
    const actions = annotationElement('div', '', 'hp-annotation-actions'); actions.append(this.copyDraft, this.revertDraft, this.retryDraft);
    const explanation = annotationElement('p', 'Copy keeps this dialog open. Revert discards unsaved browser edits; anything already saved stays in the file.', 'hp-annotation-status');
    this.dialog.append(title, this.recoveryMessage, this.recoveryDraft, explanation, this.recoveryStatus, actions);
    document.body.append(this.dialog);
    this.dialog.addEventListener('cancel', event => event.preventDefault(), this.events);
    this.copyDraft.addEventListener('click', async () => {
      this.recoveryCopied = true;
      try {
        if (!navigator.clipboard) throw new Error('Clipboard unavailable');
        await navigator.clipboard.writeText(this.recoveryDraft.value);
        this.recoveryStatus.textContent = 'Copied.';
      } catch {
        this.recoveryDraft.focus(); this.recoveryDraft.select();
        this.recoveryStatus.textContent = 'Select and copy the retained draft using your keyboard.';
      }
    }, this.events);
    this.revertDraft.addEventListener('click', () => this.revertRecovery(), this.events);
    this.retryDraft.addEventListener('click', () => this.retryRecovery(), this.events);
  }

  showRecovery(error, kind) {
    if (this.disposed || (this.recoveryKind === 'save' && kind === 'connection')) return;
    this.recoveryKind = kind;
    this.recoveryMessage.textContent = error.message;
    this.recoveryDraft.value = this.composer?.text || '';
    this.recoveryDraft.hidden = !this.composer; this.copyDraft.disabled = !this.composer;
    this.revertDraft.textContent = this.composer ? 'Revert' : 'Continue reading';
    if (!this.dialog.open) {
      this.recoveryCopied = false;
      this.recoveryStatus.textContent = '';
      this.dialog.showModal();
    }
    this.updateRecoveryButtons();
  }

  updateRecoveryButtons() {
    const pending = Boolean(this.recoveryBusy || this.composer?.inFlight);
    this.revertDraft.disabled = pending; this.retryDraft.disabled = pending;
    const waiting = 'Waiting for the current save; Copy remains available.';
    if (pending && !this.recoveryBusy) this.recoveryStatus.textContent = waiting;
    else if (this.recoveryStatus.textContent === waiting) this.recoveryStatus.textContent = '';
  }

  closeRecovery() {
    this.recoveryKind = null; this.dialog.close();
    if (this.composer) this.textarea.focus({ preventScroll: true });
  }

  revertRecovery() {
    if (this.composer?.inFlight || this.recoveryBusy) return;
    this.composer?.dispose(); this.composer = null;
    this.editor.replaceChildren(); this.clearInsertionPoint(); this.syncFootnoteCards();
    // Explicitly choosing Revert must remain usable when the service is down.
    this.connectionPaused = true; this.stopEvents();
    this.clearConnectionFault('refresh'); this.closeRecovery();
    this.applyMode(false); this.toggle.focus();
    this.refresh().catch(error => { this.connection.textContent = error.message; });
  }

  async retryRecovery() {
    if (this.composer?.inFlight || this.recoveryBusy) return;
    this.recoveryBusy = true; this.updateRecoveryButtons(); this.recoveryStatus.textContent = 'Trying again…';
    try {
      this.connectionPaused = false; this.stopEvents();
      if (this.composer) await this.retryComposer();
      else {
        await this.refresh({ reconcile: true });
        if (this.state.revision !== this.data.revision) this.state = await this.replaceSource(this.state, true);
      }
      this.closeRecovery();
    } catch (error) {
      this.recoveryMessage.textContent = error.message; this.recoveryStatus.textContent = 'Still unavailable. Your draft is retained.';
    } finally { this.recoveryBusy = false; this.updateRecoveryButtons(); }
  }

  connectionFailed(source, error) {
    if (this.connectionPaused || this.disposed) return;
    this.connection.textContent = source === 'stream' ? 'Reconnecting…' : 'Preview update pending'; this.panel.dataset.connection = 'pending';
    if (this.connectionFaults.has(source)) return;
    const timer = this.clock.setTimeout(() => {
      if (!this.connectionFaults.has(source) || this.disposed) return;
      this.connection.textContent = source === 'stream' ? 'Connection unavailable' : 'Preview update unavailable';
      this.showRecovery(error, 'connection');
    }, 2000);
    this.connectionFaults.set(source, timer);
  }

  clearConnectionFault(source, recovered = true) {
    const timer = this.connectionFaults.get(source);
    if (timer !== undefined) this.clock.clearTimeout(timer);
    this.connectionFaults.delete(source);
    if (this.connectionFaults.size) return;
    delete this.panel.dataset.connection;
    this.connection.textContent = this.state?.writable ? (this.state.storage === 'sidecar' ? 'Read-only source; saving in a sidecar.' : '') : this.state ? 'Reading only: ' + this.state.reason.replaceAll('_', ' ') : 'Loading annotations…';
    if (recovered && this.recoveryKind === 'connection' && !this.recoveryBusy && !this.recoveryCopied) this.closeRecovery();
  }

  stopEvents() { this.stream?.close(); this.stream = null; this.clearConnectionFault('stream', false); }

  connectEvents() {
    if (this.stream || this.disposed || document.hidden || this.connectionPaused) return;
    if (typeof EventSource !== 'function') {
      this.connection.textContent = 'Live updates unavailable; reload to refresh.';
      return;
    }
    const endpoint = new URL(this.data.endpoint, location.href);
    endpoint.searchParams.set('events', '1');
    const stream = new EventSource(endpoint.href);
    this.stream = stream;
    stream.onopen = () => {
      if (this.stream === stream && !this.disposed) this.clearConnectionFault('stream');
    };
    stream.addEventListener('change', () => {
      if (this.stream !== stream || this.disposed) return;
      this.queueRefresh();
    });
    stream.onerror = () => {
      if (this.stream !== stream || this.disposed) return;
      this.connectionFailed('stream', new Error('Live updates are disconnected. Your draft is retained.'));
    };
  }

  holdReader() {
    if (!this.composer) return;
    this.typingUntil = this.clock.now() + 15000;
    this.clearConnectionFault('refresh', false);
    this.scheduleRefresh();
  }

  queueRefresh() {
    this.refreshRequested = true;
    this.scheduleRefresh();
  }

  scheduleRefresh() {
    if (this.refreshTimer !== null) this.clock.clearTimeout(this.refreshTimer);
    this.refreshTimer = null;
    if (!this.refreshRequested || this.disposed || document.hidden || this.pendingRefresh || this.dialog.open || this.closing || this.composer?.dirty || this.composer?.composing) return;
    const wait = this.composer ? Math.max(0, this.typingUntil - this.clock.now()) : 0;
    this.refreshTimer = this.clock.setTimeout(() => {
      this.refreshTimer = null;
      this.refresh().catch(error => this.showFailure(error));
    }, wait);
  }

  async refresh({ reconcile = false } = {}) {
    if (this.disposed) return;
    if (!reconcile && this.deferDocumentRefresh()) { this.queueRefresh(); return; }
    if (this.pendingRefresh) {
      await this.pendingRefresh;
      if (reconcile) return this.refresh({ reconcile: true });
      return;
    }
    this.refreshRequested = false;
    this.pendingRefresh = this.loadState();
    try { await this.pendingRefresh; this.failures = 0; this.lastError = null; }
    catch (error) { this.failures += 1; this.lastError = error; throw error; }
    finally {
      this.pendingRefresh = null;
      this.connectEvents();
      this.scheduleRefresh();
    }

  }

  async loadState() {
    const sequence = this.composer?.sequence;
    let state = validateAnnotationState(await this.request(this.data.endpoint));
    if (sequence !== this.composer?.sequence) { this.queueRefresh(); return; }
    if (this.disposed) return;
    if (state.revision !== this.data.revision && !this.deferDocumentRefresh()) state = await this.replaceSource(state);
    if (this.disposed) return;
    const changed = !this.state || this.state.revision !== state.revision;
    // data describes the displayed document; state describes the latest saved file.
    this.state = state;
    this.clearConnectionFault('refresh');
    if (this.deferDocumentRefresh()) {
      if (state.revision !== this.data.revision) this.queueRefresh();
      return;
    }
    if (changed) this.renderComments(state.comments);
    if (this.composer) {
      if (!state.writable) this.composer.suspend('Saving is unavailable: ' + state.reason.replaceAll('_', ' '));
      else if (!this.composer.inFlight) {
        const replacement = this.rebaseTarget(this.composer.target);
        if (replacement) this.composer.rebase(replacement.target, replacement.revisions);
      }
      if (this.composer.paused) this.clearInsertionPoint();
      else if (!this.insertionRange) this.markInsertionPoint(this.composer.target);
    }
    this.attachToggle();
  }

  rebaseTarget(target) {
    if (target.type === 'footnote') {
      const note = this.state.footnotes.find(item => item.label === this.composer.label && item.storage === target.run);
      if (!note || note.revision !== target.exact) {
        this.composer.suspend('This footnote changed on disk. Copy your draft before reopening the current note.');
        return null;
      }
      return { target, revisions: annotationRevisions(this.state) };
    }

    if (this.data.body_revision !== this.state.body_revision) {
      this.composer?.suspend('The document text changed. Leave the editor to refresh; your draft is retained.');
      return null;
    }
    const result = resolveAnnotationTarget(target, canonicalMap(document.getElementById('hp-document')).text, this.state.body_revision, this.headingSpans());
    if (result.status !== 'resolved') { this.composer?.suspend('The insertion point is ' + result.status + '. Your draft is retained.'); return null; }
    return { target: result.target, revisions: annotationRevisions(this.state) };
  }

  headingSpans() {
    const main = document.getElementById('hp-document'); const spans = {};
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id);
      if (!node || !main.contains(node)) continue;
      const range = document.createRange(); range.selectNodeContents(node);
      const prefix = document.createRange(); prefix.selectNodeContents(main); prefix.setEndBefore(node);
      const before = authoredText(prefix.cloneContents());
      const start = Array.from(before).length + (before ? 1 : 0);
      spans[key] = { start, end: start + Array.from(authoredText(range.cloneContents())).length };
    }
    return spans;
  }

  editFootnote(label) {
    this.transition(async () => {
      await this.refresh();
      const matches = this.state.footnotes.filter(note => note.label === label);
      if (matches.length !== 1) { this.connection.textContent = 'This footnote definition is missing or ambiguous. Inspect its source before editing.'; return; }
      const note = matches[0];
      if (note.storage === 'embedded' && this.state.storage === 'sidecar') { this.connection.textContent = 'This original footnote is read-only.'; return; }
      this.openComposer({ type: 'footnote', exact: note.revision, run: note.storage }, note);
    }).catch(error => this.showComposerFailure(error));
  }

  renderComments() {
    const main = document.getElementById('hp-document');
    const fresh = main.querySelector('section.footnotes');
    if (fresh) {
      if (this.endnotes && this.endnotes !== fresh) this.endnotes.remove();
      this.endnotes = fresh;
      if (this.endnotesSlot) this.endnotesSlot.remove();
      this.endnotesSlot = document.createComment('Native footnotes'); main.append(this.endnotesSlot);
      this.placeEndnotes(!this.panel.hidden);
    } else if (this.endnotesSlot && !this.endnotesSlot.isConnected) {
      this.endnotes?.remove(); this.endnotes = null; this.endnotesSlot = null;
    }
    for (const [body] of this.footnoteMeasures || []) {
      if (!body.isConnected) { this.footnoteObserver?.unobserve(body); this.footnoteMeasures.delete(body); }
    }
    this.syncFootnoteCards();
    for (const item of this.endnotes?.querySelectorAll('li[data-hp-footnote-label]') || []) {
      if (!item.classList.contains('hp-editable-footnote')) {
        this.makeFootnoteCollapsible(item);
        item.classList.add('hp-editable-footnote');
        item.title = 'Click or press Enter to edit this footnote.';
        item.addEventListener('click', event => {
          if (this.panel.hidden || !this.state?.writable || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey || window.getSelection()?.isCollapsed === false) return;
          if (!(event.target instanceof Element) || event.target.closest('a,button,input,textarea,code,pre')) return;
          this.editFootnote(item.dataset.hpFootnoteLabel);
        }, this.events);
        item.addEventListener('keydown', event => {
          if (event.target !== item || this.panel.hidden || !this.state?.writable || !['Enter',' '].includes(event.key)) return;
          event.preventDefault(); this.editFootnote(item.dataset.hpFootnoteLabel);
        }, this.events);
      }
      item.tabIndex = !this.panel.hidden && this.state?.writable ? 0 : -1;
      const note = this.state?.footnotes.find(note => note.label === item.dataset.hpFootnoteLabel);
      if (!note?.author || !note.created_at) continue;
      for (const paragraph of item.querySelectorAll(':scope > p, :scope > .hp-footnote-body > p')) {
        const attribution = ['Created', 'Edited'].map(kind => 'Author: ' + note.author + '; ' + kind + ': ').find(prefix => paragraph.textContent.startsWith(prefix));
        if (!attribution) continue;
        const nativeDate = /^\[\d{4}-\d{2}-\d{2} [A-Za-z]{3}(?: \d{2}:\d{2})?\]$/.test(note.created_at);
        const date = new Date(note.created_at);
        if (!nativeDate && Number.isNaN(date.getTime())) continue;
        const backlinks = [...paragraph.querySelectorAll('a.footnote-back')];
        let display = note.created_at;
        if (!nativeDate) {
          const parts = new Intl.DateTimeFormat('en-IE', { day:'2-digit', month:'2-digit', year:'numeric', weekday:'short', hour:'2-digit', minute:'2-digit', hourCycle:'h23' }).formatToParts(date);
          const part = name => parts.find(item => item.type === name)?.value || '';
          display = `[${part('year')}-${part('month')}-${part('day')} ${part('weekday')} ${part('hour')}:${part('minute')}]`;
        }
        const timestamp = annotationElement('time', display);
        timestamp.dateTime = nativeDate ? note.created_at.slice(1,11) + (note.created_at.length > 16 ? 'T' + note.created_at.slice(16,21) : '') : note.created_at;
        timestamp.title = note.created_at;
        paragraph.replaceChildren(document.createTextNode(attribution), timestamp, document.createTextNode(' '), ...backlinks);
        paragraph.classList.add('hp-annotation-author');
      }
    }
  }

  makeFootnoteCollapsible(item) {
    const body = annotationElement('div', '', 'hp-footnote-body');
    body.append(...item.childNodes); item.append(body);
    const expand = annotationButton('…'); expand.className = 'hp-footnote-expand';
    expand.setAttribute('aria-label', 'Expand footnote'); expand.setAttribute('aria-expanded', 'false');
    const controls = annotationElement('div', '', 'hp-footnote-controls');
    const backlinks = annotationElement('span', '', 'hp-footnote-backlinks');
    for (const original of body.querySelectorAll('a.footnote-back')) {
      const link = annotationElement('a', original.textContent);
      link.setAttribute('href', original.getAttribute('href'));
      link.setAttribute('aria-label', original.getAttribute('aria-label') || 'Back to footnote reference');
      link.className = 'footnote-back'; backlinks.append(link);
    }
    controls.append(expand, backlinks); item.append(controls);
    const measure = () => {
      if (!body.isConnected || this.panel.hidden) return;
      const long = body.scrollHeight > parseFloat(getComputedStyle(body).fontSize) * 7.5 + 1;
      expand.hidden = !long;
    };
    const setExpanded = open => {
      item.classList.toggle('hp-footnote-expanded', open);
      expand.setAttribute('aria-expanded', String(open)); expand.setAttribute('aria-label', open ? 'Collapse footnote' : 'Expand footnote');
      expand.textContent = open ? 'Show less' : '…';
    };
    expand.addEventListener('click', () => setExpanded(expand.getAttribute('aria-expanded') !== 'true'), this.events);
    body.addEventListener('focusin', () => {
      if (!expand.hidden) setExpanded(true);
    }, this.events);
    this.footnoteMeasures ||= new Map();
    this.footnoteMeasures.set(body, measure);
    if (typeof ResizeObserver === 'function') {
      this.footnoteObserver ||= new ResizeObserver(entries => {
        for (const entry of entries) this.footnoteMeasures.get(entry.target)?.();
      });
      this.footnoteObserver.observe(body);
    }
    measure();
  }

  prepareKeyboard(on) {
    this.clearCaret();
    for (const [node, value] of this.keyboardBlocks || []) {
      if (value === null) node.removeAttribute('tabindex'); else node.setAttribute('tabindex', value);
    }
    this.keyboardBlocks = new Map();
    if (!on) return;
    for (const node of document.querySelectorAll('#hp-document :is(p,li,dt,dd,td,th,h1,h2,h3,h4,h5,h6,[role=heading])')) {
      if (node.closest('pre,code,.footnotes,[data-hp-org-drawer]') || node.querySelector('p,li,dt,dd,td,th')) continue;
      this.keyboardBlocks.set(node, node.getAttribute('tabindex')); node.tabIndex = 0;
    }
  }

  clearCaret() { this.caretMarker?.remove(); this.caretMarker = null; this.keyboardCaret = null; }

  markInsertionPoint(target) {
    if (target.type === 'footnote') {
      const note = [...(this.endnotes?.querySelectorAll('li[data-hp-footnote-label]') || [])].find(node => node.dataset.hpFootnoteLabel === this.composer?.label);
      const reference = note && [...document.querySelectorAll('#hp-document a.footnote-ref')].find(link => link.hash === '#' + note.id);
      if (reference) {
        reference.classList.add('hp-editing-footnote'); this.editingReference = reference;
      }
      return;
    }

    const map = canonicalMap(document.getElementById('hp-document'));
    const resolved = resolveAnnotationTarget(target, map.text, this.data.body_revision, this.headingSpans());
    this.insertionRange = resolved.status === 'resolved' ? map.rangeAt(resolved.target.position) : null;
    if (!this.insertionRange) { this.clearInsertionPoint(); return; }
    if (!this.insertionMarker) {
      this.insertionMarker = annotationElement('span', '', 'hp-point-marker hp-insertion-marker');
      this.insertionMarker.setAttribute('aria-hidden', 'true'); document.body.append(this.insertionMarker);
    }
    this.positionCaret();
  }

  clearInsertionPoint() { this.editingReference?.classList.remove('hp-editing-footnote'); this.editingReference = null; this.insertionMarker?.remove(); this.insertionMarker = null; this.insertionRange = null; }

  placeWithKeyboard(event) {
    if (this.panel.hidden || this.composer || !this.state?.writable || event.isComposing) return;
    if (event.key === 'Escape' && this.keyboardCaret) { event.preventDefault(); this.clearCaret(); return; }
    if (!['Enter', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    const block = event.target;
    if (!this.keyboardBlocks?.has(block)) return;
    if (!this.keyboardCaret && event.key !== 'Enter') return;
    event.preventDefault();
    if (this.keyboardCaret?.block !== block) {
      this.clearCaret();
      const nodes = []; const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        const node = walker.currentNode;
        if (node.data.trim() && !node.parentElement.closest('code,pre,button,.todo,.done,.tag,.priority,.cookie')) nodes.push(node);
      }
      if (!nodes.length) return;
      this.keyboardCaret = { block, nodes, index: 0, offset: 0 };
      this.caretMarker = annotationElement('span', '│', 'hp-point-marker'); this.caretMarker.setAttribute('aria-hidden', 'true'); document.body.append(this.caretMarker);
      this.connection.textContent = 'Use Left/Right, Home or End to place the footnote; Enter adds it, Escape cancels.';
    } else if (event.key === 'Enter') {
      const target = canonicalMap(document.getElementById('hp-document')).point(this.keyboardRange(), this.state.body_revision, this.data.explicit_ids || {});
      if (target) this.openComposer(target);
      else this.connection.textContent = 'This point cannot be matched safely. Move the caret to ordinary text.';
      return;
    } else {
      const caret = this.keyboardCaret;
      const length = Array.from(caret.nodes[caret.index].data).length;
      if (event.key === 'Home') { caret.index = 0; caret.offset = 0; }
      if (event.key === 'End') { caret.index = caret.nodes.length - 1; caret.offset = Array.from(caret.nodes[caret.index].data).length; }
      if (event.key === 'ArrowRight') {
        if (caret.offset < length) caret.offset += 1;
        else if (caret.index + 1 < caret.nodes.length) { caret.index += 1; caret.offset = 0; }
      }
      if (event.key === 'ArrowLeft') {
        if (caret.offset > 0) caret.offset -= 1;
        else if (caret.index > 0) { caret.index -= 1; caret.offset = Array.from(caret.nodes[caret.index].data).length; }
      }
    }
    this.positionCaret();
  }

  positionCaret() {
    if (this.insertionMarker && this.insertionRange) {
      const bounds = this.insertionRange.getBoundingClientRect();
      const top = document.getElementById('hp-header').getBoundingClientRect().bottom;
      const reader = document.getElementById('hp-reader').getBoundingClientRect();
      const bottom = document.body.classList.contains('hp-annotating') && reader.height > 0 ? Math.min(window.innerHeight, reader.bottom) : window.innerHeight;
      this.insertionMarker.hidden = bounds.height === 0 || bounds.bottom <= top || bounds.top >= bottom;
      this.insertionMarker.style.left = bounds.left + 'px'; this.insertionMarker.style.top = bounds.top + 'px';
      this.insertionMarker.style.height = bounds.height + 'px';
    }
    if (!this.keyboardCaret || !this.caretMarker) return;
    const bounds = this.keyboardRange().getBoundingClientRect();
    this.caretMarker.style.left = bounds.left + 'px'; this.caretMarker.style.top = bounds.top + 'px';
  }

  keyboardRange() {
    const caret = this.keyboardCaret; const node = caret.nodes[caret.index]; const range = document.createRange();
    range.setStart(node, Array.from(node.data).slice(0, caret.offset).join('').length); range.collapse(true); return range;
  }

  syncFootnoteCards(sidebar = !this.panel.hidden) {
    const active = this.composer;
    const label = active && (active.editing || active.sequence > 0) ? active.savedLabel : null;
    const items = this.endnotes?.querySelectorAll(':scope > ol > li') || [];
    Array.from(items).forEach((item, index) => {
      // Hidden list items must not renumber the other footnotes.
      item.value = index + 1;
      item.hidden = Boolean(sidebar && label && item.dataset.hpFootnoteLabel === label);
    });
  }

  placeEndnotes(sidebar) {
    this.syncFootnoteCards(sidebar);
    for (const item of this.endnotes?.querySelectorAll('.hp-editable-footnote') || []) item.tabIndex = sidebar && this.state?.writable ? 0 : -1;
    if (!this.endnotes || !this.endnotesSlot?.isConnected) return;
    if (sidebar) this.list.append(this.endnotes);
    else this.endnotesSlot.after(this.endnotes);
    this.endnotes.hidden = false;
    for (const measure of this.footnoteMeasures?.values() || []) measure();
  }

  deferDocumentRefresh() {
    return this.dialog.open || Boolean(this.composer && (this.closing || this.composer.dirty || this.composer.composing || this.clock.now() < this.typingUntil));
  }

  async replaceSource(state, leavingEditor = false) {
    for (let attempt = 0; attempt < 3; attempt += 1) {
      const html = await this.request(this.data.page_url, { html: true });
      if (this.disposed) return state;
      const page = new DOMParser().parseFromString(html, 'text/html');
      const metadata = page.getElementById('hp-annotation-data');
      const data = metadata && JSON.parse(metadata.content.textContent);
      if (data && ['revision', 'source_revision', 'body_revision'].every(key => data[key] === state[key])) {
        // Focus may have entered the editor while the rendered page was fetched.
        if (!leavingEditor && this.deferDocumentRefresh()) return state;
        await this.swapRegions(page, data); return state;
      }
      state = validateAnnotationState(await this.request(this.data.endpoint));
    }
    throw new Error('The source is still changing. Waiting for a stable version; your draft is retained.');
  }

  async swapRegions(page, data) {
    if (!page.getElementById('hp-document') || !page.getElementById('hp-header')) throw new Error('The refreshed document is incomplete.');
    const details = new Map([...detailIdentities()].filter(([, node]) => node).map(([key, node]) => [key, node.open]));
    const folds = new Map();
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id);
      if (node) folds.set(key, node.getAttribute('data-hp-fold-mode'));
    }
    const scroll = window.scrollY; const focus = document.activeElement;
    const activity = this.typingUntil;
    const panelScroll = this.panel.scrollTop;
    const reader = document.getElementById('hp-reader'); const readerScroll = reader.scrollTop;
    const editorScroll = this.textarea?.scrollTop;
    const selection = this.editor.contains(focus) && typeof focus.selectionStart === 'number'
      ? [focus.selectionStart, focus.selectionEnd, focus.selectionDirection] : null;
    let anchor;
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id); const top = node?.getBoundingClientRect().top;
      if (top !== undefined && top <= 0) anchor = { key, top };
    }
    this.clearInsertionPoint();
    await this.reinitialize(() => {
      for (const id of ['hp-header', 'hp-frontmatter', 'hp-toc', 'hp-document-title', 'hp-document', 'hp-source-data', 'hp-source-text', 'hp-annotation-data']) {
        const old = document.getElementById(id); const fresh = page.getElementById(id);
        if (fresh && old) old.replaceWith(document.importNode(fresh, true));
        else if (old) old.remove();
        else if (fresh) {
          const next = [...fresh.parentElement.children].slice([...fresh.parentElement.children].indexOf(fresh) + 1).map(node => document.getElementById(node.id)).find(Boolean);
          const inserted = document.importNode(fresh, true);
          if (next) next.before(inserted); else document.getElementById(fresh.parentElement.id)?.append(inserted);
        }
      }
      document.title = page.title;
      document.body.classList.toggle('hp-has-nav', Boolean(page.getElementById('hp-toc')));
      for (const [key, node] of detailIdentities()) {
        if (node && details.has(key)) node.dataset.hpRestoredOpen = String(details.get(key));
      }
      for (const [key, mode] of folds) {
        const node = document.getElementById(data.explicit_ids?.[key]);
        if (node && ['all', 'children', 'folded'].includes(mode)) node.setAttribute('data-hp-restored-fold', mode);
      }
    });
    this.data = data; this.attachToggle(); this.renderComments(); this.prepareKeyboard(!this.panel.hidden);
    if (activity !== this.typingUntil) return;
    if (focus?.isConnected && (document.activeElement === focus || document.activeElement === document.body)) {
      focus.focus({ preventScroll: true });
      if (selection) focus.setSelectionRange(...selection);
    }
    this.panel.scrollTop = panelScroll; reader.scrollTop = readerScroll;
    if (this.textarea?.isConnected && editorScroll !== undefined) this.textarea.scrollTop = editorScroll;
    const node = anchor && document.getElementById(data.explicit_ids?.[anchor.key]);
    const position = node ? window.scrollY + node.getBoundingClientRect().top - anchor.top : scroll;
    window.scrollTo(0, Math.max(0, Math.min(position, document.documentElement.scrollHeight - window.innerHeight)));
  }

  dispose() {
    this.disposed = true; this.stopEvents(); this.controller.abort(); this.composer?.dispose();
    if (this.refreshTimer !== null) this.clock.clearTimeout(this.refreshTimer);
    for (const timer of this.connectionFaults.values()) this.clock.clearTimeout(timer);
    this.connectionFaults.clear(); this.dialog.remove();
    this.footnoteObserver?.disconnect(); this.footnoteMeasures?.clear();
    this.markerObserver?.disconnect(); this.clearInsertionPoint();
    this.prepareKeyboard(false);
    this.placeEndnotes(false); this.endnotesSlot?.remove(); document.body.classList.remove('hp-annotating');
    this.toggle.remove(); this.panel.remove(); this.notice?.remove();
  }
}

const annotationMetadata = document.getElementById('hp-annotation-data');
if (annotationMetadata) {
  previewLifecycle.ready.then(() => new AnnotationPanel(JSON.parse(annotationMetadata.content.textContent))).catch(error => {
    const status = annotationElement('p', 'Annotations unavailable: ' + error.message, 'hp-annotation-status');
    status.setAttribute('role', 'status'); document.getElementById('hp-header').after(status);
  });
}
