(function(root, factory) {
    'use strict';

    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.RoamBenchBoardStatus = api;
    }
})(typeof window !== 'undefined' ? window : globalThis, function() {
    'use strict';

    // shortSha renders the first 7 characters of a git HEAD sha, or '' when
    // the board probe did not return one.
    function shortSha(sha) {
        var text = String(sha || '').trim();
        return text ? text.slice(0, 7) : '';
    }

    // branchLabel combines branch and short sha into one label:
    // "main (a1b2c3d)" / "main" / "(a1b2c3d)" / ''.
    function branchLabel(branch, sha) {
        var branchText = String(branch || '').trim();
        var shaText = shortSha(sha);
        if (branchText && shaText) {
            return branchText + ' (' + shaText + ')';
        }
        return branchText || shaText;
    }

    // dirtyCount sums modified + untracked entries for the dirty chip.
    function dirtyCount(git) {
        git = git || {};
        return (Number(git.changedCount) || 0) + (Number(git.untrackedCount) || 0);
    }

    // worktreeList normalizes the backend worktree array (defensive: the
    // handler may omit it entirely when the git probe failed).
    function worktreeList(git) {
        git = git || {};
        var items = Array.isArray(git.worktrees) ? git.worktrees : [];
        return items.map(function(item) {
            return {
                path: String(item && item.path || ''),
                branch: String(item && item.branch || ''),
                headSha: String(item && item.headSha || ''),
                isCurrent: Boolean(item && item.isCurrent)
            };
        });
    }

    // cardModel maps one board API project entry to the display state used by
    // the F1 status board card. It never throws: any backend shape is
    // tolerated so one malformed project cannot break the whole board.
    function cardModel(project) {
        project = project || {};
        var git = project.git || {};
        var task = project.currentTask || null;
        var worktrees = worktreeList(git);
        return {
            projectId: String(project.projectId || ''),
            name: String(project.name || ''),
            status: String(project.status || ''),
            currentTask: task
                ? {
                    taskId: String(task.taskId || ''),
                    title: String(task.title || ''),
                    state: String(task.state || ''),
                    currentPhase: String(task.currentPhase || '')
                }
                : null,
            lastActivityAt: String(project.lastActivityAt || ''),
            lastActivitySummary: String(project.lastActivitySummary || ''),
            git: {
                repoPath: String(git.repoPath || ''),
                available: Boolean(git.available),
                error: String(git.error || ''),
                branch: String(git.branch || ''),
                sha7: shortSha(git.headSha),
                branchLabel: branchLabel(git.branch, git.headSha),
                upstream: String(git.upstream || ''),
                hasUpstream: String(git.upstream || '').trim() !== '',
                ahead: Number(git.ahead) || 0,
                behind: Number(git.behind) || 0,
                dirty: Boolean(git.dirty),
                changedCount: Number(git.changedCount) || 0,
                untrackedCount: Number(git.untrackedCount) || 0,
                dirtyCount: dirtyCount(git),
                worktreeCount: worktrees.length,
                worktrees: worktrees
            }
        };
    }

    // I3 models deliberately keep missing observations unknown.
    function list(value) { return Array.isArray(value) ? value.filter(Boolean) : []; }
    function timestamp(value) {
        if (value == null || value === '') return NaN;
        return new Date(value).getTime();
    }
    function lifecycleChain(lifecycle) {
        lifecycle = lifecycle || {};
        var order = ['delegated', 'reconnaissance', 'planning', 'awaiting_confirmation', 'executing', 'awaiting_milestone_acceptance', 'completed'];
        var blocked = lifecycle.state === 'blocked';
        var index = order.indexOf(blocked ? lifecycle.previousState : lifecycle.state);
        var chain = index < 0 ? [] : order.slice(0, index + 1).map(function(key, i) {
            return { key: key, current: !blocked && i === index, done: i < index,
                milestoneId: key === 'executing' ? String(lifecycle.milestoneId || '') : '' };
        });
        if (blocked || index < 0) chain.push({ key: blocked ? 'blocked' : 'unknown', current: true, done: false, milestoneId: '' });
        return chain;
    }
    function formatDuration(ms) {
        if (typeof ms !== 'number' || !Number.isFinite(ms) || ms < 0) return '';
        var minutes = Math.floor(ms / 60000);
        if (!minutes) return ms === 0 ? '0m' : '<1m';
        if (minutes < 60) return minutes + 'm';
        return Math.floor(minutes / 60) + 'h' + (minutes % 60 ? ' ' + minutes % 60 + 'm' : '');
    }
    function dayStart(value) {
        var d = new Date(value);
        return new Date(d.getFullYear(), d.getMonth(), d.getDate());
    }
    function dateKey(value) {
        var d = new Date(value);
        if (!Number.isFinite(d.getTime())) return '';
        return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
    }
    function parseDay(value) {
        var match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(String(value || ''));
        if (!match) return null;
        var d = new Date(+match[1], +match[2] - 1, +match[3]);
        return dateKey(d) === value ? d : null;
    }
    // End is exclusive; local calendar arithmetic also handles DST days.
    function calendarRange(preset, now, from, to) {
        var start = dayStart(now), last = dayStart(now);
        if (!Number.isFinite(start.getTime())) return null;
        if (preset === 'custom') {
            start = parseDay(from); last = parseDay(to);
            if (!start || !last) return null;
            if (start > last) { var swap = start; start = last; last = swap; }
        } else if (preset === '3days' || preset === '7days') {
            start.setDate(start.getDate() - (preset === '3days' ? 2 : 6));
        } else if (preset === 'month') {
            start.setDate(1);
            last = new Date(start.getFullYear(), start.getMonth() + 1, 0);
        } else if (preset !== 'today') return null;
        var end = new Date(last); end.setDate(end.getDate() + 1);
        var days = Math.round((Date.UTC(last.getFullYear(), last.getMonth(), last.getDate()) - Date.UTC(start.getFullYear(), start.getMonth(), start.getDate())) / 86400000) + 1;
        return { start: +start, end: +end, from: dateKey(start), to: dateKey(last), days: days, granularity: days === 1 ? 'hour' : 'day' };
    }
    function calendarDays(month) {
        var first = parseDay(month + '-01');
        if (!first) return [];
        var start = new Date(first); start.setDate(1 - first.getDay());
        return Array.from({ length: 42 }, function(_, index) {
            var d = new Date(start); d.setDate(start.getDate() + index);
            return { key: dateKey(d), day: d.getDate(), other: d.getMonth() !== first.getMonth() };
        });
    }
    function rangeTicks(range) {
        if (!range) return [];
        var ticks = [], d = new Date(range.start);
        if (range.granularity === 'hour') {
            for (var hour = 0; hour <= 24; hour += 3) {
                var time = new Date(d.getFullYear(), d.getMonth(), d.getDate(), hour);
                ticks.push({ position: (+time - range.start) / (range.end - range.start) * 100, label: String(hour).padStart(2, '0') + ':00' });
            }
        } else {
            while (+d < range.end) {
                ticks.push({ position: (+d - range.start) / (range.end - range.start) * 100, label: (d.getMonth() + 1) + '/' + d.getDate() });
                d.setDate(d.getDate() + 1);
            }
        }
        return ticks;
    }
    function sessionModels(terminals, snapshot) {
        snapshot = snapshot || {};
        var seen = new Set();
        return list(terminals).filter(function(t) {
            if (t.backend !== 'tmux' || !t.id || seen.has(t.id)) return false;
            seen.add(t.id); return true;
        }).map(function(t) {
            var session = list(snapshot.sessions).find(function(s) { return s.terminalId === t.id; });
            var taskId = t.taskId || (session && session.taskId) || '';
            var task = list(snapshot.tasks).find(function(item) { return item.id === taskId; });
            var runtime = session && list(snapshot.runtimes).find(function(r) { return r.id === session.runtimeId; });
            var hermes = session && (String(session.agentType).toLowerCase() === 'hermes' || (runtime && String(runtime.kind).toLowerCase() === 'hermes'));
            return { id: t.id, name: t.name || t.id, taskId: taskId, task: task || null,
                projectId: t.projectId || (task && task.projectId) || '', session: session || null,
                status: ['active', 'idle', 'exited', 'detached'].includes(t.status) ? t.status : 'unknown',
                lastActivity: t.lastActivity || '', createdAt: t.createdAt || '',
                hermesMode: hermes ? 'managed' : 'unknown' };
        });
    }
    function statusCounts(items) {
        var counts = { active: 0, idle: 0, exited: 0, detached: 0, unknown: 0 };
        list(items).forEach(function(item) { counts[Object.hasOwn(counts, item.status) ? item.status : 'unknown']++; });
        return counts;
    }
    // Only timestamped phase attempts and checkpoint decisions establish intervals.
    // Session creation/last output are NOT execution start/stop observations.
    function taskSegments(taskId, snapshot, now) {
        snapshot = snapshot || {};
        var endNow = timestamp(now), waits = [], execution = [];
        list(snapshot.checkpoints).filter(function(c) { return c.taskId === taskId; }).forEach(function(c) {
            var decision = list(snapshot.decisions).find(function(d) { return d.id === c.resolvedByDecisionId; });
            var start = timestamp(c.requestedAt), end = c.status === 'pending' ? endNow : timestamp(decision && decision.timestamp);
            if (Number.isFinite(start) && Number.isFinite(end) && end > start) waits.push({ kind: 'wait', start: start, end: end, id: c.id });
        });
        waits.sort(function(a, b) { return a.start - b.start; });
        var merged = [];
        waits.forEach(function(w) {
            var prev = merged[merged.length - 1];
            if (prev && w.start <= prev.end) prev.end = Math.max(prev.end, w.end);
            else merged.push(Object.assign({}, w));
        });
        list(snapshot.phaseAttempts).filter(function(a) { return a.taskId === taskId; }).forEach(function(a) {
            var start = timestamp(a.startedAt), end = a.completedAt ? timestamp(a.completedAt) : a.status === 'running' ? endNow : NaN;
            if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return;
            var parts = [{ kind: 'execution', start: start, end: end, id: a.id, phaseId: a.phaseId, sessionId: a.sessionId }];
            merged.forEach(function(w) {
                parts = parts.flatMap(function(p) {
                    if (w.end <= p.start || w.start >= p.end) return [p];
                    var remaining = [];
                    if (w.start > p.start) remaining.push(Object.assign({}, p, { end: w.start }));
                    if (w.end < p.end) remaining.push(Object.assign({}, p, { start: w.end }));
                    return remaining;
                });
            });
            execution.push.apply(execution, parts);
        });
        return execution.concat(merged).sort(function(a, b) { return a.start - b.start; });
    }
    function clipSegments(segments, range) {
        if (!range || range.end <= range.start) return [];
        return list(segments).flatMap(function(s) {
            var start = Math.max(s.start, range.start), end = Math.min(s.end, range.end);
            if (end <= start) return [];
            return [Object.assign({}, s, { left: (start - range.start) / (range.end - range.start) * 100,
                width: (end - start) / (range.end - range.start) * 100, duration: end - start })];
        });
    }
    function latestVerification(taskId, snapshot) {
        return list(snapshot && snapshot.artifacts).filter(function(a) {
            return a.taskId === taskId && ['test_result', 'review_result', 'completion_check'].includes(a.kind)
                && ['pass', 'fail'].includes(a.outcome) && a.value && Number.isFinite(timestamp(a.createdAt));
        }).sort(function(a, b) { return timestamp(b.createdAt) - timestamp(a.createdAt); })[0] || null;
    }

    return {
        lifecycleChain: lifecycleChain, formatDuration: formatDuration,
        dateKey: dateKey, calendarRange: calendarRange, calendarDays: calendarDays, rangeTicks: rangeTicks,
        sessionModels: sessionModels, statusCounts: statusCounts,
        taskSegments: taskSegments, clipSegments: clipSegments, latestVerification: latestVerification,
        shortSha: shortSha,
        branchLabel: branchLabel,
        dirtyCount: dirtyCount,
        worktreeList: worktreeList,
        cardModel: cardModel
    };
});
