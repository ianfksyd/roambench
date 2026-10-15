'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const boardStatus = require('./js/board-status.js');

const fullGit = {
    repoPath: '/home/ian/proj',
    available: true,
    branch: 'main',
    headSha: 'a1b2c3d4e5f6a7b8',
    upstream: 'origin/main',
    ahead: 2,
    behind: 3,
    dirty: true,
    changedCount: 4,
    untrackedCount: 5,
    worktrees: [
        { path: '/home/ian/proj', branch: 'main', headSha: 'a1b2c3d4e5f6', isCurrent: true },
        { path: '/home/ian/proj-wt', branch: 'feat/x', headSha: '9998887', isCurrent: false }
    ]
};

test('shortSha keeps first 7 chars and tolerates missing sha', () => {
    assert.equal(boardStatus.shortSha('a1b2c3d4e5f6'), 'a1b2c3d');
    assert.equal(boardStatus.shortSha(''), '');
    assert.equal(boardStatus.shortSha(null), '');
});

test('branchLabel combines branch and sha', () => {
    assert.equal(boardStatus.branchLabel('main', 'a1b2c3d4e5f6'), 'main (a1b2c3d)');
    assert.equal(boardStatus.branchLabel('main', ''), 'main');
    assert.equal(boardStatus.branchLabel('', 'a1b2c3d4e5f6'), 'a1b2c3d');
    assert.equal(boardStatus.branchLabel('', ''), '');
});

test('dirtyCount sums changed and untracked, defaults to 0', () => {
    assert.equal(boardStatus.dirtyCount({ changedCount: 4, untrackedCount: 5 }), 9);
    assert.equal(boardStatus.dirtyCount({}), 0);
    assert.equal(boardStatus.dirtyCount(null), 0);
});

test('worktreeList tolerates missing/invalid arrays', () => {
    assert.deepEqual(boardStatus.worktreeList({}), []);
    assert.deepEqual(boardStatus.worktreeList({ worktrees: 'nope' }), []);
    const got = boardStatus.worktreeList({ worktrees: [null, { path: '/p', branch: 'b', headSha: 'h', isCurrent: 1 }] });
    assert.deepEqual(got[1], { path: '/p', branch: 'b', headSha: 'h', isCurrent: true });
});

test('cardModel maps full board project to display state', () => {
    const got = boardStatus.cardModel({
        projectId: 'p1',
        name: 'Demo',
        status: 'active',
        currentTask: { taskId: 't1', title: 'Fix bug', state: 'running', currentPhase: 'implementation' },
        lastActivityAt: '2026-10-15T06:00:00Z',
        lastActivitySummary: 'task updated',
        git: fullGit
    });
    assert.equal(got.projectId, 'p1');
    assert.equal(got.currentTask.title, 'Fix bug');
    assert.equal(got.currentTask.state, 'running');
    assert.equal(got.currentTask.currentPhase, 'implementation');
    assert.equal(got.git.available, true);
    assert.equal(got.git.sha7, 'a1b2c3d');
    assert.equal(got.git.branchLabel, 'main (a1b2c3d)');
    assert.equal(got.git.hasUpstream, true);
    assert.equal(got.git.ahead, 2);
    assert.equal(got.git.behind, 3);
    assert.equal(got.git.dirty, true);
    assert.equal(got.git.dirtyCount, 9);
    assert.equal(got.git.worktreeCount, 2);
    assert.equal(got.git.worktrees[0].isCurrent, true);
});

test('cardModel reports no current task and unavailable git without throwing', () => {
    const got = boardStatus.cardModel({
        projectId: 'p2',
        name: 'Bare',
        git: { available: false, error: 'not a git repository' }
    });
    assert.equal(got.currentTask, null);
    assert.equal(got.git.available, false);
    assert.equal(got.git.error, 'not a git repository');
    assert.equal(got.git.hasUpstream, false);
    assert.equal(got.git.worktreeCount, 0);
    assert.equal(got.git.branchLabel, '');
});

test('cardModel survives null/undefined project entries', () => {
    const got = boardStatus.cardModel(null);
    assert.equal(got.projectId, '');
    assert.equal(got.currentTask, null);
    assert.equal(got.git.available, false);
});
