'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const board = require('./js/board-status.js');
const at = minute => new Date(Date.UTC(2026, 9, 17, 9, minute)).toISOString();

test('task states map to the eight labels and existing color tones', () => {
    const cases = [
        [{state:'planned'}, 'planning', 'neutral'],
        [{state:'waiting_review'}, 'approval', 'warning'],
        [{state:'queued',dispatchState:'ready'}, 'ready', 'neutral'],
        [{state:'running'}, 'running', 'info'],
        [{state:'running',pauseReason:'human decision'}, 'paused', 'warning'],
        [{state:'execution_complete'}, 'completed', 'success'],
        [{state:'blocked'}, 'blocked', 'danger'],
        [{state:'failed'}, 'failed', 'danger']
    ];
    cases.forEach(([task,status,tone],index) => {
        assert.equal(board.taskStatus(task), status);
        assert.equal(board.taskStatusTone(status), tone);
        assert.equal(board.taskStatusRank(status), index);
    });
    assert.equal(board.taskStatus(null), 'unknown');
    assert.equal(board.taskStatusTone('unrecognized'), 'neutral');
    assert.equal(board.taskStatusRank('unrecognized'), 8);
});

test('planning, dispatch and approval signals do not mislabel a draftless task as a finished plan', () => {
    const task = {id:'t',state:'queued',dispatchState:'awaiting_approval'};
    const snapshot = {checkpoints:[{taskId:'t',kind:'dispatch',status:'pending'}]};
    assert.equal(board.taskStatus(task,snapshot), 'planning');
    snapshot.artifacts = [{taskId:'other',kind:'plan',value:'Unrelated plan'}];
    assert.equal(board.taskStatus(task,snapshot), 'planning');
    snapshot.artifacts.push({taskId:'t',kind:'plan',value:'Plan ready'});
    assert.equal(board.taskStatus(task,snapshot), 'approval');
    assert.equal(board.taskStatus({...task,dispatchState:'ready'},{}), 'ready');
    assert.equal(board.taskStatus({...task,dispatchState:'claimed'},{}), 'running');
    assert.equal(board.taskStatus({...task,dispatchState:'rejected'},{}), 'blocked');
});

test('human gates, pauses, failures and completion have explicit precedence', () => {
    const snapshot = {checkpoints:[{taskId:'t',kind:'final_acceptance',status:'pending'}]};
    assert.equal(board.taskStatus({id:'t',state:'execution_complete'},snapshot), 'approval');
    assert.equal(board.taskStatus({id:'t',state:'archived'},snapshot), 'completed');
    assert.equal(board.taskStatus({id:'t',state:'running',pauseReason:'stop'},snapshot), 'paused');
    assert.equal(board.taskStatus({id:'t',state:'failed',dispatchState:'claimed'},snapshot), 'failed');
    assert.equal(board.taskStatus({id:'t',state:'blocked',dispatchState:'ready'},snapshot), 'blocked');
    assert.equal(board.taskStatus({state:'cancelled'}), 'paused');
});

test('task timestamps use real observations and ignore invalid and unrelated data', () => {
    const snapshot = {
        checkpoints:[{taskId:'t',requestedAt:at(10),resolvedByDecisionId:'d'}, {taskId:'other',requestedAt:at(55)}],
        decisions:[{id:'d',timestamp:at(20)}],
        artifacts:[{taskId:'t',createdAt:'invalid'}],
        phaseAttempts:[{taskId:'t',startedAt:at(15),completedAt:at(19)}]
    };
    assert.equal(board.taskTimestamp({id:'t',audit:[]},snapshot), Date.parse(at(20)));
    assert.equal(board.taskTimestamp({id:'t',updatedAt:at(25),createdAt:at(0)},snapshot), Date.parse(at(25)));
    assert.equal(board.taskTimestamp({id:'t',audit:[{timestamp:at(30)}]},snapshot), Date.parse(at(30)));
    assert.equal(board.taskTimestamp({updatedAt:'bad',createdAt:at(5)}),Date.parse(at(5)));
    assert.equal(board.taskTimestamp({id:'missing'},snapshot),0);
    assert.equal(board.taskTimestamp(null),0);
});

test('list is globally newest first, with missing dates last and stable state/id ties, without mutation', () => {
    const tasks = [
        {id:'old',state:'running',createdAt:at(0),projectId:'p1'},
        {id:'unknown',state:'queued'},
        {id:'new',state:'archived',updatedAt:at(30),projectId:'p2'},
        {id:'b',state:'running',updatedAt:at(10)},
        {id:'a',state:'running',updatedAt:at(10)},
        {id:'ready',state:'queued',dispatchState:'ready',updatedAt:at(10)}
    ];
    const before = structuredClone(tasks);
    assert.deepEqual(board.sortTasksNewestFirst(tasks).map(t=>t.id), ['new','ready','a','b','old','unknown']);
    assert.deepEqual(tasks,before);
    assert.deepEqual(board.sortTasksNewestFirst(null),[]);
    assert.deepEqual(board.sortTasksNewestFirst([null,...tasks], {checkpoints:[{taskId:'old',requestedAt:at(50)}]}).map(t=>t.id), ['old','new','ready','a','b','unknown']);
});

test('folder registration validates input and returns only the required fields', () => {
    assert.deepEqual(board.projectEntry({source:' /home/user/work/ ',goal:' Build a tool '}), {source:'/home/user/work/',goal:'Build a tool',name:'work'});
    assert.equal(board.projectEntry({source:'relative/path',goal:'x'}).error,'absolutePath');
    assert.equal(board.projectEntry({source:'/home/\nbad',goal:'x'}).error,'invalidEntry');
    assert.equal(board.projectEntry({source:'/home',goal:'x'.repeat(301)}).error,'invalidEntry');
    assert.equal(board.projectEntry(null).error,'required');
    assert.deepEqual(board.folderBreadcrumbs('/home/user/a/b'),[
        {name:'/',path:'/'}, {name:'home',path:'/home'}, {name:'user',path:'/home/user'},
        {name:'a',path:'/home/user/a'}, {name:'b',path:'/home/user/a/b'}
    ]);
    assert.deepEqual(board.folderBreadcrumbs('/'),[{name:'/',path:'/'}]);
    assert.deepEqual(board.folderBreadcrumbs('~/work'),[{name:'~',path:'~'},{name:'work',path:'~/work'}]);
    assert.deepEqual(board.folderBreadcrumbs('relative/path'),[]);
    assert.deepEqual(board.folderBreadcrumbs('/home/user/../root'),[]);
    assert.deepEqual(board.folderBreadcrumbs(''),[]);
});

test('board font preferences clamp to the supported range', () => {
    assert.equal(board.boardFontSize(null),14);
    assert.equal(board.boardFontSize('bad'),14);
    assert.equal(board.boardFontSize(10),12);
    assert.equal(board.boardFontSize(30),20);
    assert.equal(board.boardFontSize(15.6),16);
});
