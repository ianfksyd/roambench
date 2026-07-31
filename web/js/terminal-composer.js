// RoamBench — ANSI-rich terminal buffer renderer and native composer helpers.
(function(root, factory) {
    'use strict';

    const api = factory();

    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.RoamBenchTerminalComposer = api;
    }
}(typeof window !== 'undefined' ? window : null, function() {
    'use strict';

    const ANSI_KEYS = [
        'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
        'brightBlack', 'brightRed', 'brightGreen', 'brightYellow',
        'brightBlue', 'brightMagenta', 'brightCyan', 'brightWhite'
    ];
    const COLOR_CUBE_STEPS = [0, 95, 135, 175, 215, 255];
    const RENDER_OVERSCAN_ROWS = 24;

    function byteHex(value) {
        return Math.max(0, Math.min(255, Number(value) || 0)).toString(16).padStart(2, '0');
    }

    function rgbToHex(red, green, blue) {
        return '#' + byteHex(red) + byteHex(green) + byteHex(blue);
    }

    function packedRGBToHex(value) {
        const packed = Number(value) || 0;
        return rgbToHex((packed >> 16) & 255, (packed >> 8) & 255, packed & 255);
    }

    function paletteColor(index, settings) {
        const paletteIndex = Math.max(0, Math.min(255, Number(index) || 0));
        const ansi = settings && settings.ansi ? settings.ansi : {};

        if (paletteIndex < ANSI_KEYS.length) {
            return ansi[ANSI_KEYS[paletteIndex]] || (settings && settings.foreground) || '#ffffff';
        }

        if (paletteIndex < 232) {
            const cubeIndex = paletteIndex - 16;
            const red = COLOR_CUBE_STEPS[Math.floor(cubeIndex / 36) % 6];
            const green = COLOR_CUBE_STEPS[Math.floor(cubeIndex / 6) % 6];
            const blue = COLOR_CUBE_STEPS[cubeIndex % 6];
            return rgbToHex(red, green, blue);
        }

        const gray = 8 + ((paletteIndex - 232) * 10);
        return rgbToHex(gray, gray, gray);
    }

    function callCellFlag(cell, method) {
        return Boolean(cell && typeof cell[method] === 'function' && cell[method]());
    }

    function callCellValue(cell, method, fallback) {
        if (!cell || typeof cell[method] !== 'function') {
            return fallback;
        }
        return cell[method]();
    }

    function resolveCellColor(cell, prefix, settings) {
        const isDefault = callCellFlag(cell, 'is' + prefix + 'Default');
        const isRGB = callCellFlag(cell, 'is' + prefix + 'RGB');
        const isPalette = callCellFlag(cell, 'is' + prefix + 'Palette');
        let value = callCellValue(cell, 'get' + prefix + 'Color', 0);

        if (isDefault || (!isRGB && !isPalette)) {
            return {
                color: prefix === 'Fg' ? settings.foreground : settings.background,
                defaultColor: true,
                paletteIndex: null
            };
        }
        if (isRGB) {
            return { color: packedRGBToHex(value), defaultColor: false, paletteIndex: null };
        }

        value = Math.max(0, Math.min(255, Number(value) || 0));
        return { color: paletteColor(value, settings), defaultColor: false, paletteIndex: value };
    }

    function terminalCellStyle(cell, settings) {
        const normalizedSettings = settings || {};
        const bold = callCellFlag(cell, 'isBold');
        let foreground = resolveCellColor(cell, 'Fg', normalizedSettings);
        let background = resolveCellColor(cell, 'Bg', normalizedSettings);
        let underlineColor = null;

        if (callCellFlag(cell, 'isInverse')) {
            const swapped = foreground;
            foreground = background;
            background = swapped;
        }

        if (bold && foreground.paletteIndex != null && foreground.paletteIndex < 8) {
            foreground = Object.assign({}, foreground, {
                color: paletteColor(foreground.paletteIndex + 8, normalizedSettings),
                paletteIndex: foreground.paletteIndex + 8
            });
        }

        if (callCellFlag(cell, 'isInvisible')) {
            foreground = Object.assign({}, foreground, { color: background.color });
        }

        if (typeof cell.getUnderlineColor === 'function') {
            if (callCellFlag(cell, 'isUnderlineColorRGB')) {
                underlineColor = packedRGBToHex(cell.getUnderlineColor());
            } else if (callCellFlag(cell, 'isUnderlineColorPalette')) {
                let underlineIndex = Math.max(0, Math.min(255, Number(cell.getUnderlineColor()) || 0));
                if (bold && underlineIndex < 8) {
                    underlineIndex += 8;
                }
                underlineColor = paletteColor(underlineIndex, normalizedSettings);
            }
        }
        if (!underlineColor) {
            underlineColor = foreground.color;
        }

        return {
            foreground: foreground.color,
            background: background.color,
            bold: bold,
            dim: callCellFlag(cell, 'isDim'),
            italic: callCellFlag(cell, 'isItalic'),
            underline: callCellFlag(cell, 'isUnderline'),
            underlineStyle: Number(callCellValue(cell, 'getUnderlineStyle', 1)) || 1,
            underlineColor: underlineColor,
            blink: callCellFlag(cell, 'isBlink'),
            strikethrough: callCellFlag(cell, 'isStrikethrough'),
            overline: callCellFlag(cell, 'isOverline')
        };
    }

    function styleSignature(style, cursor) {
        return [
            style.foreground,
            style.background,
            style.bold ? 1 : 0,
            style.dim ? 1 : 0,
            style.italic ? 1 : 0,
            style.underline ? 1 : 0,
            style.underlineStyle,
            style.underlineColor,
            style.blink ? 1 : 0,
            style.strikethrough ? 1 : 0,
            style.overline ? 1 : 0,
            cursor ? 1 : 0
        ].join('|');
    }

    function buildLineRuns(line, settings, cursorColumn) {
        const runs = [];
        let activeRun = null;

        if (!line || typeof line.length !== 'number' || typeof line.getCell !== 'function') {
            return runs;
        }

        for (let column = 0; column < line.length; column += 1) {
            const cell = line.getCell(column);
            const width = Number(callCellValue(cell, 'getWidth', 1));
            const cursor = column === cursorColumn;
            const style = terminalCellStyle(cell, settings);
            const signature = styleSignature(style, cursor);
            let chars;

            if (width === 0) {
                continue;
            }

            chars = String(callCellValue(cell, 'getChars', '') || ' ');
            if (!activeRun || activeRun.signature !== signature) {
                activeRun = {
                    text: '',
                    style: style,
                    cursor: cursor,
                    signature: signature
                };
                runs.push(activeRun);
            }
            activeRun.text += chars;
        }

        return runs;
    }

    function normalizeTerminalInput(text) {
        return String(text == null ? '' : text).replace(/\r?\n/g, '\r');
    }

    function buildSubmissionPayload(text, bracketedPasteMode) {
        let payload = normalizeTerminalInput(text);

        if (payload && bracketedPasteMode) {
            payload = '\x1b[200~' + payload + '\x1b[201~';
        }
        return payload + '\r';
    }

    function createElement(documentRef, tagName, className) {
        const node = documentRef.createElement(tagName);
        node.className = className;
        return node;
    }

    function RichTerminalRenderer(options) {
        this.term = options.term;
        this.output = options.output;
        this.linesHost = options.linesHost;
        this.topSpacer = options.topSpacer;
        this.bottomSpacer = options.bottomSpacer;
        this.getSettings = options.getSettings;
        this.isActive = options.isActive;
        this.onFollowOutputChange = options.onFollowOutputChange;
        this.window = options.window || window;
        this.document = this.output.ownerDocument;
        this.followOutput = true;
        this.renderFrame = 0;
        this.forceRender = false;
        this.selectionDeferred = false;
        this.showCursor = true;
        this.termDisposables = [];
        this.resizeObserver = null;

        this.handleScroll = this.handleScroll.bind(this);
        this.handleSelectionChange = this.handleSelectionChange.bind(this);
        this.handleBufferChange = this.handleBufferChange.bind(this);
        this.output.addEventListener('scroll', this.handleScroll, { passive: true });
        this.document.addEventListener('selectionchange', this.handleSelectionChange);

        this.bindTerminal(this.term);
        if (typeof this.window.ResizeObserver === 'function') {
            this.resizeObserver = new this.window.ResizeObserver(this.schedule.bind(this, true));
            this.resizeObserver.observe(this.output);
        }
    }

    RichTerminalRenderer.prototype.isOutputSelectionActive = function() {
        const selection = this.window.getSelection ? this.window.getSelection() : null;
        const anchor = selection && selection.anchorNode;
        const focus = selection && selection.focusNode;

        if (!selection || selection.isCollapsed || !anchor || !focus) {
            return false;
        }
        return this.output.contains(anchor) || this.output.contains(focus);
    };

    RichTerminalRenderer.prototype.handleSelectionChange = function() {
        if (this.selectionDeferred && !this.isOutputSelectionActive()) {
            this.selectionDeferred = false;
            this.schedule(true);
        }
    };

    RichTerminalRenderer.prototype.handleBufferChange = function() {
        this.followOutput = true;
        this.schedule(true);
    };

    RichTerminalRenderer.prototype.handleScroll = function() {
        const distance = this.output.scrollHeight - this.output.clientHeight - this.output.scrollTop;
        const lineHeight = this.getLineHeight();
        const wasFollowing = this.followOutput;

        this.followOutput = distance <= (lineHeight * 2);
        if (wasFollowing !== this.followOutput && typeof this.onFollowOutputChange === 'function') {
            this.onFollowOutputChange(this.followOutput);
        }
        this.schedule(false);
    };

    RichTerminalRenderer.prototype.disposeTerminalBindings = function() {
        this.termDisposables.forEach(function(disposable) {
            if (disposable && typeof disposable.dispose === 'function') {
                disposable.dispose();
            }
        });
        this.termDisposables = [];
    };

    RichTerminalRenderer.prototype.bindTerminal = function(term) {
        this.disposeTerminalBindings();
        this.term = term;

        if (this.term && typeof this.term.onWriteParsed === 'function') {
            this.termDisposables.push(this.term.onWriteParsed(this.schedule.bind(this, false)));
        }
        if (this.term && typeof this.term.onResize === 'function') {
            this.termDisposables.push(this.term.onResize(this.schedule.bind(this, true)));
        }
        if (this.term && this.term.buffer && typeof this.term.buffer.onBufferChange === 'function') {
            this.termDisposables.push(this.term.buffer.onBufferChange(this.handleBufferChange));
        }
    };

    RichTerminalRenderer.prototype.setTerminal = function(term, options) {
        const settings = options || {};

        if (!term) {
            return;
        }
        this.bindTerminal(term);
        this.showCursor = settings.showCursor !== false;
        this.schedule(false);
    };

    RichTerminalRenderer.prototype.getLineHeight = function() {
        const settings = this.getSettings() || {};
        return Math.max(14, (Number(settings.fontSize) || 14) * 1.35);
    };

    RichTerminalRenderer.prototype.applyTheme = function() {
        const settings = this.getSettings() || {};

        this.output.style.fontFamily = settings.fontFamily || 'monospace';
        this.output.style.fontSize = (Number(settings.fontSize) || 14) + 'px';
        this.output.style.lineHeight = '1.35';
        this.output.style.color = settings.foreground || '#ffffff';
        this.output.style.backgroundColor = settings.background || '#000000';
        this.output.style.setProperty('--terminal-composer-cursor', settings.cursor || settings.foreground || '#ffffff');
        this.output.style.setProperty('--terminal-composer-cursor-accent', settings.background || '#000000');
    };

    RichTerminalRenderer.prototype.schedule = function(force) {
        const renderer = this;

        if (force) {
            this.forceRender = true;
        }
        if (!this.isActive() || this.renderFrame) {
            return;
        }

        this.renderFrame = this.window.requestAnimationFrame(function() {
            renderer.renderFrame = 0;
            renderer.render(renderer.forceRender);
            renderer.forceRender = false;
        });
    };

    RichTerminalRenderer.prototype.setActive = function(active) {
        if (!active) {
            return;
        }
        this.followOutput = true;
        this.applyTheme();
        this.schedule(true);
    };

    RichTerminalRenderer.prototype.refreshTheme = function() {
        this.applyTheme();
        this.schedule(true);
    };

    RichTerminalRenderer.prototype.appendRun = function(lineNode, run, settings) {
        const span = createElement(this.document, 'span', 'terminal-composer-run');
        const decorations = [];

        span.textContent = run.text;
        span.style.color = run.style.foreground;
        span.style.backgroundColor = run.style.background;
        if (run.style.bold) {
            span.style.fontWeight = 'bold';
        }
        if (run.style.dim) {
            span.style.opacity = '0.55';
        }
        if (run.style.italic) {
            span.style.fontStyle = 'italic';
        }
        if (run.style.underline) {
            decorations.push('underline');
            span.style.textDecorationColor = run.style.underlineColor;
            span.style.textDecorationStyle = ({
                1: 'solid',
                2: 'double',
                3: 'wavy',
                4: 'dotted',
                5: 'dashed'
            })[run.style.underlineStyle] || 'solid';
        }
        if (run.style.strikethrough) {
            decorations.push('line-through');
        }
        if (run.style.overline) {
            decorations.push('overline');
        }
        if (decorations.length) {
            span.style.textDecorationLine = decorations.join(' ');
        }
        if (run.style.blink) {
            span.classList.add('terminal-composer-run-blink');
        }
        if (run.cursor) {
            span.classList.add('terminal-composer-run-cursor');
            if (settings.cursorBlink) {
                span.classList.add('terminal-composer-run-cursor-blink');
            }
        }
        lineNode.appendChild(span);
    };

    RichTerminalRenderer.prototype.render = function(force) {
        const buffer = this.term && this.term.buffer ? this.term.buffer.active : null;
        const settings = this.getSettings() || {};
        const lineHeight = this.getLineHeight();
        const totalLines = buffer && typeof buffer.length === 'number' ? buffer.length : 0;
        const viewportHeight = Math.max(this.output.clientHeight || lineHeight, lineHeight);
        const maximumScrollTop = Math.max(0, (totalLines * lineHeight) - viewportHeight);
        const projectedScrollTop = this.followOutput
            ? maximumScrollTop
            : Math.max(0, Math.min(this.output.scrollTop, maximumScrollTop));
        const firstVisible = Math.max(0, Math.floor(projectedScrollTop / lineHeight));
        const start = Math.max(0, firstVisible - RENDER_OVERSCAN_ROWS);
        const end = Math.min(totalLines, Math.ceil((projectedScrollTop + viewportHeight) / lineHeight) + RENDER_OVERSCAN_ROWS);
        const cursorRow = this.showCursor && buffer ? (Number(buffer.baseY) || 0) + (Number(buffer.cursorY) || 0) : -1;
        const cursorColumn = this.showCursor && buffer ? Number(buffer.cursorX) || 0 : -1;
        const fragment = this.document.createDocumentFragment();

        if (!this.isActive()) {
            return;
        }
        if (!force && this.isOutputSelectionActive()) {
            this.selectionDeferred = true;
            return;
        }

        this.applyTheme();
        this.topSpacer.style.height = (start * lineHeight) + 'px';
        this.bottomSpacer.style.height = (Math.max(0, totalLines - end) * lineHeight) + 'px';

        for (let row = start; row < end; row += 1) {
            const line = buffer.getLine(row);
            const lineNode = createElement(this.document, 'div', 'terminal-composer-line');
            const runs = buildLineRuns(line, settings, row === cursorRow ? cursorColumn : -1);

            lineNode.style.height = lineHeight + 'px';
            lineNode.style.lineHeight = lineHeight + 'px';
            if (runs.length) {
                runs.forEach(function(run) {
                    this.appendRun(lineNode, run, settings);
                }, this);
            } else {
                lineNode.appendChild(this.document.createTextNode(' '));
            }
            fragment.appendChild(lineNode);
        }

        this.linesHost.textContent = '';
        this.linesHost.appendChild(fragment);

        if (this.followOutput) {
            this.output.scrollTop = Math.max(0, this.output.scrollHeight - this.output.clientHeight);
        }
    };

    RichTerminalRenderer.prototype.dispose = function() {
        if (this.renderFrame) {
            this.window.cancelAnimationFrame(this.renderFrame);
            this.renderFrame = 0;
        }
        this.output.removeEventListener('scroll', this.handleScroll);
        this.document.removeEventListener('selectionchange', this.handleSelectionChange);
        this.disposeTerminalBindings();
        if (this.resizeObserver) {
            this.resizeObserver.disconnect();
            this.resizeObserver = null;
        }
    };

    function createRenderer(options) {
        return new RichTerminalRenderer(options);
    }

    return {
        ANSI_KEYS: ANSI_KEYS,
        paletteColor: paletteColor,
        packedRGBToHex: packedRGBToHex,
        terminalCellStyle: terminalCellStyle,
        buildLineRuns: buildLineRuns,
        normalizeTerminalInput: normalizeTerminalInput,
        buildSubmissionPayload: buildSubmissionPayload,
        createRenderer: createRenderer
    };
}));
