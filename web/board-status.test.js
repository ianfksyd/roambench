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

test('lifecycle chain stops at the observed state and keeps the milestone identifier', () => {
    const chain = boardStatus.lifecycleChain({state: 'executing', milestoneId: 'M2'});
    assert.deepEqual(chain.map(s => s.key), ['delegated', 'reconnaissance', 'planning', 'awaiting_confirmation', 'executing']);
    assert.equal(chain.filter(s => s.current).length, 1);
    assert.equal(chain.at(-1).milestoneId, 'M2');
    assert.ok(chain.slice(0, -1).every(s => s.done));
    assert.equal(boardStatus.lifecycleChain({state: 'completed'}).at(-1).key, 'completed');
});

test('blocked lifecycle retains the previous state without claiming it was completed', () => {
    const chain = boardStatus.lifecycleChain({state: 'blocked', previousState: 'executing', milestoneId: 'M1'});
    assert.equal(chain.at(-1).key, 'blocked');
    assert.equal(chain.at(-1).current, true);
    assert.equal(chain.at(-2).done, false);
    assert.equal(chain.at(-2).current, false);
    assert.deepEqual(boardStatus.lifecycleChain(null).map(s => s.key), ['unknown']);
    assert.deepEqual(boardStatus.lifecycleChain({state: 'invented'}).map(s => s.key), ['unknown']);
    assert.deepEqual(boardStatus.lifecycleChain({state: 'blocked'}).map(s => s.key), ['blocked']);
});

test('duration formatting does not turn unknown values into zero or round up elapsed time', () => {
    for (const invalid of [null, undefined, NaN, Infinity, -1, '60000']) assert.equal(boardStatus.formatDuration(invalid), '');
    for (const [ms, text] of [[0,'0m'],[1,'<1m'],[59999,'<1m'],[60000,'1m'],[3599999,'59m'],[3600000,'1h'],[3660000,'1h 1m'],[90000000,'25h']]) {
        assert.equal(boardStatus.formatDuration(ms), text);
    }
});

const localNow = new Date(2026, 0, 2, 14, 32);
test('calendar presets use full local calendar days across year boundaries', () => {
    const today = boardStatus.calendarRange('today', localNow);
    assert.equal(today.start, +new Date(2026, 0, 2));
    assert.equal(today.end, +new Date(2026, 0, 3));
    assert.equal(today.granularity, 'hour');
    const three = boardStatus.calendarRange('3days', localNow);
    assert.equal(three.from, '2025-12-31');
    assert.equal(three.to, '2026-01-02');
    assert.equal(three.days, 3);
    assert.equal(three.granularity, 'day');
    assert.equal(boardStatus.calendarRange('7days', localNow).from, '2025-12-27');
    assert.equal(boardStatus.calendarRange('month', new Date(2024, 1, 10)).to, '2024-02-29');
    assert.equal(boardStatus.calendarRange('month', new Date(2025, 1, 10)).days, 28);
});

test('custom calendar range includes both endpoints, accepts reversed selection and rejects invalid dates', () => {
    const range = boardStatus.calendarRange('custom', localNow, '2026-03-02', '2026-02-28');
    assert.equal(range.from, '2026-02-28');
    assert.equal(range.to, '2026-03-02');
    assert.equal(range.end, +new Date(2026, 2, 3));
    assert.equal(range.days, 3);
    assert.equal(boardStatus.calendarRange('custom', localNow, '2026-01-02', '2026-01-02').granularity, 'hour');
    for (const invalid of ['2026-02-30', '', '2026-13-01', '2026-1-02']) assert.equal(boardStatus.calendarRange('custom', localNow, invalid, '2026-03-01'), null);
    assert.equal(boardStatus.calendarRange('other', localNow), null);
    assert.equal(boardStatus.calendarRange('today', NaN), null);
});

test('calendar and axis agree about day boundaries and never put the last date at the next midnight', () => {
    const days = boardStatus.calendarDays('2026-10');
    assert.equal(days.length, 42);
    assert.equal(days[0].key, '2026-09-27');
    assert.equal(days[0].other, true);
    assert.equal(days.filter(d => !d.other).length, 31);
    assert.deepEqual(boardStatus.calendarDays('invalid'), []);
    const range = boardStatus.calendarRange('3days', localNow);
    const ticks = boardStatus.rangeTicks(range);
    assert.equal(ticks.length, 3);
    assert.equal(ticks[2].label, '1/2');
    assert.ok(Math.abs(ticks[2].position - 200 / 3) < 1e-10);
    const hourly = boardStatus.rangeTicks(boardStatus.calendarRange('today', localNow));
    assert.equal(hourly[0].label, '00:00');
    assert.equal(hourly.at(-1).label, '24:00');
    assert.equal(hourly.at(-1).position, 100);
});

test('local calendar days survive DST without assuming 24 hour durations', () => {
    const {execFileSync} = require('node:child_process');
    const result = JSON.parse(execFileSync(process.execPath, ['-e', `
        const b = require(${JSON.stringify(require.resolve('./js/board-status.js'))});
        const r = b.calendarRange('custom', new Date(2026, 2, 8), '2026-03-08', '2026-03-08');
        console.log(JSON.stringify({hours:(r.end-r.start)/3600000, days:r.days, ticks:b.rangeTicks(r)}));
    `], {env: {...process.env, TZ: 'America/New_York'}, encoding:'utf8'}));
    assert.equal(result.hours, 23);
    assert.equal(result.days, 1);
    assert.equal(result.ticks.at(-1).position, 100);
});

test('wall normalizes one cell per tmux terminal and does not infer runtime status from task state', () => {
    const snapshot = {sessions:[{id:'s1',terminalId:'a',taskId:'t1',runtimeId:'r1'}],
        tasks:[{id:'t1',projectId:'p1',state:'blocked'}], runtimes:[{id:'r1',kind:'hermes'}]};
    const models = boardStatus.sessionModels([
        {id:'a',backend:'tmux',status:'active',name:'build',lastActivity:'2026-01-02T03:00:00Z'},
        {id:'a',backend:'tmux',status:'exited'}, {id:'b',backend:'pty',status:'active'},
        {id:'c',backend:'tmux',status:'idle',projectId:'explicit',taskId:'t1'},
        {id:'d',backend:'tmux',status:'bad'}, null
    ], snapshot);
    assert.equal(models.length, 3);
    assert.equal(models[0].projectId, 'p1');
    assert.equal(models[0].status, 'active');
    assert.equal(models[0].hermesMode, 'managed');
    assert.equal(models[1].projectId, 'explicit');
    assert.equal(models[1].hermesMode, 'unknown');
    assert.equal(models[2].status, 'unknown');
    assert.deepEqual(boardStatus.statusCounts(models), {active:1,idle:1,exited:0,detached:0,unknown:1});
    assert.deepEqual(boardStatus.sessionModels(null, null), []);
});

const at = minute => new Date(Date.UTC(2026, 9, 16, 9, minute)).toISOString();
test('timeline splits execution around timestamped approval waits and keeps parallel attempts', () => {
    const snapshot = {
        phaseAttempts:[{id:'a1',taskId:'t',status:'completed',startedAt:at(0),completedAt:at(60)},
            {id:'a2',taskId:'t',status:'running',startedAt:at(50)},
            {id:'other',taskId:'other',status:'running',startedAt:at(0)}],
        checkpoints:[{id:'c',taskId:'t',requestedAt:at(20),status:'approved',resolvedByDecisionId:'d'},
            {id:'c2',taskId:'t',requestedAt:at(25),status:'approved',resolvedByDecisionId:'d'}],
        decisions:[{id:'d',timestamp:at(40)}]
    };
    const segments = boardStatus.taskSegments('t', snapshot, at(70));
    assert.deepEqual(segments.map(s => [s.kind, (s.start-Date.parse(at(0)))/60000, (s.end-Date.parse(at(0)))/60000]),
        [['execution',0,20],['wait',20,40],['execution',40,60],['execution',50,70]]);
    assert.equal(snapshot.phaseAttempts[0].completedAt, at(60));
});

test('timeline never fabricates intervals from session lifetimes, unresolved historical decisions or failed attempts', () => {
    const snapshot = {sessions:[{taskId:'t',startedAt:at(0)}], phaseAttempts:[
        {taskId:'t',status:'failed',startedAt:at(0)},
        {taskId:'t',status:'completed',startedAt:'bad',completedAt:at(30)},
        {taskId:'t',status:'completed',startedAt:at(40),completedAt:at(30)}
    ], checkpoints:[{taskId:'t',status:'approved',requestedAt:at(0)},
        {id:'pending',taskId:'t',status:'pending',requestedAt:at(30)}]};
    const segments = boardStatus.taskSegments('t', snapshot, at(40));
    assert.equal(segments.length, 1);
    assert.equal(segments[0].kind, 'wait');
    assert.equal(segments[0].end, Date.parse(at(40)));
    assert.deepEqual(boardStatus.taskSegments('t', {}, at(40)), []);
    assert.deepEqual(boardStatus.taskSegments('t', snapshot, 'bad'), []);
});

test('visible segments are clipped to the range without changing recorded intervals', () => {
    const segments = [{kind:'execution',start:0,end:200}, {kind:'wait',start:200,end:400}, {kind:'execution',start:500,end:600}];
    const got = boardStatus.clipSegments(segments, {start:100,end:300});
    assert.deepEqual(got.map(s => [s.left,s.width,s.duration]), [[0,50,100],[50,50,100]]);
    assert.equal(got[0].start, 0);
    assert.equal(segments[1].end, 400);
    assert.deepEqual(boardStatus.clipSegments(segments, {start:400,end:500}), []);
});

test('hover evidence excludes self-report, notes and unverifiable artifact outcomes', () => {
    const snapshot = {artifacts:[
        {taskId:'t',kind:'evidence_note',outcome:'pass',value:'I passed',createdAt:at(60)},
        {taskId:'t',kind:'test_result',outcome:'recorded',value:'Tests planned',createdAt:at(50)},
        {taskId:'t',kind:'test_result',outcome:'pass',value:'suite: 20 passed',createdAt:at(30)},
        {taskId:'t',kind:'review_result',outcome:'fail',value:'2 issues',createdAt:at(40)},
        {taskId:'other',kind:'test_result',outcome:'pass',value:'other suite',createdAt:at(70)}
    ]};
    assert.equal(boardStatus.latestVerification('t',snapshot).value, '2 issues');
    assert.equal(boardStatus.latestVerification('missing',snapshot), null);
    assert.equal(boardStatus.latestVerification('t',null), null);
});
