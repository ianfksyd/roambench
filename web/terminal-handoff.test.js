'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, 'js/app.js'), 'utf8');

// Exercise the handlers shipped in app.js, including native paste and the
// xterm input callback; do not duplicate their draft state machine here.
function fixture() {
    const reports = [], bytes = [];
    const terminal = {ws: {readyState: 1}, term: {}};
    const context = vm.createContext({
        state: {terminals: {target: terminal}, ctrlActive: false},
        WebSocket: {OPEN: 1}, t: terminal,
        term: {onData(callback) { context.input = callback; }},
        fetchJSON: async (url, request) => {
            reports.push({action: url.split('/').pop(), ...JSON.parse(request.body)});
            return url.endsWith('/submit') ? {submitted: true, taskId: 'accepted'} : {pending: true};
        },
        sendTerminalSequence: (_, value) => { bytes.push(value); return true; },
        getActiveTerminal: () => terminal,
        shouldHandleViewerEmptyClipboardPaste: () => false,
        shouldHandleTerminalClipboardEvent: () => true,
        setCtrlActive() {}, showHandoffToast() {},
        CustomEvent: function() {}, document: {dispatchEvent() {}}
    });
    function evaluate(begin, end) {
        const start = source.indexOf(begin), stop = source.indexOf(end, start);
        assert(start >= 0 && stop > start, 'handler boundaries must exist');
        vm.runInContext(source.slice(start, stop), context);
    }
    evaluate('    var handoffReportChain = Promise.resolve();', '    // Lightweight non-blocking hint');
    evaluate('        term.onData(function(data) {', '        term.onResize(function(size) {');
    evaluate('    function handleDocumentPaste(', '    function handleGlobalTerminalClipboardShortcut(');
    return {
        reports, bytes, context,
        paste(text) {
            context.handleDocumentPaste({preventDefault() {}, stopPropagation() {}, clipboardData: {getData: () => text}});
        },
        input: value => context.input(value),
        flush: () => context.handoffReportChain
    };
}

test('pasted LF is a draft; typing review instructions and navigating still permits submission', async () => {
    const f = fixture();
    f.paste('source A\n');
    f.input('Please review this text.');
    f.input('\x1b[D');
    await f.flush();
    assert.deepEqual(f.reports.map(r => r.action), ['paste']);
    f.input('\r');
    await f.flush();
    assert.deepEqual(f.reports.map(r => r.action), ['paste', 'submit']);
    assert.equal(f.reports[0].draftId, f.reports[1].draftId);
    assert.equal(f.reports[1].text, 'source A\n');
});

test('multiple pasted sources share one draft and Enter only submits once', async () => {
    const f = fixture();
    f.paste('source A');
    f.input(' review alongside ');
    f.paste('source C');
    f.input('\r');
    f.input('\r');
    await f.flush();
    assert.deepEqual(f.reports.map(r => r.action), ['paste', 'paste', 'submit']);
    assert(f.reports[0].draftId);
    assert.equal(new Set(f.reports.map(r => r.draftId)).size, 1);
});

test('Ctrl-C and Ctrl-U cancel the server draft before a new paste submits', async () => {
    for (const control of ['\x03', '\x15']) {
        const f = fixture();
        f.paste('abandoned A');
        f.input(control);
        f.input('unrelated input');
        f.input('\r');
        f.paste('fresh C');
        f.input('\r');
        await f.flush();
        assert.deepEqual(f.reports.map(r => r.action), ['paste', 'cancel', 'paste', 'submit']);
        assert.equal(f.reports[0].draftId, f.reports[1].draftId);
        assert.notEqual(f.reports[1].draftId, f.reports[2].draftId);
        assert.equal(f.reports[2].draftId, f.reports[3].draftId);
    }
});

test('on-screen Ctrl input uses the same draft cancellation path', async () => {
    const f = fixture();
    f.paste('abandoned A');
    f.context.state.ctrlActive = true;
    f.input('c');
    f.context.state.ctrlActive = false;
    f.input('\r');
    await f.flush();
    assert.deepEqual(f.reports.map(r => r.action), ['paste', 'cancel']);
    assert.equal(f.bytes[1], '\x03');
});
