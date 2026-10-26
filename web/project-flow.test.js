'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const flow = require('./js/board-flow.js');
const status = require('./js/board-status.js');
const source = fs.readFileSync(path.join(__dirname, 'js/project-panel.js'), 'utf8');

function evaluate(context, begin, end) {
    const a = source.indexOf(begin), b = source.indexOf(end, a);
    assert(a >= 0 && b > a);
    vm.runInContext(source.slice(a, b), context);
}

function fixture() {
    const tasks = [
        {id: 'new-review', title: '终端提交目标', projectId: 'epo', state: 'planned', createdAt: '2026-10-26T12:49:00Z'},
        {id: 'original', title: 'Original task <img src=x onerror=bad()>', projectId: 'epo', dispatchState: 'unverified', createdAt: '2026-10-23T05:46:00Z'}
    ];
    const context = vm.createContext({
        window: {RoamBenchBoardFlow: flow},
        projects: () => [{id: 'epo', name: 'EPO'}], tasks: () => tasks,
        findById: (items, id) => items.find(item => item.id === id),
        tr: (_, fallback) => fallback, boardRelative: () => 'today',
        formatTime: value => String(value),
        boardQuickActions: () => [], boardTerminalButton: () => '',
        itemCountText: n => n + ' items', countProjectsText: n => n + ' projects',
        boardAPI: () => status, taskInterval: () => ({start: 1, end: 2}),
        boardRangeWindow: () => ({start: 0, end: 3}), i4Button: () => '', renderBoardGantt: () => '',
        BOARD_STAT_ORDER: ['planning', 'unverified']
    });
    evaluate(context, '    var state = {', '    var TOOL_RUN_REFRESH_INTERVAL_MS');
    context.state.snapshot = {};
    context.state.sourceRefs = [{sourceTaskId: 'original', targetTaskId: 'new-review', relation: 'reviews'}];
    evaluate(context, '    function escapeHTML(', '    function formatTime(');
    evaluate(context, '    function renderBoardGraph(', '    function renderApprovals(');
    evaluate(context, '    function boardStatusClass(', '    function countProjectsText(');
    evaluate(context, '    function renderBoard() {', '    // taskTerminalIds extracts');
    return context;
}

test('default board renders source then review with an SVG connection and meaningful review title', () => {
    const context = fixture();
    assert.equal(context.state.boardTab, 'graph');
    const html = context.renderBoard();
    assert.match(html, /id="board-tab-graph"[^>]*aria-selected="true"/);
    assert.match(html, /data-flow-source="original" data-flow-target="new-review"/);
    assert(html.indexOf('data-flow-node="original"') < html.indexOf('data-flow-node="new-review"'));
    assert.match(html, /Review task/);
    assert.match(html, /st-unverified kind-task/);
    assert(!html.includes('<img'), 'task titles must remain escaped');
    assert(html.includes('&lt;img'));
});

test('flow tab clicks and keyboard navigation actually select graph rather than falling back to list', () => {
    const context = fixture();
    const nodes = ['graph', 'gantt', 'wall'].map(tab => ({
        tab, handlers: {}, getAttribute: () => tab,
        addEventListener(event, callback) { this.handlers[event] = callback; }, focus() {}
    }));
    const panel = {
        querySelectorAll: selector => selector === '[data-board-tab]' ? nodes : [],
        querySelector: selector => nodes.find(node => selector.includes('"' + node.tab + '"'))
    };
    context.render = () => {};
    context.getPanel = () => panel;
    evaluate(context, '    function bindInteractions(panel) {', '    function render() {');
    context.bindInteractions(panel);
    context.state.boardTab = 'wall';
    nodes[0].handlers.click();
    assert.equal(context.state.boardTab, 'graph');
    nodes[1].handlers.keydown({key: 'ArrowLeft', preventDefault() {}});
    assert.equal(context.state.boardTab, 'graph');
});

test('flow layout module is loaded before the project panel', () => {
    const html = fs.readFileSync(path.join(__dirname, 'index.html'), 'utf8');
    assert(html.indexOf('/js/board-flow.js') >= 0);
    assert(html.indexOf('/js/board-flow.js') < html.indexOf('/js/project-panel.js'));
});
