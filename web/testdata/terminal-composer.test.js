'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const composer = require('../js/terminal-composer.js');

const settings = {
    foreground: '#d4d4d4',
    background: '#101010',
    cursor: '#ffffff',
    ansi: {
        black: '#000000',
        red: '#aa0000',
        green: '#00aa00',
        yellow: '#aa5500',
        blue: '#0000aa',
        magenta: '#aa00aa',
        cyan: '#00aaaa',
        white: '#aaaaaa',
        brightBlack: '#555555',
        brightRed: '#ff5555',
        brightGreen: '#55ff55',
        brightYellow: '#ffff55',
        brightBlue: '#5555ff',
        brightMagenta: '#ff55ff',
        brightCyan: '#55ffff',
        brightWhite: '#ffffff'
    }
};

function fakeCell(options) {
    const config = Object.assign({
        chars: '',
        width: 1,
        fgMode: 'default',
        fg: 0,
        bgMode: 'default',
        bg: 0
    }, options || {});
    const flags = [
        'Bold', 'Dim', 'Italic', 'Underline', 'Blink', 'Inverse',
        'Invisible', 'Strikethrough', 'Overline'
    ];
    const cell = {
        getChars: () => config.chars,
        getWidth: () => config.width,
        getFgColor: () => config.fg,
        getBgColor: () => config.bg,
        isFgDefault: () => config.fgMode === 'default',
        isFgRGB: () => config.fgMode === 'rgb',
        isFgPalette: () => config.fgMode === 'palette',
        isBgDefault: () => config.bgMode === 'default',
        isBgRGB: () => config.bgMode === 'rgb',
        isBgPalette: () => config.bgMode === 'palette'
    };

    flags.forEach((flag) => {
        cell['is' + flag] = () => Boolean(config[flag.charAt(0).toLowerCase() + flag.slice(1)]);
    });
    return cell;
}

test('resolves themed, 256-color, and grayscale palette entries', () => {
    assert.equal(composer.paletteColor(1, settings), '#aa0000');
    assert.equal(composer.paletteColor(9, settings), '#ff5555');
    assert.equal(composer.paletteColor(16, settings), '#000000');
    assert.equal(composer.paletteColor(21, settings), '#0000ff');
    assert.equal(composer.paletteColor(231, settings), '#ffffff');
    assert.equal(composer.paletteColor(232, settings), '#080808');
    assert.equal(composer.paletteColor(255, settings), '#eeeeee');
});

test('resolves RGB, bold bright colors, inverse, and text attributes', () => {
    const rgb = composer.terminalCellStyle(fakeCell({
        fgMode: 'rgb',
        fg: 0x123456,
        bgMode: 'palette',
        bg: 4,
        italic: true,
        underline: true,
        strikethrough: true
    }), settings);
    const bold = composer.terminalCellStyle(fakeCell({
        fgMode: 'palette',
        fg: 1,
        bold: true
    }), settings);
    const invertedBold = composer.terminalCellStyle(fakeCell({
        fgMode: 'palette',
        fg: 1,
        bold: true,
        inverse: true
    }), settings);

    assert.equal(rgb.foreground, '#123456');
    assert.equal(rgb.background, '#0000aa');
    assert.equal(rgb.italic, true);
    assert.equal(rgb.underline, true);
    assert.equal(rgb.strikethrough, true);
    assert.equal(bold.foreground, '#ff5555');
    assert.equal(invertedBold.foreground, '#101010');
    assert.equal(invertedBold.background, '#aa0000');
});

test('groups adjacent cells while preserving wide characters and cursor boundaries', () => {
    const cells = [
        fakeCell({ chars: 'A' }),
        fakeCell({ chars: '界', width: 2 }),
        fakeCell({ width: 0 }),
        fakeCell({ chars: 'B' })
    ];
    const line = {
        length: cells.length,
        getCell: (column) => cells[column]
    };
    const runs = composer.buildLineRuns(line, settings, 1);

    assert.deepEqual(runs.map((run) => run.text), ['A', '界', 'B']);
    assert.deepEqual(runs.map((run) => run.cursor), [false, true, false]);
});

test('normalizes multiline input and adds bracketed paste before the final Return', () => {
    assert.equal(composer.normalizeTerminalInput('one\ntwo\r\nthree'), 'one\rtwo\rthree');
    assert.equal(composer.buildSubmissionPayload('hello', false), 'hello\r');
    assert.equal(
        composer.buildSubmissionPayload('one\ntwo', true),
        '\x1b[200~one\rtwo\x1b[201~\r'
    );
    assert.equal(composer.buildSubmissionPayload('', true), '\r');
});

test('defaults composer on for mobile-width viewports only', () => {
    assert.equal(composer.shouldDefaultComposer((query) => ({
        matches: query === '(max-width: 980px)'
    })), true);
    assert.equal(composer.shouldDefaultComposer(() => ({ matches: false })), false);
    assert.equal(composer.shouldDefaultComposer(null), false);
});
