// ABOUTME: Owns one annotation composer's autosave, acknowledgement and close lifecycle.
// ABOUTME: Injects clock and transport boundaries for native-browser regression tests.
export function defaultFootnoteID(author, labels = []) {
  let prefix = author.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  if (!prefix) prefix = 'annotation';
  if (/^[0-9]/.test(prefix)) prefix = 'annotation-' + prefix;
  prefix = prefix.slice(0, 48).replace(/-+$/g, '');
  let highest = 0n;
  for (const label of labels) {
    const normalized = label.toLowerCase();
    if (!normalized.startsWith(prefix + '-')) continue;
    const suffix = normalized.slice(prefix.length + 1);
    if (/^[0-9]+$/.test(suffix) && BigInt(suffix) > highest) highest = BigInt(suffix);
  }
  const result = prefix + '-' + String(highest + 1n).padStart(3, '0');
  if (result.length > 64) throw new Error('The default footnote ID exceeds its limit. Enter a shorter unique ID.');
  return result;
}

export class AnnotationComposer {
  constructor(options) {
    this.send = options.send;
    this.clock = options.clock || { now: () => performance.now(), setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: id => clearTimeout(id) };
    this.jitter = options.jitter || (() => Math.floor(Math.random() * 251));
    this.onState = options.onState || (() => {});
    this.onStale = options.onStale;
    this.revisions = { ...options.revisions };
    this.target = structuredClone(options.target);
    this.annotationID = crypto.randomUUID();
    this.composerID = crypto.randomUUID();
    const secret = crypto.getRandomValues(new Uint8Array(32));
    this.secret = btoa(String.fromCharCode(...secret)).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
    this.text = ''; this.savedText = ''; this.version = 0; this.savedVersion = 0;
    this.sequence = 0; this.savedTarget = this.target;
    this.inFlight = null; this.failed = null; this.error = null;
    this.closed = false; this.disposed = false; this.composing = false; this.paused = false;
    this.dirtySince = null; this.timer = null; this.delay = null;
    this.emit();
  }

  get dirty() { return this.version !== this.savedVersion || Boolean(this.inFlight); }

  emit() {
    if (this.disposed) return;
    let status = this.closed ? 'Saved' : this.dirty ? 'Saving' : this.sequence ? 'Autosaved draft' : 'Not saved';
    if (this.error || this.paused) status = 'Not saved';
    this.onState({ status, error: this.error, dirty: this.dirty, text: this.text, savedText: this.savedText, closed: this.closed, pending: Boolean(this.inFlight) });
  }

  input(text) {
    if (this.closed || this.disposed) return;
    this.text = text; this.version += 1;
    if (text === this.savedText && !this.inFlight && !this.failed) this.savedVersion = this.version;
    if (!this.failed) this.error = null;
    if (this.dirtySince === null && this.dirty) this.dirtySince = this.clock.now();
    this.schedule(); this.emit();
  }

  valid() { return this.text.trim() !== '' && !this.text.includes('\0') && Array.from(this.text).length <= 4000 && new TextEncoder().encode(this.text).length <= 16384; }

  composition(active) {
    this.composing = active;
    if (active) this.cancelTimer();
    else { this.dirtySince = this.clock.now(); this.schedule(); }
  }

  cancelTimer() { if (this.timer !== null) this.clock.clearTimeout(this.timer); this.timer = null; }

  schedule() {
    this.cancelTimer();
    if (this.disposed || this.closed || this.composing || this.paused || this.failed || !this.dirty) return;
    if (!this.valid()) { this.error = new Error(this.savedText ? 'The last autosaved draft remains. Restore it or enter a correction.' : 'Enter a comment of up to 4,000 characters.'); return; }
    const wait = Math.max(0, Math.min(300, 2000 - (this.clock.now() - this.dirtySince)));
    this.timer = this.clock.setTimeout(() => {
      this.timer = null;
      this.flush().catch(error => { this.error = error; this.emit(); });
    }, wait);
  }

  snapshot(kind) {
    return { version: this.version, request: {
      operation_id: crypto.randomUUID(), annotation_id: this.annotationID, composer_id: this.composerID,
      sequence: this.sequence + 1, ...this.revisions, kind,
      target: structuredClone(kind === 'close' ? this.savedTarget : this.target),
      text: kind === 'close' ? this.savedText : this.text,
    } };
  }

  async pause(delay) {
    await new Promise(resolve => { this.delay = { resolve, id: this.clock.setTimeout(() => { this.delay = null; resolve(); }, delay) }; });
    if (this.disposed) throw new Error('Composer was disposed.');
  }

  transient(error) { return !['composer_capacity', 'grant_capacity'].includes(error.code) && (!error.status || error.status === 429 || error.status === 503); }

  async transmit(snapshot) {
    let retries = 0; let rebases = 0;
    for (;;) {
      if (this.disposed) throw new Error('Composer was disposed.');
      try {
        const response = await this.send(snapshot.request, this.secret);
        if (response.annotation_id !== this.annotationID || response.sequence !== snapshot.request.sequence || response.closed !== (snapshot.request.kind === 'close')) {
          const error = new Error('The service returned an inconsistent acknowledgement.'); error.status = 400; throw error;
        }
        return response;
      } catch (error) {
        if (error.code === 'stale_source' && this.onStale && rebases < 2) {
          rebases += 1;
          const replacement = await this.onStale(snapshot.request.target);
          if (!replacement) throw error;
          this.rebase(replacement.target, replacement.revisions);
          const target = snapshot.request.kind === 'close' ? snapshot.request.target : replacement.target;
          snapshot.request = { ...snapshot.request, ...replacement.revisions, target: structuredClone(target), operation_id: crypto.randomUUID() };
          continue;
        }
        if (!this.transient(error) || retries >= 3) throw error;
        await this.pause(1000 * (2 ** retries) + this.jitter()); retries += 1;
      }
    }
  }

  async perform(snapshot) {
    this.error = null;
    const pending = this.transmit(snapshot);
    this.inFlight = pending; this.emit();
    try {
      const receipt = await pending;
      this.savedReceipt = receipt;
      this.sequence = receipt.sequence;
      this.savedText = snapshot.request.text; this.savedTarget = snapshot.request.target;
      this.savedVersion = snapshot.version; this.failed = null;
      for (const key of ['revision', 'source_revision', 'body_revision']) if (receipt[key]) this.revisions[key] = receipt[key];
      if (receipt.closed) this.closed = true;
    } catch (error) {
      this.failed = snapshot; this.error = error; throw error;
    } finally {
      this.inFlight = null;
      this.dirtySince = this.version === this.savedVersion ? null : this.clock.now();
      this.emit();
    }
  }

  async flush() {
    this.cancelTimer();
    while (this.inFlight) { await this.inFlight; await Promise.resolve(); }
    if (this.failed) throw this.error;
    if (this.disposed || this.closed || this.composing || this.paused || this.version === this.savedVersion) return;
    if (!this.valid()) throw this.error || new Error('The draft is empty or exceeds its limit.');
    await this.perform(this.snapshot('draft'));
    if (this.version !== this.savedVersion) await this.flush();
  }

  async retry() {
    if (this.inFlight) return;
    if (this.failed) { const snapshot = this.failed; this.failed = null; await this.perform(snapshot); }
    await this.flush();
  }

  async close() {
    if (this.closed) return;
    if (!this.text && !this.sequence && !this.inFlight) { this.closed = true; this.emit(); return; }
    if (this.paused) throw new Error('The selected passage is unresolved. The draft remains available.');
    await this.flush();
    if (this.composing) throw new Error('Finish composing text before closing.');
    await this.perform(this.snapshot('close'));
  }

  rebase(target, revisions) {
    if (target.type !== this.target.type || target.exact !== this.target.exact) throw new Error('A comment cannot change its target.');
    this.target = structuredClone(target); this.revisions = { ...revisions }; this.paused = false;
    if (this.error?.code === 'stale_source' || this.error?.code?.startsWith('target_')) { this.error = null; this.failed = null; }
    if (!this.inFlight) this.schedule();
  }

  suspend(reason) { this.paused = true; this.error = new Error(reason); this.error.code = 'target_unresolved'; this.cancelTimer(); this.emit(); }
  restoreSaved() { this.input(this.savedText); }

  dispose() {
    this.disposed = true; this.cancelTimer();
    if (this.delay) { this.clock.clearTimeout(this.delay.id); this.delay.resolve(); this.delay = null; }
  }
}
