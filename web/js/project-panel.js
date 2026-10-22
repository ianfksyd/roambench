(function() {
    'use strict';

    var state = {
        authenticated: false,
        snapshot: null,
        error: '',

        selectedTaskId: '',
        currentView: 'board',
        updatingTaskId: '',

        phaseUpdatingTaskId: '',
        toolRunRefreshTimer: null,
        boardLoading: false,
        boardError: '',

        boardEpoch: 0,
        boardProjectFilter: '',
        boardTab: 'wall',
        boardRange: 'all',

        boardTimer: null
    };

    var TOOL_RUN_REFRESH_INTERVAL_MS = 1200;
    var TOOL_RUN_REFRESH_ATTEMPTS = 130;
    var BOARD_REFRESH_INTERVAL_MS = 30000;

    function app() {
        return window.RoamBenchApp || null;
    }

    function tr(key, fallback, vars) {
        var bridge = app();
        var value = bridge && typeof bridge.t === 'function' ? bridge.t(key, vars) : key;
        return value === key ? fallback : value;
    }

    function countText(count, singularKey, singularFallback, pluralKey, pluralFallback) {
        return tr(count === 1 ? singularKey : pluralKey, count === 1 ? singularFallback : pluralFallback, { count: count });
    }

    function itemCountText(count) {
        return countText(count, 'project.countItemsSingular', '{count} item', 'project.countItemsPlural', '{count} items');
    }

    function getPanel() {
        return document.getElementById('project-panel');
    }

    function getBadge() {
        return document.getElementById('approvals-badge');
    }

    function escapeHTML(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function formatTime(value) {
        if (!value) {
            return '—';
        }
        try {
            return new Intl.DateTimeFormat((app() && app().getLanguage && app().getLanguage()) || 'en', {
                year: 'numeric',
                month: 'short',
                day: 'numeric',
                hour: 'numeric',
                minute: '2-digit'
            }).format(new Date(value));
        } catch (_) {
            return value;
        }
    }

    function titleCaseWords(value) {
        return String(value || '').split(/\s+/).filter(Boolean).map(function(part) {
            return part.charAt(0).toUpperCase() + part.slice(1);
        }).join(' ');
    }

    function humanizeToken(value) {
        return titleCaseWords(String(value || '').trim().replace(/[_-]+/g, ' ')) || '—';
    }

    function parseWorkspaceRef(value) {
        var text = String(value || '').trim();
        var separator = text.indexOf(':');
        if (!text) {
            return { kind: '', path: '' };
        }
        if (separator > 0) {
            return {
                kind: text.slice(0, separator).trim(),
                path: text.slice(separator + 1).trim()
            };
        }
        return { kind: text, path: '' };
    }

    function humanizeWorkspaceRef(value) {
        var parsed = parseWorkspaceRef(value);
        var kind = parsed.kind ? humanizeToken(parsed.kind) : '';
        if (!parsed.path) {
            return kind || '—';
        }
        return (kind ? kind + ' - ' : '') + parsed.path;
    }

    function humanizeTaskState(taskState) {
        switch (String(taskState || '').trim().toLowerCase()) {
        case 'planned':
            return tr('project.statusNotStarted', 'Not started');
        case 'queued':
            return tr('project.statusQueued', 'Queued');
        case 'running':
            return tr('project.statusRunning', 'Running');
        case 'waiting_review':
            return tr('project.statusWaitingReview', 'Waiting for review');
        case 'waiting_human':
            return tr('project.statusWaitingInput', 'Waiting for input');
        case 'blocked':
            return tr('project.statusBlocked', 'Blocked');
        case 'failed':
            return tr('project.statusFailed', 'Failed');
        case 'execution_complete':
            return tr('project.statusExecutionDone', 'Execution done');
        case 'archived':
            return tr('project.statusArchived', 'Archived');
        default:
            return humanizeToken(taskState);
        }
    }

    function humanizePhase(phaseId) {
        switch (String(phaseId || '').trim().toLowerCase()) {
        case 'plan':
            return tr('project.phasePlan', 'Plan');
        case 'implement':
            return tr('project.phaseImplement', 'Implement');
        case 'write':
            return tr('project.phaseWrite', 'Write');
        case 'test':
            return tr('project.phaseTest', 'Test');
        case 'review':
            return tr('project.phaseReview', 'Review');
        case 'fix_or_replan':
            return tr('project.phaseFixOrReplan', 'Fix or replan');
        case 'final_validation':
            return tr('project.phaseFinalValidation', 'Final validation');
        case 'ready_for_acceptance':
            return tr('project.phaseReadyForAcceptance', 'Ready for acceptance');
        default:
            return humanizeToken(phaseId);
        }
    }

    function humanizeArtifactKind(kind) {
        switch (String(kind || '').trim().toLowerCase()) {
        case 'plan':
            return tr('project.artifactPlan', 'Plan');
        case 'diff_summary':
            return tr('project.artifactDiffSummary', 'Diff summary');
        case 'doc_summary':
            return tr('project.artifactDocSummary', 'Doc summary');
        case 'test_result':
            return tr('project.artifactTestResult', 'Test result');
        case 'review_result':
            return tr('project.artifactReviewResult', 'Review result');
        case 'completion_check':
            return tr('project.artifactCompletionCheck', 'Completion check');
        default:
            return humanizeToken(kind);
        }
    }

    function humanizeArtifactOutcome(outcome) {
        switch (String(outcome || '').trim().toLowerCase()) {
        case 'pass':
            return tr('project.outcomePass', 'Pass');
        case 'fail':
            return tr('project.outcomeFail', 'Fail');
        default:
            return tr('project.outcomeRecorded', 'Recorded');
        }
    }

    function humanizeTool(toolId) {
        switch (String(toolId || '').trim().toLowerCase()) {
        case 'go_test':
            return tr('project.toolGoTest', 'Go test');
        case 'diff_capture':
            return tr('project.toolDiffCapture', 'Diff capture');
        case 'repo_status':
            return tr('project.toolRepoStatus', 'Repo status');
        default:
            return humanizeToken(toolId);
        }
    }

    function humanizeExecutionRole(role) {
        switch (String(role || '').trim().toLowerCase()) {
        case 'plan':
            return tr('project.phasePlanning', 'Planning');
        case 'implement':
            return tr('project.phaseImplementation', 'Implementation');
        case 'test':
            return tr('project.phaseTesting', 'Testing');
        case 'review':
            return tr('project.phaseReview', 'Review');
        case 'verify':
            return tr('project.phaseVerification', 'Verification');
        default:
            return humanizeToken(role);
        }
    }

    function humanizeWriteAccess(access) {
        switch (String(access || '').trim().toLowerCase()) {
        case 'read_only':
            return tr('project.writeAccessReadOnly', 'Read only');
        case 'scoped_write':
            return tr('project.writeAccessScoped', 'Scoped write');
        case 'full_write':
            return tr('project.writeAccessFull', 'Full write');
        default:
            return humanizeToken(access);
        }
    }

    function fetchJSON(url, options) {
        var bridge = app();
        return fetch(bridge && bridge.withBasePath ? bridge.withBasePath(url) : url, options).then(function(response) {
            return response.json().catch(function() { return null; }).then(function(data) {
                var error;
                if (!response.ok) {
                    error = new Error(data && data.error ? data.error : tr('common.requestFailed', 'Request failed'));
                    error.status = response.status;
                    error.payload = data || null;
                    throw error;
                }
                return data;
            });
        });
    }

    function updateBadge() {
        var badge = getBadge();
        if (!badge) return;
        var count = checkpoints().filter(function(item) { return item.status === 'pending'; }).length;
        badge.textContent = tr('project.approvals', 'Approvals') + ' · ' + count;
        badge.classList.toggle('has-pending', count > 0);
        badge.title = tr('project.badgePending', 'Pending approvals: {count}', { count: count });
    }

    function tasks() {
        return state.snapshot && Array.isArray(state.snapshot.tasks) ? state.snapshot.tasks : [];
    }

    function projects() {
        return state.snapshot && Array.isArray(state.snapshot.projects) ? state.snapshot.projects : [];
    }

    function workstreams() {
        return state.snapshot && Array.isArray(state.snapshot.workstreams) ? state.snapshot.workstreams : [];
    }

    function checkpoints() {
        return state.snapshot && Array.isArray(state.snapshot.checkpoints) ? state.snapshot.checkpoints : [];
    }

    function runbooks() {
        return state.snapshot && Array.isArray(state.snapshot.runbooks) ? state.snapshot.runbooks : [];
    }

    function phaseAttempts() {
        return state.snapshot && Array.isArray(state.snapshot.phaseAttempts) ? state.snapshot.phaseAttempts : [];
    }

    function toolRuns() {
        return state.snapshot && Array.isArray(state.snapshot.toolRuns) ? state.snapshot.toolRuns : [];
    }

    function artifacts() {
        return state.snapshot && Array.isArray(state.snapshot.artifacts) ? state.snapshot.artifacts : [];
    }

    function findById(items, id) {
        return (items || []).find(function(item) { return item.id === id; }) || null;
    }

    function runbookForTask(task) {
        var items = runbooks();
        var runbookId = task && task.runbookId ? task.runbookId : '';
        return findById(items, runbookId) || items[0] || null;
    }

    function phasesForTask(task) {
        var runbook = runbookForTask(task);
        return runbook && Array.isArray(runbook.phases) ? runbook.phases : [];
    }

    function taskArtifacts(taskId) {
        return artifacts().filter(function(item) { return item.taskId === taskId; });
    }

    function taskPhaseAttempts(taskId, phaseId) {
        return phaseAttempts().filter(function(item) {
            return item.taskId === taskId && (!phaseId || item.phaseId === phaseId);
        });
    }

    function latestPhaseAttempt(taskId, phaseId) {
        var items = taskPhaseAttempts(taskId, phaseId).slice();
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.startedAt || '');
            var bt = String(b.startedAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function taskToolRuns(taskId) {
        return toolRuns().filter(function(item) {
            return item.taskId === taskId;
        });
    }

    function toolRunsForPhaseAttempt(attempt) {
        if (!attempt || !attempt.id) {
            return [];
        }
        return toolRuns().filter(function(item) {
            return item.phaseAttemptId === attempt.id;
        });
    }

    function latestToolRunForAttempt(attempt) {
        var items = toolRunsForPhaseAttempt(attempt).slice();
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.startedAt || a.completedAt || '');
            var bt = String(b.startedAt || b.completedAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function latestRunningToolRunForAttempt(attempt) {
        var items = toolRunsForPhaseAttempt(attempt).filter(function(item) {
            return item.status === 'running';
        });
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.startedAt || '');
            var bt = String(b.startedAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function sortedToolRunsNewestFirst(items) {
        return (items || []).slice().sort(function(a, b) {
            var at = String(a.startedAt || a.completedAt || '');
            var bt = String(b.startedAt || b.completedAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
    }

    function hasRunningToolRunsForTask(taskId) {
        return taskToolRuns(taskId).some(function(item) {
            return item.status === 'running';
        });
    }

    function artifactForToolRun(run) {
        if (!run) {
            return null;
        }
        if (run.artifactId) {
            return findById(artifacts(), run.artifactId);
        }
        return null;
    }

    function renderMultilineHTML(value) {
        return escapeHTML(value || '').replace(/\n/g, '<br>');
    }

    function toolRunDetailText(run) {
        var artifact = artifactForToolRun(run);
        if (artifact && artifact.value) {
            return String(artifact.value || '').trim();
        }
        if (run && run.error) {
            return String(run.error || '').trim();
        }
        if (run && run.summary) {
            return String(run.summary || '').trim();
        }
        return '';
    }

    function canRetryToolRun(currentAttempt, run, canComplete, toolRunning) {
        return !!run
            && !!currentAttempt
            && !!canComplete
            && !toolRunning
            && run.phaseAttemptId === currentAttempt.id
            && run.status !== 'running';
    }

    function toolRunSupportsPhaseCompletion(phase, run) {
        var requiredKind = artifactKindForPhase(phase);
        var artifact = artifactForToolRun(run);
        var detailText = toolRunDetailText(run);
        if (!phase || !run || run.status !== 'completed' || !requiredKind || !detailText) {
            return null;
        }
        if (artifact
                && String(artifact.kind || '').trim().toLowerCase() === requiredKind
                && (!phaseRequiresPassingOutcome(requiredKind) || String(artifact.outcome || '').trim().toLowerCase() === 'pass')) {
            return {
                artifactKind: requiredKind,
                artifactOutcome: artifact.outcome || requiredOutcome,
                artifactLabel: humanizeArtifactKind(requiredKind),
                artifactValue: artifact.value || detailText,
                sourceToolRunId: run.id,
                sourceToolId: run.toolId
            };
        }
        return null;
    }

    function latestCompletionAssistForPhaseAttempt(attempt, phase) {
        var runs = sortedToolRunsNewestFirst(toolRunsForPhaseAttempt(attempt));
        var i;
        var payload;
        for (i = 0; i < runs.length; i += 1) {
            payload = toolRunSupportsPhaseCompletion(phase, runs[i]);
            if (payload) {
                return payload;
            }
        }
        return null;
    }

    function taskMissingEvidence(task) {
        return task && Array.isArray(task.missingEvidence) ? task.missingEvidence : [];
    }

    function artifactKindForPhase(phase) {
        return phase && Array.isArray(phase.requiredArtifacts) && phase.requiredArtifacts.length
            ? String(phase.requiredArtifacts[0] || '').trim()
            : '';
    }

    function currentPhaseForTask(task) {
        return String(task && task.currentPhase || '').trim() || 'plan';
    }

    function currentRunbookPhase(task) {
        var current = currentPhaseForTask(task);
        return phasesForTask(task).filter(function(phase) { return phase.id === current; })[0] || null;
    }

    function phaseRequiresPassingOutcome(kind) {
        kind = String(kind || '').trim().toLowerCase();
        return kind === 'test_result' || kind === 'review_result' || kind === 'completion_check';
    }

    function defaultOutcomeForArtifact(kind) {
        return phaseRequiresPassingOutcome(kind) ? 'pass' : 'recorded';
    }

    function humanizeEvidenceRequirement(requirement) {
        var parts = String(requirement || '').trim().split(':');
        var kind = parts[0] || '';
        var outcome = parts[1] || '';
        var label = humanizeArtifactKind(kind);
        return outcome ? label + ' / ' + humanizeArtifactOutcome(outcome) : label;
    }

    function renderMissingEvidencePills(missing, emptyLabel) {
        var items = Array.isArray(missing) ? missing : [];
        if (!items.length) {
            return '<span class="project-pill tone-success">' + escapeHTML(emptyLabel || tr('project.evidenceComplete', 'Evidence complete')) + '</span>';
        }
        return items.map(function(item) {
            return '<span class="project-pill tone-warning">' + escapeHTML(humanizeEvidenceRequirement(item)) + '</span>';
        }).join('');
    }

    function phaseArtifactPlaceholder(kind) {
        switch (String(kind || '').trim().toLowerCase()) {
        case 'plan':
            return tr('project.placeholderPlanArtifact', 'Scope, approach, risks, acceptance check');
        case 'diff_summary':
            return tr('project.placeholderDiffSummary', 'Files changed and behavior delta');
        case 'doc_summary':
            return tr('project.placeholderDocSummary', 'Docs changed and rationale');
        case 'test_result':
            return tr('project.placeholderTestResult', 'Command and passing result');
        case 'review_result':
            return tr('project.placeholderReviewResult', 'Review finding summary');
        case 'completion_check':
            return tr('project.placeholderCompletionCheck', 'Evidence checked and ready for acceptance');
        default:
            return humanizeArtifactKind(kind);
        }
    }

    function renderPhaseArtifactPills(task, phase, attempt) {
        var required = phase && Array.isArray(phase.requiredArtifacts) ? phase.requiredArtifacts : [];
        if (!required.length) {
            return '<span class="project-pill tone-neutral">' + escapeHTML(tr('project.noRequiredArtifact', 'No required artifact')) + '</span>';
        }
        return required.map(function(kind) {
            var artifact = phaseAttemptArtifactForKind(attempt, kind);
            var tone = artifact ? artifactOutcomeTone(artifact.outcome) : 'warning';
            var label = humanizeArtifactKind(kind);
            if (artifact) {
                label += ' / ' + humanizeArtifactOutcome(artifact.outcome);
            }
            return '<span class="project-pill tone-' + escapeHTML(tone) + '">' + escapeHTML(label) + '</span>';
        }).join('');
    }

    function renderPhaseOutcomeField(artifactKind, defaultOutcome) {
        return '<label class="project-phase-field"><span>' + escapeHTML(tr('project.outcome', 'Outcome')) + '</span>'
            + '<div class="project-phase-static-field"><span class="project-pill tone-' + escapeHTML(artifactOutcomeTone(defaultOutcome)) + '">' + escapeHTML(humanizeArtifactOutcome(defaultOutcome)) + '</span></div>'
            + '<input type="hidden" name="artifactOutcome" value="' + escapeHTML(defaultOutcome) + '"></label>';
    }

    function artifactOutcomeTone(outcome) {
        switch (String(outcome || '').trim().toLowerCase()) {
        case 'pass':
            return 'success';
        case 'fail':
            return 'danger';
        default:
            return 'info';
        }
    }

    function taskArtifactForKind(taskId, kind) {
        var normalizedKind = String(kind || '').trim().toLowerCase();
        var items = taskArtifacts(taskId).filter(function(item) {
            return String(item.kind || '').trim().toLowerCase() === normalizedKind;
        });
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.createdAt || '');
            var bt = String(b.createdAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function phaseAttemptArtifactForKind(attempt, kind) {
        var normalizedKind = String(kind || '').trim().toLowerCase();
        var artifactIds = attempt && Array.isArray(attempt.artifactIds) ? attempt.artifactIds : [];
        var items;
        if (!attempt || !normalizedKind) {
            return null;
        }
        items = artifacts().filter(function(item) {
            return item.taskId === attempt.taskId
                && String(item.kind || '').trim().toLowerCase() === normalizedKind
                && (item.phaseAttemptId === attempt.id || artifactIds.indexOf(item.id) !== -1);
        });
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.createdAt || '');
            var bt = String(b.createdAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function currentPhaseAttemptForTask(task) {
        return latestPhaseAttempt(task && task.id, currentPhaseForTask(task));
    }

    function phaseStatusForTask(task, phase) {
        var current = currentPhaseForTask(task);
        if (!phase) {
            return current === 'ready_for_acceptance' ? 'completed' : 'pending';
        }
        var attempt = latestPhaseAttempt(task && task.id, phase && phase.id);
        var artifact = taskArtifactForKind(task && task.id, artifactKindForPhase(phase));
        if (attempt && attempt.status === 'running') {
            return 'running';
        }
        if ((attempt && attempt.status === 'failed') || (artifact && artifact.outcome === 'fail')) {
            return 'failed';
        }
        if ((attempt && attempt.status === 'completed') || artifact) {
            return 'completed';
        }
        if (phase && phase.id === current) {
            return 'current';
        }
        return 'pending';
    }

    function phaseStatusTone(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'completed':
            return 'success';
        case 'running':
            return 'info';
        case 'failed':
            return 'danger';
        case 'current':
            return 'warning';
        default:
            return 'neutral';
        }
    }

    function phaseStatusLabel(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'completed':
            return tr('project.phaseStatusCompleted', 'Completed');
        case 'running':
            return tr('project.phaseStatusRunning', 'Running');
        case 'failed':
            return tr('project.phaseStatusFailed', 'Failed');
        case 'current':
            return tr('project.phaseStatusCurrent', 'Current');
        default:
            return tr('project.phaseStatusPending', 'Pending');
        }
    }

    function toolRunStatusLabel(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'completed':
            return tr('project.toolRunCompleted', 'Completed');
        case 'running':
            return tr('project.toolRunRunning', 'Running');
        case 'failed':
            return tr('project.toolRunFailed', 'Failed');
        case 'cancelled':
            return tr('project.toolRunCancelled', 'Cancelled');
        default:
            return humanizeToken(status);
        }
    }

    function renderToolRunDisclosure(task, run, currentAttempt, canComplete, toolRunning, currentPhase, expanded) {
        var artifact = artifactForToolRun(run);
        var detailText = toolRunDetailText(run);
        var canRetry = canRetryToolRun(currentAttempt, run, canComplete, toolRunning);
        var completedAt = run && run.completedAt ? formatTime(run.completedAt) : '';
        var startedAt = run && run.startedAt ? formatTime(run.startedAt) : '';
        var timestampLabel = completedAt ? startedAt + ' -> ' + completedAt : startedAt;
        return '<details class="project-disclosure project-disclosure-compact"' + (expanded ? ' open' : '') + '>'
            + '<summary><span>' + escapeHTML(humanizeTool(run.toolId) + ' / ' + toolRunStatusLabel(run.status)) + '</span>'
            + '<small>' + escapeHTML(timestampLabel || humanizePhase(run.phaseId)) + '</small></summary>'
            + (run.summary ? '<div class="project-list-item"><strong>' + escapeHTML(tr('project.summary', 'Summary')) + ':</strong> ' + escapeHTML(run.summary) + '</div>' : '')
            + (run.workspaceRef ? '<div class="project-list-item"><strong>' + escapeHTML(tr('project.workspace', 'Workspace')) + ':</strong> ' + escapeHTML(humanizeWorkspaceRef(run.workspaceRef)) + '</div>' : '')
            + (artifact ? '<div class="project-list-item"><strong>' + escapeHTML(tr('project.artifact', 'Artifact')) + ':</strong> ' + escapeHTML(humanizeArtifactKind(artifact.kind) + ' / ' + humanizeArtifactOutcome(artifact.outcome)) + '</div>' : '')
            + (detailText ? '<div class="project-list-item"><div class="project-artifact-value">' + renderMultilineHTML(detailText) + '</div></div>' : '')
            + (canRetry
                ? '<div class="project-disclosure-actions"><button type="button" class="project-inline-btn" data-run-phase-tool-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase || run.phaseId || '') + '" data-tool-id="' + escapeHTML(run.toolId) + '">' + escapeHTML(tr('project.retryTool', 'Retry')) + '</button></div>'
                : '')
            + '</details>';
    }

    function renderToolRunHistory(task, runs, currentAttempt, canComplete, toolRunning, currentPhase, expandedLatestOnly) {
        var items = sortedToolRunsNewestFirst(runs || []);
        if (!items.length) {
            return '<div class="project-list-item muted">' + escapeHTML(tr('project.noToolRuns', 'No tool runs recorded yet.')) + '</div>';
        }
        return items.map(function(run, index) {
            return renderToolRunDisclosure(task, run, currentAttempt, canComplete, toolRunning, currentPhase, !!expandedLatestOnly && index === 0 && run.status !== 'completed');
        }).join('');
    }

    function hasRunningCurrentPhase(task) {
        var phaseId = currentPhaseForTask(task);
        var attempt = latestPhaseAttempt(task && task.id, phaseId);
        return !!(attempt && attempt.status === 'running');
    }

    function canRequestAcceptanceReview(task) {
        return !!task
            && task.acceptanceStatus === 'ready_for_acceptance'
            && task.state === 'execution_complete'
            && taskMissingEvidence(task).length === 0;
    }

    function canApproveFinalAcceptance(task) {
        return !!task
            && task.state === 'execution_complete'
            && task.acceptanceStatus === 'under_human_review'
            && taskMissingEvidence(task).length === 0;
    }

    function getSelectedTask() {
        return findById(tasks(), state.selectedTaskId);
    }

    // The v3 prototype opens task details as a modal dialog, not a page.
    var taskDialog = null;

    function closeTaskDialog() {
        if (taskDialog) {
            taskDialog.close();
            taskDialog.remove();
            taskDialog = null;
        }
    }

    function renderTaskDialogContent(dialog) {
        var task = getSelectedTask();
        if (!task || !dialog || !dialog.isConnected) {
            closeTaskDialog();
            return;
        }
        var status = boardAPI().taskStatus(task, state.snapshot);
        var label = tr('project.taskStatus.' + status, status);
        var stamp = boardAPI().taskTimestamp(task, state.snapshot);
        var project = findById(projects(), task.projectId);
        dialog.querySelector('#board-task-dialog-body').innerHTML = renderTaskOverviewCards(task);
        dialog.querySelector('#board-task-dialog-subtitle').textContent =
            (project ? project.name : task.projectId || '') + ' · ' + label + ' · ' + boardRelative(stamp);
        bindInteractions(dialog);
    }

    function openTask(taskId) {
        state.selectedTaskId = taskId;
        state.error = '';
        render();
        if (!getSelectedTask()) {
            return;
        }
        if (!taskDialog) {
            var dialog = document.createElement('dialog');
            dialog.className = 'board-v3 board-detail-dialog';
            dialog.setAttribute('aria-labelledby', 'board-task-dialog-title');
            dialog.innerHTML = '<header class="dialog-header"><div><h2 id="board-task-dialog-title"></h2>'
                + '<p class="board-detail-subtitle" id="board-task-dialog-subtitle"></p></div>'
                + '<button type="button" class="btn small dialog-close" data-task-dialog-close aria-label="' + escapeHTML(i4('close')) + '" title="' + escapeHTML(i4('close')) + '">' + escapeHTML(i4('closeSymbol')) + '</button></header>'
                + '<div class="dialog-body" id="board-task-dialog-body"></div>';
            document.body.appendChild(dialog);
            taskDialog = dialog;
            dialog.querySelector('[data-task-dialog-close]').addEventListener('click', closeTaskDialog);
            bindDialogDismiss(dialog, closeTaskDialog);
        }
        taskDialog.querySelector('#board-task-dialog-title').textContent = getSelectedTask().title || getSelectedTask().id;
        renderTaskDialogContent(taskDialog);
        if (!taskDialog.open) {
            taskDialog.showModal();
        }
    }

    function openApprovals() {
        state.currentView = 'approvals';
        state.error = '';
        render();
    }

    function refreshSnapshotAfterConflict(message) {
        return fetchJSON('/api/project-control').then(function(snapshot) {
            state.snapshot = snapshot;
            state.error = message;
            updateBadge();
            render();
            return snapshot;
        }).catch(function(err) {
            state.error = err.message || message || tr('project.refreshFailed', 'Failed to refresh project panel');
            render();
            return null;
        });
    }

    function refreshSnapshotSilently() {
        return fetchJSON('/api/project-control').then(function(snapshot) {
            state.snapshot = snapshot;
            updateBadge();
            render();
            return snapshot;
        }).catch(function(err) {
            state.error = err.message || tr('project.refreshFailed', 'Failed to refresh project panel');
            render();
            return null;
        });
    }

    function scheduleToolRunRefresh(taskId, remaining) {
        if (state.toolRunRefreshTimer) {
            window.clearTimeout(state.toolRunRefreshTimer);
            state.toolRunRefreshTimer = null;
        }
        remaining = remaining == null ? TOOL_RUN_REFRESH_ATTEMPTS : remaining;
        if (remaining < 1 || !taskId) {
            return;
        }
        state.toolRunRefreshTimer = window.setTimeout(function() {
            state.toolRunRefreshTimer = null;
            refreshSnapshotSilently().then(function() {
                if (hasRunningToolRunsForTask(taskId)) {
                    scheduleToolRunRefresh(taskId, remaining - 1);
                }
            });
        }, TOOL_RUN_REFRESH_INTERVAL_MS);
    }

    function updateTaskInline(taskId, action, extraPayload) {
        var task = findById(tasks(), taskId);
        var body;
        if (!task) {
            return;
        }
        state.updatingTaskId = taskId;
        state.phaseUpdatingTaskId = taskId;
        state.error = '';
        render();
        body = {
            expectedRowVersion: task.rowVersion,
            action: action
        };
        Object.keys(extraPayload || {}).forEach(function(key) {
            body[key] = extraPayload[key];
        });
        fetchJSON('/api/project-control/tasks/' + encodeURIComponent(taskId), {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        }).then(function(snapshot) {
            state.updatingTaskId = '';
            state.phaseUpdatingTaskId = '';
            state.snapshot = snapshot;
            openTask(taskId);
            updateBadge();
            render();
            if ((action === 'run_tool' || action === 'start_execution') && hasRunningToolRunsForTask(taskId)) {
                scheduleToolRunRefresh(taskId, TOOL_RUN_REFRESH_ATTEMPTS);
            }
        }).catch(function(err) {
            state.updatingTaskId = '';
            state.phaseUpdatingTaskId = '';
            if (err && err.status === 409) {
                refreshSnapshotAfterConflict((err.message || tr('project.updateTaskFailed', 'Update task failed')) + ' ' + tr('project.reloadedLatest', 'Reloaded latest state.'));
                return;
            }
            state.error = err.message || tr('project.updateTaskFailed', 'Update task failed');
            render();
        });
    }

    function renderActionButtons(buttons, dataAttr, entityId, updating) {
        return (buttons || []).map(function(button) {
            var tone = button.tone ? ' ' + button.tone : '';
            var disabled = updating || button.disabled ? ' disabled' : '';
            return '<button type="button" class="project-inline-btn' + tone + '" ' + dataAttr + '="' + escapeHTML(entityId) + '" data-action="' + escapeHTML(button.action) + '"' + disabled + '>' + escapeHTML(button.label) + '</button>';
        }).join('');
    }

    function recommendedTaskActions(task) {
        var currentPhase = currentPhaseForTask(task);
        var phaseStartAction = { action: 'start_phase', label: tr('project.actionStartPhase', 'Start phase') + ': ' + humanizePhase(currentPhase), tone: 'primary' };
        if (task.acceptanceStatus === 'ready_for_acceptance') {
            return [
                { action: 'request_acceptance_review', label: tr('project.actionSendApproval', 'Send for approval'), tone: 'primary', disabled: !canRequestAcceptanceReview(task) },
                { action: 'reopen_task', label: tr('project.actionResumeTask', 'Resume task') }
            ];
        }
        if (task.acceptanceStatus === 'under_human_review') {
            return [{ action: 'reopen_task', label: tr('project.actionResumeTask', 'Resume task') }];
        }
        if (task.acceptanceStatus === 'rejected') {
            return [{ action: 'reopen_task', label: tr('project.actionResumeChanges', 'Resume changes'), tone: 'primary' }];
        }
        switch (task.state) {
        case 'planned':
            return [
                { action: 'start_execution', label: tr('project.actionStartExecution', 'Start execution'), tone: 'primary' },
                { action: 'queue_task', label: tr('project.actionAddQueue', 'Add to queue') }
            ];
        case 'queued':
            return [
                { action: 'start_execution', label: tr('project.actionStartExecution', 'Start execution'), tone: 'primary' },
                { action: 'mark_blocked', label: tr('project.actionMarkBlocked', 'Mark blocked') }
            ];
        case 'running':
            return (hasRunningCurrentPhase(task) ? [] : [phaseStartAction]).concat([
                { action: 'request_human_input', label: tr('project.actionNeedInput', 'Need input') },
                { action: 'mark_blocked', label: tr('project.actionMarkBlocked', 'Mark blocked') }
            ]);
        case 'waiting_review':
            return (hasRunningCurrentPhase(task) ? [] : [phaseStartAction]).concat([
                { action: 'resume_execution', label: tr('project.actionResumeTask', 'Resume task') }
            ]);
        case 'waiting_human':
            return [
                { action: 'resume_execution', label: tr('project.actionResumeTask', 'Resume task'), tone: 'primary' },
                { action: 'mark_blocked', label: tr('project.actionMarkBlocked', 'Mark blocked') }
            ];
        case 'blocked':
            return [{ action: 'resume_execution', label: tr('project.actionResumeTask', 'Resume task'), tone: 'primary' }];
        case 'execution_complete':
            if (task.acceptanceStatus === 'accepted') {
                return [
                    { action: 'archive', label: tr('project.actionArchiveTask', 'Archive task'), tone: 'primary' },
                    { action: 'reopen_task', label: tr('project.actionResumeTask', 'Resume task') }
                ];
            }
            if (task.acceptanceStatus === 'not_ready') {
                return [
                    { action: 'request_archive_override', label: tr('project.actionArchiveException', 'Request archive exception') },
                    { action: 'reopen_task', label: tr('project.actionResumeTask', 'Resume task') }
                ];
            }
            return [{ action: 'reopen_task', label: tr('project.actionResumeTask', 'Resume task') }];
        case 'archived':
            return [{ action: 'unarchive', label: tr('project.actionReopenArchivedTask', 'Reopen archived task'), tone: 'primary' }];
        default:
            return [];
        }
    }

    function renderTaskInlineControls(task) {
        var updating = state.updatingTaskId === task.id;
        var actions = recommendedTaskActions(task);
        if (!actions.length && !updating) {
            return '';
        }
        return '<div class="project-inline-controls" data-task-controls="' + escapeHTML(task.id) + '">'
            + '<div class="project-action-group">'
            + renderActionButtons(actions, 'data-update-task', task.id, updating)
            + (updating ? '<span class="project-inline-meta">' + escapeHTML(tr('project.saving', 'Saving…')) + '</span>' : '')
            + '</div>'
            + '</div>';
    }

    function decisionById(id) {
        var decisions = state.snapshot && Array.isArray(state.snapshot.decisions) ? state.snapshot.decisions : [];
        return findById(decisions, id);
    }

    function renderDecisionCard(decision, title) {
        if (!decision) {
            return '<div class="project-list-item muted">' + escapeHTML(tr('project.noDecisionRecorded', 'No decision recorded yet.')) + '</div>';
        }
        return '<div class="project-decision-card">'
            + '<div class="project-card-title">' + escapeHTML(title || tr('project.decision', 'Decision')) + '</div>'
            + '<div class="project-task-badges">'
            + '<span class="project-pill lane-decision">' + escapeHTML(humanizeToken(decision.decisionType || 'decision')) + '</span>'
            + (decision.checkpointId ? '<span class="project-pill lane-checkpoint">' + escapeHTML(decision.checkpointId) + '</span>' : '')
            + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.actor', 'Actor')) + ':</strong> ' + escapeHTML(decision.actor || '—') + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.when', 'When')) + ':</strong> ' + escapeHTML(formatTime(decision.timestamp)) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.summary', 'Summary')) + ':</strong> ' + escapeHTML(decision.summary || '—') + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.decisionId', 'Decision ID')) + ':</strong> ' + escapeHTML(decision.id || '—') + '</div>'
            + '</div>';
    }

    function approvalKindLabel(kind) {
        switch (String(kind || '').trim()) {
        case 'final_acceptance':
            return tr('project.finalAcceptance', 'Final acceptance');
        case 'archive_override':
            return tr('project.archiveOverride', 'Archive override');
        case 'dispatch':
            return tr('project.dispatchCheckpoint', 'Dispatch approval');
        default:
            return String(kind || '').trim() || tr('project.checkpoint', 'checkpoint');
        }
    }

    function approvalStatusLabel(status) {
        switch (String(status || '').trim()) {
        case 'pending':
            return tr('project.needsDecision', 'Needs decision');
        case 'approved':
            return tr('project.approved', 'Approved');
        case 'rejected':
            return tr('project.rejected', 'Rejected');
        case 'rerouted':
            return tr('project.rerouted', 'Rerouted');
        case 'expired':
            return tr('project.expired', 'Expired');
        default:
            return String(status || '').trim() || tr('project.unknown', 'unknown');
        }
    }

    function approvalActionLabel(action) {
        switch (String(action || '').trim()) {
        case 'approve':
            return tr('project.approve', 'Approve');
        case 'reject':
            return tr('project.reject', 'Reject');
        case 'reroute':
            return tr('project.reroute', 'Reroute');
        default:
            return String(action || '').trim() || tr('project.action', 'Action');
        }
    }

    function renderCheckpointActionButton(action, checkpointId, source, task, kind) {
        var disabled = kind === 'final_acceptance' && action === 'approve' && !canApproveFinalAcceptance(task);
        return '<button type="button" class="project-inline-btn ' + (action === 'approve' ? 'primary' : '') + '" data-checkpoint-id="' + escapeHTML(checkpointId) + '" data-checkpoint-action="' + escapeHTML(action) + '"'
            + (source ? ' data-checkpoint-source="' + escapeHTML(source) + '"' : '')
            + (disabled ? ' disabled' : '') + '>' + escapeHTML(approvalActionLabel(action)) + '</button>';
    }

    function approvalKindSummary(item) {
        var kind = String(item && item.kind || '').trim();
        if (kind === 'final_acceptance') {
            return item && item.status === 'pending'
                ? tr('project.finalAcceptancePendingSummary', 'A person needs to sign off before this task can count as accepted work.')
                : tr('project.finalAcceptanceResolvedSummary', 'Shows the result of the final human sign-off for this task.');
        }
        if (kind === 'archive_override') {
            return item && item.status === 'pending'
                ? tr('project.archiveOverridePendingSummary', 'A person needs to approve closing this task without final acceptance.')
                : tr('project.archiveOverrideResolvedSummary', 'Shows the result of the archive exception decision for this task.');
        }
        if (kind === 'dispatch') {
            return item && item.status === 'pending'
                ? tr('project.dispatchPendingSummary', 'Approving lets an agent pick up this task and start working on it.')
                : tr('project.dispatchResolvedSummary', 'Shows the result of the dispatch approval for this task.');
        }
        return tr('project.genericApprovalSummary', 'This item is waiting on a human decision.');
    }

    function latestTaskCheckpoint(taskId, kind) {
        var items = checkpoints().filter(function(item) {
            return item.taskId === taskId && item.kind === kind;
        });
        if (!items.length) {
            return null;
        }
        items.sort(function(a, b) {
            var at = String(a.requestedAt || '');
            var bt = String(b.requestedAt || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items[0];
    }

    function currentTaskApprovalState(task, kind) {
        var checkpoint = task ? latestTaskCheckpoint(task.id, kind) : null;
        if (checkpoint && checkpoint.status === 'pending') {
            return {
                statusLabel: approvalStatusLabel(checkpoint.status),
                summary: approvalKindSummary(checkpoint),
                meta: formatTime(checkpoint.requestedAt) + ' • ' + (checkpoint.decisionSummary || checkpoint.reason || ''),
                actions: checkpoint.allowedActions || [] ,
                checkpointId: checkpoint.id
            };
        }
        if (!task) {
            return null;
        }
        if (kind === 'final_acceptance' && task.acceptanceDecisionId && (task.acceptanceStatus === 'accepted' || task.acceptanceStatus === 'rejected')) {
            var acceptanceDecision = decisionById(task.acceptanceDecisionId);
            if (acceptanceDecision) {
                return {
                    statusLabel: acceptanceDecision.decisionType === 'final_acceptance_approved' ? tr('project.approved', 'Approved') : tr('project.rejected', 'Rejected'),
                    summary: acceptanceDecision.summary || tr('project.finalAcceptanceRecorded', 'Final acceptance decision recorded.'),
                    meta: formatTime(acceptanceDecision.timestamp) + ' • ' + (acceptanceDecision.id || '')
                };
            }
        }
        if (kind === 'archive_override' && task.archiveDecisionId) {
            var archiveDecision = decisionById(task.archiveDecisionId);
            if (archiveDecision) {
                return {
                    statusLabel: archiveDecision.decisionType === 'archive_override_approved' ? tr('project.approved', 'Approved') : tr('project.rejected', 'Rejected'),
                    summary: archiveDecision.summary || tr('project.archiveOverrideRecorded', 'Archive override decision recorded.'),
                    meta: formatTime(archiveDecision.timestamp) + ' • ' + (archiveDecision.id || '')
                };
            }
        }
        return null;
    }

    function renderTaskApprovalStatusCard(task) {
        var finalAcceptance = currentTaskApprovalState(task, 'final_acceptance');
        var archiveOverride = currentTaskApprovalState(task, 'archive_override');
        var dispatch = currentTaskApprovalState(task, 'dispatch');
        function renderStatusRow(label, approval, kind) {
            if (!approval) {
                return '';
            }
            var tone = toneFromText((approval.statusLabel || '') + ' ' + (approval.summary || ''));
            return '<div class="project-list-item"><div class="project-approval-status-row tone-' + escapeHTML(tone) + '">'
                + '<div class="project-approval-status-top"><div class="project-fact-label">' + escapeHTML(label) + '</div>'
                + '<span class="project-pill tone-' + escapeHTML(tone) + '">' + escapeHTML(approval.statusLabel) + '</span></div>'
                + '<div class="project-approval-summary">' + escapeHTML(approval.summary || '') + '</div>'
                + '<div class="project-list-meta">' + escapeHTML(approval.meta || '') + '</div>'
                + ((approval.actions || []).length
                    ? '<div class="project-approval-actions">'
                        + approval.actions.map(function(action) {
                            return renderCheckpointActionButton(action, approval.checkpointId, 'task', task, kind);
                        }).join('')
                        + '</div>'
                    : '')
                + '</div></div>';
        }

        if (!finalAcceptance && !archiveOverride && !dispatch) {
            return '';
        }
        return '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.currentApprovals', 'Current approvals')) + '</h3>'
            + renderStatusRow(tr('project.dispatchCheckpoint', 'Dispatch approval'), dispatch, 'dispatch')
            + renderStatusRow(tr('project.finalAcceptance', 'Final acceptance'), finalAcceptance, 'final_acceptance')
            + renderStatusRow(tr('project.archiveOverride', 'Archive override'), archiveOverride, 'archive_override')
            + '</div>';
    }

    function decisions() {
        return state.snapshot && Array.isArray(state.snapshot.decisions) ? state.snapshot.decisions : [];
    }

    function toneFromText(text) {
        var value = String(text || '').trim().toLowerCase();
        if (!value) {
            return 'neutral';
        }
        if (value.indexOf('blocked') !== -1 || value.indexOf('offline') !== -1 || value.indexOf('error') !== -1 || value.indexOf('failed') !== -1 || value.indexOf('rejected') !== -1) {
            return 'danger';
        }
        if (value.indexOf('pending') !== -1 || value.indexOf('waiting') !== -1 || value.indexOf('review') !== -1 || value.indexOf('approval') !== -1 || value.indexOf('degraded') !== -1) {
            return 'warning';
        }
        if (value.indexOf('running') !== -1 || value.indexOf('online') !== -1 || value.indexOf('healthy') !== -1 || value.indexOf('ready') !== -1 || value.indexOf('complete') !== -1 || value.indexOf('attached') !== -1) {
            return 'success';
        }
        return 'info';
    }

    function taskActionSummary(task) {
        if (!task) {
            return tr('project.noNextStep', 'No next step recorded.');
        }
        return task.nextStep || task.recentSummary || task.goal || tr('project.noNextStep', 'No next step recorded.');
    }

    function approvalContext(item) {
        var task = item && item.taskId ? findById(tasks(), item.taskId) : null;
        var workstream = task && task.workstreamId ? findById(workstreams(), task.workstreamId) : null;
        var projectId = item && item.projectId ? item.projectId : (task ? task.projectId : '');
        var project = projectId ? findById(projects(), projectId) : null;
        return {
            task: task,
            workstream: workstream,
            project: project
        };
    }

    function renderProjectPill(label, tone, extraClass) {
        if (!label) {
            return '';
        }
        return '<span class="project-pill' + (tone ? ' tone-' + tone : '') + (extraClass ? ' ' + extraClass : '') + '">' + escapeHTML(label) + '</span>';
    }

    function renderStructuredEventStream(items, emptyCopy) {
        var list = Array.isArray(items) ? items : [];
        if (!list.length) {
            return '<div class="project-list-item muted">' + escapeHTML(emptyCopy || tr('project.noActivity', 'No activity yet.')) + '</div>';
        }
        return '<div class="project-activity-stream">'
            + list.map(function(item) {
                var tone = toneFromText((item && item.action || '') + ' ' + (item && item.detail || ''));
                var actor = item && item.actor ? item.actor : tr('project.systemActor', 'system');
                return '<div class="project-activity-item tone-' + escapeHTML(tone) + '">'
                    + '<div class="project-activity-marker"><span class="project-audit-dot tone-' + escapeHTML(tone) + '"></span></div>'
                    + '<div class="project-activity-body"><div class="project-list-title">' + escapeHTML(item && item.action || tr('project.update', 'Update')) + '</div>'
                    + '<div class="project-activity-copy">' + escapeHTML(item && item.detail || '—') + '</div>'
                    + '<div class="project-list-meta">' + escapeHTML(actor + ' • ' + formatTime(item && item.timestamp)) + '</div></div>'
                    + '</div>';
            }).join('')
            + '</div>';
    }

    function renderFactList(items, emptyCopy) {
        var list = Array.isArray(items) ? items : [];
        if (!list.length) {
            return '<div class="project-list-item muted">' + escapeHTML(emptyCopy || tr('project.none', 'None')) + '</div>';
        }
        return '<div class="project-fact-list">'
            + list.map(function(item) {
                return '<div class="project-fact-row"><div class="project-fact-label">' + escapeHTML(item.label || '—') + '</div>'
                    + '<div class="project-fact-value">' + escapeHTML(item.value || '—') + '</div></div>';
            }).join('')
            + '</div>';
    }

    function renderFileDiffPanel(paths, diffSummary) {
        var files = Array.isArray(paths) ? paths : [];
        return (files.length
            ? '<div class="project-file-chip-list">' + files.map(function(path) {
                return '<span class="project-file-chip">' + escapeHTML(path) + '</span>';
            }).join('') + '</div>'
            : '<div class="project-list-item muted">' + escapeHTML(tr('project.noFilesChanged', 'No files recorded.')) + '</div>')
            + '<div class="project-diff-summary">' + escapeHTML(diffSummary || tr('project.noDiffSummary', 'No diff summary available.')) + '</div>';
    }

    function statusTone(status) {
        var value = String(status || '').trim().toLowerCase();
        if (value === 'running' || value === 'active' || value === 'online') {
            return 'info';
        }
        if (value === 'blocked' || value === 'failed' || value === 'rejected') {
            return 'danger';
        }
        if (value === 'waiting_human' || value === 'waiting_review' || value === 'pending' || value === 'under_human_review') {
            return 'warning';
        }
        if (value === 'execution_complete' || value === 'complete' || value === 'completed' || value === 'accepted' || value === 'resolved') {
            return 'success';
        }
        return 'neutral';
    }

    function approvalTone(item) {
        var value = String(item && item.status || '').trim().toLowerCase();
        if (value === 'pending' || value === 'requested' || value === 'open') {
            return 'warning';
        }
        if (value === 'approved' || value === 'accepted' || value === 'resolved') {
            return 'success';
        }
        if (value === 'rejected' || value === 'declined' || value === 'blocked') {
            return 'danger';
        }
        return statusTone(value);
    }

    function renderBreadcrumbs() {
        if (state.currentView === 'board') return '';
        return '<button type="button" class="project-breadcrumb" data-open-board>&#x2190; ' + escapeHTML(tr('project.boardTitle', 'Task board')) + '</button>';
    }

    function renderTaskRunbookPanel(task) {
        var phases = phasesForTask(task);
        var currentPhase = currentPhaseForTask(task);
        var currentPhaseDef = currentRunbookPhase(task);
        var missing = taskMissingEvidence(task);
        var updating = state.phaseUpdatingTaskId === task.id || state.updatingTaskId === task.id;
        var currentAttempt = currentPhaseAttemptForTask(task);
        var runningToolRun = latestRunningToolRunForAttempt(currentAttempt);
        var currentToolRun = runningToolRun || latestToolRunForAttempt(currentAttempt);
        var currentAttemptToolRuns = sortedToolRunsNewestFirst(toolRunsForPhaseAttempt(currentAttempt));
        var toolRunning = !!runningToolRun;
        var disabled = updating || toolRunning ? ' disabled' : '';
        var isRunning = currentAttempt && currentAttempt.status === 'running';
        var artifactKind = artifactKindForPhase(currentPhaseDef);
        var defaultOutcome = defaultOutcomeForArtifact(artifactKind);
        var canStart = currentPhaseDef && currentPhase !== 'ready_for_acceptance' && !isRunning && task.acceptanceStatus !== 'accepted' && task.state !== 'archived';
        var canComplete = currentPhaseDef && currentPhase !== 'ready_for_acceptance' && isRunning;
        var completionAssist = latestCompletionAssistForPhaseAttempt(currentAttempt, currentPhaseDef);
        var currentStatus = phaseStatusForTask(task, currentPhaseDef);
        var currentTone = phaseStatusTone(currentStatus);
        var phaseRows = phases.map(function(phase, index) {
            var status = phaseStatusForTask(task, phase);
            var tone = phaseStatusTone(status);
            var artifact = taskArtifactForKind(task.id, artifactKindForPhase(phase));
            var statusIcon = status === 'completed' ? '\u2713 ' : status === 'running' ? '\u25CB ' : status === 'failed' ? '\u2717 ' : '';
            return '<div class="project-phase-row tone-' + escapeHTML(tone) + '">'
                + '<div class="project-phase-index">' + escapeHTML(statusIcon + String(index + 1)) + '</div>'
                + '<div class="project-phase-main"><div class="project-phase-title">' + escapeHTML(humanizePhase(phase.id)) + '</div>'
                + '<div class="project-phase-meta">' + escapeHTML(artifact ? humanizeArtifactKind(artifact.kind) + ' / ' + humanizeArtifactOutcome(artifact.outcome) : humanizeArtifactKind(artifactKindForPhase(phase))) + '</div></div>'
                + '<span class="project-pill tone-' + escapeHTML(tone) + '">' + escapeHTML(phaseStatusLabel(status)) + '</span>'
                + '</div>';
        }).join('');
        var missingHTML = '<div class="project-missing-evidence">' + renderMissingEvidencePills(missing) + '</div>';
        var currentMetaHTML = currentPhaseDef
            ? '<div class="project-phase-workbench-meta">'
                + '<span>' + escapeHTML(tr('project.executionRole', 'Execution role')) + ': <strong>' + escapeHTML(humanizeExecutionRole(currentPhaseDef.executionRole)) + '</strong></span>'
                + '<span>' + escapeHTML(tr('project.writeAccess', 'Write access')) + ': <strong>' + escapeHTML(humanizeWriteAccess(currentPhaseDef.writeAccess)) + '</strong></span>'
              + '</div>'
            : '';
        var requiredHTML = currentPhaseDef
            ? '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.requiredEvidence', 'Required evidence')) + '</div>'
                + '<div class="project-missing-evidence compact">' + renderPhaseArtifactPills(task, currentPhaseDef, currentAttempt) + '</div></div>'
            : '';
        var toolButtons = [];
        var toolRunHTML = currentToolRun
            ? '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.latestToolRun', 'Latest tool run')) + '</div>'
                + renderToolRunHistory(task, [currentToolRun], currentAttempt, canComplete, toolRunning, currentPhase, true)
                + '</div>'
            : '';
        var completionAssistHTML = completionAssist && canComplete && !toolRunning
            ? '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.toolCompletionAssist', 'Tool result ready')) + '</div>'
                + '<div class="project-action-group">'
                + '<button type="button" class="project-inline-btn primary" data-complete-from-tool-run-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '" data-tool-run-id="' + escapeHTML(completionAssist.sourceToolRunId) + '"' + disabled + '>' + escapeHTML(tr('project.completeFromToolRun', 'Complete from latest tool result')) + '</button>'
                + '</div>'
                + '<div class="project-list-meta">' + escapeHTML(humanizeTool(completionAssist.sourceToolId) + ' -> ' + humanizeArtifactKind(completionAssist.artifactKind)) + '</div>'
                + '</div>'
            : '';
        if (canComplete && currentPhaseDef) {
            toolButtons.push('<button type="button" class="project-inline-btn" data-run-phase-tool-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '" data-tool-id="repo_status"' + disabled + '>' + escapeHTML(tr('project.captureRepoStatus', 'Repo status')) + '</button>');
        }
        if (canComplete && currentPhaseDef && currentPhaseDef.writeAccess === 'scoped_write') {
            toolButtons.push('<button type="button" class="project-inline-btn primary" data-run-phase-tool-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '" data-tool-id="diff_capture"' + disabled + '>' + escapeHTML(tr('project.captureDiff', 'Capture diff')) + '</button>');
        }
        if (canComplete && (currentPhase === 'test' || currentPhase === 'final_validation')) {
            toolButtons.push('<button type="button" class="project-inline-btn primary" data-run-phase-tool-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '" data-tool-id="go_test"' + disabled + '>' + escapeHTML(tr('project.runTests', 'Run tests')) + '</button>');
        }
        var toolHTML = toolButtons.length ? '<div class="project-action-group">' + toolButtons.join('') + '</div>' : '';
        var toolHistoryHTML = currentAttemptToolRuns.length > 1
            ? '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.phaseToolRuns', 'Phase tool runs')) + '</div>'
                + renderToolRunHistory(task, currentAttemptToolRuns.slice(1), currentAttempt, canComplete, toolRunning, currentPhase, false)
                + '</div>'
            : '';
        var controlsHTML = '';

        if (canStart) {
            controlsHTML = '<div class="project-action-group">'
                + '<button type="button" class="project-inline-btn primary" data-start-phase-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '"' + disabled + '>' + escapeHTML(tr('project.actionStartPhase', 'Start phase')) + ': ' + escapeHTML(humanizePhase(currentPhase)) + '</button>'
                + '</div>';
        } else if (canComplete) {
            controlsHTML = '<div class="project-phase-control-grid">'
                + toolHTML
                + '<form class="project-phase-form" data-phase-complete-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '" data-artifact-kind="' + escapeHTML(artifactKind) + '">'
                + '<div class="project-phase-fields project-phase-fields-expanded">'
                + '<label class="project-phase-field project-phase-field-wide"><span>' + escapeHTML(humanizeArtifactKind(artifactKind)) + '</span><textarea name="artifactValue" maxlength="640" rows="4" placeholder="' + escapeHTML(phaseArtifactPlaceholder(artifactKind)) + '"></textarea></label>'
                + renderPhaseOutcomeField(artifactKind, defaultOutcome)
                + '</div><div class="project-action-group"><button type="submit" class="project-inline-btn primary"' + disabled + '>' + escapeHTML(tr('project.completePhase', 'Complete phase')) + '</button></div>'
                + '</form>'
                + '<form class="project-phase-form project-phase-fail-form" data-phase-fail-task="' + escapeHTML(task.id) + '" data-phase-id="' + escapeHTML(currentPhase) + '">'
                + '<label class="project-phase-field"><span>' + escapeHTML(tr('project.failureReason', 'Failure reason')) + '</span><input type="text" name="failureReason" maxlength="220" placeholder="' + escapeHTML(humanizePhase(currentPhase)) + '"></label>'
                + '<div class="project-action-group"><button type="submit" class="project-inline-btn"' + disabled + '>' + escapeHTML(tr('project.failPhase', 'Fail phase')) + '</button></div>'
                + '</form>'
                + '</div>';
        } else if (currentPhase === 'ready_for_acceptance' && canRequestAcceptanceReview(task)) {
            controlsHTML = '<div class="project-action-group"><button type="button" class="project-inline-btn primary" data-update-task="' + escapeHTML(task.id) + '" data-action="request_acceptance_review"' + disabled + '>' + escapeHTML(tr('project.actionSendApproval', 'Send for approval')) + '</button></div>';
        } else if (!currentPhaseDef && currentPhase !== 'ready_for_acceptance') {
            controlsHTML = '<div class="project-list-item muted">' + escapeHTML(tr('project.phaseUnavailable', 'Current phase is not available in this runbook.')) + '</div>';
        }

        return '<div class="project-card-list project-card-visual project-runbook-panel"><div class="project-card-title-row"><h3>' + escapeHTML(tr('project.runbook', 'Runbook')) + '</h3>'
            + '<span class="project-pill tone-info">' + escapeHTML(humanizePhase(currentPhase)) + '</span></div>'
            + '<div class="project-phase-workbench tone-' + escapeHTML(currentTone) + '">'
            + '<div class="project-phase-workbench-head"><div><div class="project-section-kicker">' + escapeHTML(tr('project.currentPhase', 'Current phase')) + '</div>'
            + '<div class="project-phase-workbench-title">' + escapeHTML(humanizePhase(currentPhase)) + '</div></div>'
            + '<span class="project-pill tone-' + escapeHTML(currentTone) + '">' + escapeHTML(phaseStatusLabel(currentStatus)) + '</span></div>'
            + currentMetaHTML
            + requiredHTML
            + toolRunHTML
            + completionAssistHTML
            + toolHistoryHTML
            + '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.completionGate', 'Completion gate')) + '</div>' + missingHTML + '</div>'
            + controlsHTML
            + (updating ? '<div class="project-list-meta">' + escapeHTML(tr('project.saving', 'Saving…')) + '</div>' : '')
            + '</div>'
            + '<div class="project-phase-list">' + phaseRows + '</div>'
            + '</div>';
    }

    function renderTaskArtifactsPanel(task) {
        var items = taskArtifacts(task.id);
        var missing = taskMissingEvidence(task);
        return '<div class="project-card-list project-card-visual project-artifact-panel"><div class="project-card-title-row"><h3>' + escapeHTML(tr('project.artifacts', 'Artifacts')) + '</h3>'
            + '<span class="project-pill tone-' + escapeHTML(missing.length ? 'warning' : 'success') + '">' + escapeHTML(missing.length ? String(missing.length) : tr('project.statusClear', 'Clear')) + '</span></div>'
            + (missing.length ? '<div class="project-missing-evidence compact">' + renderMissingEvidencePills(missing) + '</div>' : '')
            + '<div class="project-artifact-list">'
            + (items.length ? items.map(function(item) {
                var tone = artifactOutcomeTone(item.outcome);
                return '<div class="project-artifact-row tone-' + escapeHTML(tone) + '">'
                    + '<div class="project-artifact-main"><div class="project-fact-label">' + escapeHTML(humanizeArtifactKind(item.kind)) + '</div>'
                    + '<div class="project-artifact-value">' + escapeHTML(item.value || item.label || humanizeArtifactKind(item.kind)) + '</div>'
                    + '<div class="project-list-meta">' + escapeHTML(formatTime(item.createdAt)) + '</div></div>'
                    + '<span class="project-pill tone-' + escapeHTML(tone) + '">' + escapeHTML(humanizeArtifactOutcome(item.outcome)) + '</span>'
                    + '</div>';
            }).join('') : '<div class="project-list-item muted">' + escapeHTML(tr('project.noArtifacts', 'No artifacts recorded yet.')) + '</div>')
            + '</div></div>';
    }

    function renderTaskEvidencePanel(task) {
        var currentAttempt = currentPhaseAttemptForTask(task);
        var currentPhase = currentPhaseForTask(task);
        var canComplete = !!currentAttempt && currentPhase !== 'ready_for_acceptance' && currentAttempt.status === 'running';
        var toolRunning = !!latestRunningToolRunForAttempt(currentAttempt);
        return '<div class="project-tab-sections project-task-detail-sections">'
            + '<section class="project-self-report"><h3>' + escapeHTML(tr('project.claims', 'Self-reports')) + '</h3><p>' + escapeHTML(task.recentSummary || '—') + '</p></section>'
            + renderTaskArtifactsPanel(task)
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.toolRuns', 'Tool runs')) + '</h3>'
            + renderToolRunHistory(task, taskToolRuns(task.id), currentAttempt, canComplete, toolRunning, currentPhase, false)
            + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.evidence', 'Evidence')) + '</h3>'
            + renderFactList(task.evidence || [], tr('project.noEvidence', 'No evidence recorded yet.')) + '</div>'
            + '</div>';
    }

    function renderTaskFilesDiffPanel(task) {
        return '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.filesDiff', 'Files & Diff')) + '</h3>'
            + renderFileDiffPanel(task.filesChanged || [], task.diffSummary || '')
            + '</div>';
    }

    function renderTaskAuditPanel(task, decisionCards) {
        return '<div class="project-task-detail-stack">'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.audit', 'Audit')) + '</h3>'
            + renderStructuredEventStream(task.audit || [], tr('project.noAudit', 'No audit entries yet.'))
            + '</div>'
            + (decisionCards || '')
            + '</div>';
    }

    // Overview follows the v3 prototype's detail dialog: summary pills plus
    // flat info cards (task info, evidence, self-report) — no forms, no tabs.
    function renderTaskOverviewCards(task) {
        var status = boardAPI().taskStatus(task, state.snapshot);
        var cls = boardStatusClass(status);
        var label = tr('project.taskStatus.' + status, status);
        var phase = currentPhaseForTask(task);
        var workstream = task.workstreamId ? findById(workstreams(), task.workstreamId) : null;
        var stamp = boardAPI().taskTimestamp(task, state.snapshot);
        var artifacts = taskArtifacts(task.id).slice().sort(function(a, b) {
            return String(b.createdAt || '').localeCompare(String(a.createdAt || ''));
        }).slice(0, 4);
        var audit = (Array.isArray(task.audit) ? task.audit : []).slice(-3).reverse();
        function def(term, value) {
            return '<div class="definition-row"><dt>' + escapeHTML(term) + '</dt><dd>' + value + '</dd></div>';
        }
        var summary = '<div class="detail-summary">'
            + '<span class="pill' + (status === 'running' ? ' accent' : '') + '"><span class="board-task-dot ' + cls + '" aria-hidden="true"></span>' + escapeHTML(label) + '</span>'
            + '<span class="pill">' + escapeHTML(humanizePhase(phase)) + '</span>'
            + (workstream ? '<span class="pill">' + escapeHTML(workstream.title) + '</span>' : '')
            + '</div>';
        var info = '<section class="info-card"><h3>' + escapeHTML(tr('project.taskInfoTitle', 'Task info')) + '</h3><dl>'
            + def(tr('project.taskInfoState', 'State'), escapeHTML(humanizeTaskState(task.state)))
            + def(tr('project.currentPhase', 'Current phase'), escapeHTML(humanizePhase(phase)))
            + def(tr('project.taskInfoGoal', 'Goal'), escapeHTML(task.goal || '—'))
            + def(tr('project.taskInfoNextStep', 'Next step'), escapeHTML(task.nextStep || task.recentSummary || tr('project.noNextStep', 'No next step recorded.')))
            + def(tr('project.taskInfoUpdated', 'Last update'), escapeHTML(stamp ? formatTime(stamp) : '—'))
            + '</dl></section>';
        var evidence = '<section class="info-card evidence-card"><h3>' + escapeHTML(tr('project.evidence', 'Evidence')) + '</h3>'
            + (artifacts.length
                ? '<ul class="evidence-list">' + artifacts.map(function(item) {
                    return '<li><div class="evidence-heading">'
                        + '<span class="source-badge">' + escapeHTML(humanizeArtifactOutcome(item.outcome)) + '</span>'
                        + '<strong>' + escapeHTML(humanizeArtifactKind(item.kind)) + '</strong>'
                        + '<span class="source-note">' + escapeHTML(formatTime(item.createdAt)) + '</span></div>'
                        + (item.value ? '<pre class="evidence-value">' + escapeHTML(item.value) + '</pre>' : '')
                        + '</li>';
                }).join('') + '</ul>'
                : '<p class="muted">' + escapeHTML(tr('project.noArtifacts', 'No artifacts recorded yet.')) + '</p>')
            + '</section>';
        var report = task.recentSummary
            ? '<section class="info-card report-card"><div class="card-heading"><h3>' + escapeHTML(tr('project.taskReportTitle', 'Self-report')) + '</h3>'
                + '<span class="source-badge">' + escapeHTML(tr('project.taskReportTitle', 'Self-report')) + '</span>'
                + '<span class="pill">' + escapeHTML(tr('project.taskUnverified', 'Unverified')) + '</span></div>'
                + '<blockquote>' + escapeHTML(task.recentSummary) + '</blockquote></section>'
            : '';
        var activity = '<section class="info-card"><h3>' + escapeHTML(tr('project.taskActivityTitle', 'Recent activity')) + '</h3>'
            + (audit.length
                ? audit.map(function(item) {
                    return '<p class="source-note">' + escapeHTML((item && item.action || tr('project.update', 'Update')) + ' · ' + (item && item.detail || '—') + ' · ' + formatTime(item && item.timestamp)) + '</p>';
                }).join('')
                : '<p class="muted">' + escapeHTML(tr('project.noActivity', 'No activity yet.')) + '</p>')
            + '</section>';
        return summary
            + info
            + '<div class="detail-columns' + (report ? '' : ' single') + '">' + evidence + report + '</div>'
            + activity
            + renderTaskApprovalStatusCard(task);
    }

    function renderApprovalCard(item) {
        var kindLabel = approvalKindLabel(item.kind);
        var statusLabel = approvalStatusLabel(item.status);
        var tone = approvalTone(item);
        var context = approvalContext(item);
        var headline = context && context.task ? context.task.title : item.title;
        var missing = context && context.task && item.kind === 'final_acceptance' ? taskMissingEvidence(context.task) : [];
        return '<div class="project-approval-card decision-card tone-' + escapeHTML(tone) + '">'
            + '<div class="project-approval-rail tone-' + escapeHTML(tone) + '"></div>'
            + '<div class="project-approval-body">'
            + '<div class="project-approval-header"><div><div class="project-task-badges">'
            + renderProjectPill(kindLabel, tone)
            + renderProjectPill(statusLabel, '', 'outline')
            + (context && context.project ? renderProjectPill(context.project.name, '', 'outline') : '')
            + (context && context.workstream ? renderProjectPill(context.workstream.title, '', 'outline') : '')
            + '</div><div class="project-card-title">' + escapeHTML(headline) + '</div>'
            + (context && context.task ? '<div class="project-card-meta">' + escapeHTML(item.title || kindLabel) + '</div>' : '')
            + '</div>'
            + (item.taskId ? '<button type="button" class="project-inline-btn" data-task-id="' + escapeHTML(item.taskId) + '">' + escapeHTML(tr('project.openTask', 'Open task')) + '</button>' : '') + '</div>'
            + '<div class="project-approval-summary">' + escapeHTML(approvalKindSummary(item)) + '</div>'
            + (missing.length ? '<div class="project-missing-evidence compact">' + renderMissingEvidencePills(missing) + '</div>' : '')
            + '<div class="project-card-copy">' + escapeHTML(item.reason || (context && context.task ? taskActionSummary(context.task) : '')) + '</div>'
            + '<div class="project-approval-footer"><div class="project-card-meta">' + escapeHTML(formatTime(item.requestedAt)) + '</div>'
            + '<div class="project-approval-actions">'
            + (item.allowedActions || []).map(function(action) {
                return renderCheckpointActionButton(action, item.id, '', context && context.task, item.kind);
            }).join('')
            + '</div></div>'
            + (item.decisionSummary ? '<div class="project-approval-note">' + escapeHTML(item.decisionSummary) + '</div>' : '')
            + '</div></div>';
    }

    function renderApprovals() {
        var all = checkpoints();
        var pending = all.filter(function(item) { return item.status === 'pending'; });
        var resolved = all.filter(function(item) { return item.status !== 'pending'; });
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.approvals', 'Approvals Inbox')) + '</div>'
            + '<h2>' + escapeHTML(tr('project.approvalsTitle', 'Items waiting for your decision')) + '</h2>'
            + '<p>' + escapeHTML(tr('project.approvalsHint', 'Review the task context, then approve, reject, or reroute the request.')) + '</p></div></div>'
            + '<div class="project-approval-section"><div class="project-section-kicker">' + escapeHTML(tr('project.reviewNow', 'Review now')) + '</div>'
            + (pending.length
                ? pending.map(renderApprovalCard).join('')
                : '<div class="project-list-item muted">' + escapeHTML(tr('project.noPending', 'No pending approvals.')) + '</div>')
            + '</div>'
            + (resolved.length
                ? '<details class="project-disclosure"><summary><span>' + escapeHTML(tr('project.recentlyResolved', 'Recently resolved')) + '</span><small>' + escapeHTML(itemCountText(resolved.length)) + '</small></summary><div class="project-approval-section">'
                  + resolved.map(renderApprovalCard).join('')
                  + '</div></details>'
                : '')
            + '</section>';
    }

    function boardAPI() { return window.RoamBenchBoardStatus; }

    function boardRelative(stamp) {
        if (!stamp) return tr('project.unknownTime', 'Unknown time');
        var seconds = Math.max(0, Math.floor((Date.now() - stamp) / 1000));
        var unit = seconds < 60 ? 'second' : seconds < 3600 ? 'minute' : seconds < 86400 ? 'hour' : 'day';
        var divisor = { second: 1, minute: 60, hour: 3600, day: 86400 }[unit];
        return new Intl.RelativeTimeFormat((app() && app().getLanguage && app().getLanguage()) || 'en', { numeric: 'auto' }).format(-Math.floor(seconds / divisor), unit);
    }

    var BOARD_STAT_ORDER = ['running', 'approval', 'ready', 'planning', 'paused', 'blocked', 'failed', 'completed'];

    // Status color classes follow the v3.3 board prototype's flat VS Code palette.
    function boardStatusClass(status) {
        return {
            planning: 'st-planning', approval: 'st-approval', ready: 'st-ready',
            running: 'st-running', paused: 'st-paused', completed: 'st-completed',
            blocked: 'st-blocked', failed: 'st-failed'
        }[status] || 'st-planning';
    }

    function countProjectsText(count) {
        return countText(count, 'project.countProjectsSingular', '{count} project', 'project.countProjectsPlural', '{count} projects');
    }

    // Task interval for the timeline: from the earliest recorded observation to the latest.
    function taskInterval(task, latest) {
        var end = latest || 0;
        var starts = [];
        function push(value) {
            var stamp = Date.parse(value || '');
            if (Number.isFinite(stamp)) starts.push(stamp);
        }
        push(task.createdAt);
        (Array.isArray(task.audit) ? task.audit : []).forEach(function(item) { push(item && item.timestamp); });
        checkpoints().forEach(function(item) {
            if (item.taskId === task.id) push(item.requestedAt);
        });
        var start = starts.length ? starts.reduce(function(min, value) { return Math.min(min, value); }, Infinity) : 0;
        if (!end) end = Date.now();
        if (!start || !Number.isFinite(start)) start = end;
        return { start: Math.min(start, end), end: end };
    }

    var BOARD_RANGES = [
        { id: 'today', labelKey: 'project.boardRangeToday', fallback: 'Today' },
        { id: '7d', labelKey: 'project.boardRange7d', fallback: 'Last 7 days' },
        { id: '30d', labelKey: 'project.boardRange30d', fallback: 'Last 30 days' },
        { id: 'all', labelKey: 'project.boardRangeAll', fallback: 'All' }
    ];

    function boardRangeWindow(rangeId, intervals) {
        var now = Date.now();
        if (rangeId === 'today') {
            var dayStart = new Date();
            dayStart.setHours(0, 0, 0, 0);
            return { start: dayStart.getTime(), end: Math.max(now, dayStart.getTime() + 60000) };
        }
        if (rangeId === '7d' || rangeId === '30d') {
            var days = rangeId === '7d' ? 7 : 30;
            return { start: now - days * 86400000, end: now };
        }
        var start = intervals.reduce(function(min, item) { return Math.min(min, item.start); }, Infinity);
        var end = intervals.reduce(function(max, item) { return Math.max(max, item.end); }, 0);
        if (!Number.isFinite(start) || !end) return { start: now - 3600000, end: now };
        if (end - start < 60000) {
            start -= 60000;
            end += 60000;
        }
        return { start: start, end: Math.max(end, Math.min(now, end)) };
    }

    function boardTickLabel(stamp, spanMs) {
        var lang = (app() && app().getLanguage && app().getLanguage()) || 'en';
        try {
            if (spanMs <= 36 * 3600000) {
                return new Intl.DateTimeFormat(lang, { hour: '2-digit', minute: '2-digit' }).format(stamp);
            }
            return new Intl.DateTimeFormat(lang, { month: 'numeric', day: 'numeric' }).format(stamp);
        } catch (_) {
            return new Date(stamp).toISOString().slice(0, spanMs <= 36 * 3600000 ? 16 : 10);
        }
    }

    function renderBoardGantt(visible, range, dataEnd) {
        var intervals = visible.map(function(row) {
            return Object.assign({ interval: taskInterval(row.task, row.stamp || dataEnd) }, row);
        }).filter(function(row) {
            // Inclusive bounds: tasks whose only timestamp equals a range edge still show.
            return row.interval.end >= range.start && row.interval.start <= range.end;
        });
        var groups = [];
        var byProject = {};
        intervals.forEach(function(row) {
            var pid = row.task.projectId || '';
            if (!byProject[pid]) {
                var project = findById(projects(), pid);
                byProject[pid] = { id: pid, name: project ? (project.name || project.id) : (pid || '—'), rows: [] };
                groups.push(byProject[pid]);
            }
            byProject[pid].rows.push(row);
        });
        var tickCount = 6;
        var ticks = [];
        for (var i = 0; i < tickCount; i += 1) {
            var tickStamp = range.start + (range.end - range.start) * i / (tickCount - 1);
            ticks.push('<span style="--tick:' + (i / (tickCount - 1) * 100) + '%">' + escapeHTML(boardTickLabel(tickStamp, range.end - range.start)) + '</span>');
        }
        var nowPos = Math.max(0, Math.min(100, (Date.now() - range.start) / (range.end - range.start) * 100));
        var rangePills = BOARD_RANGES.map(function(range) {
            return '<button type="button" class="board-range-pill" data-board-range="' + range.id + '" aria-pressed="' + (state.boardRange === range.id) + '">'
                + escapeHTML(tr(range.labelKey, range.fallback)) + '</button>';
        }).join('');
        if (!intervals.length) {
            return '<section class="board-panel" id="board-panel-gantt" role="tabpanel" aria-labelledby="board-tab-gantt"' + (state.boardTab === 'gantt' ? '' : ' hidden') + '>'
                + '<div class="board-panel-heading"><h2>' + escapeHTML(tr('project.boardTimelineTitle', 'Task timeline')) + '</h2>'
                + '<div class="board-range-pills" role="group">' + rangePills + '</div></div>'
                + '<div class="board-empty-note">' + escapeHTML(tr('project.boardTimelineEmpty', 'No tasks in this time range.')) + '</div>'
                + '</section>';
        }
        return '<section class="board-panel" id="board-panel-gantt" role="tabpanel" aria-labelledby="board-tab-gantt"' + (state.boardTab === 'gantt' ? '' : ' hidden') + '>'
            + '<div class="board-panel-heading"><h2>' + escapeHTML(tr('project.boardTimelineTitle', 'Task timeline')) + '</h2>'
            + '<div class="board-range-pills" role="group">' + rangePills + '</div></div>'
            + '<div class="board-gantt-scroll" role="region" tabindex="0" style="--now:' + nowPos.toFixed(3) + '%">'
            + '<table class="board-gantt-table">'
            + '<caption class="sr-only">' + escapeHTML(itemCountText(intervals.length)) + '</caption>'
            + '<colgroup><col class="board-gantt-name-col"><col></colgroup>'
            + '<thead><tr><th scope="col" class="board-gantt-label">' + escapeHTML(tr('project.boardTimelineNameCol', 'Task · click for details')) + '</th>'
            + '<th><span class="sr-only">' + escapeHTML(boardTickLabel(range.start, range.end - range.start) + ' – ' + boardTickLabel(range.end, 0)) + '</span>'
            + '<div class="board-time-axis" aria-hidden="true">' + ticks.join('') + '</div></th></tr></thead>'
            + groups.map(function(group) {
                return '<tbody aria-label="' + escapeHTML(group.name) + '">'
                    + '<tr class="board-gantt-project"><th scope="rowgroup" class="board-gantt-label"><span class="board-gantt-project-name">'
                    + escapeHTML(group.name + ' · ' + itemCountText(group.rows.length)) + '</span></th><td></td></tr>'
                    + group.rows.map(function(row) {
                        var clipStart = Math.max(row.interval.start, range.start);
                        var clipEnd = Math.min(row.interval.end, range.end);
                        var startPct = (clipStart - range.start) / (range.end - range.start) * 100;
                        var durationPct = Math.max(0, (clipEnd - clipStart) / (range.end - range.start) * 100);
                        var barTitle = formatTime(row.interval.start) + ' – ' + formatTime(row.interval.end);
                        return '<tr class="board-gantt-row"><th scope="row" class="board-gantt-label">'
                            + '<button type="button" class="board-gantt-name" data-task-id="' + escapeHTML(row.task.id) + '"'
                            + ' aria-label="' + escapeHTML((row.task.title || row.task.id) + ', ' + row.label) + '">'
                            + '<span class="board-task-dot ' + row.cls + '" aria-hidden="true"></span>'
                            + '<span class="board-gantt-name-text">' + escapeHTML(row.task.title || row.task.id) + '</span></button></th>'
                            + '<td><div class="board-timeline-track">'
                            + '<span class="board-timeline-bar ' + row.cls + '" style="--start:' + startPct.toFixed(3) + '%; --duration:' + durationPct.toFixed(3) + '%" title="' + escapeHTML(barTitle) + '"></span>'
                            + '</div></td></tr>';
                    }).join('')
                    + '</tbody>';
            }).join('')
            + '</table></div>'
            + '<div class="board-gantt-footnote"><p>│ ' + escapeHTML(tr('project.boardTimelineNow', 'now')) + '</p></div>'
            + '</section>';
    }

    function renderBoard() {
        var filter = state.boardProjectFilter || 'all';
        var rows = boardAPI().sortTasksNewestFirst(tasks(), state.snapshot).map(function(task) {
            var status = boardAPI().taskStatus(task, state.snapshot);
            return {
                task: task,
                status: status,
                cls: boardStatusClass(status),
                label: tr('project.taskStatus.' + status, status),
                stamp: boardAPI().taskTimestamp(task, state.snapshot)
            };
        });
        var visible = filter === 'all' ? rows : rows.filter(function(row) { return row.task.projectId === filter; });
        var groups = [];
        var byProject = {};
        function groupFor(projectId) {
            if (!byProject[projectId]) {
                var project = findById(projects(), projectId);
                byProject[projectId] = { id: projectId, name: project ? (project.name || project.id) : (projectId || '—'), rows: [] };
                groups.push(byProject[projectId]);
            }
            return byProject[projectId];
        }
        (filter === 'all' ? projects().map(function(project) { return project.id; }) : [filter]).forEach(groupFor);
        visible.forEach(function(row) { groupFor(row.task.projectId || '').rows.push(row); });
        groups.forEach(function(group) {
            group.latest = group.rows.reduce(function(latest, row) { return Math.max(latest, row.stamp || 0); }, 0);
        });
        groups.sort(function(a, b) { return b.latest - a.latest; });
        var counts = BOARD_STAT_ORDER.map(function(status) {
            return { status: status, cls: boardStatusClass(status), count: visible.filter(function(row) { return row.status === status; }).length };
        }).filter(function(entry) { return entry.count > 0; });
        var projectOptions = projects().map(function(project) {
            return '<option value="' + escapeHTML(project.id) + '"' + (filter === project.id ? ' selected' : '') + '>' + escapeHTML(project.name || project.id) + '</option>';
        }).join('');
        // Legacy tasks can lack every timestamp; anchor them at the latest
        // known activity so they stay visible on the timeline.
        var dataEnd = visible.reduce(function(max, row) { return Math.max(max, row.stamp || 0); }, 0) || Date.now();
        var timelineWindow = boardRangeWindow(state.boardRange || 'all', visible.map(function(row) { return taskInterval(row.task, row.stamp || dataEnd); }));
        var timelineCount = visible.filter(function(row) {
            var interval = taskInterval(row.task, row.stamp || dataEnd);
            return interval.end >= timelineWindow.start && interval.start <= timelineWindow.end;
        }).length;
        var tab = state.boardTab === 'gantt' ? 'gantt' : 'wall';
        return '<section class="project-main-section project-board-view board-v3">'
            + '<div class="page-heading"><div><h1>' + escapeHTML(tr('project.boardTitle', 'Task board')) + '</h1>'
            + '<p class="page-meta">' + escapeHTML(countProjectsText(filter === 'all' ? projects().length : 1)) + ' · ' + escapeHTML(itemCountText(rows.length)) + '</p></div>'
            + '<div class="heading-actions">'
            + '<label class="board-filter"><span>' + escapeHTML(tr('project.boardFilterLabel', 'Project')) + '</span>'
            + '<select data-board-filter aria-label="' + escapeHTML(tr('project.boardFilterLabel', 'Project')) + '">'
            + '<option value="all"' + (filter === 'all' ? ' selected' : '') + '>' + escapeHTML(tr('project.boardFilterAll', 'All projects')) + '</option>'
            + projectOptions
            + '</select></label>'
            + '<button type="button" class="btn" data-board-refresh' + (state.boardLoading ? ' disabled' : '') + '>' + escapeHTML(tr('project.boardRefresh', 'Refresh')) + '</button>'
            + i4Button('newProject', 'data-create-project', '', true)
            + '</div></div>'
            + (state.boardError ? '<p role="alert" class="board-inline-error">' + escapeHTML(state.boardError) + '</p>' : '')
            + (state.boardLoading && !state.snapshot ? '<p role="status">' + escapeHTML(tr('project.loading', 'Loading…')) + '</p>' : '')
            + '<div class="view-toolbar">'
            + '<div class="view-tabs" role="tablist" aria-label="' + escapeHTML(tr('project.boardTitle', 'Task board')) + '">'
            + '<button type="button" class="view-tab" id="board-tab-wall" data-board-tab="wall" role="tab" aria-selected="' + (tab === 'wall') + '" aria-controls="board-panel-wall" tabindex="' + (tab === 'wall' ? 0 : -1) + '">' + escapeHTML(tr('project.boardTabWall', 'Status wall')) + '</button>'
            + '<button type="button" class="view-tab" id="board-tab-gantt" data-board-tab="gantt" role="tab" aria-selected="' + (tab === 'gantt') + '" aria-controls="board-panel-gantt" tabindex="' + (tab === 'gantt' ? 0 : -1) + '">' + escapeHTML(tr('project.boardTabTimeline', 'Timeline')) + '</button>'
            + '</div>'
            + (counts.length ? '<ul class="board-legend">' + counts.map(function(entry) {
                return '<li><span class="board-task-dot ' + entry.cls + '" aria-hidden="true"></span>' + escapeHTML(tr('project.taskStatus.' + entry.status, entry.status))
                    + ' <span class="board-legend-count">' + entry.count + '</span></li>';
            }).join('') + '</ul>' : '')
            + '</div>'
            + '<section class="board-panel" id="board-panel-wall" role="tabpanel" aria-labelledby="board-tab-wall"' + (tab === 'wall' ? '' : ' hidden') + '>'
            + (rows.length || filter !== 'all' ? '<div class="board-panel-heading"><h2>' + escapeHTML(tr('project.boardWallTitle', 'Task status')) + '</h2>'
                + '<p class="board-panel-note">' + escapeHTML(tr('project.boardNote', '{count} tasks · grouped by project · click a task to open it', { count: visible.length })) + '</p></div>' : '')
            + (groups.length ? groups.map(function(group) {
                return '<section class="board-project-group">'
                    + '<div class="board-project-heading"><h3>' + escapeHTML(group.name) + '</h3>'
                    + '<span class="board-project-count">' + escapeHTML(itemCountText(group.rows.length)) + '</span></div>'
                    + (group.rows.length ? '<ul class="board-task-grid">' + group.rows.map(function(row) {
                        var task = row.task;
                        var rel = boardRelative(row.stamp);
                        return '<li class="board-task-cell"><button type="button" class="board-task-open" data-task-id="' + escapeHTML(task.id) + '"'
                            + (row.stamp ? ' title="' + escapeHTML(formatTime(row.stamp)) + '"' : '')
                            + ' aria-label="' + escapeHTML((task.title || task.id) + ', ' + row.label + ', ' + rel) + '">'
                            + '<span class="board-task-name-row"><span class="board-task-dot ' + row.cls + '" aria-hidden="true"></span>'
                            + '<span class="board-task-name">' + escapeHTML(task.title || task.id) + '</span></span>'
                            + '<span class="board-task-meta"><span class="board-task-state">' + escapeHTML(row.label + ' · ' + rel) + '</span></span>'
                            + '</button></li>';
                    }).join('') + '</ul>' : '<div class="board-empty-note">' + escapeHTML(tr('project.boardEmptyProject', 'No tasks in this project yet.')) + '</div>')
                    + '</section>';
            }).join('') : '')
            + (!rows.length && filter === 'all' && !state.boardLoading && !state.boardError ? '<div class="board-empty-note">' + escapeHTML(tr('project.boardEmpty', 'No tasks yet')) + '</div>' : '')
            + '</section>'
            + renderBoardGantt(visible, timelineWindow, dataEnd)
            + (rows.length ? '<footer class="board-footer"><p>' + escapeHTML(itemCountText(tab === 'wall' ? visible.length : timelineCount)) + '</p></footer>' : '')
            + '</section>';
    }
    var boardDialog = null;
    function closeBoardDialog() {
        if (folderDialog) { folderDialog.close(); folderDialog.remove(); folderDialog = null; }
        if (boardDialog) { boardDialog.close(); boardDialog.remove(); boardDialog = null; }
    }
    function showBoardDialog(title, content) {
        closeBoardDialog();
        var dialog = document.createElement('dialog');
        dialog.className = 'board-v3 project-dialog';
        dialog.setAttribute('aria-labelledby', 'board-dialog-title');
        dialog.innerHTML = '<header class="dialog-header"><h2 id="board-dialog-title">' + escapeHTML(title) + '</h2><button type="button" class="btn small dialog-close" data-board-dialog-close aria-label="' + escapeHTML(i4('close')) + '" title="' + escapeHTML(i4('close')) + '">' + escapeHTML(i4('closeSymbol')) + '</button></header><div class="dialog-body">' + content + '</div>';
        document.body.appendChild(dialog); boardDialog = dialog;
        dialog.querySelector('[data-board-dialog-close]').addEventListener('click', closeBoardDialog);
        bindDialogDismiss(dialog, closeBoardDialog);
        dialog.showModal();
        return dialog;
    }

    var folderDialog = null;
    function i4(key, vars) { return tr('project.boardI4.' + key, key, vars); }
    function i4Button(key, attr, value, primary) {
        return '<button type="button" class="btn small' + (primary ? ' primary' : '') + '" ' + attr + '="' + escapeHTML(value || '') + '">' + escapeHTML(i4(key)) + '</button>';
    }
    // The close affordance is a symbol, not a word: less to read, and the
    // accessible name stays on the button.
    function i4CloseButton(attr) {
        return '<button type="button" class="btn small dialog-close" ' + attr + ' aria-label="' + escapeHTML(i4('close')) + '" title="' + escapeHTML(i4('close')) + '">' + escapeHTML(i4('closeSymbol')) + '</button>';
    }
    function boardRequest(url, method, body) {
        return fetchJSON('/api/project-control/' + url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    }

    function bindDialogDismiss(dialog, close) {
        var origin = document.activeElement, downOutside = false;
        function outside(event) {
            var box = dialog.getBoundingClientRect();
            return event.target === dialog && (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom);
        }
        dialog.addEventListener('pointerdown', function(event) { downOutside = outside(event); });
        dialog.addEventListener('click', function(event) { if (downOutside && outside(event)) close(); downOutside = false; });
        dialog.addEventListener('cancel', function(event) { event.preventDefault(); close(); });
        dialog.addEventListener('close', function() { if (origin && origin.isConnected) origin.focus({ preventScroll: true }); });
    }
    function openFolderPicker(input) {
        if (folderDialog) return;
        var dialog = document.createElement('dialog'), current = '', version = 0, roots = [], allowUserRoots = false;
        dialog.className = 'board-v3 fs-dialog';
        dialog.setAttribute('aria-labelledby', 'board-folder-title');
        dialog.innerHTML = '<header class="dialog-header"><h2 id="board-folder-title">' + escapeHTML(i4('chooseFolder')) + '</h2>' + i4CloseButton('data-fs-close') + '</header><div class="dialog-body">'
            + '<nav class="fs-roots" aria-label="' + escapeHTML(i4('roots')) + '"><span class="fs-roots-label">' + escapeHTML(i4('roots')) + '</span></nav>'
            + '<div class="fs-add" hidden><input class="fs-add-input" aria-label="' + escapeHTML(i4('rootPath')) + '" placeholder="/data/…" maxlength="1000" autocomplete="off">'
            + '<button type="button" class="btn small primary" data-fs-add-save>' + escapeHTML(i4('add')) + '</button>'
            + '<button type="button" class="btn small" data-fs-add-cancel>' + escapeHTML(i4('cancel')) + '</button></div>'
            + '<nav class="fs-crumb" aria-label="' + escapeHTML(i4('folderPath')) + '"></nav><ul class="fs-list"></ul><p class="form-error" role="alert"></p></div>'
            + '<footer class="dialog-footer">' + i4Button('home', 'data-fs-home') + i4Button('selectFolder', 'data-fs-select', '', true) + '</footer>';
        document.body.appendChild(dialog); folderDialog = dialog;
        function close() { version++; dialog.close(); dialog.remove(); if (folderDialog === dialog) folderDialog = null; }
        bindDialogDismiss(dialog, close);
        dialog.querySelector('[data-fs-close]').onclick = close;
        dialog.querySelector('[data-fs-home]').onclick = function() { navigate('~'); };
        dialog.querySelector('[data-fs-select]').onclick = function() { if (current) { input.value = current; input.dispatchEvent(new Event('input', { bubbles: true })); close(); } };
        var addBox = dialog.querySelector('.fs-add');
        var addInput = dialog.querySelector('.fs-add-input');
        dialog.querySelector('[data-fs-add-cancel]').onclick = function() { addBox.hidden = true; };
        addInput.addEventListener('keydown', function(event) { if (event.key === 'Enter') { event.preventDefault(); saveRoot(); } });
        dialog.querySelector('[data-fs-add-save]').onclick = saveRoot;

        // Roots are quick-access entries: any absolute path stays reachable,
        // the chips only shorten the way there.
        function renderRoots() {
            var nav = dialog.querySelector('.fs-roots');
            nav.innerHTML = '<span class="fs-roots-label">' + escapeHTML(i4('roots')) + '</span>';
            roots.forEach(function(root) {
                var chip = document.createElement('span');
                chip.className = 'fs-root' + (root.source === 'user' ? ' fs-root-user' : '');
                var open = document.createElement('button');
                open.type = 'button';
                open.className = 'fs-root-open';
                open.textContent = root.label;
                open.title = root.path;
                open.onclick = function() { navigate(root.path); };
                chip.appendChild(open);
                if (root.source === 'user' && allowUserRoots) {
                    var remove = document.createElement('button');
                    remove.type = 'button';
                    remove.className = 'fs-root-remove';
                    remove.title = i4('removeRoot');
                    remove.setAttribute('aria-label', i4('removeRoot'));
                    remove.textContent = '×';
                    remove.onclick = function() { removeRoot(root.path); };
                    chip.appendChild(remove);
                }
                nav.appendChild(chip);
            });
            if (allowUserRoots) {
                var add = document.createElement('button');
                add.type = 'button';
                add.className = 'fs-root-add';
                add.title = i4('addRoot');
                add.setAttribute('aria-label', i4('addRoot'));
                add.textContent = '+';
                add.onclick = function() { addBox.hidden = !addBox.hidden; if (!addBox.hidden) addInput.focus(); };
                nav.appendChild(add);
            }
        }
        async function loadRoots() {
            try {
                var data = await fetchJSON('/api/fs/roots');
                roots = Array.isArray(data.roots) ? data.roots : [];
                allowUserRoots = Boolean(data.allow_user_roots);
            } catch (error) {
                roots = [];
                allowUserRoots = false;
            }
            renderRoots();
        }
        async function saveRoot() {
            var path = addInput.value.trim();
            if (!path) return;
            dialog.querySelector('.form-error').textContent = '';
            try {
                await fetchJSON('/api/fs/roots', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: path }) });
                addBox.hidden = true;
                addInput.value = '';
                await loadRoots();
                navigate(path);
            } catch (error) {
                dialog.querySelector('.form-error').textContent = error.message;
            }
        }
        async function removeRoot(path) {
            dialog.querySelector('.form-error').textContent = '';
            try {
                await fetchJSON('/api/fs/roots?path=' + encodeURIComponent(path), { method: 'DELETE' });
                await loadRoots();
            } catch (error) {
                dialog.querySelector('.form-error').textContent = error.message;
            }
        }
        async function navigate(path, initial) {
            var request = ++version, select = dialog.querySelector('[data-fs-select]');
            select.disabled = true; current = '';
            dialog.querySelector('.form-error').textContent = '';
            dialog.querySelector('.fs-list').innerHTML = '<li class="fs-empty">' + escapeHTML(i4('loading')) + '</li>';
            try {
                var data = await fetchJSON('/api/fs/list?path=' + encodeURIComponent(path || ''));
                if (request !== version || !dialog.isConnected) return;
                var crumbs = boardAPI().folderBreadcrumbs(data.path);
                if (!crumbs.length || !Array.isArray(data.directories)) throw new Error(i4('invalidFolder'));
                current = data.path;
                if (Array.isArray(data.roots)) { roots = data.roots; renderRoots(); }
                dialog.querySelector('.fs-crumb').innerHTML = crumbs.map(function(c) { return i4Button('home', 'data-fs-path', c.path).replace(escapeHTML(i4('home')) + '</button>', escapeHTML(c.name) + '</button>'); }).join('<span class="sep">/</span>');
                dialog.querySelector('.fs-list').innerHTML = data.directories.length ? data.directories.map(function(name) {
                    return '<li><button type="button" class="fs-item" data-fs-path="' + escapeHTML(current + '/' + name) + '"><span class="ficon" aria-hidden="true">▱</span>' + escapeHTML(name) + '</button></li>';
                }).join('') : '<li class="fs-empty">' + escapeHTML(i4('emptyFolder')) + '</li>';
                dialog.querySelectorAll('[data-fs-path]').forEach(function(node) { node.onclick = function() { navigate(node.dataset.fsPath); }; });
                select.disabled = false;
                if (initial && input.value.trim()) navigate(input.value.trim());
            } catch (error) {
                if (request !== version || !dialog.isConnected) return;
                dialog.querySelector('.fs-list').innerHTML = '';
                dialog.querySelector('.form-error').textContent = error.message;
            }
        }
        dialog.showModal();
        loadRoots().then(function() { navigate(roots.length ? roots[0].path : '', true); });
    }
    function openNewProject() {
        var dialog = showBoardDialog(i4('newProject'), '<form data-board-entry><fieldset>'
            + '<div class="field" data-entry-folder><label for="board-entry-path">' + escapeHTML(i4('folderPath')) + '</label><div class="path-row"><input id="board-entry-path" name="path" required maxlength="1000" autocomplete="off" placeholder="/data/…">'
            + '<button class="path-pick" type="button" data-pick-folder aria-haspopup="dialog" aria-label="' + escapeHTML(i4('chooseFolder')) + '"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4l2 2.5h7A1.5 1.5 0 0 1 19 9v8.5A1.5 1.5 0 0 1 17.5 19h-13A1.5 1.5 0 0 1 3 17.5z"/></svg></button></div></div>'
            + '<label class="field"><span>' + escapeHTML(i4('goal')) + '</span><input name="goal" required maxlength="300" autocomplete="off"></label>'
            + '</fieldset>'
            + '<p class="form-error" role="alert"></p><footer class="dialog-footer"><button type="submit" class="btn primary">' + escapeHTML(i4('submit')) + '</button></footer></form>');
        dialog.classList.add('create-dialog');
        var form = dialog.querySelector('form'), progress = { projectId: '', workstreamId: '' }, busy = false, entry = null;
        form.querySelector('[data-pick-folder]').onclick = function() { openFolderPicker(form.elements.path); };
        form.onsubmit = async function(event) {
            event.preventDefault(); if (busy) return;
            if (!entry) entry = boardAPI().projectEntry({ source: form.elements.path.value, goal: form.elements.goal.value });
            var errorNode = form.querySelector('.form-error'), submit = form.querySelector('[type="submit"]');
            if (entry.error) { errorNode.textContent = i4(entry.error); entry = null; return; }
            busy = true; submit.disabled = true; form.querySelector('fieldset').disabled = true; errorNode.textContent = ''; submit.textContent = i4('saving');
            var epoch = state.boardEpoch;
            try {
                var snapshot;
                if (!progress.projectId) {
                    snapshot = await boardRequest('projects', 'POST', { name: entry.name, currentGoal: entry.goal, repoPath: entry.source, brief: { understanding: entry.goal } });
                    progress.projectId = snapshot.activeProjectId;
                    if (epoch !== state.boardEpoch) return;
                    state.snapshot = snapshot;
                }
                if (!progress.workstreamId) {
                    snapshot = await boardRequest('workstreams', 'POST', { projectId: progress.projectId, title: entry.goal, scopeSummary: entry.goal, priority: 'medium' });
                    if (epoch !== state.boardEpoch) return;
                    state.snapshot = snapshot;
                    var workstream = findCreatedWorkstream(snapshot, progress.projectId, entry.goal);
                    if (!workstream) throw new Error(i4('registrationIncomplete'));
                    progress.workstreamId = workstream.id;
                }
                snapshot = await boardRequest('tasks', 'POST', { projectId: progress.projectId, workstreamId: progress.workstreamId, title: entry.goal, goal: entry.goal, priority: 'medium', riskLevel: 'medium' });
                if (epoch !== state.boardEpoch) return;
                state.snapshot = snapshot; updateBadge();
                if (boardDialog === dialog) { closeBoardDialog(); openBoard(); }
            } catch (error) {
                if (!dialog.isConnected) return;
                errorNode.textContent = (progress.projectId ? i4('partialRegistration') + ' ' : '') + error.message;
                // Known successful stages are retained so retry never recreates them.
                if (!progress.projectId) { entry = null; form.querySelector('fieldset').disabled = false; }
            } finally { busy = false; submit.disabled = false; submit.textContent = i4('submit'); }
        };
    }

    function initBoardFont() {
        var size = 14;
        try { size = boardAPI().boardFontSize(localStorage.getItem('roambench-board-font-size')); } catch (_) { /* Use default. */ }
        function apply(next) {
            size = boardAPI().boardFontSize(next); document.documentElement.style.setProperty('--board-fs', size + 'px');
            try { localStorage.setItem('roambench-board-font-size', String(size)); } catch (_) { /* Keep the current value. */ }
            document.querySelectorAll('[data-board-font]').forEach(function(node) { node.disabled = node.dataset.boardFont === '-1' ? size <= 12 : node.dataset.boardFont === '1' ? size >= 20 : size === 14; });
        }
        document.querySelectorAll('[data-board-font]').forEach(function(node) { node.addEventListener('click', function() { apply(node.dataset.boardFont === 'reset' ? 14 : size + Number(node.dataset.boardFont)); }); });
        apply(size);
    }

    function loadBoard(options) {
        if (!state.authenticated || state.boardLoading) return Promise.resolve(null);
        options = options || {};
        var epoch = state.boardEpoch;
        state.boardLoading = true;
        state.boardError = '';
        if (!options.silent) render();
        // The current board endpoint has project summaries only. The snapshot
        // supplies full tasks, approval gates and evidence for task details.
        return Promise.all([fetchJSON('/api/project-control/board'), fetchJSON('/api/project-control')]).then(function(results) {
            if (!state.authenticated || epoch !== state.boardEpoch) return null;
            var board = results[0], snapshot = results[1];
            if (!board || !Array.isArray(board.projects) || !snapshot || !Array.isArray(snapshot.tasks)) throw new Error(tr('project.loadFailed', 'Failed to load tasks'));
            state.snapshot = Object.assign({}, snapshot, {
                tasks: Array.isArray(board.tasks) ? board.tasks : snapshot.tasks,
                checkpoints: Array.isArray(board.checkpoints) ? board.checkpoints : snapshot.checkpoints
            });
            state.boardLoading = false;
            var focused = document.activeElement;
            var taskId = focused && focused.getAttribute('data-task-id');
            render();
            if (taskId) {
                var row = getPanel().querySelector('[data-task-id="' + CSS.escape(taskId) + '"]');
                if (row) row.focus({ preventScroll: true });
            }
            scheduleBoardRefresh();
            return board;
        }).catch(function(error) {
            if (!state.authenticated || epoch !== state.boardEpoch) return null;
            state.boardLoading = false;
            state.boardError = error.message || tr('project.loadFailed', 'Failed to load tasks');
            render();
            scheduleBoardRefresh();
            return null;
        });
    }

    function scheduleBoardRefresh() {
        if (state.boardTimer) {
            window.clearTimeout(state.boardTimer);
            state.boardTimer = null;
        }
        if (!state.authenticated || state.currentView !== 'board') {
            return;
        }
        state.boardTimer = window.setTimeout(function() {
            state.boardTimer = null;
            if (state.authenticated && state.currentView === 'board') {
                loadBoard({ silent: true });
            }
        }, BOARD_REFRESH_INTERVAL_MS);
    }

    function openBoard() {
        state.currentView = 'board';
        state.selectedTaskId = '';
        state.error = '';
        render();
        return loadBoard();
    }

    function renderMain() {
        if (state.currentView === 'approvals') return renderApprovals();
        return renderBoard();
    }

    function createProject() { openNewProject(); }

    function findCreatedWorkstream(snapshot, projectId, title) {
        var items = snapshot && Array.isArray(snapshot.workstreams) ? snapshot.workstreams : [];
        var i;
        for (i = items.length - 1; i >= 0; i -= 1) {
            if (items[i].projectId === projectId && items[i].title === title) {
                return items[i];
            }
        }
        return null;
    }

    function startTaskPhase(taskId, phaseId) {
        updateTaskInline(taskId, 'start_phase', {
            phaseId: phaseId || currentPhaseForTask(findById(tasks(), taskId))
        });
    }

    function runPhaseTool(taskId, phaseId, toolId) {
        updateTaskInline(taskId, 'run_tool', {
            phaseId: phaseId || currentPhaseForTask(findById(tasks(), taskId)),
            toolId: toolId || ''
        });
    }

    function completePhaseFromToolRun(taskId, phaseId, toolRunId) {
        var task = findById(tasks(), taskId);
        var phase;
        var run;
        var payload;
        if (!task) {
            return;
        }
        phase = phasesForTask(task).filter(function(item) {
            return item.id === (phaseId || currentPhaseForTask(task));
        })[0] || null;
        run = findById(toolRuns(), toolRunId);
        payload = toolRunSupportsPhaseCompletion(phase, run);
        if (!payload) {
            return;
        }
        updateTaskInline(taskId, 'complete_phase', {
            phaseId: phaseId || currentPhaseForTask(task),
            artifactKind: payload.artifactKind,
            artifactOutcome: payload.artifactOutcome,
            artifactLabel: payload.artifactLabel,
            artifactValue: payload.artifactValue
        });
    }

    function submitPhaseCompleteForm(form) {
        var taskId = form.getAttribute('data-phase-complete-task') || '';
        var phaseId = form.getAttribute('data-phase-id') || '';
        var artifactKind = form.getAttribute('data-artifact-kind') || '';
        var artifactValue = form.querySelector('[name="artifactValue"]');
        var artifactOutcome = form.querySelector('[name="artifactOutcome"]');
        updateTaskInline(taskId, 'complete_phase', {
            phaseId: phaseId,
            artifactKind: artifactKind,
            artifactOutcome: artifactOutcome && artifactOutcome.value ? artifactOutcome.value : defaultOutcomeForArtifact(artifactKind),
            artifactLabel: humanizeArtifactKind(artifactKind),
            artifactValue: artifactValue && artifactValue.value ? artifactValue.value.trim() : ''
        });
    }

    function submitPhaseFailForm(form) {
        var taskId = form.getAttribute('data-phase-fail-task') || '';
        var phaseId = form.getAttribute('data-phase-id') || '';
        var failureReason = form.querySelector('[name="failureReason"]');
        updateTaskInline(taskId, 'fail_phase', {
            phaseId: phaseId,
            failureReason: failureReason && failureReason.value ? failureReason.value.trim() : ''
        });
    }

    function bindInteractions(panel) {
        panel.querySelectorAll('[data-task-id]').forEach(function(node) {
            node.addEventListener('click', function() {
                openTask(node.getAttribute('data-task-id'));
            });
        });
        panel.querySelectorAll('[data-open-approvals]').forEach(function(node) {
            node.addEventListener('click', openApprovals);
        });
        panel.querySelectorAll('[data-open-board]').forEach(function(node) {
            node.addEventListener('click', openBoard);
        });
        panel.querySelectorAll('[data-board-refresh]').forEach(function(node) {
            node.addEventListener('click', function() {
                loadBoard();
            });
        });
        panel.querySelectorAll('[data-board-filter]').forEach(function(node) {
            node.addEventListener('change', function() {
                state.boardProjectFilter = node.value || 'all';
                render();
            });
        });
        panel.querySelectorAll('[data-update-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                updateTaskInline(node.getAttribute('data-update-task'), node.getAttribute('data-action') || '');
            });
        });
        panel.querySelectorAll('[data-start-phase-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                startTaskPhase(node.getAttribute('data-start-phase-task'), node.getAttribute('data-phase-id') || '');
            });
        });
        panel.querySelectorAll('[data-run-phase-tool-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                runPhaseTool(node.getAttribute('data-run-phase-tool-task'), node.getAttribute('data-phase-id') || '', node.getAttribute('data-tool-id') || '');
            });
        });
        panel.querySelectorAll('[data-complete-from-tool-run-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                completePhaseFromToolRun(
                    node.getAttribute('data-complete-from-tool-run-task'),
                    node.getAttribute('data-phase-id') || '',
                    node.getAttribute('data-tool-run-id') || ''
                );
            });
        });
        panel.querySelectorAll('[data-phase-complete-task]').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                submitPhaseCompleteForm(form);
            });
        });
        panel.querySelectorAll('[data-phase-fail-task]').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                submitPhaseFailForm(form);
            });
        });
        panel.querySelectorAll('[data-create-project]').forEach(function(node) {
            node.addEventListener('click', createProject);
        });
        panel.querySelectorAll('[data-board-tab]').forEach(function(node) {
            node.addEventListener('click', function() {
                state.boardTab = node.getAttribute('data-board-tab') === 'gantt' ? 'gantt' : 'wall';
                render();
            });
        });
        (function() {
            var tabs = Array.prototype.slice.call(panel.querySelectorAll('[data-board-tab]'));
            tabs.forEach(function(node, index) {
                node.addEventListener('keydown', function(event) {
                    var next;
                    if (event.key === 'ArrowRight') next = tabs[(index + 1) % tabs.length];
                    if (event.key === 'ArrowLeft') next = tabs[(index - 1 + tabs.length) % tabs.length];
                    if (event.key === 'Home') next = tabs[0];
                    if (event.key === 'End') next = tabs[tabs.length - 1];
                    if (!next) return;
                    event.preventDefault();
                    state.boardTab = next.getAttribute('data-board-tab') === 'gantt' ? 'gantt' : 'wall';
                    render();
                    var focusTarget = getPanel().querySelector('[data-board-tab="' + state.boardTab + '"]');
                    if (focusTarget) focusTarget.focus();
                });
            });
        })();
        panel.querySelectorAll('[data-board-range]').forEach(function(node) {
            node.addEventListener('click', function() {
                state.boardRange = node.getAttribute('data-board-range') || 'all';
                render();
            });
        });
        panel.querySelectorAll('[data-checkpoint-action]').forEach(function(node) {
            node.addEventListener('click', function() {
                var checkpointId = node.getAttribute('data-checkpoint-id');
                var action = node.getAttribute('data-checkpoint-action');
                var source = node.getAttribute('data-checkpoint-source') || 'approvals';
                fetchJSON('/api/project-control/checkpoints/' + encodeURIComponent(checkpointId) + '/decision', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action: action })
                }).then(function(snapshot) {
                    state.snapshot = snapshot;
                    if (source === 'task' && state.selectedTaskId) {
                        openTask(state.selectedTaskId);
                    } else {
                        state.currentView = 'approvals';
                    }
                    updateBadge();
                    render();
                }).catch(function(err) {
                    state.error = err.message || tr('project.decisionFailed', 'Decision failed');
                    render();
                });
            });
        });
    }

    function render() {
        var panel = getPanel();
        if (!panel) return;
        if (state.currentView !== 'board' && state.boardTimer) {
            window.clearTimeout(state.boardTimer);
            state.boardTimer = null;
        }
        updateBadge();
        panel.classList.toggle('board-full', state.authenticated && state.currentView === 'board');
        if (!state.authenticated) { panel.innerHTML = ''; return; }
        panel.innerHTML = (state.error ? '<div class="project-banner error">' + escapeHTML(state.error) + '</div>' : '')
            + '<main class="project-panel-main"><div class="project-breadcrumbs">' + renderBreadcrumbs() + '</div>' + renderMain() + '</main>';
        bindInteractions(panel);
        if (taskDialog && taskDialog.isConnected && taskDialog.open) {
            renderTaskDialogContent(taskDialog);
        }
    }

    function handleAuth(event) {
        state.authenticated = Boolean(event && event.detail && event.detail.authenticated);
        if (!state.authenticated) {
            closeBoardDialog();
            closeTaskDialog();
            state.snapshot = null;
            state.error = '';
            state.currentView = 'board';
            state.selectedTaskId = '';
            state.boardEpoch++;
            state.boardLoading = false;
            state.boardError = '';
            state.boardProjectFilter = '';
            state.boardTab = 'wall';
            state.boardRange = 'all';
            window.clearTimeout(state.boardTimer);
            window.clearTimeout(state.toolRunRefreshTimer);
            state.boardTimer = null;
            state.toolRunRefreshTimer = null;
            render();
        } else if (app() && app().getMode && app().getMode() === 'project') {
            openBoard();
        }
    }

    function handleModeChange(event) {
        var mode = event && event.detail ? event.detail.mode : '';
        if (mode === 'project' && state.authenticated) openBoard();
        else {
            closeBoardDialog();
            closeTaskDialog();
            window.clearTimeout(state.boardTimer);
            state.boardTimer = null;
        }
    }

    function init() {
        initBoardFont();
        var badge = getBadge();
        if (badge) badge.addEventListener('click', function() {
            if (app() && app().setMode) app().setMode('project');
            openApprovals();
            if (!state.snapshot && state.authenticated) loadBoard();
        });
        document.addEventListener('roambench:auth', handleAuth);
        document.addEventListener('roambench:modechange', handleModeChange);
        if (app() && app().getUsername && app().getUsername()) {
            state.authenticated = true;
            if (app().getMode() === 'project') openBoard();
        }
        render();
    }

    window.addEventListener('load', init);
})();
