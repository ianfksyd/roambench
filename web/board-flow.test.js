'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const flow = require('./js/board-flow.js');

const task = (id, extra) => Object.assign({id, projectId: 'epo', title: id}, extra);
const review = (source, target) => ({sourceTaskId: source, targetTaskId: target, relation: 'reviews'});

test('a newer review follows its source even when the board supplies newest first', () => {
    const graph = flow.layout([
        task('review', {createdAt: '2026-10-26T12:49:00Z'}),
        task('original', {createdAt: '2026-10-23T05:46:00Z'})
    ], [review('original', 'review')]);
    assert.deepEqual(graph.nodes.map(n => n.id), ['original', 'review']);
    assert(graph.nodes[1].x > graph.nodes[0].x + graph.nodes[0].width);
    assert.equal(graph.nodes[1].y, graph.nodes[0].y);
    assert.equal(graph.nodes[1].kind, 'review');
    assert.equal(graph.edges[0].kind, 'review');
    assert.deepEqual(graph.nodes[1].task.dependsOn, undefined, 'review provenance does not become an execution dependency');
});

test('branches occupy separate rows and synthesis follows every source', () => {
    const graph = flow.layout([
        task('root'), task('a', {dependsOn: ['root']}),
        task('b', {dependsOn: ['root']}), task('joined', {dependsOn: ['a', 'b']})
    ], []);
    const nodes = new Map(graph.nodes.map(n => [n.id, n]));
    assert.equal(nodes.get('a').level, nodes.get('b').level);
    assert(Math.abs(nodes.get('a').y - nodes.get('b').y) >= nodes.get('a').height);
    assert.equal(nodes.get('a').kind, 'branch');
    assert.equal(nodes.get('joined').kind, 'synthesis');
    for (const edge of graph.edges) assert(nodes.get(edge.target).x > nodes.get(edge.source).x);
});

test('duplicate relations merge without losing dependency semantics; missing tasks do not create nodes', () => {
    const graph = flow.layout([task('a'), task('b', {dependsOn: ['a', 'missing']})], [review('a', 'b'), review('a', 'b'), review('missing', 'b')]);
    assert.equal(graph.edges.length, 1);
    assert.deepEqual(graph.edges[0].relations, ['depends_on', 'reviews']);
    assert.equal(graph.nodes.length, 2);
});

test('layout is stable across board ordering and never mutates task data', () => {
    const input = [task('a'), task('b'), task('c', {dependsOn: ['a']})];
    const before = JSON.stringify(input);
    assert.deepEqual(flow.layout(input, []), flow.layout(input.slice().reverse(), []));
    assert.equal(JSON.stringify(input), before);
});

test('mutual references remain visible without recursive layering or overlapping nodes', () => {
    const graph = flow.layout([task('a'), task('b'), task('next', {dependsOn: ['b']})], [review('a', 'b'), review('b', 'a')]);
    assert.equal(graph.nodes.filter(n => n.cyclic).length, 2);
    assert.equal(graph.nodes.find(n => n.id === 'next').level, 1);
    assert.equal(graph.edges.length, 3);
    graph.edges.forEach(edge => assert(!/NaN|undefined/.test(edge.path)));
    assert.notEqual(graph.nodes[0].y, graph.nodes[1].y);
});

test('project lanes do not overlap and cross-project provenance keeps its connection', () => {
    const graph = flow.layout([task('a'), task('b', {projectId: 'second'}), task('c')], [review('a', 'b')]);
    assert.equal(graph.lanes.length, 2);
    assert(graph.lanes[1].y >= graph.lanes[0].y + graph.lanes[0].height);
    assert.equal(graph.edges.length, 1);
    assert(graph.nodes.find(n => n.id === 'b').x > graph.nodes.find(n => n.id === 'a').x);
});

test('empty graphs and filtered-out references do not invent relationships', () => {
    assert.deepEqual(flow.layout([], []), {nodes: [], edges: [], lanes: [], width: 0, height: 0});
    assert.equal(flow.layout([task('only')], [review('outside', 'only')]).edges.length, 0);
    assert.equal(flow.layout([task('only')], [review('outside', 'only')]).nodes[0].kind, 'review');
});
