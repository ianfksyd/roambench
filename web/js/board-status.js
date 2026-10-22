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

    function list(value) { return Array.isArray(value) ? value.filter(Boolean) : []; }

    // State precedence resolves overlapping task, dispatch and approval signals.
    function taskStatus(task, snapshot) {
        task = task || {}; snapshot = snapshot || {};
        if (task.state === 'failed') return 'failed';
        if (task.state === 'blocked') return 'blocked';
        if (task.state === 'archived' || task.state === 'completed') return 'completed';
        if (task.pauseReason || task.state === 'paused' || task.state === 'cancelled') return 'paused';
        var pending = list(snapshot.checkpoints).filter(function(c) { return c.taskId === task.id && c.status === 'pending'; });
        var draft = list(snapshot.artifacts).some(function(a) { return a.taskId === task.id && a.kind === 'plan' && String(a.value || '').trim(); });
        var planning = ['planned', 'queued'].includes(task.state) && !draft
            && !['ready', 'claimed', 'rejected'].includes(task.dispatchState);
        if (pending.some(function(c) { return c.kind !== 'dispatch'; })) return 'approval';
        if (planning) return 'planning';
        if (pending.length || ['waiting_human', 'waiting_review'].includes(task.state)
            || task.dispatchState === 'awaiting_approval') return 'approval';
        if (task.state === 'execution_complete' || task.dispatchState === 'done') return 'completed';
        if (task.dispatchState === 'rejected') return 'blocked';
        if (task.state === 'running' || task.dispatchState === 'claimed') return 'running';
        if (task.dispatchState === 'ready' || task.dispatchState === 'released' || ['planned', 'queued'].includes(task.state)) return 'ready';
        return 'unknown';
    }

    function taskStatusRank(status) {
        var index = ['planning', 'approval', 'ready', 'running', 'paused', 'completed', 'blocked', 'failed'].indexOf(status);
        return index < 0 ? 8 : index;
    }

    function taskStatusTone(status) {
        return { planning: 'neutral', approval: 'warning', ready: 'neutral', running: 'info',
            paused: 'warning', completed: 'success', blocked: 'danger', failed: 'danger' }[status] || 'neutral';
    }

    // Legacy tasks lack createdAt/updatedAt. Use timestamped task observations,
    // including the registration checkpoint, without borrowing project activity.
    function taskTimestamp(task, snapshot) {
        task = task || {}; snapshot = snapshot || {};
        var values = [task.updatedAt, task.createdAt];
        list(task.audit).forEach(function(item) { values.push(item.timestamp); });
        ['checkpoints', 'artifacts', 'phaseAttempts', 'toolRuns', 'memories'].forEach(function(key) {
            list(snapshot[key]).filter(function(item) { return task.id && item.taskId === task.id; }).forEach(function(item) {
                values.push(item.updatedAt, item.createdAt, item.requestedAt, item.startedAt, item.completedAt);
                if (key === 'checkpoints') {
                    var decision = list(snapshot.decisions).find(function(d) { return d.id === item.resolvedByDecisionId; });
                    if (decision) values.push(decision.timestamp);
                }
            });
        });
        return values.reduce(function(latest, value) {
            var stamp = value ? Date.parse(value) : NaN;
            return Number.isFinite(stamp) && stamp > latest ? stamp : latest;
        }, 0);
    }

    function sortTasksNewestFirst(tasks, snapshot) {
        return list(tasks).map(function(task) {
            return { task: task, time: taskTimestamp(task, snapshot), rank: taskStatusRank(taskStatus(task, snapshot)) };
        }).sort(function(a, b) {
            return b.time - a.time || a.rank - b.rank || String(a.task.id || '').localeCompare(String(b.task.id || ''));
        }).map(function(row) { return row.task; });
    }

    function boardFontSize(value) {
        var size = Number(value);
        return Number.isFinite(size) && size > 0 ? Math.min(20, Math.max(12, Math.round(size))) : 14;
    }
    function folderBreadcrumbs(home, path) {
        home = String(home || '').replace(/\/+$/, '');
        path = String(path || '').replace(/\/+$/, '');
        if (!home || (path !== home && !path.startsWith(home + '/'))) return [];
        var parts = path.slice(home.length).split('/').filter(Boolean), current = home;
        if (parts.some(function(part) { return part === '..' || part === '.'; })) return [];
        return [{ name: home, path: home }].concat(parts.map(function(name) {
            current += '/' + name; return { name: name, path: current };
        }));
    }
    function projectEntry(input) {
        input = input || {};
        var source = String(input.source || '').trim(), goal = String(input.goal || '').trim();
        if (!source || !goal) return { error: 'required' };
        if (/[\r\n\0]/.test(source) || source.length > 1000 || goal.length > 300) return { error: 'invalidEntry' };
        if (!source.startsWith('/')) return { error: 'absolutePath' };
        var name = source.replace(/\/+$/, '').split('/').pop() || goal;
        return { source: source, goal: goal, name: name };
    }

    return {
        taskStatus: taskStatus, taskStatusRank: taskStatusRank, taskStatusTone: taskStatusTone,
        taskTimestamp: taskTimestamp, sortTasksNewestFirst: sortTasksNewestFirst,
        boardFontSize: boardFontSize, folderBreadcrumbs: folderBreadcrumbs, projectEntry: projectEntry
    };
});
