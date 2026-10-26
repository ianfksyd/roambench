'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, 'js/app.js'), 'utf8');

function fixture() {
    const helper = {tagName: 'TEXTAREA'};
    const editor = {tagName: 'TEXTAREA'};
    const reports = [], bytes = [];
    const terminal = {
        ws: {readyState: 1},
        term: {textarea: helper, element: {contains: node => node === helper}, clearSelection() {}}
    };
    const context = vm.createContext({
        WebSocket: {OPEN: 1},
        document: {activeElement: helper, getElementById: () => ({style: {display: 'flex'}})},
        shouldKeepTabForUI: node => node && node.tagName === 'TEXTAREA',
        getActiveTerminal: () => terminal,
        getTerminalSelectionText: () => 'copied review source',
        terminalIdOf: () => 'target',
        shouldHandleViewerEmptyClipboardPaste: () => false,
        reportHandoffCopy: (id, text) => reports.push({action: 'copy', id, text}),
        reportHandoffPaste: (_, text) => reports.push({action: 'paste', text}),
        sendTerminalSequence: (_, text) => bytes.push(text)
    });
    function evaluate(begin, end) {
        const start = source.indexOf(begin), stop = source.indexOf(end, start);
        assert(start >= 0 && stop > start);
        vm.runInContext(source.slice(start, stop), context);
    }
    evaluate('    function shouldKeepClipboardForUI(', '    function handleGlobalTerminalClipboardShortcut(');
    evaluate('    function handleGlobalTerminalClipboardShortcut(', '    window.copyActiveTerminalSelection');
    context.term = {
        hasSelection: () => true,
        attachCustomKeyEventHandler(callback) { context.keyInput = callback; }
    };
    context.t = terminal;
    context.getPageSelectionText = () => '';
    context.pasteToTerminal = () => bytes.push('keyboard paste');
    context.copyTerminalSelection = () => reports.push({action: 'keyboard copy'});
    const keyStart = source.indexOf('        term.attachCustomKeyEventHandler(function(event) {');
    const keyStop = source.indexOf('        term.onData(function(data) {', keyStart);
    assert(keyStart >= 0 && keyStop > keyStart);
    vm.runInContext(source.slice(keyStart, keyStop), context);
    function nativeEvent(type) {
        const event = {
            defaultPrevented: false, propagationStopped: false,
            preventDefault() { this.defaultPrevented = true; },
            stopPropagation() { this.propagationStopped = true; },
            clipboardData: {getData: () => 'copied review source', setData() {}}
        };
        // Use the actual listener registration to choose event order. The
        // vendored xterm target paste handler calls stopPropagation itself.
        const registration = source.match(new RegExp("document\\.addEventListener\\('" + type + "', handleDocument(?:Copy|Paste)(, true)?\\)"));
        assert(registration, 'native clipboard listener must be registered');
        const ours = () => context[type === 'copy' ? 'handleDocumentCopy' : 'handleDocumentPaste'](event);
        const xterm = () => {
            if (type === 'paste') { bytes.push('copied review source'); event.stopPropagation(); }
        };
        if (registration[1]) { ours(); if (!event.propagationStopped) xterm(); }
        else { xterm(); if (!event.propagationStopped) ours(); }
        return event;
    }
    return {context, helper, editor, terminal, reports, bytes, nativeEvent};
}

test('native copy with focus in xterm textarea reports its source', () => {
    const f = fixture();
    const event = f.nativeEvent('copy');
    assert.deepEqual(f.reports, [{action: 'copy', id: 'target', text: 'copied review source'}]);
    assert.equal(event.defaultPrevented, true);
});

test('native paste reaches handoff before xterm stops bubbling and sends bytes once', () => {
    const f = fixture();
    const event = f.nativeEvent('paste');
    assert.deepEqual(f.reports, [{action: 'paste', text: 'copied review source'}]);
    assert.deepEqual(f.bytes, ['copied review source']);
    assert.equal(event.defaultPrevented, true);
});

test('ordinary editor textarea keeps native copy and paste without terminal attribution', () => {
    const f = fixture();
    f.context.document.activeElement = f.editor;
    assert.equal(f.context.shouldHandleTerminalClipboardEvent(), false);
    const event = {preventDefault() { throw new Error('editor event was intercepted'); }, clipboardData: {getData: () => 'editor content'}};
    f.context.handleDocumentCopy(event);
    f.context.handleDocumentPaste(event);
    assert.deepEqual(f.reports, []);
    assert.deepEqual(f.bytes, []);
});

test('xterm clipboard shortcuts do not run again as global shortcuts or native default actions', () => {
    for (const key of ['c', 'v']) {
        const f = fixture();
        const event = {
            type: 'keydown', key, ctrlKey: true,
            defaultPrevented: false, propagationStopped: false,
            preventDefault() { this.defaultPrevented = true; },
            stopPropagation() { this.propagationStopped = true; }
        };
        f.context.keyInput(event);
        if (!event.propagationStopped) f.context.handleGlobalTerminalClipboardShortcut(event);
        if (!event.defaultPrevented) f.nativeEvent(key === 'v' ? 'paste' : 'copy');
        assert.equal(event.defaultPrevented, true);
        assert.equal(event.propagationStopped, true);
        assert.equal(key === 'v' ? f.bytes.length : f.reports.length, 1);
    }
});
