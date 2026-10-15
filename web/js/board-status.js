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

    return {
        shortSha: shortSha,
        branchLabel: branchLabel,
        dirtyCount: dirtyCount,
        worktreeList: worktreeList,
        cardModel: cardModel
    };
});
