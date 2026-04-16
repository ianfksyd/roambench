(function() {
    'use strict';

    var state = {
        authenticated: false,
        snapshot: null,
        loading: false,
        error: '',
        eventsLoading: false,
        eventsError: '',
        currentEvents: [],
        currentEventsCursor: '',
        currentEventFilters: null,
        replayLoading: false,
        currentReplay: null,
        selectedEventLane: 'all',
        selectedReplayLane: 'all',
        eventSearchQuery: '',
        replaySearchQuery: '',
        selectedProjectId: '',
        selectedWorkstreamId: '',
        selectedTaskId: '',
        selectedSessionId: '',
        currentView: 'dashboard',
        updatingWorkstreamId: '',
        updatingTaskId: '',
        creatingWorkstreamProjectId: '',
        creatingWorkstreamTitle: '',
        creatingWorkstreamScope: '',
        creatingWorkstreamSaving: false,
        creatingTaskWorkstreamId: '',
        creatingTaskTitle: '',
        creatingTaskGoal: '',
        creatingTaskSelectedSkill: '',
        creatingTaskRunbookId: '',
        creatingTaskSaving: false,
        phaseUpdatingTaskId: '',
        toolRunRefreshTimer: null
    };

    var TOOL_RUN_REFRESH_INTERVAL_MS = 1200;
    var TOOL_RUN_REFRESH_ATTEMPTS = 130;

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

    function taskCountText(count) {
        return countText(count, 'project.countTasksSingular', '{count} task', 'project.countTasksPlural', '{count} tasks');
    }

    function laneCountText(count) {
        return countText(count, 'project.countLanesSingular', '{count} lane', 'project.countLanesPlural', '{count} lanes');
    }

    function workstreamCountText(count) {
        return countText(count, 'project.countWorkstreamsSingular', '{count} workstream', 'project.countWorkstreamsPlural', '{count} workstreams');
    }

    function agentCountText(count) {
        return countText(count, 'project.countAgentsSingular', '{count} agent', 'project.countAgentsPlural', '{count} agents');
    }

    function sessionCountText(count) {
        return countText(count, 'project.countSessionsSingular', '{count} session', 'project.countSessionsPlural', '{count} sessions');
    }

    function itemCountText(count) {
        return countText(count, 'project.countItemsSingular', '{count} item', 'project.countItemsPlural', '{count} items');
    }

    function eventCountText(count) {
        return countText(count, 'project.countEventsSingular', '{count} event', 'project.countEventsPlural', '{count} events');
    }

    function getPanel() {
        return document.getElementById('project-panel');
    }

    function getBadge() {
        return document.getElementById('approvals-badge');
    }

    function storageKey(suffix) {
        var bridge = app();
        var username = bridge && typeof bridge.getUsername === 'function' ? bridge.getUsername() : 'anon';
        return 'roambench.projectPanel.' + username + '.' + suffix;
    }

    function loadFilterPreferences() {
        try {
            state.selectedEventLane = localStorage.getItem(storageKey('selectedEventLane')) || state.selectedEventLane;
            state.selectedReplayLane = localStorage.getItem(storageKey('selectedReplayLane')) || state.selectedReplayLane;
            state.eventSearchQuery = localStorage.getItem(storageKey('eventSearchQuery')) || state.eventSearchQuery;
            state.replaySearchQuery = localStorage.getItem(storageKey('replaySearchQuery')) || state.replaySearchQuery;
        } catch (_) {}
    }

    function persistFilterPreferences() {
        try {
            localStorage.setItem(storageKey('selectedEventLane'), state.selectedEventLane || 'all');
            localStorage.setItem(storageKey('selectedReplayLane'), state.selectedReplayLane || 'all');
            localStorage.setItem(storageKey('eventSearchQuery'), state.eventSearchQuery || '');
            localStorage.setItem(storageKey('replaySearchQuery'), state.replaySearchQuery || '');
        } catch (_) {}
    }

    function escapeHTML(value) {
        return String(value == null ? '' : value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function escapeRegExp(value) {
        return String(value == null ? '' : value).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }

    function highlightHTML(value, query) {
        var escaped = escapeHTML(value);
        var pattern;
        query = String(query || '').trim();
        if (!query) {
            return escaped;
        }
        pattern = new RegExp('(' + escapeRegExp(query) + ')', 'ig');
        return escaped.replace(pattern, '<mark class="project-highlight">$1</mark>');
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

    function workstreamLabel(plural) {
        return plural ? tr('project.workstreamPlural', 'Workstreams') : tr('project.workstreamSingular', 'Workstream');
    }

    function humanizePriority(priority) {
        switch (String(priority || '').trim().toLowerCase()) {
        case 'critical':
            return tr('project.priorityCritical', 'Critical');
        case 'high':
            return tr('project.priorityHigh', 'High');
        case 'low':
            return tr('project.priorityLow', 'Low');
        default:
            return tr('project.priorityMedium', 'Medium');
        }
    }

    function humanizeRisk(risk) {
        switch (String(risk || '').trim().toLowerCase()) {
        case 'critical':
            return tr('project.riskCritical', 'Critical');
        case 'high':
            return tr('project.riskHigh', 'High');
        case 'low':
            return tr('project.riskLow', 'Low');
        default:
            return tr('project.riskMedium', 'Medium');
        }
    }

    function humanizeWorkstreamStatus(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'planned':
            return tr('project.statusNotStarted', 'Not started');
        case 'running':
            return tr('project.statusInProgress', 'In progress');
        case 'waiting_human':
            return tr('project.statusWaitingInput', 'Waiting for input');
        case 'blocked':
            return tr('project.statusBlocked', 'Blocked');
        case 'failed':
            return tr('project.statusFailed', 'Failed');
        case 'completed':
            return tr('project.statusCompleted', 'Completed');
        case 'archived':
            return tr('project.statusArchived', 'Archived');
        default:
            return humanizeToken(status);
        }
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

    function humanizeAcceptanceStatus(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'not_ready':
            return tr('project.acceptanceNotReady', 'Not ready');
        case 'ready_for_acceptance':
            return tr('project.acceptanceReady', 'Ready for approval');
        case 'under_human_review':
            return tr('project.acceptanceInApproval', 'In approval');
        case 'accepted':
            return tr('project.acceptanceAccepted', 'Accepted');
        case 'rejected':
            return tr('project.acceptanceChangesRequested', 'Changes requested');
        default:
            return humanizeToken(status);
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

    function humanizeSkill(skillOrId) {
        var skill = typeof skillOrId === 'object' && skillOrId ? skillOrId : findById(skills(), skillOrId);
        if (skill && skill.name) {
            return skill.name;
        }
        switch (String(skillOrId || '').trim().toLowerCase()) {
        case 'code_change':
            return tr('project.skillCodeChange', 'Code change');
        case 'docs_update':
            return tr('project.skillDocsUpdate', 'Docs update');
        default:
            return humanizeToken(skillOrId);
        }
    }

    function humanizeAgentLabel(agentLabel) {
        switch (String(agentLabel || '').trim().toLowerCase()) {
        case 'worker':
            return tr('project.agentWorker', 'Worker');
        case 'reviewer':
            return tr('project.agentReviewer', 'Reviewer');
        case 'policy_engine':
            return tr('project.agentPolicyEngine', 'Policy engine');
        default:
            return humanizeToken(agentLabel);
        }
    }

    function humanizeExecutionRole(role) {
        switch (String(role || '').trim().toLowerCase()) {
        case 'plan':
            return tr('project.sessionPlanning', 'Planning');
        case 'implement':
            return tr('project.sessionImplementation', 'Implementation');
        case 'test':
            return tr('project.sessionTesting', 'Testing');
        case 'review':
            return tr('project.sessionReview', 'Review');
        case 'verify':
            return tr('project.sessionVerification', 'Verification');
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

    function humanizeSystemRole(role) {
        switch (String(role || '').trim().toLowerCase()) {
        case 'worker':
            return tr('project.systemWorker', 'Worker');
        case 'orchestrator':
            return tr('project.systemOrchestrator', 'Orchestrator');
        default:
            return humanizeToken(role);
        }
    }

    function compactText(value, fallback, maxLength) {
        var text = String(value || fallback || '').trim().replace(/\s+/g, ' ');
        var limit = maxLength || 84;
        if (!text) {
            return '';
        }
        return text.length > limit ? text.slice(0, limit - 1).trim() + '…' : text;
    }

    function shortStatusLabel(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'running':
        case 'active':
        case 'online':
            return tr('project.statusLive', 'Live');
        case 'waiting_human':
        case 'waiting_review':
        case 'pending':
        case 'under_human_review':
            return tr('project.statusReview', 'Review');
        case 'blocked':
        case 'failed':
            return tr('project.statusBlocked', 'Blocked');
        case 'execution_complete':
        case 'complete':
        case 'completed':
        case 'accepted':
        case 'approved':
        case 'resolved':
            return tr('project.statusDone', 'Done');
        case 'planned':
        case 'queued':
            return tr('project.statusNext', 'Next');
        case 'rejected':
            return tr('project.statusFix', 'Fix');
        case 'archived':
            return tr('project.statusArchive', 'Archive');
        default:
            return status ? humanizeToken(status) : tr('project.statusActive', 'Active');
        }
    }

    function shortPriorityLabel(priority) {
        switch (String(priority || '').trim().toLowerCase()) {
        case 'critical':
            return 'P0';
        case 'high':
            return 'P1';
        case 'low':
            return 'P3';
        default:
            return 'P2';
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
        var count = state.snapshot && typeof state.snapshot.approvalsCount === 'number'
            ? state.snapshot.approvalsCount
            : 0;
        if (!badge) {
            return;
        }
        badge.textContent = String(count);
        badge.classList.toggle('has-pending', count > 0);
        badge.title = count > 0
            ? tr('project.badgePending', 'Pending approvals: {count}', { count: count })
            : tr('project.badgeClear', 'No pending approvals');
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

    function sessions() {
        return state.snapshot && Array.isArray(state.snapshot.sessions) ? state.snapshot.sessions : [];
    }

    function checkpoints() {
        return state.snapshot && Array.isArray(state.snapshot.checkpoints) ? state.snapshot.checkpoints : [];
    }

    function skills() {
        return state.snapshot && Array.isArray(state.snapshot.skills) ? state.snapshot.skills : [];
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

    function runtimes() {
        return state.snapshot && Array.isArray(state.snapshot.runtimes) ? state.snapshot.runtimes : [];
    }

    function findById(items, id) {
        return (items || []).find(function(item) { return item.id === id; }) || null;
    }

    function defaultSkill() {
        return skills()[0] || { id: 'code_change', name: tr('project.skillCodeChange', 'Code change'), defaultRunbookId: 'code_change_default', allowedRunbookIds: ['code_change_default'] };
    }

    function skillForId(skillId) {
        return findById(skills(), skillId) || defaultSkill();
    }

    function runbookForSkill(skillId, preferredRunbookId) {
        var skill = skillForId(skillId);
        var allowed = Array.isArray(skill.allowedRunbookIds) ? skill.allowedRunbookIds : [];
        var preferred = preferredRunbookId && allowed.indexOf(preferredRunbookId) !== -1 ? preferredRunbookId : '';
        var runbookId = preferred || skill.defaultRunbookId || allowed[0] || '';
        return findById(runbooks(), runbookId) || runbooks()[0] || { id: runbookId || 'code_change_default' };
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
        var requiredOutcome = defaultOutcomeForArtifact(requiredKind);
        var artifact = artifactForToolRun(run);
        var detailText = toolRunDetailText(run);
        var lines = [];
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
        if (phase.id !== 'review' && phase.id !== 'final_validation') {
            return null;
        }
        lines.push('Source tool: ' + humanizeTool(run.toolId));
        if (run.summary) {
            lines.push('Summary: ' + run.summary);
        }
        lines.push('');
        lines.push(detailText);
        return {
            artifactKind: requiredKind,
            artifactOutcome: requiredOutcome,
            artifactLabel: humanizeArtifactKind(requiredKind),
            artifactValue: lines.join('\n').trim(),
            sourceToolRunId: run.id,
            sourceToolId: run.toolId
        };
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

    function sessionForPhaseAttempt(attempt) {
        return attempt && attempt.sessionId ? findById(sessions(), attempt.sessionId) : null;
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

    function toolRunStatusTone(status) {
        switch (String(status || '').trim().toLowerCase()) {
        case 'completed':
            return 'success';
        case 'running':
            return 'info';
        case 'failed':
            return 'danger';
        case 'cancelled':
            return 'neutral';
        default:
            return 'neutral';
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

    function getSelectedProject() {
        return findById(projects(), state.selectedProjectId || (state.snapshot && state.snapshot.activeProjectId));
    }

    function getSelectedWorkstream() {
        return findById(workstreams(), state.selectedWorkstreamId);
    }

    function getSelectedTask() {
        return findById(tasks(), state.selectedTaskId);
    }

    function getSelectedSession() {
        return findById(sessions(), state.selectedSessionId);
    }

    function tasksForWorkstream(workstreamId) {
        return tasks().filter(function(task) {
            return task.workstreamId === workstreamId;
        });
    }

    function sessionsForTask(taskId) {
        return sessions().filter(function(session) {
            return session.taskId === taskId;
        });
    }

    function workstreamsForProject(projectId) {
        return workstreams().filter(function(item) {
            return item.projectId === projectId;
        });
    }

    function tasksForProject(projectId) {
        return tasks().filter(function(item) {
            return item.projectId === projectId;
        });
    }

    function taskSortPriority(task) {
        switch (String(task && task.state || '').trim().toLowerCase()) {
        case 'running':
            return 0;
        case 'queued':
            return 1;
        case 'planned':
            return 2;
        case 'waiting_review':
            return 3;
        case 'waiting_human':
            return 4;
        case 'blocked':
            return 5;
        case 'execution_complete':
            return 6;
        default:
            return 7;
        }
    }

    function pickPreferredWorkstream(items) {
        var list = (items || []).slice();
        if (!list.length) {
            return null;
        }
        list.sort(function(a, b) {
            var aRunning = a.status === 'running' ? 0 : 1;
            var bRunning = b.status === 'running' ? 0 : 1;
            if (aRunning !== bRunning) {
                return aRunning - bRunning;
            }
            return String(a.title || '').localeCompare(String(b.title || ''));
        });
        return list[0];
    }

    function pickPreferredTask(items) {
        var list = (items || []).slice();
        if (!list.length) {
            return null;
        }
        list.sort(function(a, b) {
            var byState = taskSortPriority(a) - taskSortPriority(b);
            if (byState !== 0) {
                return byState;
            }
            return String(a.title || '').localeCompare(String(b.title || ''));
        });
        return list[0];
    }

    function primaryExecutionSession(taskSessions) {
        var list = taskSessions || [];
        return list.find(function(item) {
            return item && item.supportsAttach && item.terminalId;
        }) || list.find(function(item) {
            return item && item.state === 'active';
        }) || list[0] || null;
    }

    function ensureSelection() {
        var activeProject = state.snapshot && state.snapshot.activeProjectId;
        var checkpointList = checkpoints();
        if (!state.selectedProjectId) {
            state.selectedProjectId = activeProject || '';
        }
        if (state.currentView === 'approvals' && !checkpointList.length) {
            state.currentView = 'dashboard';
        }
        if (state.currentView === 'workstream' && !getSelectedWorkstream()) {
            state.currentView = 'dashboard';
            state.selectedWorkstreamId = '';
        }
        if (state.currentView === 'task' && !getSelectedTask()) {
            state.currentView = state.selectedWorkstreamId ? 'workstream' : 'dashboard';
            state.selectedTaskId = '';
        }
        if (state.currentView === 'session' && !getSelectedSession()) {
            state.currentView = state.selectedTaskId ? 'task' : 'dashboard';
            state.selectedSessionId = '';
        }
    }

    function loadSnapshot(retryCount) {
        if (!state.authenticated) {
            return Promise.resolve(null);
        }
        retryCount = retryCount || 0;
        state.loading = true;
        state.error = '';
        render();
        return fetchJSON('/api/project-control').then(function(snapshot) {
            state.snapshot = snapshot;
            state.loading = false;
            ensureSelection();
            updateBadge();
            render();
            return snapshot;
        }).catch(function(err) {
            state.loading = false;
            state.error = err.message || tr('project.loadFailed', 'Failed to load project panel');
            render();
            if (retryCount < 2) {
                setTimeout(function() { loadSnapshot(retryCount + 1); }, 3000 * (retryCount + 1));
            }
        });
    }

    function resetWorkstreamWizard() {
        state.creatingWorkstreamProjectId = '';
        state.creatingWorkstreamTitle = '';
        state.creatingWorkstreamScope = '';
        state.creatingWorkstreamSaving = false;
    }

    function resetTaskWizard() {
        state.creatingTaskWorkstreamId = '';
        state.creatingTaskTitle = '';
        state.creatingTaskGoal = '';
        state.creatingTaskSelectedSkill = '';
        state.creatingTaskRunbookId = '';
        state.creatingTaskSaving = false;
    }

    function focusWorkstreamWizard() {
        window.setTimeout(function() {
            var input = document.querySelector('[data-workstream-title-input]');
            if (input) {
                input.focus();
            }
        }, 0);
    }

    function focusTaskWizard() {
        window.setTimeout(function() {
            var input = document.querySelector('[data-task-title-input]');
            if (input) {
                input.focus();
            }
        }, 0);
    }

    function startWorkstreamWizard(projectId) {
        if (!projectId) {
            return;
        }
        resetTaskWizard();
        if (state.creatingWorkstreamProjectId !== projectId) {
            state.creatingWorkstreamTitle = '';
            state.creatingWorkstreamScope = '';
        }
        state.creatingWorkstreamProjectId = projectId;
        state.creatingWorkstreamSaving = false;
        state.selectedProjectId = projectId;
        state.selectedWorkstreamId = '';
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        state.currentView = 'dashboard';
        state.error = '';
        render();
        focusWorkstreamWizard();
    }

    function openDashboard(projectId) {
        resetWorkstreamWizard();
        resetTaskWizard();
        state.selectedProjectId = projectId || state.selectedProjectId;
        state.selectedWorkstreamId = '';
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        state.currentView = 'dashboard';
        render();
    }

    function openWorkstream(workstreamId) {
        var workstream = findById(workstreams(), workstreamId);
        resetWorkstreamWizard();
        resetTaskWizard();
        state.selectedWorkstreamId = workstreamId;
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        if (workstream) {
            state.selectedProjectId = workstream.projectId;
        }
        state.currentView = 'workstream';
        render();
    }

    function openTask(taskId) {
        var task = findById(tasks(), taskId);
        resetWorkstreamWizard();
        resetTaskWizard();
        state.selectedTaskId = taskId;
        state.selectedSessionId = '';
        if (task) {
            state.selectedProjectId = task.projectId;
            state.selectedWorkstreamId = task.workstreamId;
        }
        state.currentView = 'task';
        render();
    }

    function openSession(sessionId) {
        var session = findById(sessions(), sessionId);
        resetWorkstreamWizard();
        resetTaskWizard();
        state.selectedSessionId = sessionId;
        if (session) {
            state.selectedTaskId = session.taskId;
            openTask(session.taskId);
            state.selectedSessionId = sessionId;
        }
        state.currentView = 'session';
        render();
    }

    function openApprovals() {
        resetWorkstreamWizard();
        resetTaskWizard();
        state.currentView = 'approvals';
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        render();
    }

    function loadEvents(filters, append) {
        var params = new URLSearchParams();
        var query;
        state.eventsLoading = true;
        state.eventsError = '';
        state.currentEventFilters = Object.assign({}, filters || {});
        if (!append) {
            state.currentEvents = [];
            state.currentEventsCursor = '';
        }
        render();
        Object.keys(state.currentEventFilters || {}).forEach(function(key) {
            var value = state.currentEventFilters[key];
            if (value !== undefined && value !== null && String(value).trim() !== '') {
                params.set(key, String(value).trim());
            }
        });
        if (!params.has('limit')) {
            params.set('limit', '20');
        }
        if (append && state.currentEventsCursor) {
            params.set('cursor', state.currentEventsCursor);
        }
        query = params.toString();
        return fetchJSON('/api/project-control/events' + (query ? '?' + query : ''))
            .then(function(payload) {
                var nextEvents = payload && Array.isArray(payload.events) ? payload.events : [];
                state.currentEvents = append ? state.currentEvents.concat(nextEvents) : nextEvents;
                state.currentEventsCursor = payload && payload.nextCursor ? payload.nextCursor : '';
                state.eventsLoading = false;
                render();
                return state.currentEvents;
            })
            .catch(function(err) {
                state.eventsLoading = false;
                state.eventsError = err.message || tr('project.loadEventsFailed', 'Failed to load events');
                render();
            });
    }

    function loadTaskReplay(taskId) {
        state.replayLoading = true;
        state.currentReplay = null;
        state.selectedReplayLane = 'all';
        state.replaySearchQuery = '';
        state.eventsError = '';
        render();
        return fetchJSON('/api/project-control/tasks/' + encodeURIComponent(taskId) + '/replay')
            .then(function(payload) {
                state.replayLoading = false;
                state.currentReplay = payload || null;
                state.currentView = 'replay';
                render();
                return payload;
            })
            .catch(function(err) {
                state.replayLoading = false;
                state.eventsError = err.message || tr('project.loadReplayFailed', 'Failed to load replay');
                render();
            });
    }

    function openProjectHistory(projectId) {
        state.selectedProjectId = projectId || state.selectedProjectId;
        state.selectedEventLane = 'all';
        state.eventSearchQuery = '';
        state.currentView = 'events';
        loadEvents({ projectId: state.selectedProjectId, limit: 20 });
    }

    function openTaskHistory(taskId) {
        var task = findById(tasks(), taskId);
        if (task) {
            state.selectedProjectId = task.projectId;
            state.selectedWorkstreamId = task.workstreamId;
            state.selectedTaskId = task.id;
        }
        state.selectedEventLane = 'all';
        state.eventSearchQuery = '';
        state.currentView = 'events';
        loadEvents({ taskId: taskId, limit: 20 });
    }

    function metricNumericValue(value) {
        var numeric = Number(value);
        return isFinite(numeric) ? numeric : 0;
    }

    function renderMetric(label, value, tone, options) {
        var numericValue = metricNumericValue(value);
        var maxValue = options && options.maxValue ? options.maxValue : Math.max(numericValue, 1);
        var barWidth = maxValue > 0 && numericValue > 0 ? Math.max(14, Math.round((numericValue / maxValue) * 100)) : 0;
        var chip = options && options.chip ? options.chip : '';
        return '<div class="project-metric ' + (tone ? 'tone-' + tone : '') + '">'
            + '<div class="project-metric-topline">'
            + '<span class="project-metric-mark tone-' + escapeHTML(tone || 'neutral') + '"></span>'
            + (chip ? '<div class="project-metric-chip ' + (tone ? 'tone-' + tone : '') + '">' + escapeHTML(chip) + '</div>' : '')
            + '</div>'
            + '<div class="project-metric-value-row"><div class="project-metric-value">' + escapeHTML(value) + '</div>'
            + '<div class="project-metric-label">' + escapeHTML(label) + '</div></div>'
            + '<div class="project-metric-bar"><span class="project-metric-bar-fill ' + (tone ? 'tone-' + tone : '') + '" style="width:' + escapeHTML(String(barWidth)) + '%"></span></div>'
            + '</div>';
    }


    function renderSelectOptions(options, currentValue) {
        return options.map(function(option) {
            var selected = option.value === currentValue ? ' selected' : '';
            return '<option value="' + escapeHTML(option.value) + '"' + selected + '>' + escapeHTML(option.label) + '</option>';
        }).join('');
    }

    function workstreamStatusOptions() {
        return [
            { value: 'planned', label: 'planned' },
            { value: 'running', label: 'running' },
            { value: 'waiting_human', label: 'waiting_human' },
            { value: 'blocked', label: 'blocked' },
            { value: 'failed', label: 'failed' },
            { value: 'completed', label: 'completed' },
            { value: 'archived', label: 'archived' }
        ];
    }

    function taskStateOptions() {
        return [
            { value: 'planned', label: 'planned' },
            { value: 'queued', label: 'queued' },
            { value: 'running', label: 'running' },
            { value: 'waiting_review', label: 'waiting_review' },
            { value: 'waiting_human', label: 'waiting_human' },
            { value: 'blocked', label: 'blocked' },
            { value: 'failed', label: 'failed' },
            { value: 'execution_complete', label: 'execution_complete' },
            { value: 'archived', label: 'archived' }
        ];
    }

    function acceptanceOptions() {
        return [
            { value: 'not_ready', label: 'not_ready' },
            { value: 'ready_for_acceptance', label: 'ready_for_acceptance' },
            { value: 'under_human_review', label: 'under_human_review' },
            { value: 'accepted', label: 'accepted' },
            { value: 'rejected', label: 'rejected' }
        ];
    }

    function priorityOptions() {
        return [
            { value: 'low', label: 'low' },
            { value: 'medium', label: 'medium' },
            { value: 'high', label: 'high' },
            { value: 'critical', label: 'critical' }
        ];
    }

    function refreshSnapshotAfterConflict(message) {
        return fetchJSON('/api/project-control').then(function(snapshot) {
            state.snapshot = snapshot;
            state.error = message;
            ensureSelection();
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
            ensureSelection();
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

    function updateWorkstreamInline(workstreamId, action) {
        var workstream = findById(workstreams(), workstreamId);
        if (!workstream) {
            return;
        }
        state.updatingWorkstreamId = workstreamId;
        state.error = '';
        render();
        fetchJSON('/api/project-control/workstreams/' + encodeURIComponent(workstreamId), {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                expectedRowVersion: workstream.rowVersion,
                action: action
            })
        }).then(function(snapshot) {
            state.updatingWorkstreamId = '';
            state.snapshot = snapshot;
            openWorkstream(workstreamId);
            updateBadge();
            render();
        }).catch(function(err) {
            state.updatingWorkstreamId = '';
            if (err && err.status === 409) {
                refreshSnapshotAfterConflict((err.message || tr('project.updateWorkstreamFailed', 'Update workstream failed')) + ' ' + tr('project.reloadedLatest', 'Reloaded latest state.'));
                return;
            }
            state.error = err.message || tr('project.updateWorkstreamFailed', 'Update workstream failed');
            render();
        });
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
            if (action === 'run_tool' && hasRunningToolRunsForTask(taskId)) {
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

    function recommendedWorkstreamActions(workstream) {
        switch (workstream.status) {
        case 'planned':
            return [{ action: 'start_execution', label: tr('project.actionStartWorkstream', 'Start workstream'), tone: 'primary' }];
        case 'running':
            return [
                { action: 'request_human_input', label: tr('project.actionNeedInput', 'Need input') },
                { action: 'mark_blocked', label: tr('project.actionMarkBlocked', 'Mark blocked') },
                { action: 'mark_completed', label: tr('project.actionMarkDone', 'Mark done'), tone: 'primary' }
            ];
        case 'waiting_human':
            return [
                { action: 'resume_execution', label: tr('project.actionResumeWorkstream', 'Resume workstream'), tone: 'primary' },
                { action: 'mark_blocked', label: tr('project.actionMarkBlocked', 'Mark blocked') }
            ];
        case 'blocked':
            return [{ action: 'resume_execution', label: tr('project.actionResumeWorkstream', 'Resume workstream'), tone: 'primary' }];
        case 'completed':
            return [{ action: 'archive', label: tr('project.actionArchiveWorkstream', 'Archive workstream') }];
        default:
            return [];
        }
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
                { action: 'queue_task', label: tr('project.actionAddQueue', 'Add to queue') },
                phaseStartAction
            ];
        case 'queued':
            return [
                phaseStartAction,
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

    function renderWorkstreamInlineControls(workstream) {
        var updating = state.updatingWorkstreamId === workstream.id;
        var actions = recommendedWorkstreamActions(workstream);
        if (!actions.length && !updating) {
            return '';
        }
        return '<div class="project-inline-controls" data-workstream-controls="' + escapeHTML(workstream.id) + '">'
            + '<div class="project-action-group">'
            + renderActionButtons(actions, 'data-update-workstream', workstream.id, updating)
            + (updating ? '<span class="project-inline-meta">' + escapeHTML(tr('project.saving', 'Saving…')) + '</span>' : '')
            + '</div>'
            + '</div>';
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

    function eventLaneForAction(action) {
        action = String(action || '').trim();
        if (action.indexOf('final_acceptance_') === 0 || action.indexOf('archive_override_') === 0 || action === 'decision_made') {
            return 'decision';
        }
        if (action === 'checkpoint_raised' || action === 'checkpoint_resolved' || action === 'checkpoint_expired' || action.indexOf('checkpoint_') === 0) {
            return 'checkpoint';
        }
        if (action.indexOf('acceptance_') === 0 || action.indexOf('ready_for_acceptance') !== -1) {
            return 'acceptance';
        }
        return 'execution';
    }

    function laneLabel(lane) {
        switch (lane) {
        case 'decision':
            return tr('project.laneDecision', 'Decision lane');
        case 'checkpoint':
            return tr('project.laneCheckpoint', 'Checkpoint lane');
        case 'acceptance':
            return tr('project.laneAcceptance', 'Acceptance lane');
        default:
            return tr('project.laneExecution', 'Execution lane');
        }
    }

    function renderLanePill(lane) {
        return '<span class="project-pill lane-' + escapeHTML(lane) + '">' + escapeHTML(laneLabel(lane)) + '</span>';
    }

    function groupEventsByLane(events) {
        var groups = [];
        (events || []).forEach(function(event) {
            var lane = eventLaneForAction(event.action);
            var last = groups.length ? groups[groups.length - 1] : null;
            if (!last || last.lane !== lane) {
                last = { lane: lane, items: [] };
                groups.push(last);
            }
            last.items.push(event);
        });
        return groups;
    }

    function laneOptions() {
        return [
            { value: 'all', label: tr('project.filterAllLanes', 'All lanes') },
            { value: 'execution', label: laneLabel('execution') },
            { value: 'acceptance', label: laneLabel('acceptance') },
            { value: 'checkpoint', label: laneLabel('checkpoint') },
            { value: 'decision', label: laneLabel('decision') }
        ];
    }

    function filterEventsByLane(events, selectedLane) {
        if (!selectedLane || selectedLane === 'all') {
            return events || [];
        }
        return (events || []).filter(function(event) {
            return eventLaneForAction(event.action) === selectedLane;
        });
    }

    function filterEventsByQuery(events, query) {
        query = String(query || '').trim().toLowerCase();
        if (!query) {
            return events || [];
        }
        return (events || []).filter(function(event) {
            var haystack = [event.action, event.detail, event.actor].join(' ').toLowerCase();
            return haystack.indexOf(query) !== -1;
        });
    }

    function filterTransitionsByLane(transitions, selectedLane) {
        if (!selectedLane || selectedLane === 'all') {
            return transitions || [];
        }
        return (transitions || []).filter(function(item) {
            var lane = item.type === 'decision' || item.type === 'decision_recorded'
                ? 'decision'
                : (item.type === 'acceptance_state' ? 'acceptance' : (item.type === 'checkpoint_resolution' ? 'checkpoint' : 'execution'));
            return lane === selectedLane;
        });
    }

    function filterTransitionsByQuery(transitions, query) {
        query = String(query || '').trim().toLowerCase();
        if (!query) {
            return transitions || [];
        }
        return (transitions || []).filter(function(item) {
            var haystack = [item.type, item.from, item.to, item.reason].join(' ').toLowerCase();
            return haystack.indexOf(query) !== -1;
        });
    }

    function renderLaneFilterControls(kind, selectedLane) {
        return '<div class="project-lane-filters">'
            + laneOptions().map(function(option) {
                var active = option.value === selectedLane ? ' active' : '';
                return '<button type="button" class="project-inline-btn project-filter-btn' + active + '" data-lane-filter-kind="' + escapeHTML(kind) + '" data-lane-filter="' + escapeHTML(option.value) + '">' + escapeHTML(option.label) + '</button>';
            }).join('')
            + '</div>';
    }

    function renderTextFilterControl(kind, value) {
        var placeholder = kind === 'events' ? tr('project.searchHistory', 'Search history…') : tr('project.searchReplay', 'Search replay…');
        return '<div class="project-text-filter">'
            + '<input type="search" class="project-filter-input" data-filter-input-kind="' + escapeHTML(kind) + '" value="' + escapeHTML(value || '') + '" placeholder="' + escapeHTML(placeholder) + '">'
            + '</div>';
    }

    function renderLaneEventCard(event, index, query) {
        var lane = eventLaneForAction(event.action);
        return '<div class="project-lane-event lane-' + escapeHTML(lane) + '">'
            + '<div class="project-lane-event-header">'
            + renderLanePill(lane)
            + '<span class="project-list-title">' + highlightHTML(String(index + 1) + '. ' + event.action, query) + '</span>'
            + '</div>'
            + '<div>' + highlightHTML(event.detail, query) + '</div>'
            + '<div class="project-list-meta">' + highlightHTML(event.actor + ' • ' + formatTime(event.timestamp), query) + '</div>'
            + '</div>';
    }

    function renderLaneGroups(events, query) {
        return groupEventsByLane(events).map(function(group) {
            return '<div class="project-lane-section lane-' + escapeHTML(group.lane) + '">'
                + '<div class="project-lane-header">'
                + renderLanePill(group.lane)
                + '<span class="project-list-meta">' + escapeHTML(eventCountText(group.items.length)) + '</span>'
                + '</div>'
                + group.items.map(function(event, index) {
                    return renderLaneEventCard(event, index, query);
                }).join('')
                + '</div>';
        }).join('');
    }

    function renderTransitionCard(item, query) {
        var lane = item.type === 'decision' || item.type === 'decision_recorded'
            ? 'decision'
            : (item.type === 'acceptance_state' ? 'acceptance' : (item.type === 'checkpoint_resolution' ? 'checkpoint' : 'execution'));
        return '<div class="project-lane-event lane-' + escapeHTML(lane) + '">'
            + '<div class="project-lane-event-header">'
            + renderLanePill(lane)
            + '<span class="project-list-title">' + highlightHTML(item.type, query) + '</span>'
            + '</div>'
            + '<div>' + highlightHTML(item.from + ' → ' + item.to, query) + '</div>'
            + '<div class="project-list-meta">' + highlightHTML(item.reason, query) + '</div>'
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

        if (!finalAcceptance && !archiveOverride) {
            return '';
        }
        return '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.currentApprovals', 'Current approvals')) + '</h3>'
            + renderStatusRow(tr('project.finalAcceptance', 'Final acceptance'), finalAcceptance, 'final_acceptance')
            + renderStatusRow(tr('project.archiveOverride', 'Archive override'), archiveOverride, 'archive_override')
            + '</div>';
    }

    function taskApprovalBadges(task) {
        var badges = [];
        var finalAcceptance = currentTaskApprovalState(task, 'final_acceptance');
        var archiveOverride = currentTaskApprovalState(task, 'archive_override');

        if (finalAcceptance) {
            if (finalAcceptance.actions && finalAcceptance.actions.length) {
                badges.push({ className: 'approval-pending', label: tr('project.finalAcceptancePending', 'Final acceptance pending') });
            } else if (task && task.acceptanceStatus === 'accepted') {
                badges.push({ className: 'approval-approved', label: tr('project.acceptanceAccepted', 'Accepted') });
            } else if (task && task.acceptanceStatus === 'rejected') {
                badges.push({ className: 'approval-rejected', label: tr('project.finalAcceptanceRejected', 'Final acceptance rejected') });
            }
        }

        if (archiveOverride) {
            if (archiveOverride.actions && archiveOverride.actions.length) {
                badges.push({ className: 'approval-pending', label: tr('project.archiveOverridePending', 'Archive override pending') });
            } else if (task && task.archiveDecisionId) {
                badges.push({
                    className: archiveOverride.statusLabel === tr('project.approved', 'Approved') ? 'approval-approved' : 'approval-rejected',
                    label: tr('project.archiveOverrideStatus', 'Archive override {status}', { status: String(archiveOverride.statusLabel || '').toLowerCase() })
                });
            }
        }

        return badges;
    }

    function renderTaskApprovalBadges(task) {
        var badges = taskApprovalBadges(task);
        if (!badges.length) {
            return '';
        }
        return badges.map(function(item) {
            return '<span class="project-pill ' + escapeHTML(item.className) + '">' + escapeHTML(item.label) + '</span>';
        }).join('');
    }

    function decisions() {
        return state.snapshot && Array.isArray(state.snapshot.decisions) ? state.snapshot.decisions : [];
    }

    function decisionProjectId(item) {
        var taskId = item && (item.taskId || item.taskID);
        var task = taskId ? findById(tasks(), taskId) : null;
        return item && item.projectId ? item.projectId : (task ? task.projectId : '');
    }

    function pendingApprovalCountByKind(kind) {
        return checkpoints().filter(function(item) {
            return item.kind === kind && item.status === 'pending';
        }).length;
    }

    function recentDecisionSummaries(limit, projectId) {
        var items = decisions().filter(function(item) {
            return !projectId || decisionProjectId(item) === projectId;
        }).slice();
        items.sort(function(a, b) {
            var at = String(a.timestamp || '');
            var bt = String(b.timestamp || '');
            if (at === bt) {
                return String(b.id || '').localeCompare(String(a.id || ''));
            }
            return bt.localeCompare(at);
        });
        return items.slice(0, limit || 3);
    }

    function renderApprovalOverviewCard(projectId) {
        var pending = checkpoints().filter(function(item) {
            return item.status === 'pending' && (!projectId || checkpointProjectId(item) === projectId);
        });
        var resolved = checkpoints().filter(function(item) {
            return item.status !== 'pending' && (!projectId || checkpointProjectId(item) === projectId);
        });
        var recent = recentDecisionSummaries(3, projectId);
        return '<div class="project-card-list project-card-visual project-approval-snapshot"><h3>' + escapeHTML(tr('project.approvalOverview', 'Approvals')) + '</h3>'
            + '<div class="project-snapshot-count tone-' + escapeHTML(pending.length ? 'warning' : 'success') + '"><strong>' + escapeHTML(String(pending.length)) + '</strong><span>' + escapeHTML(pending.length ? tr('project.statusReview', 'Review') : tr('project.statusClear', 'Clear')) + '</span></div>'
            + '<div class="project-fact-list compact">'
            + (recent.length
                ? recent.map(function(item) {
                    return '<div class="project-fact-row"><div class="project-fact-label">' + escapeHTML(humanizeToken(item.decisionType || 'decision')) + '</div>'
                        + '<div class="project-fact-value">' + escapeHTML(compactText(item.summary || '—', '', 78)) + '</div></div>';
                }).join('')
                : '<div class="project-list-item muted">' + escapeHTML(tr('project.noRecentDecisions', 'No decisions recorded yet.')) + '</div>')
            + '</div>'
            + '<div class="project-action-group"><button type="button" class="project-inline-btn" data-open-approvals="1">' + escapeHTML(tr('project.openApprovals', 'Open')) + '</button></div>'
            + '</div>';
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

    function renderRuntimeSignals(lines) {
        var items = Array.isArray(lines) ? lines.slice(0, 3) : [];
        if (!items.length) {
            return '<div class="project-list-item muted">' + escapeHTML(tr('project.noRuntimeSignals', 'No runtime signals yet.')) + '</div>';
        }
        return '<div class="project-signal-list">'
            + items.map(function(line, index) {
                var tone = toneFromText(line);
                return '<div class="project-signal-row tone-' + escapeHTML(tone) + '">'
                    + '<span class="project-sidebar-signal tone-' + escapeHTML(tone) + '"></span>'
                    + '<span class="project-signal-copy">' + escapeHTML(compactText(line, '', 76)) + '</span>'
                    + '<span class="project-signal-index">' + escapeHTML(String(index + 1)) + '</span>'
                    + '</div>';
            }).join('')
            + '</div>';
    }

    function renderTimelineStream(lines) {
        var items = Array.isArray(lines) ? lines.slice(0, 3) : [];
        if (!items.length) {
            return '<div class="project-list-item muted">' + escapeHTML(tr('project.noTimeline', 'No timeline entries yet.')) + '</div>';
        }
        return '<div class="project-timeline-stream">'
            + items.map(function(line, index) {
                var tone = toneFromText(line);
                return '<div class="project-timeline-item tone-' + escapeHTML(tone) + '">'
                    + '<div class="project-timeline-marker"><span class="project-timeline-dot tone-' + escapeHTML(tone) + '"></span></div>'
                    + '<div class="project-timeline-body"><div class="project-timeline-label">' + escapeHTML(index === 0 ? tr('project.timelineLatest', 'Latest') : tr('project.timelineEarlier', 'Earlier')) + '</div>'
                    + '<div class="project-timeline-copy">' + escapeHTML(compactText(line, '', 78)) + '</div></div>'
                    + '</div>';
            }).join('')
            + '</div>';
    }

    function acceptanceTone(status) {
        var value = String(status || '').trim().toLowerCase();
        if (value === 'accepted' || value === 'approved' || value === 'complete') {
            return 'success';
        }
        if (value === 'rejected' || value === 'declined') {
            return 'danger';
        }
        if (value === 'pending' || value === 'review' || value === 'needs_review' || value === 'under_human_review') {
            return 'warning';
        }
        return 'neutral';
    }

    function riskTone(level) {
        var value = String(level || '').trim().toLowerCase();
        if (value === 'high' || value === 'critical') {
            return 'danger';
        }
        if (value === 'medium') {
            return 'warning';
        }
        if (value === 'low') {
            return 'success';
        }
        return 'neutral';
    }

    function renderSummaryStat(label, value, tone, extraClass) {
        var displayValue = (value === 0 || value === '0') ? '0' : (value || '—');
        return '<div class="project-summary-stat ' + (tone ? 'tone-' + tone : 'tone-neutral') + (extraClass ? ' ' + extraClass : '') + '">'
            + '<div class="project-summary-stat-label">' + escapeHTML(label) + '</div>'
            + '<div class="project-summary-stat-value">' + escapeHTML(displayValue) + '</div>'
            + '</div>';
    }

    function renderTaskSummaryGrid(task) {
        return '<div class="project-task-summary-grid">'
            + renderSummaryStat(tr('project.state', 'State'), humanizeTaskState(task.state), statusTone(task.state))
            + renderSummaryStat(tr('project.acceptance', 'Acceptance'), humanizeAcceptanceStatus(task.acceptanceStatus), acceptanceTone(task.acceptanceStatus))
            + renderSummaryStat(tr('project.risk', 'Risk'), humanizeRisk(task.riskLevel), riskTone(task.riskLevel))
            + renderSummaryStat(tr('project.nextStep', 'Next step'), taskActionSummary(task), 'info', 'wide')
            + '</div>';
    }

    function renderWorkstreamSummaryGrid(workstream, taskList) {
        var waiting = taskList.filter(function(item) {
            return item.state === 'waiting_human' || item.state === 'waiting_review';
        }).length;
        var blocked = taskList.filter(function(item) {
            return item.state === 'blocked' || item.state === 'failed';
        }).length;
        var nextTask = pickPreferredTask(taskList);
        return '<div class="project-task-summary-grid">'
            + renderSummaryStat(tr('project.state', 'State'), humanizeWorkstreamStatus(workstream.status), statusTone(workstream.status))
            + renderSummaryStat(tr('project.tasks', 'Tasks'), String(taskList.length), taskList.length ? 'info' : 'neutral')
            + renderSummaryStat(tr('project.attention', 'Attention'), String(waiting + blocked), waiting + blocked ? 'warning' : 'success')
            + renderSummaryStat(tr('project.nextStep', 'Next step'), nextTask ? nextTask.title : tr('project.addFirstTask', 'Add the first task'), nextTask ? 'info' : 'neutral', 'wide')
            + '</div>';
    }

    function renderGuideStep(number, title, copy, actionHTML) {
        return '<div class="project-guide-step">'
            + '<div class="project-guide-step-number">' + escapeHTML(String(number)) + '</div>'
            + '<div class="project-guide-step-body"><div class="project-card-title">' + escapeHTML(title) + '</div>'
            + '<div class="project-card-copy">' + escapeHTML(copy) + '</div>'
            + (actionHTML ? '<div class="project-guide-step-actions">' + actionHTML + '</div>' : '')
            + '</div></div>';
    }

    function renderWorkstreamWizard(project) {
        var projectId = project && project.id ? project.id : '';
        var disabled = state.creatingWorkstreamSaving ? ' disabled' : '';
        return '<form class="project-card-list project-workstream-wizard" data-workstream-wizard="' + escapeHTML(projectId) + '">'
            + '<h3>' + escapeHTML(tr('project.newWorkstream', 'New workstream')) + '</h3>'
            + '<div class="project-wizard-fields">'
            + '<label class="project-wizard-field"><span>' + escapeHTML(tr('project.workstreamName', 'Name')) + '</span>'
            + '<input type="text" name="title" maxlength="120" data-workstream-title-input value="' + escapeHTML(state.creatingWorkstreamTitle || '') + '" placeholder="' + escapeHTML(tr('project.workstreamNamePlaceholder', 'Name')) + '"' + disabled + '></label>'
            + '<label class="project-wizard-field"><span>' + escapeHTML(tr('project.workstreamScope', 'Scope')) + '</span>'
            + '<input type="text" name="scopeSummary" maxlength="220" data-workstream-scope-input value="' + escapeHTML(state.creatingWorkstreamScope || '') + '" placeholder="' + escapeHTML(tr('project.workstreamScopePlaceholder', 'Scope (optional)')) + '"' + disabled + '></label>'
            + '</div>'
            + '<div class="project-action-group">'
            + '<button type="button" class="project-inline-btn" data-cancel-workstream-wizard="1"' + disabled + '>' + escapeHTML(tr('project.cancel', 'Cancel')) + '</button>'
            + '<button type="submit" class="project-inline-btn primary"' + disabled + '>' + escapeHTML(state.creatingWorkstreamSaving ? tr('project.saving', 'Saving...') : tr('project.create', 'Create')) + '</button>'
            + '</div>'
            + '</form>';
    }

    function renderTaskWizard(workstream) {
        var workstreamId = workstream && workstream.id ? workstream.id : '';
        var selectedSkillId = state.creatingTaskSelectedSkill || (defaultSkill().id || 'code_change');
        var selectedSkill = skillForId(selectedSkillId);
        var selectedRunbook = runbookForSkill(selectedSkill.id, state.creatingTaskRunbookId);
        var disabled = state.creatingTaskSaving ? ' disabled' : '';
        var skillOptions = skills().length ? skills() : [defaultSkill()];
        return '<form class="project-card-list project-workstream-wizard project-task-wizard" data-task-wizard="' + escapeHTML(workstreamId) + '">'
            + '<h3>' + escapeHTML(tr('project.newTask', 'New task')) + '</h3>'
            + '<div class="project-wizard-fields">'
            + '<label class="project-wizard-field"><span>' + escapeHTML(tr('project.taskTitle', 'Title')) + '</span>'
            + '<input type="text" name="title" maxlength="140" data-task-title-input value="' + escapeHTML(state.creatingTaskTitle || '') + '" placeholder="' + escapeHTML(tr('project.taskTitlePlaceholder', 'Task title')) + '"' + disabled + '></label>'
            + '<label class="project-wizard-field"><span>' + escapeHTML(tr('project.taskGoal', 'Goal')) + '</span>'
            + '<input type="text" name="goal" maxlength="260" data-task-goal-input value="' + escapeHTML(state.creatingTaskGoal || '') + '" placeholder="' + escapeHTML(tr('project.taskGoalPlaceholder', 'Goal (optional)')) + '"' + disabled + '></label>'
            + '<label class="project-wizard-field"><span>' + escapeHTML(tr('project.skill', 'Skill')) + '</span>'
            + '<select name="selectedSkill" data-task-skill-select' + disabled + '>'
            + skillOptions.map(function(skill) {
                var selected = skill.id === selectedSkill.id ? ' selected' : '';
                return '<option value="' + escapeHTML(skill.id) + '"' + selected + '>' + escapeHTML(humanizeSkill(skill)) + '</option>';
            }).join('')
            + '</select></label>'
            + '<div class="project-wizard-meta"><span>' + escapeHTML(tr('project.runbook', 'Runbook')) + '</span><strong>' + escapeHTML(selectedRunbook.id || '') + '</strong></div>'
            + '</div>'
            + '<div class="project-action-group">'
            + '<button type="button" class="project-inline-btn" data-cancel-task-wizard="1"' + disabled + '>' + escapeHTML(tr('project.cancel', 'Cancel')) + '</button>'
            + '<button type="submit" class="project-inline-btn primary"' + disabled + '>' + escapeHTML(state.creatingTaskSaving ? tr('project.saving', 'Saving...') : tr('project.create', 'Create')) + '</button>'
            + '</div>'
            + '</form>';
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

    function renderFocusCard(options) {
        var tone = options && options.tone ? options.tone : 'neutral';
        var kicker = options && options.kicker ? options.kicker : tr('project.focusKicker', 'Focus');
        var title = options && options.title ? options.title : tr('project.focusDefaultTitle', 'Review this first');
        var copy = options && options.copy ? options.copy : '';
        var note = options && options.note ? options.note : '';
        var actionsHTML = options && options.actionsHTML ? options.actionsHTML : '';
        var badgesHTML = ((options && options.badges) || []).filter(Boolean).join('');

        return '<div class="project-card-list project-card-visual project-focus-card tone-' + escapeHTML(tone) + '">'
            + '<div class="project-focus-topline">'
            + '<div class="project-focus-indicator tone-' + escapeHTML(tone) + '" aria-hidden="true"></div>'
            + '<div class="project-focus-copy"><div class="project-section-kicker">' + escapeHTML(kicker) + '</div>'
            + '<h3>' + escapeHTML(title) + '</h3>'
            + (copy ? '<div class="project-detail-copy">' + escapeHTML(compactText(copy, '', 110)) + '</div>' : '')
            + '</div>'
            + (badgesHTML ? '<div class="project-task-badges project-focus-badges">' + badgesHTML + '</div>' : '')
            + '</div>'
            + (note ? '<div class="project-focus-note">' + escapeHTML(compactText(note, '', 120)) + '</div>' : '')
            + (actionsHTML ? '<div class="project-action-group">' + actionsHTML + '</div>' : '')
            + '</div>';
    }

    function renderProjectFocusCard(project, projectSnapshot) {
        var pendingItem = projectSnapshot.pendingApprovals[0] || null;
        var blockedTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'blocked' || task.state === 'failed';
        }));
        var waitingTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'waiting_human' || task.state === 'waiting_review';
        }));
        var runningTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'running';
        }));
        var readyTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'execution_complete' && task.acceptanceStatus !== 'accepted';
        }));
        var nextTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'planned' || task.state === 'queued';
        }));
        var focusWorkstream = projectSnapshot.focusWorkstream;
        var context = null;
        var focusWorkstreamForTask = null;
        var tone = 'neutral';
        var title = '';
        var copy = '';
        var note = '';
        var actionsHTML = '';
        var badges = [
            renderProjectPill(tr('project.countLive', '{count} live', { count: projectSnapshot.runningWorkstreams }), projectSnapshot.runningWorkstreams ? 'info' : ''),
            renderProjectPill(tr('project.countWaitingShort', '{count} wait', { count: projectSnapshot.waitingTasks }), projectSnapshot.waitingTasks ? 'warning' : ''),
            renderProjectPill(tr('project.countBlockedShort', '{count} block', { count: projectSnapshot.blockedTasks }), projectSnapshot.blockedTasks ? 'danger' : ''),
            renderProjectPill(tr('project.countReview', '{count} review', { count: projectSnapshot.pendingApprovals.length }), projectSnapshot.pendingApprovals.length ? 'warning' : '')
        ];

        if (!projectSnapshot.workstreams.length && !projectSnapshot.tasks.length && !projectSnapshot.pendingApprovals.length) {
            return renderQuickStartCard(project);
        }

        if (pendingItem) {
            context = approvalContext(pendingItem);
            tone = 'warning';
            title = projectSnapshot.pendingApprovals.length === 1
                ? tr('project.focusDecisionWaitingSingular', 'Decision waiting')
                : tr('project.focusDecisionWaitingPlural', '{count} decisions waiting', { count: projectSnapshot.pendingApprovals.length });
            copy = context.task ? context.task.title : approvalKindSummary(pendingItem);
            note = tr('project.focusHumanSignOffRequired', 'Human sign-off required.');
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-open-approvals="1">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>'
                + (context.task ? '<button type="button" class="project-inline-btn" data-task-id="' + escapeHTML(context.task.id) + '">' + escapeHTML(tr('project.taskSingular', 'Task')) + '</button>' : '');
        } else if (blockedTask) {
            focusWorkstreamForTask = findById(workstreams(), blockedTask.workstreamId);
            tone = 'danger';
            title = tr('project.statusBlocked', 'Blocked');
            copy = blockedTask.title;
            note = taskActionSummary(blockedTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(blockedTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
                + (focusWorkstreamForTask ? '<button type="button" class="project-inline-btn" data-workstream-id="' + escapeHTML(focusWorkstreamForTask.id) + '">' + escapeHTML(workstreamLabel(false)) + '</button>' : '');
        } else if (waitingTask) {
            tone = 'warning';
            title = tr('project.statusWaiting', 'Waiting');
            copy = waitingTask.title;
            note = taskActionSummary(waitingTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(waitingTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
                + ((waitingTask.acceptanceStatus === 'ready_for_acceptance' || waitingTask.acceptanceStatus === 'under_human_review')
                    ? '<button type="button" class="project-inline-btn" data-open-approvals="1">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>'
                    : '');
        } else if (runningTask) {
            tone = 'info';
            title = tr('project.focusLiveTask', 'Live task');
            copy = runningTask.title;
            note = taskActionSummary(runningTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(runningTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-open-terminal-mode="1">' + escapeHTML(tr('header.terminal', 'Terminal')) + '</button>';
        } else if (readyTask) {
            tone = 'success';
            title = tr('project.statusReady', 'Ready');
            copy = readyTask.title;
            note = taskActionSummary(readyTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(readyTask.id) + '">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>';
        } else if (nextTask) {
            tone = 'info';
            title = tr('project.focusNextTask', 'Next task');
            copy = nextTask.title;
            note = taskActionSummary(nextTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(nextTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>';
        } else if (focusWorkstream) {
            tone = statusTone(focusWorkstream.status);
            title = tr('project.focusNextWorkstream', 'Next workstream');
            copy = focusWorkstream.title;
            note = focusWorkstream.scopeSummary || focusWorkstream.description || workstreamFlowSummary(focusWorkstream);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-workstream-id="' + escapeHTML(focusWorkstream.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(focusWorkstream.id) + '">' + escapeHTML(tr('project.add', 'Add')) + '</button>';
        }

        return renderFocusCard({
            kicker: tr('project.focusKicker', 'Focus'),
            title: title,
            copy: copy,
            note: note,
            tone: tone,
            badges: badges,
            actionsHTML: actionsHTML
        });
    }

    function renderApprovalFocusCard(pending, resolved) {
        var ordered = (pending || []).slice().sort(function(a, b) {
            var aPriority = a.kind === 'final_acceptance' ? 0 : 1;
            var bPriority = b.kind === 'final_acceptance' ? 0 : 1;
            if (aPriority !== bPriority) {
                return aPriority - bPriority;
            }
            return String(b.requestedAt || '').localeCompare(String(a.requestedAt || ''));
        });
        var item = ordered[0] || null;
        var context = item ? approvalContext(item) : null;
        var badges = [
            renderProjectPill(tr('project.countPending', '{count} pending', { count: (pending || []).length }), (pending || []).length ? 'warning' : 'success'),
            renderProjectPill(tr('project.countResolved', '{count} resolved', { count: (resolved || []).length }), (resolved || []).length ? 'success' : ''),
            item ? renderProjectPill(approvalKindLabel(item.kind), approvalTone(item)) : ''
        ];
        var actionsHTML = '';

        if (!item) {
            return renderFocusCard({
                kicker: tr('project.approvalQueue', 'Approval queue'),
                title: tr('project.noApprovalsWaiting', 'No approvals are waiting'),
                copy: tr('project.noApprovalsWaitingCopy', 'Nothing is blocked on a human decision right now.'),
                tone: 'success',
                badges: badges
            });
        }

        if (context && context.project) {
            badges.unshift(renderProjectPill(context.project.name, '', 'outline'));
        }
        if (context && context.workstream) {
            badges.unshift(renderProjectPill(context.workstream.title, '', 'outline'));
        }
        actionsHTML = context && context.task
            ? '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(context.task.id) + '">' + escapeHTML(tr('project.openTask', 'Open task')) + '</button>'
            : '';

        return renderFocusCard({
            kicker: tr('project.startHere', 'Start here'),
            title: (pending || []).length === 1 ? tr('project.reviewApprovalNext', 'Review this approval next') : tr('project.approvalsNeedDecisions', '{count} approvals need decisions', { count: (pending || []).length }),
            copy: context && context.task
                ? tr('project.taskWaitingOnApproval', '{title} is waiting on {kind}.', { title: context.task.title, kind: approvalKindLabel(item.kind).toLowerCase() })
                : approvalKindSummary(item),
            note: item.reason || approvalKindSummary(item),
            tone: approvalTone(item),
            badges: badges,
            actionsHTML: actionsHTML
        });
    }

    function renderQuickStartCard(project) {
        if (state.creatingWorkstreamProjectId === project.id) {
            return renderWorkstreamWizard(project);
        }
        return '<div class="project-card-list project-empty-workstream-card">'
            + '<h3>' + escapeHTML(tr('project.noWorkstreamsYet', 'No workstreams')) + '</h3>'
            + '<button type="button" class="project-inline-btn primary" data-create-workstream="' + escapeHTML(project.id) + '">' + escapeHTML(tr('project.newWorkstreamShort', '+ Workstream')) + '</button>'
            + '</div>';
    }

    function renderWorkstreamGuide(workstream, taskList) {
        var waitingCount = taskList.filter(function(item) { return item.state === 'waiting_human' || item.state === 'waiting_review'; }).length;
        var blockedTask = pickPreferredTask(taskList.filter(function(item) {
            return item.state === 'blocked' || item.state === 'failed';
        }));
        var waitingTask = pickPreferredTask(taskList.filter(function(item) {
            return item.state === 'waiting_human' || item.state === 'waiting_review';
        }));
        var runningTask = pickPreferredTask(taskList.filter(function(item) {
            return item.state === 'running';
        }));
        var nextTask = pickPreferredTask(taskList.filter(function(item) {
            return item.state === 'planned' || item.state === 'queued';
        }));
        var doneTask = pickPreferredTask(taskList.filter(function(item) {
            return item.state === 'execution_complete' && item.acceptanceStatus !== 'accepted';
        }));
        var tone = statusTone(workstream.status);
        var title = tr('project.guideWorkstreamMovingTitle', 'Keep this workstream moving');
        var copy = tr('project.guideWorkstreamMovingCopy', 'Add the next task, open it, then come back only when the workstream needs a decision.');
        var note = workstream.scopeSummary || workstream.description || workstreamFlowSummary(workstream);
        var actionsHTML = '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';

        if (blockedTask) {
            tone = 'danger';
            title = tr('project.guideResolveBlockerTitle', 'Resolve the blocker first');
            copy = tr('project.guideBlockedCopy', '{title} is stopping this workstream.', { title: blockedTask.title });
            note = taskActionSummary(blockedTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(blockedTask.id) + '">' + escapeHTML(tr('project.openBlockedTask', 'Open blocked task')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';
        } else if (waitingTask) {
            tone = 'warning';
            title = tr('project.guideDecisionWaitingTitle', 'A decision is waiting');
            copy = tr('project.guideWaitingCopy', '{title} needs review or explicit input before this workstream can move cleanly.', { title: waitingTask.title });
            note = taskActionSummary(waitingTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(waitingTask.id) + '">' + escapeHTML(tr('project.openWaitingTask', 'Open waiting task')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';
        } else if (runningTask) {
            tone = 'info';
            title = tr('project.guideContinueLiveTitle', 'Continue the live task');
            copy = tr('project.guideRunningCopy', '{title} is already active inside this workstream.', { title: runningTask.title });
            note = taskActionSummary(runningTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(runningTask.id) + '">' + escapeHTML(tr('project.openLiveTask', 'Open live task')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';
        } else if (doneTask) {
            tone = 'success';
            title = tr('project.guideCloseFinishedTitle', 'Close out finished work');
            copy = tr('project.guideDoneCopy', '{title} is done executing and needs follow-through.', { title: doneTask.title });
            note = taskActionSummary(doneTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(doneTask.id) + '">' + escapeHTML(tr('project.reviewFinishedTask', 'Review finished task')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';
        } else if (nextTask) {
            tone = 'info';
            title = tr('project.guideStartNextTitle', 'Start the next task');
            copy = tr('project.guideNextCopy', '{title} is ready to be picked up next.', { title: nextTask.title });
            note = taskActionSummary(nextTask);
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(nextTask.id) + '">' + escapeHTML(tr('project.openNextTask', 'Open next task')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button>';
        } else if (!taskList.length) {
            tone = 'neutral';
            title = tr('project.addFirstTask', 'Add the first task');
            copy = tr('project.guideEmptyWorkstreamCopy', 'This workstream exists, but nothing concrete is queued yet.');
            note = tr('project.guideEmptyWorkstreamNote', 'Start with the next smallest executable step.');
            actionsHTML = '<button type="button" class="project-inline-btn primary" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addFirstTask', 'Add first task')) + '</button>';
        }

        return renderFocusCard({
            kicker: tr('project.workstreamFocus', '{workstream} focus', { workstream: workstreamLabel(false) }),
            title: title,
            copy: copy,
            note: note,
            tone: tone,
            badges: [
                renderProjectPill(humanizeWorkstreamStatus(workstream.status), statusTone(workstream.status)),
                renderProjectPill(taskCountText(taskList.length)),
                renderProjectPill(tr('project.countWaiting', '{count} waiting', { count: waitingCount }), waitingCount ? 'warning' : ''),
                renderProjectPill(tr('project.priorityLabel', '{priority} priority', { priority: humanizePriority(workstream.priority) }))
            ],
            actionsHTML: actionsHTML
        });
    }

    function renderTaskExecutionGuide(task, taskSessions) {
        var workstream = findById(workstreams(), task.workstreamId);
        var session = primaryExecutionSession(taskSessions);
        var header = tr('project.runThisTask', 'Run this task');
        var copy = '';
        var note = task.nextStep || '';
        var actions = '';
        var tone = 'info';

        if (task.state === 'execution_complete') {
            header = tr('project.executionIsDone', 'Execution is done');
            copy = task.acceptanceStatus === 'accepted'
                ? tr('project.executionAcceptedCopy', 'Accepted. Archive when ready.')
                : tr('project.executionReviewCopy', 'Review or reopen.');
            tone = task.acceptanceStatus === 'accepted' ? 'success' : 'warning';
        } else if (session && session.supportsAttach && session.terminalId) {
            header = tr('project.liveTerminalReady', 'Live terminal is ready');
            copy = tr('project.continueInTerminal', 'Continue in Terminal.');
            actions = '<button type="button" class="project-inline-btn primary" data-attach-terminal="' + escapeHTML(session.terminalId) + '">' + escapeHTML(tr('project.openLiveTerminal', 'Open live terminal')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-session-id="' + escapeHTML(session.id) + '">' + escapeHTML(tr('project.sessionDetails', 'Session details')) + '</button>';
            tone = 'info';
        } else if (session) {
            header = tr('project.sessionHistoryAttached', 'Session history is attached');
            copy = tr('project.sessionHistoryCopy', 'Open the session or continue in Terminal.');
            actions = '<button type="button" class="project-inline-btn" data-session-id="' + escapeHTML(session.id) + '">' + escapeHTML(tr('project.sessionDetails', 'Session details')) + '</button>'
                + '<button type="button" class="project-inline-btn" data-open-terminal-mode="1">' + escapeHTML(tr('project.openTerminalWorkspace', 'Open Terminal workspace')) + '</button>';
            tone = 'neutral';
        } else {
            copy = task.state === 'running'
                ? tr('project.runningNoTerminal', 'Running, but no live terminal is attached.')
                : tr('project.startOrContinueTerminal', 'Start or continue in Terminal.');
            actions = '<button type="button" class="project-inline-btn primary" data-open-terminal-mode="1">' + escapeHTML(tr('project.openTerminalWorkspace', 'Open Terminal workspace')) + '</button>';
            tone = task.state === 'running' ? 'warning' : 'info';
        }

        return renderFocusCard({
            kicker: tr('project.execution', 'Execution'),
            title: header,
            copy: copy,
            note: note ? tr('project.nextPrefix', 'Next: {text}', { text: compactText(note, '', 70) }) : '',
            tone: tone,
            badges: [
                renderProjectPill(humanizeTaskState(task.state), statusTone(task.state)),
                renderProjectPill((workstream ? workstream.title : workstreamLabel(false)) || workstreamLabel(false), '', 'outline'),
                renderProjectPill(sessionCountText(taskSessions.length))
            ],
            actionsHTML: actions
        });
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
                    + '<div class="project-activity-marker"><span class="project-timeline-dot tone-' + escapeHTML(tone) + '"></span></div>'
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

    function renderApprovalQueueSummary(pending, resolved) {
        var pendingFinal = pending.filter(function(item) { return item.kind === 'final_acceptance'; }).length;
        var pendingArchive = pending.filter(function(item) { return item.kind === 'archive_override'; }).length;
        return '<div class="project-queue-summary-grid">'
            + renderSummaryStat(tr('project.pending', 'Pending'), String(pending.length), pending.length ? 'warning' : 'success')
            + renderSummaryStat(tr('project.resolved', 'Resolved'), String(resolved.length), resolved.length ? 'success' : 'neutral')
            + renderSummaryStat(tr('project.finalAcceptance', 'Final acceptance'), String(pendingFinal), pendingFinal ? 'warning' : 'neutral')
            + renderSummaryStat(tr('project.archiveOverride', 'Archive override'), String(pendingArchive), pendingArchive ? 'info' : 'neutral')
            + '</div>';
    }

    function formatCompactPriority(priority) {
        var value = String(priority || '').trim().toLowerCase();
        if (!value) {
            return 'M';
        }
        if (value === 'high') {
            return 'H';
        }
        if (value === 'low') {
            return 'L';
        }
        return 'M';
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

    function countTasks(workstreamId, stateName) {
        return tasksForWorkstream(workstreamId).filter(function(task) {
            return !stateName || task.state === stateName;
        }).length;
    }

    function countTasksByStates(workstreamId, stateNames) {
        var states = Array.isArray(stateNames) ? stateNames : [stateNames];
        return tasksForWorkstream(workstreamId).filter(function(task) {
            return states.indexOf(task.state) !== -1;
        }).length;
    }

    function prioritySortValue(priority) {
        switch (String(priority || '').trim().toLowerCase()) {
        case 'critical':
            return 0;
        case 'high':
            return 1;
        case 'low':
            return 3;
        default:
            return 2;
        }
    }

    function workstreamAttentionRank(item) {
        var blocked = countTasksByStates(item.id, ['blocked', 'failed']);
        var waiting = countTasksByStates(item.id, ['waiting_human', 'waiting_review']);
        var running = countTasksByStates(item.id, ['running']);
        if (blocked || item.status === 'blocked') {
            return 0;
        }
        if (waiting || item.status === 'waiting_human') {
            return 1;
        }
        if (running || item.status === 'running') {
            return 2;
        }
        if (item.status === 'planned') {
            return 3;
        }
        if (item.status === 'completed' || item.status === 'execution_complete') {
            return 4;
        }
        if (item.status === 'archived') {
            return 6;
        }
        return 5;
    }

    function sortWorkstreamsByAttention(items) {
        return (items || []).slice().sort(function(a, b) {
            var byAttention = workstreamAttentionRank(a) - workstreamAttentionRank(b);
            var byPriority;
            if (byAttention !== 0) {
                return byAttention;
            }
            byPriority = prioritySortValue(a.priority) - prioritySortValue(b.priority);
            if (byPriority !== 0) {
                return byPriority;
            }
            return String(a.title || '').localeCompare(String(b.title || ''));
        });
    }

    function workstreamFlowSummary(item) {
        var running = countTasksByStates(item.id, ['running']);
        var waiting = countTasksByStates(item.id, ['waiting_human', 'waiting_review']);
        var blocked = countTasksByStates(item.id, ['blocked', 'failed']);
        var total = countTasks(item.id);
        var parts = [];
        if (running) {
            parts.push(tr('project.countLive', '{count} live', { count: running }));
        }
        if (waiting) {
            parts.push(tr('project.countWaiting', '{count} waiting', { count: waiting }));
        }
        if (blocked) {
            parts.push(tr('project.countBlocked', '{count} blocked', { count: blocked }));
        }
        if (!parts.length) {
            parts.push(total ? tr('project.countTracked', '{count} tracked', { count: total }) : tr('project.noTasksYet', 'No tasks yet'));
        }
        return parts.join(' • ');
    }

    function renderStatusMeter(segments) {
        var total = (segments || []).reduce(function(sum, item) {
            return sum + (Number(item.count) || 0);
        }, 0);
        if (!total) {
            return '<div class="project-workstream-meter empty"></div>';
        }
        return '<div class="project-workstream-meter">'
            + segments.map(function(item) {
                var count = Number(item.count) || 0;
                if (!count) {
                    return '';
                }
                return '<span class="tone-' + escapeHTML(item.tone || 'neutral') + '" style="flex:' + escapeHTML(String(count)) + ' 1 0" title="' + escapeHTML(String(count) + ' ' + (item.label || 'items')) + '"></span>';
            }).join('')
            + '</div>';
    }

    function renderMiniCount(value, label, tone) {
        return '<span class="project-workstream-stat tone-' + escapeHTML(tone || 'neutral') + '">'
            + '<strong>' + escapeHTML(String(value)) + '</strong><span>' + escapeHTML(label) + '</span>'
            + '</span>';
    }

    function countProjectTasksByStates(taskList, stateNames) {
        var states = Array.isArray(stateNames) ? stateNames : [stateNames];
        return (taskList || []).filter(function(task) {
            return states.indexOf(task.state) !== -1;
        }).length;
    }

    function shortRiskLabel(risk) {
        switch (String(risk || '').trim().toLowerCase()) {
        case 'critical':
            return 'C';
        case 'high':
            return 'H';
        case 'low':
            return 'L';
        default:
            return 'M';
        }
    }

    function taskNextActionLabel(task) {
        if (!task) {
            return tr('project.statusOpen', 'Open');
        }
        if (task.acceptanceStatus === 'ready_for_acceptance' || task.acceptanceStatus === 'under_human_review') {
            return tr('project.statusReview', 'Review');
        }
        if (task.acceptanceStatus === 'rejected') {
            return tr('project.statusFix', 'Fix');
        }
        switch (task.state) {
        case 'planned':
        case 'queued':
            return tr('project.statusStart', 'Start');
        case 'running':
            return tr('project.statusContinue', 'Continue');
        case 'waiting_review':
            return tr('project.statusReview', 'Review');
        case 'waiting_human':
            return tr('project.statusInput', 'Input');
        case 'blocked':
        case 'failed':
            return tr('project.statusUnblock', 'Unblock');
        case 'execution_complete':
            return task.acceptanceStatus === 'accepted' ? tr('project.statusArchive', 'Archive') : tr('project.statusClose', 'Close');
        case 'archived':
            return tr('project.statusArchived', 'Archived');
        default:
            return tr('project.statusOpen', 'Open');
        }
    }

    function taskWorkstreamName(task) {
        var workstream = task && task.workstreamId ? findById(workstreams(), task.workstreamId) : null;
        return workstream ? workstream.title : workstreamLabel(false);
    }

    function taskAgentName(task) {
        if (task && task.agentLabel) {
            return humanizeAgentLabel(task.agentLabel);
        }
        return compactText(taskWorkstreamName(task), tr('project.agentFallback', 'Agent'), 28);
    }

    function sortTasksForManager(taskList) {
        return (taskList || []).slice().sort(function(a, b) {
            var byState = taskSortPriority(a) - taskSortPriority(b);
            var byPriority;
            if (byState !== 0) {
                return byState;
            }
            byPriority = prioritySortValue(a.priority) - prioritySortValue(b.priority);
            if (byPriority !== 0) {
                return byPriority;
            }
            return String(a.title || '').localeCompare(String(b.title || ''));
        });
    }

    function renderManagerStat(label, value, tone) {
        return '<div class="project-manager-stat tone-' + escapeHTML(tone || 'neutral') + '">'
            + '<span class="project-manager-stat-value">' + escapeHTML(String(value)) + '</span>'
            + '<span class="project-manager-stat-label">' + escapeHTML(label) + '</span>'
            + '</div>';
    }

    function renderManagerStats(items) {
        return '<div class="project-manager-stats">'
            + (items || []).map(function(item) {
                return renderManagerStat(item.label, item.value, item.tone);
            }).join('')
            + '</div>';
    }

    function renderCommandStat(label, value, tone) {
        return '<div class="project-command-stat tone-' + escapeHTML(tone || 'neutral') + '">'
            + '<span class="project-command-stat-value">' + escapeHTML(String(value)) + '</span>'
            + '<span class="project-command-stat-label">' + escapeHTML(label) + '</span>'
            + '</div>';
    }

    function renderCommandStats(items) {
        return '<div class="project-command-stats">'
            + (items || []).map(function(item) {
                return renderCommandStat(item.label, item.value, item.tone);
            }).join('')
            + '</div>';
    }

    function laneAttentionCounts(workstreamId) {
        return {
            running: countTasksByStates(workstreamId, ['running']),
            waiting: countTasksByStates(workstreamId, ['waiting_human', 'waiting_review']),
            blocked: countTasksByStates(workstreamId, ['blocked', 'failed']),
            next: countTasksByStates(workstreamId, ['planned', 'queued']),
            done: countTasksByStates(workstreamId, ['execution_complete'])
        };
    }

    function renderCommandCountChip(value, label, tone) {
        if (!value) {
            return '';
        }
        return '<span class="project-command-chip tone-' + escapeHTML(tone || 'neutral') + '">'
            + '<strong>' + escapeHTML(String(value)) + '</strong>'
            + '<span>' + escapeHTML(label) + '</span>'
            + '</span>';
    }

    function renderCommandHero(project, projectSnapshot) {
        var focus = resolveProjectFocus(projectSnapshot);
        var context = taskCountText(projectSnapshot.tasks.length) + ' / ' + workstreamCountText(projectSnapshot.workstreams.length);
        return '<div class="project-command-hero tone-' + escapeHTML(focus.tone) + '">'
            + '<div class="project-command-hero-main">'
            + '<div class="project-command-hero-mark">' + renderSidebarSignal(focus.tone) + '</div>'
            + '<div class="project-command-hero-copy">'
            + '<div class="project-section-kicker">' + escapeHTML(tr('project.nextAction', 'Next action')) + '</div>'
            + '<h3>' + escapeHTML(compactText(focus.title, project.name, 78)) + '</h3>'
            + '<p>' + escapeHTML(compactText(focus.meta || context, context, 96)) + '</p>'
            + '</div>'
            + '</div>'
            + '<div class="project-command-hero-side">'
            + '<span class="project-command-focus-label tone-' + escapeHTML(focus.tone) + '">' + escapeHTML(focus.label) + '</span>'
            + '<span class="project-command-context">' + escapeHTML(context) + '</span>'
            + (focus.actionHTML ? '<div class="project-action-group">' + focus.actionHTML + '</div>' : '')
            + '</div>'
            + '</div>';
    }

    function renderCommandLaneRow(workstream, index) {
        var counts = laneAttentionCounts(workstream.id);
        var taskList = tasksForWorkstream(workstream.id);
        var currentTask = pickPreferredTask(taskList);
        var tone = counts.blocked ? 'danger' : (counts.waiting ? 'warning' : (counts.running ? 'info' : statusTone(workstream.status)));
        var chips = [
            renderCommandCountChip(counts.blocked, tr('project.blockShort', 'block'), 'danger'),
            renderCommandCountChip(counts.waiting, tr('project.waitShort', 'wait'), 'warning'),
            renderCommandCountChip(counts.running, tr('project.runShort', 'run'), 'info')
        ].join('');
        if (!chips) {
            chips = '<span class="project-command-chip tone-success"><strong>' + escapeHTML(String(taskList.length)) + '</strong><span>' + escapeHTML(taskList.length === 1 ? tr('project.taskSingular', 'task') : tr('project.taskPlural', 'tasks')) + '</span></span>';
        }
        return '<button type="button" class="project-command-lane-row tone-' + escapeHTML(tone) + '" data-workstream-id="' + escapeHTML(workstream.id) + '">'
            + '<span class="project-command-lane-index">' + escapeHTML(index < 9 ? '0' + String(index + 1) : String(index + 1)) + '</span>'
            + '<span class="project-command-lane-main">'
            + '<span class="project-command-lane-title">' + escapeHTML(compactText(workstream.title, workstreamLabel(false), 42)) + '</span>'
            + '<span class="project-command-lane-task">' + escapeHTML(currentTask ? compactText(currentTask.title, '', 64) : tr('project.noActiveTask', 'No active task')) + '</span>'
            + '</span>'
            + '<span class="project-command-lane-chips">' + chips + '</span>'
            + '</button>';
    }

    function renderCommandLanes(projectSnapshot) {
        var lanes = projectSnapshot.workstreams || [];
        var attentionLanes = lanes.filter(function(workstream) {
            var counts = laneAttentionCounts(workstream.id);
            return counts.blocked || counts.waiting || counts.running;
        });
        var visibleLanes = (attentionLanes.length ? attentionLanes : lanes).slice(0, 4);
        var hiddenLanes = lanes.filter(function(workstream) {
            return visibleLanes.indexOf(workstream) === -1;
        });
        var title = attentionLanes.length ? tr('project.needsAttention', 'Needs attention') : tr('project.workstreamLanes', 'Workstreams');

        if (!lanes.length) {
            return '';
        }
        return '<div class="project-command-lanes">'
            + '<div class="project-manager-block-head"><h3>' + escapeHTML(title) + '</h3><span>' + escapeHTML(tr('project.countTotal', '{count} total', { count: lanes.length })) + '</span></div>'
            + '<div class="project-command-lane-list">'
            + visibleLanes.map(function(workstream, index) {
                return renderCommandLaneRow(workstream, index);
            }).join('')
            + '</div>'
            + (hiddenLanes.length
                ? '<details class="project-disclosure project-disclosure-compact"><summary><span>' + escapeHTML(tr('project.showCalmerLanes', 'Show calmer lanes')) + '</span><small>' + escapeHTML(tr('project.countHidden', '{count} hidden', { count: hiddenLanes.length })) + '</small></summary>'
                    + '<div class="project-command-lane-list">'
                    + hiddenLanes.map(function(workstream, index) {
                        return renderCommandLaneRow(workstream, visibleLanes.length + index);
                    }).join('')
                    + '</div></details>'
                : '')
            + '</div>';
    }

    function renderCollapsedLaneStates(columns) {
        if (!columns.length) {
            return '';
        }
        return '<div class="project-lane-collapsed-states">'
            + columns.map(function(column) {
                return '<span class="project-lane-state-chip tone-' + escapeHTML(column.tone) + '">'
                    + escapeHTML(column.label) + '<strong>0</strong></span>';
            }).join('')
            + '</div>';
    }

    function resolveProjectFocus(projectSnapshot) {
        var pendingItem = projectSnapshot.pendingApprovals[0] || null;
        var blockedTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'blocked' || task.state === 'failed';
        }));
        var waitingTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'waiting_human' || task.state === 'waiting_review';
        }));
        var runningTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'running';
        }));
        var readyTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'execution_complete' && task.acceptanceStatus !== 'accepted';
        }));
        var nextTask = pickPreferredTask(projectSnapshot.tasks.filter(function(task) {
            return task.state === 'planned' || task.state === 'queued';
        }));
        var context;

        if (pendingItem) {
            context = approvalContext(pendingItem);
            return {
                tone: 'warning',
                label: tr('project.statusReview', 'Review'),
                title: context.task ? context.task.title : approvalKindLabel(pendingItem.kind),
                meta: approvalKindLabel(pendingItem.kind),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-open-approvals="1">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>'
            };
        }
        if (blockedTask) {
            return {
                tone: 'danger',
                label: tr('project.statusBlocked', 'Blocked'),
                title: blockedTask.title,
                meta: taskWorkstreamName(blockedTask),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(blockedTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
            };
        }
        if (waitingTask) {
            return {
                tone: 'warning',
                label: tr('project.statusWaiting', 'Waiting'),
                title: waitingTask.title,
                meta: taskWorkstreamName(waitingTask),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(waitingTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
            };
        }
        if (runningTask) {
            return {
                tone: 'info',
                label: tr('project.statusLive', 'Live'),
                title: runningTask.title,
                meta: taskWorkstreamName(runningTask),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(runningTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
                    + '<button type="button" class="project-inline-btn" data-open-terminal-mode="1">' + escapeHTML(tr('header.terminal', 'Terminal')) + '</button>'
            };
        }
        if (readyTask) {
            return {
                tone: 'success',
                label: tr('project.statusClose', 'Close'),
                title: readyTask.title,
                meta: taskWorkstreamName(readyTask),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(readyTask.id) + '">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>'
            };
        }
        if (nextTask) {
            return {
                tone: 'info',
                label: tr('project.statusNext', 'Next'),
                title: nextTask.title,
                meta: taskWorkstreamName(nextTask),
                actionHTML: '<button type="button" class="project-inline-btn primary" data-task-id="' + escapeHTML(nextTask.id) + '">' + escapeHTML(tr('project.statusOpen', 'Open')) + '</button>'
            };
        }
        return {
            tone: 'success',
            label: tr('project.statusClear', 'Clear'),
            title: projectSnapshot.tasks.length ? tr('project.noUrgentTask', 'No urgent task') : tr('project.noTasksYet', 'No tasks yet'),
            meta: projectSnapshot.tasks.length ? tr('project.allTrackedCalm', 'All tracked work is calm') : tr('project.createWorkstreamStart', 'Add a task to start'),
            actionHTML: ''
        };
    }

    function renderManagerFocusStrip(projectSnapshot) {
        var focus = resolveProjectFocus(projectSnapshot);
        return '<div class="project-manager-focus tone-' + escapeHTML(focus.tone) + '">'
            + '<div class="project-manager-focus-main">'
            + renderSidebarSignal(focus.tone)
            + '<span class="project-manager-focus-label">' + escapeHTML(focus.label) + '</span>'
            + '<strong>' + escapeHTML(compactText(focus.title, '', 54)) + '</strong>'
            + '<span>' + escapeHTML(compactText(focus.meta, '', 38)) + '</span>'
            + '</div>'
            + (focus.actionHTML ? '<div class="project-action-group">' + focus.actionHTML + '</div>' : '')
            + '</div>';
    }

    function renderWorkstreamTile(workstream, index) {
        var taskList = tasksForWorkstream(workstream.id);
        var currentTask = pickPreferredTask(taskList);
        var runningCount = countTasksByStates(workstream.id, ['running']);
        var waitingCount = countTasksByStates(workstream.id, ['waiting_human', 'waiting_review']);
        var blockedCount = countTasksByStates(workstream.id, ['blocked', 'failed']);
        var doneCount = countTasksByStates(workstream.id, ['execution_complete']);
        var tone = blockedCount ? 'danger' : (waitingCount ? 'warning' : (runningCount ? 'info' : statusTone(workstream.status)));
        var workstreamNumber = index + 1;
        var workstreamCode = workstreamNumber < 10 ? 'W0' + workstreamNumber : 'W' + workstreamNumber;
        return '<button type="button" class="project-workstream-tile tone-' + escapeHTML(tone) + '" data-workstream-id="' + escapeHTML(workstream.id) + '">'
            + '<span class="project-workstream-tile-avatar tone-' + escapeHTML(tone) + '">' + escapeHTML(workstreamCode) + '</span>'
            + '<span class="project-workstream-tile-body">'
            + '<span class="project-workstream-tile-name">' + escapeHTML(compactText(workstream.title, workstreamLabel(false), 34)) + '</span>'
            + '<span class="project-workstream-tile-task">' + escapeHTML(currentTask ? compactText(currentTask.title, '', 42) : tr('project.idle', 'Idle')) + '</span>'
            + renderStatusMeter([
                { count: runningCount, tone: 'info', label: tr('project.statusRunning', 'running') },
                { count: waitingCount, tone: 'warning', label: tr('project.statusWaiting', 'waiting') },
                { count: blockedCount, tone: 'danger', label: tr('project.statusBlocked', 'blocked') },
                { count: doneCount, tone: 'success', label: tr('project.statusDone', 'done') }
            ])
            + '</span>'
            + '<span class="project-workstream-tile-counts">'
            + '<span><strong>' + escapeHTML(String(runningCount)) + '</strong> ' + escapeHTML(tr('project.runShortTitle', 'Run')) + '</span>'
            + '<span><strong>' + escapeHTML(String(waitingCount)) + '</strong> ' + escapeHTML(tr('project.waitShortTitle', 'Wait')) + '</span>'
            + '<span><strong>' + escapeHTML(String(blockedCount)) + '</strong> ' + escapeHTML(tr('project.blockShortTitle', 'Block')) + '</span>'
            + '</span>'
            + '</button>';
    }

    function renderWorkstreamOverview(projectSnapshot) {
        var workstreamList = projectSnapshot.workstreams || [];
        if (!workstreamList.length) {
            return '';
        }
        return '<div class="project-manager-block">'
            + '<div class="project-manager-block-head"><h3>' + escapeHTML(tr('project.workstreams', 'Workstreams')) + '</h3><span>' + escapeHTML(workstreamCountText(workstreamList.length)) + '</span></div>'
            + '<div class="project-workstream-tile-grid">'
            + workstreamList.map(function(workstream, index) {
                return renderWorkstreamTile(workstream, index);
            }).join('')
            + '</div>'
            + '</div>';
    }

    function renderTaskManagerRow(task) {
        var tone = statusTone(task.state);
        return '<button type="button" class="project-task-row tone-' + escapeHTML(tone) + '" data-task-id="' + escapeHTML(task.id) + '">'
            + '<span class="project-task-cell project-task-cell-status">' + renderSidebarSignal(tone) + '<span>' + escapeHTML(shortStatusLabel(task.state)) + '</span></span>'
            + '<span class="project-task-cell project-task-cell-title"><strong>' + escapeHTML(compactText(task.title, '', 74)) + '</strong></span>'
            + '<span class="project-task-cell project-task-cell-agent">' + escapeHTML(taskAgentName(task)) + '</span>'
            + '<span class="project-task-cell project-task-cell-priority">' + escapeHTML(shortPriorityLabel(task.priority)) + '</span>'
            + '<span class="project-task-cell project-task-cell-risk">' + escapeHTML(shortRiskLabel(task.riskLevel)) + '</span>'
            + '<span class="project-task-cell project-task-cell-action"><span class="project-task-action-chip tone-' + escapeHTML(tone) + '">' + escapeHTML(taskNextActionLabel(task)) + '</span></span>'
            + '</button>';
    }

    function renderTaskManagerTable(taskList, emptyCopy) {
        var sortedTasks = sortTasksForManager(taskList);
        if (!sortedTasks.length) {
            return '<div class="project-list-item muted">' + escapeHTML(emptyCopy || tr('project.noTasksYetSentence', 'No tasks yet.')) + '</div>';
        }
        return '<div class="project-task-table">'
            + '<div class="project-task-row project-task-row-head">'
            + '<span>' + escapeHTML(tr('project.state', 'Status')) + '</span><span>' + escapeHTML(tr('project.taskSingular', 'Task')) + '</span><span>' + escapeHTML(tr('project.agentFallback', 'Agent')) + '</span><span>' + escapeHTML(tr('project.priorityShort', 'Pri')) + '</span><span>' + escapeHTML(tr('project.risk', 'Risk')) + '</span><span>' + escapeHTML(tr('project.statusNext', 'Next')) + '</span>'
            + '</div>'
            + sortedTasks.map(renderTaskManagerRow).join('')
            + '</div>';
    }

    function renderBoardTaskCard(task) {
        var tone = statusTone(task.state);
        return '<button type="button" class="project-task-card project-task-card-compact task-state-' + escapeHTML(task.state) + '" data-task-id="' + escapeHTML(task.id) + '">'
            + '<div class="project-task-head"><div class="project-task-title">' + escapeHTML(compactText(task.title, '', 62)) + '</div><span class="project-task-action-chip tone-' + escapeHTML(tone) + '">' + escapeHTML(taskNextActionLabel(task)) + '</span></div>'
            + '<div class="project-task-meta-row"><span class="project-task-meta">' + escapeHTML(taskAgentName(task)) + '</span><span class="project-task-meta">' + escapeHTML(shortPriorityLabel(task.priority) + ' / R' + shortRiskLabel(task.riskLevel)) + '</span></div>'
            + '</button>';
    }

    function checkpointProjectId(item) {
        var task = item && item.taskId ? findById(tasks(), item.taskId) : null;
        return item && item.projectId ? item.projectId : (task ? task.projectId : '');
    }

    function pluralSuffix(count) {
        return count === 1 ? '' : 's';
    }

    function buildProjectSnapshot(projectId) {
        var projectWorkstreams = sortWorkstreamsByAttention(workstreamsForProject(projectId));
        var projectTasks = tasksForProject(projectId);
        var pendingApprovals = checkpoints().filter(function(item) {
            return item.status === 'pending' && checkpointProjectId(item) === projectId;
        });
        var blockedTasks = projectTasks.filter(function(task) {
            return task.state === 'blocked' || task.state === 'failed';
        }).length;
        var waitingTasks = projectTasks.filter(function(task) {
            return task.state === 'waiting_human' || task.state === 'waiting_review';
        }).length;
        var runningTasks = projectTasks.filter(function(task) {
            return task.state === 'running';
        }).length;
        var runningWorkstreams = projectWorkstreams.filter(function(item) {
            return item.status === 'running';
        }).length;
        return {
            workstreams: projectWorkstreams,
            tasks: projectTasks,
            pendingApprovals: pendingApprovals,
            blockedTasks: blockedTasks,
            waitingTasks: waitingTasks,
            runningTasks: runningTasks,
            runningWorkstreams: runningWorkstreams,
            focusWorkstream: projectWorkstreams[0] || null
        };
    }

    function renderSidebarSignal(tone) {
        return '<span class="project-sidebar-signal tone-' + escapeHTML(tone || 'neutral') + '"></span>';
    }

    function renderSidebarBadge(value, tone, label) {
        return '<span class="project-sidebar-badge tone-' + escapeHTML(tone || 'neutral') + '">'
            + '<span class="project-sidebar-badge-value">' + escapeHTML(String(value)) + '</span>'
            + (label ? '<span class="project-sidebar-badge-label">' + escapeHTML(label) + '</span>' : '')
            + '</span>';
    }

    function renderSidebar() {
        var projectList = (state.snapshot && state.snapshot.projects) || [];
        var workstreamList = workstreams();
        var runtimeList = runtimes();
        var approvalsPending = checkpoints().filter(function(item) { return item.status === 'pending'; });

        return '<aside class="project-panel-sidebar">'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarProjects', 'Projects')) + '</div>'
            + projectList.map(function(project) {
                var projectWorkstreamCount = workstreamList.filter(function(item) { return item.projectId === project.id; }).length;
                return '<button type="button" class="project-nav-btn project-nav-compact' + (project.id === state.selectedProjectId && state.currentView === 'dashboard' ? ' active' : '') + '" data-project-id="' + escapeHTML(project.id) + '">'
                    + '<span class="project-nav-row">'
                    + '<span class="project-nav-main">' + renderSidebarSignal(statusTone(project.status)) + '<span class="project-nav-title">' + escapeHTML(project.name) + '</span></span>'
                    + renderSidebarBadge(projectWorkstreamCount, 'neutral', workstreamLabel(projectWorkstreamCount !== 1).toLowerCase())
                    + '</span>'
                    + '<span class="project-nav-meta-row"><span class="project-nav-meta">' + escapeHTML(shortStatusLabel(project.status || 'active')) + '</span></span>'
                    + '</button>';
            }).join('')
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarWorkstreams', workstreamLabel(true))) + '</div>'
            + workstreamList.map(function(item) {
                return '<button type="button" class="project-nav-btn project-nav-compact' + (item.id === state.selectedWorkstreamId && state.currentView === 'workstream' ? ' active' : '') + '" data-workstream-id="' + escapeHTML(item.id) + '">'
                    + '<span class="project-nav-row">'
                    + '<span class="project-nav-main">' + renderSidebarSignal(statusTone(item.status)) + '<span class="project-nav-title">' + escapeHTML(item.title) + '</span></span>'
                    + renderSidebarBadge(countTasks(item.id), statusTone(item.status), tr('project.taskPlural', 'tasks'))
                    + '</span>'
                    + '<span class="project-nav-meta-row"><span class="project-nav-meta">' + escapeHTML(shortStatusLabel(item.status) + ' • ' + shortPriorityLabel(item.priority)) + '</span></span>'
                    + '</button>';
            }).join('')
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarActions', 'Actions')) + '</div>'
            + '<button type="button" class="project-nav-btn project-nav-compact project-nav-action' + (state.currentView === 'approvals' ? ' active' : '') + '" data-open-approvals="1">'
            + '<span class="project-nav-row"><span class="project-nav-main">' + renderSidebarSignal(approvalsPending.length ? 'warning' : 'success') + '<span class="project-nav-title">' + escapeHTML(tr('project.approvals', 'Approvals')) + '</span></span>'
            + renderSidebarBadge(approvalsPending.length, approvalsPending.length ? 'warning' : 'success', approvalsPending.length === 1 ? tr('project.itemSingular', 'item') : tr('project.itemPlural', 'items')) + '</span>'
            + '<span class="project-nav-meta-row"><span class="project-nav-meta">' + escapeHTML(approvalsPending.length ? tr('project.statusReview', 'Review') : tr('project.statusClear', 'Clear')) + '</span></span>'
            + '</button>'
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarRuntimes', 'Runtimes')) + '</div>'
            + runtimeList.map(function(runtime) {
                return '<div class="project-runtime-card project-runtime-compact tone-' + escapeHTML(statusTone(runtime.status)) + '">'
                    + '<div class="project-nav-row"><div class="project-nav-main">' + renderSidebarSignal(statusTone(runtime.status)) + '<div class="project-runtime-name">' + escapeHTML(runtime.name) + '</div></div>'
                    + renderSidebarBadge(runtime.kind || tr('project.localRuntime', 'local'), 'neutral', '') + '</div>'
                    + '<div class="project-runtime-meta-row"><div class="project-runtime-meta">' + escapeHTML(shortStatusLabel(runtime.status || 'unknown')) + '</div></div>'
                    + '</div>';
            }).join('')
            + '</div>'
            + '</aside>';
    }

    function renderBreadcrumbs() {
        var crumbs = [];
        var project = getSelectedProject();
        var workstream = getSelectedWorkstream();
        var task = getSelectedTask();
        var session = getSelectedSession();

        if (project) {
            crumbs.push('<button type="button" class="project-breadcrumb" data-project-id="' + escapeHTML(project.id) + '">' + escapeHTML(project.name) + '</button>');
        }
        if (workstream) {
            crumbs.push('<button type="button" class="project-breadcrumb" data-workstream-id="' + escapeHTML(workstream.id) + '">' + escapeHTML(workstream.title) + '</button>');
        }
        if (task) {
            crumbs.push('<button type="button" class="project-breadcrumb" data-task-id="' + escapeHTML(task.id) + '">' + escapeHTML(task.title) + '</button>');
        }
        if (session) {
            crumbs.push('<span class="project-breadcrumb current">' + escapeHTML(session.name) + '</span>');
        }
        if (state.currentView === 'approvals') {
            crumbs.push('<span class="project-breadcrumb current">' + escapeHTML(tr('project.approvals', 'Approvals Inbox')) + '</span>');
        }
        return crumbs.join('<span class="project-breadcrumb-sep">/</span>');
    }

    function renderDashboard() {
        var project = getSelectedProject();
        var projectSnapshot;
        var taskList;
        var waitingTotal;
        var stats;
        if (!project) {
            return '<div class="project-empty-state">' + escapeHTML(tr('project.empty', 'No project data yet.')) + '</div>';
        }
        projectSnapshot = buildProjectSnapshot(project.id);
        taskList = projectSnapshot.tasks;
        waitingTotal = projectSnapshot.waitingTasks + projectSnapshot.pendingApprovals.length;
        stats = [
            { label: tr('project.statusRunning', 'Running'), value: projectSnapshot.runningTasks, tone: projectSnapshot.runningTasks ? 'info' : 'neutral' },
            { label: tr('project.statusWaiting', 'Waiting'), value: waitingTotal, tone: waitingTotal ? 'warning' : 'success' },
            { label: tr('project.statusBlocked', 'Blocked'), value: projectSnapshot.blockedTasks, tone: projectSnapshot.blockedTasks ? 'danger' : 'success' }
        ];
        return '<section class="project-main-section project-manager-view project-command-view">'
            + '<div class="project-section-header">'
            + '<div><div class="project-section-kicker">' + escapeHTML(tr('project.taskManager', 'Task Manager')) + '</div>'
            + '<h2>' + escapeHTML(project.name) + '</h2><p>' + escapeHTML(taskCountText(taskList.length) + ' / ' + workstreamCountText(projectSnapshot.workstreams.length)) + '</p></div>'
            + '<div class="project-header-actions">'
            + (projectSnapshot.focusWorkstream ? '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(projectSnapshot.focusWorkstream.id) + '">' + escapeHTML(tr('project.newTaskShort', '+ Task')) + '</button>' : '')
            + '<button type="button" class="project-inline-btn" data-create-workstream="' + escapeHTML(project.id) + '">' + escapeHTML(tr('project.newWorkstreamShort', '+ Workstream')) + '</button>'
            + (projectSnapshot.pendingApprovals.length ? '<button type="button" class="project-inline-btn primary" data-open-approvals="1">' + escapeHTML(tr('project.statusReview', 'Review')) + '</button>' : '')
            + '</div>'
            + '</div>'
            + renderCommandStats(stats)
            + (state.creatingWorkstreamProjectId === project.id ? renderWorkstreamWizard(project) : (!projectSnapshot.workstreams.length && !taskList.length ? renderQuickStartCard(project) : renderCommandHero(project, projectSnapshot)))
            + renderCommandLanes(projectSnapshot)
            + (taskList.length
                ? '<details class="project-disclosure"><summary><span>' + escapeHTML(tr('project.taskList', 'Task list')) + '</span><small>' + escapeHTML(tr('project.countTotal', '{count} total', { count: taskList.length })) + '</small></summary>'
                    + '<div class="project-disclosure-actions"><button type="button" class="project-inline-btn" data-project-history="' + escapeHTML(project.id) + '">' + escapeHTML(tr('project.history', 'History')) + '</button></div>'
                    + renderTaskManagerTable(taskList, tr('project.noTasksYetSentence', 'No tasks yet.'))
                    + '</details>'
                : '')
            + '</section>';
    }

    function renderWorkstreamBoard() {
        var workstream = getSelectedWorkstream();
        var tasks = workstream ? tasksForWorkstream(workstream.id) : [];
        var runCount;
        var waitCount;
        var blockCount;
        var columnModels;
        var visibleColumns;
        var emptyColumns;
        var columns = [
            {
                key: 'up_next',
                label: tr('project.columnUpNext', 'Up next'),
                subtitle: tr('project.columnUpNextSubtitle', 'Start one of these next.'),
                empty: tr('project.columnUpNextEmpty', 'Nothing is queued yet.'),
                tone: 'neutral',
                states: ['planned', 'queued']
            },
            {
                key: 'in_progress',
                label: tr('project.columnInProgress', 'In progress'),
                subtitle: tr('project.columnInProgressSubtitle', 'Work currently moving.'),
                empty: tr('project.columnInProgressEmpty', 'No live task right now.'),
                tone: 'info',
                states: ['running']
            },
            {
                key: 'needs_decision',
                label: tr('project.columnNeedsDecision', 'Needs review or input'),
                subtitle: tr('project.columnNeedsDecisionSubtitle', 'Waiting on a human or reviewer.'),
                empty: tr('project.columnNeedsDecisionEmpty', 'No decisions are pending.'),
                tone: 'warning',
                states: ['waiting_review', 'waiting_human']
            },
            {
                key: 'blocked',
                label: tr('project.statusBlocked', 'Blocked'),
                subtitle: tr('project.columnBlockedSubtitle', 'Needs intervention before it can move.'),
                empty: tr('project.columnBlockedEmpty', 'Nothing is blocked.'),
                tone: 'danger',
                states: ['blocked', 'failed']
            },
            {
                key: 'ready',
                label: tr('project.columnReadyClose', 'Ready to close'),
                subtitle: tr('project.columnReadyCloseSubtitle', 'Execution finished, follow-through remains.'),
                empty: tr('project.columnReadyCloseEmpty', 'Nothing is awaiting closeout.'),
                tone: 'success',
                states: ['execution_complete']
            }
        ];
        if (!workstream) {
            return renderDashboard();
        }
        runCount = countTasksByStates(workstream.id, ['running']);
        waitCount = countTasksByStates(workstream.id, ['waiting_human', 'waiting_review']);
        blockCount = countTasksByStates(workstream.id, ['blocked', 'failed']);
        columnModels = columns.map(function(column) {
            return {
                key: column.key,
                label: column.label,
                subtitle: column.subtitle,
                empty: column.empty,
                tone: column.tone,
                states: column.states,
                tasks: tasks.filter(function(task) { return column.states.indexOf(task.state) !== -1; })
            };
        });
        visibleColumns = columnModels.filter(function(column) {
            return column.tasks.length;
        });
        emptyColumns = columnModels.filter(function(column) {
            return !column.tasks.length;
        });
        return '<section class="project-main-section project-manager-view">'
            + '<div class="project-section-header">'
            + '<div><div class="project-section-kicker">' + escapeHTML(tr('project.workstreamLane', 'Workstream')) + '</div>'
            + '<h2>' + escapeHTML(workstream.title) + '</h2><p>' + escapeHTML(workstreamFlowSummary(workstream)) + '</p></div>'
            + '<div class="project-header-actions">'
            + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.newTaskShort', '+ Task')) + '</button>'
            + renderWorkstreamInlineControls(workstream)
            + '</div>'
            + '</div>'
            + renderCommandStats([
                { label: tr('project.statusRunning', 'Running'), value: runCount, tone: runCount ? 'info' : 'neutral' },
                { label: tr('project.statusWaiting', 'Waiting'), value: waitCount, tone: waitCount ? 'warning' : 'success' },
                { label: tr('project.statusBlocked', 'Blocked'), value: blockCount, tone: blockCount ? 'danger' : 'success' }
            ])
            + (state.creatingTaskWorkstreamId === workstream.id ? renderTaskWizard(workstream) : '')
            + renderWorkstreamGuide(workstream, tasks)
            + (visibleColumns.length
                ? '<div class="project-board project-board-progressive">'
                    + visibleColumns.map(function(column) {
                        return '<div class="project-board-column tone-' + escapeHTML(column.tone) + ' lane-' + escapeHTML(column.key) + '"><div class="project-board-column-header"><div class="project-board-column-title-row"><div class="project-board-column-title">' + escapeHTML(column.label) + '</div><div class="project-board-column-count">' + escapeHTML(String(column.tasks.length)) + '</div></div><div class="project-board-column-subtitle">' + escapeHTML(column.subtitle) + '</div></div>'
                    + column.tasks.map(function(task) {
                        return renderBoardTaskCard(task);
                    }).join('')
                        + '</div>';
                    }).join('')
                    + '</div>'
                : '<div class="project-lane-empty-state"><div><strong>' + escapeHTML(tr('project.noTasksInLane', 'No tasks in this lane yet')) + '</strong><span>' + escapeHTML(tr('project.noTasksInLaneCopy', 'Add one concrete next step when this lane is ready to move.')) + '</span></div><button type="button" class="project-inline-btn primary" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.addTask', 'Add task')) + '</button></div>')
            + renderCollapsedLaneStates(emptyColumns)
            + '</section>';
    }

    function renderTaskRunbookPanel(task) {
        var phases = phasesForTask(task);
        var currentPhase = currentPhaseForTask(task);
        var currentPhaseDef = currentRunbookPhase(task);
        var missing = taskMissingEvidence(task);
        var updating = state.phaseUpdatingTaskId === task.id || state.updatingTaskId === task.id;
        var currentAttempt = currentPhaseAttemptForTask(task);
        var currentSession = sessionForPhaseAttempt(currentAttempt);
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
            return '<div class="project-phase-row tone-' + escapeHTML(tone) + '">'
                + '<div class="project-phase-index">' + escapeHTML(String(index + 1)) + '</div>'
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
        var sessionHTML = currentSession
            ? '<div class="project-phase-requirements"><div class="project-fact-label">' + escapeHTML(tr('project.phaseSession', 'Phase session')) + '</div>'
                + '<div class="project-action-group">'
                + '<button type="button" class="project-inline-btn" data-session-id="' + escapeHTML(currentSession.id) + '">' + escapeHTML(tr('project.sessionDetails', 'Session details')) + '</button>'
                + (currentSession.supportsAttach && currentSession.terminalId ? '<button type="button" class="project-inline-btn primary" data-attach-terminal="' + escapeHTML(currentSession.terminalId) + '">' + escapeHTML(tr('project.attachTerminal', 'Attach Terminal')) + '</button>' : '')
                + '</div></div>'
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
            + sessionHTML
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

    function renderTaskEvidenceDisclosure(task, taskSessions, decisionCards) {
        var currentAttempt = currentPhaseAttemptForTask(task);
        var currentPhase = currentPhaseForTask(task);
        var canComplete = !!currentAttempt && currentPhase !== 'ready_for_acceptance' && currentAttempt.status === 'running';
        var toolRunning = !!latestRunningToolRunForAttempt(currentAttempt);
        return '<details class="project-disclosure project-evidence-disclosure"><summary><span>' + escapeHTML(tr('project.evidenceHistory', 'Evidence & history')) + '</span><small>' + escapeHTML(tr('project.evidenceHistorySubtitle', 'Timeline, files, sessions, audit')) + '</small></summary>'
            + '<div class="project-tab-sections">'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.timeline', 'Timeline')) + '</h3>'
            + renderStructuredEventStream(task.timeline || [], tr('project.noTimeline', 'No timeline entries yet.')) + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.artifacts', 'Artifacts')) + '</h3>'
            + (taskArtifacts(task.id).length ? taskArtifacts(task.id).map(function(item) {
                return '<div class="project-list-item"><strong>' + escapeHTML(humanizeArtifactKind(item.kind)) + ':</strong> ' + escapeHTML(humanizeArtifactOutcome(item.outcome)) + ' • ' + escapeHTML(item.value || item.label || '') + '</div>';
            }).join('') : '<div class="project-list-item muted">' + escapeHTML(tr('project.noArtifacts', 'No artifacts recorded yet.')) + '</div>') + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.toolRuns', 'Tool runs')) + '</h3>'
            + renderToolRunHistory(task, taskToolRuns(task.id), currentAttempt, canComplete, toolRunning, currentPhase, false)
            + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.evidence', 'Evidence')) + '</h3>'
            + renderFactList(task.evidence || [], tr('project.noEvidence', 'No evidence recorded yet.')) + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.filesDiff', 'Files & Diff')) + '</h3>'
            + renderFileDiffPanel(task.filesChanged || [], task.diffSummary || '') + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.sessions', 'Sessions')) + '</h3>'
            + (taskSessions.length ? taskSessions.map(function(session) {
                var tone = statusTone(session.state);
                tone = tone === 'neutral' ? toneFromText(session.state || session.name || '') : tone;
                return '<button type="button" class="project-list-button project-session-button tone-' + escapeHTML(tone) + '" data-session-id="' + escapeHTML(session.id) + '">'
                    + '<span class="project-nav-row"><span class="project-nav-main">' + renderSidebarSignal(tone) + '<span>' + escapeHTML(session.name) + '</span></span>'
                    + (session.supportsAttach && session.terminalId ? renderSidebarBadge('TTY', 'info', '') : '') + '</span><span class="project-list-meta">' + escapeHTML(session.state + ' • ' + session.durationLabel) + '</span></button>';
            }).join('') : '<div class="project-list-item muted">' + escapeHTML(tr('project.noSessions', 'No sessions recorded yet.')) + '</div>') + '</div>'
            + '<div class="project-card-list project-card-visual"><h3>' + escapeHTML(tr('project.audit', 'Audit')) + '</h3>'
            + renderStructuredEventStream(task.audit || [], tr('project.noAudit', 'No audit entries yet.'))
            + '<button type="button" class="project-inline-btn" data-task-history="' + escapeHTML(task.id) + '">' + escapeHTML(tr('project.viewHistory', 'View history')) + '</button>'
            + '<button type="button" class="project-inline-btn" data-task-replay="' + escapeHTML(task.id) + '">' + escapeHTML(tr('project.openReplay', 'Open replay')) + '</button>'
            + '</div>'
            + (decisionCards || '')
            + '</div></details>';
    }

    function renderTaskDetail() {
        var task = getSelectedTask();
        var acceptanceDecision = task && task.acceptanceDecisionId ? decisionById(task.acceptanceDecisionId) : null;
        var archiveDecision = task && task.archiveDecisionId ? decisionById(task.archiveDecisionId) : null;
        var taskSessions = task ? sessionsForTask(task.id) : [];
        var decisionCards = '';
        if (!task) {
            return renderDashboard();
        }
        if (acceptanceDecision) {
            decisionCards += '<div class="project-card-list"><h3>' + escapeHTML(tr('project.acceptanceDecision', 'Acceptance Decision')) + '</h3>'
                + renderDecisionCard(acceptanceDecision, tr('project.acceptanceDecision', 'Acceptance Decision'))
                + '</div>';
        }
        if (archiveDecision) {
            decisionCards += '<div class="project-card-list"><h3>' + escapeHTML(tr('project.archiveDecision', 'Archive Decision')) + '</h3>'
                + renderDecisionCard(archiveDecision, tr('project.archiveDecision', 'Archive Decision'))
                + '</div>';
        }
        return '<section class="project-main-section project-manager-view">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.taskDetail', 'Task Detail')) + '</div>'
            + '<h2>' + escapeHTML(task.title) + '</h2><p>' + escapeHTML(taskAgentName(task) + ' / ' + taskWorkstreamName(task)) + '</p></div><div class="project-header-actions project-task-actions">' + renderTaskInlineControls(task) + '</div></div>'
            + renderCommandStats([
                { label: tr('project.state', 'State'), value: shortStatusLabel(task.state), tone: statusTone(task.state) },
                { label: tr('project.skill', 'Skill'), value: humanizeSkill(task.selectedSkill), tone: 'info' },
                { label: tr('project.phase', 'Phase'), value: humanizePhase(currentPhaseForTask(task)), tone: phaseStatusTone(phaseStatusForTask(task, currentRunbookPhase(task))) },
                { label: tr('project.evidence', 'Evidence'), value: taskMissingEvidence(task).length ? String(taskMissingEvidence(task).length) : tr('project.statusClear', 'Clear'), tone: taskMissingEvidence(task).length ? 'warning' : 'success' },
                { label: tr('project.risk', 'Risk'), value: shortRiskLabel(task.riskLevel), tone: riskTone(task.riskLevel) },
                { label: tr('project.sessions', 'Sessions'), value: taskSessions.length, tone: taskSessions.length ? 'info' : 'neutral' }
            ])
            + renderTaskExecutionGuide(task, taskSessions)
            + '<div class="project-two-column project-task-control-grid">' + renderTaskRunbookPanel(task) + renderTaskArtifactsPanel(task) + '</div>'
            + renderTaskApprovalStatusCard(task)
            + renderTaskEvidenceDisclosure(task, taskSessions, decisionCards)
            + '</section>';
    }

    function renderSessionDetail() {
        var session = getSelectedSession();
        if (!session) {
            return renderTaskDetail();
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.sessionDetail', 'Session Detail')) + '</div>'
            + '<h2>' + escapeHTML(session.name) + '</h2><p>' + escapeHTML(session.summary) + '</p></div>'
            + (session.supportsAttach && session.terminalId ? '<button type="button" class="project-inline-btn primary" data-attach-terminal="' + escapeHTML(session.terminalId) + '">' + escapeHTML(tr('project.attachTerminal', 'Attach Terminal')) + '</button>' : '')
            + '</div>'
            + '<div class="project-two-column">'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.sessionInfo', 'Session Info')) + '</h3>'
            + '<div class="project-list-item"><strong>ID:</strong> ' + escapeHTML(session.id) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.runtime', 'Runtime')) + ':</strong> ' + escapeHTML(session.runtimeId) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.executionRole', 'Execution role')) + ':</strong> ' + escapeHTML(humanizeExecutionRole(session.executionRole)) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.systemRole', 'System role')) + ':</strong> ' + escapeHTML(humanizeSystemRole(session.systemRole || 'worker')) + '</div>'
            + (session.phaseAttemptId ? '<div class="project-list-item"><strong>' + escapeHTML(tr('project.phaseAttempt', 'Phase attempt')) + ':</strong> ' + escapeHTML(session.phaseAttemptId) + '</div>' : '')
            + (session.workspaceRef ? '<div class="project-list-item"><strong>' + escapeHTML(tr('project.workspace', 'Workspace')) + ':</strong> ' + escapeHTML(humanizeToken(session.workspaceRef)) + '</div>' : '')
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.startedAt', 'Started')) + ':</strong> ' + escapeHTML(formatTime(session.startedAt)) + '</div>'
            + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.claims', 'Claims')) + '</h3>'
            + (session.claims || []).map(function(item) { return '<div class="project-list-item">' + escapeHTML(item) + '</div>'; }).join('')
            + '<h3>' + escapeHTML(tr('project.artifacts', 'Artifacts')) + '</h3>'
            + (session.artifacts || []).map(function(item) { return '<div class="project-list-item">' + escapeHTML(item) + '</div>'; }).join('')
            + '</div>'
            + '</div>'
            + '</section>';
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
            + renderApprovalFocusCard(pending, resolved)
            + renderApprovalQueueSummary(pending, resolved)
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

    function renderEventsView() {
        var header = state.selectedTaskId ? tr('project.taskHistory', 'Task Event History') : tr('project.projectHistory', 'Project Event History');
        var subcopy = state.selectedTaskId ? tr('project.taskHistoryHint', 'Replay-ready task events, newest first.') : tr('project.projectHistoryHint', 'Project-scoped event history, newest first.');
        var visibleEvents = filterEventsByQuery(filterEventsByLane(state.currentEvents, state.selectedEventLane), state.eventSearchQuery);
        if (state.eventsLoading) {
            return '<section class="project-main-section"><div class="project-empty-state">' + escapeHTML(tr('project.loadingEvents', 'Loading events…')) + '</div></section>';
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.history', 'History')) + '</div><h2>' + escapeHTML(header) + '</h2><p>' + escapeHTML(subcopy) + '</p></div></div>'
            + (state.eventsError ? '<div class="project-banner error">' + escapeHTML(state.eventsError) + '</div>' : '')
            + renderLaneFilterControls('events', state.selectedEventLane)
            + renderTextFilterControl('events', state.eventSearchQuery)
            + '<div class="project-card-list">'
            + (visibleEvents.length ? renderLaneGroups(visibleEvents, state.eventSearchQuery) : '<div class="project-list-item muted">' + escapeHTML(tr('project.noEvents', 'No events matched this filter.')) + '</div>')
            + (state.currentEventsCursor ? '<button type="button" class="project-inline-btn" data-events-load-more="1">' + escapeHTML(tr('project.loadMoreEvents', 'Load more')) + '</button>' : '')
            + '</div>'
            + '</section>';
    }

    function renderReplayView() {
        var visibleReplaySteps = filterEventsByQuery(filterEventsByLane(state.currentReplay && state.currentReplay.steps, state.selectedReplayLane), state.replaySearchQuery);
        var visibleReplayTransitions = filterTransitionsByQuery(filterTransitionsByLane(state.currentReplay && state.currentReplay.transitions, state.selectedReplayLane), state.replaySearchQuery);
        var visibleReplaySections = (state.currentReplay && state.currentReplay.sections ? state.currentReplay.sections : []).filter(function(section) {
            return state.selectedReplayLane === 'all' || section.kind === state.selectedReplayLane;
        }).map(function(section) {
            return {
                kind: section.kind,
                title: section.title,
                steps: filterEventsByQuery(section.steps, state.replaySearchQuery)
            };
        }).filter(function(section) {
            return section.steps.length > 0;
        });
        if (state.replayLoading) {
            return '<section class="project-main-section"><div class="project-empty-state">' + escapeHTML(tr('project.loadingReplay', 'Loading replay…')) + '</div></section>';
        }
        if (!state.currentReplay) {
            return '<section class="project-main-section"><div class="project-empty-state">' + escapeHTML(tr('project.noReplay', 'No replay data available.')) + '</div></section>';
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.replay', 'Replay')) + '</div><h2>' + escapeHTML(state.currentReplay.title) + '</h2><p>' + escapeHTML(tr('project.replayHint', 'Ordered task events for step-by-step playback.')) + '</p></div></div>'
            + renderLaneFilterControls('replay', state.selectedReplayLane)
            + renderTextFilterControl('replay', state.replaySearchQuery)
            + '<div class="project-two-column">'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.replaySections', 'Replay sections')) + '</h3>'
            + visibleReplaySections.map(function(section) {
                return '<div class="project-lane-section lane-' + escapeHTML(section.kind) + '">'
                    + '<div class="project-lane-header">'
                    + renderLanePill(section.kind)
                    + '<span class="project-list-meta">' + escapeHTML(section.title) + '</span>'
                    + '</div>'
                    + section.steps.map(function(step, index) {
                        return renderLaneEventCard(step, index, state.replaySearchQuery);
                    }).join('')
                    + '</div>';
            }).join('')
            + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.replayTransitions', 'Transitions')) + '</h3>'
            + visibleReplayTransitions.map(function(item) {
                return renderTransitionCard(item, state.replaySearchQuery);
            }).join('')
            + '<h3>' + escapeHTML(tr('project.acceptanceDecision', 'Acceptance Decision')) + '</h3>'
            + renderDecisionCard(state.currentReplay.acceptanceDecision, tr('project.replayAcceptanceDecision', 'Replay Acceptance Decision'))
            + '<h3>' + escapeHTML(tr('project.archiveDecision', 'Archive Decision')) + '</h3>'
            + renderDecisionCard(state.currentReplay.archiveDecision, tr('project.replayArchiveDecision', 'Replay Archive Decision'))
            + '<h3>' + escapeHTML(tr('project.replaySteps', 'All steps')) + '</h3>'
            + (visibleReplaySteps.length ? renderLaneGroups(visibleReplaySteps, state.replaySearchQuery) : '<div class="project-list-item muted">' + escapeHTML(tr('project.noReplay', 'No replay data available.')) + '</div>')
            + '</div>'
            + '</div>'
            + '</section>';
    }

    function renderMain() {
        if (!state.snapshot) {
            return '<div class="project-empty-state">' + escapeHTML(tr('project.empty', 'No project data yet.')) + '</div>';
        }
        if (state.currentView === 'workstream') {
            return renderWorkstreamBoard();
        }
        if (state.currentView === 'task') {
            return renderTaskDetail();
        }
        if (state.currentView === 'session') {
            return renderSessionDetail();
        }
        if (state.currentView === 'approvals') {
            return renderApprovals();
        }
        if (state.currentView === 'events') {
            return renderEventsView();
        }
        if (state.currentView === 'replay') {
            return renderReplayView();
        }
        return renderDashboard();
    }

    function createProject() {
        var name = window.prompt(tr('project.promptProjectName', 'Project name:'), tr('project.defaultNewProject', 'New Project'));
        var description;
        var goal;
        if (name === null || !String(name).trim()) {
            return;
        }
        description = window.prompt(tr('project.promptProjectDescription', 'Project description:'), '');
        if (description === null) {
            description = '';
        }
        goal = window.prompt(tr('project.promptProjectGoal', 'Current goal:'), '');
        if (goal === null) {
            goal = '';
        }
        fetchJSON('/api/project-control/projects', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                key: String(name).trim().toLowerCase().replace(/[^a-z0-9]+/g, '-'),
                name: String(name).trim(),
                description: String(description || '').trim(),
                currentGoal: String(goal || '').trim()
            })
        }).then(function(snapshot) {
            state.snapshot = snapshot;
            openDashboard(snapshot.activeProjectId);
            updateBadge();
            render();
        }).catch(function(err) {
            state.error = err.message || tr('project.createProjectFailed', 'Create project failed');
            render();
        });
    }

    function createWorkstream(projectId) {
        startWorkstreamWizard(projectId);
    }

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

    function submitWorkstreamWizard(form) {
        var projectId = form.getAttribute('data-workstream-wizard') || state.creatingWorkstreamProjectId;
        var titleInput = form.querySelector('[name="title"]');
        var scopeInput = form.querySelector('[name="scopeSummary"]');
        var title = String(titleInput && titleInput.value ? titleInput.value : '').trim();
        var scopeSummary = String(scopeInput && scopeInput.value ? scopeInput.value : '').trim();
        var created;

        state.creatingWorkstreamTitle = title;
        state.creatingWorkstreamScope = scopeSummary;
        if (!projectId || !title) {
            state.error = tr('project.workstreamNameRequired', 'Name required.');
            render();
            focusWorkstreamWizard();
            return;
        }
        state.error = '';
        state.creatingWorkstreamSaving = true;
        render();
        fetchJSON('/api/project-control/workstreams', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                projectId: projectId,
                title: title,
                description: '',
                priority: 'medium',
                scopeSummary: scopeSummary
            })
        }).then(function(snapshot) {
            created = findCreatedWorkstream(snapshot, projectId, title);
            state.snapshot = snapshot;
            state.selectedProjectId = projectId;
            resetWorkstreamWizard();
            if (created) {
                state.selectedWorkstreamId = created.id;
                state.selectedTaskId = '';
                state.selectedSessionId = '';
                state.currentView = 'workstream';
            } else {
                state.currentView = 'dashboard';
            }
            updateBadge();
            render();
        }).catch(function(err) {
            state.creatingWorkstreamSaving = false;
            state.error = err.message || tr('project.createWorkstreamFailed', 'Create workstream failed');
            render();
            focusWorkstreamWizard();
        });
    }

    function startTaskWizard(workstreamId) {
        var workstream = findById(workstreams(), workstreamId);
        var skill = defaultSkill();
        var runbook;
        if (!workstream) {
            return;
        }
        if (state.creatingTaskWorkstreamId !== workstreamId) {
            state.creatingTaskTitle = '';
            state.creatingTaskGoal = '';
        }
        runbook = runbookForSkill(skill.id, '');
        state.creatingTaskWorkstreamId = workstreamId;
        state.creatingTaskSelectedSkill = skill.id || 'code_change';
        state.creatingTaskRunbookId = runbook.id || 'code_change_default';
        state.creatingTaskSaving = false;
        state.selectedProjectId = workstream.projectId;
        state.selectedWorkstreamId = workstream.id;
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        state.currentView = 'workstream';
        state.error = '';
        render();
        focusTaskWizard();
    }

    function submitTaskWizard(form) {
        var workstreamId = form.getAttribute('data-task-wizard') || state.creatingTaskWorkstreamId;
        var workstream = findById(workstreams(), workstreamId);
        var titleInput = form.querySelector('[name="title"]');
        var goalInput = form.querySelector('[name="goal"]');
        var skillInput = form.querySelector('[name="selectedSkill"]');
        var title = titleInput && titleInput.value ? titleInput.value.trim() : '';
        var goal = goalInput && goalInput.value ? goalInput.value.trim() : '';
        var selectedSkill = skillInput && skillInput.value ? skillInput.value.trim() : (state.creatingTaskSelectedSkill || defaultSkill().id || 'code_change');
        var runbook = runbookForSkill(selectedSkill, state.creatingTaskRunbookId);

        state.creatingTaskTitle = title;
        state.creatingTaskGoal = goal;
        state.creatingTaskSelectedSkill = selectedSkill;
        state.creatingTaskRunbookId = runbook.id || '';
        if (!workstream || !title) {
            state.error = tr('project.taskTitleRequired', 'Task title required.');
            render();
            focusTaskWizard();
            return;
        }
        state.error = '';
        state.creatingTaskSaving = true;
        render();
        fetchJSON('/api/project-control/tasks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                projectId: workstream.projectId,
                workstreamId: workstream.id,
                title: title,
                goal: goal,
                priority: 'medium',
                riskLevel: 'medium',
                selectedSkill: selectedSkill,
                runbookId: runbook.id || ''
            })
        }).then(function(snapshot) {
            state.snapshot = snapshot;
            resetTaskWizard();
            openWorkstream(workstreamId);
            updateBadge();
            render();
        }).catch(function(err) {
            state.creatingTaskSaving = false;
            state.error = err.message || tr('project.createTaskFailed', 'Create task failed');
            render();
            focusTaskWizard();
        });
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
        panel.querySelectorAll('[data-project-id]').forEach(function(node) {
            node.addEventListener('click', function() {
                openDashboard(node.getAttribute('data-project-id'));
            });
        });
        panel.querySelectorAll('[data-workstream-id]').forEach(function(node) {
            node.addEventListener('click', function() {
                openWorkstream(node.getAttribute('data-workstream-id'));
            });
        });
        panel.querySelectorAll('[data-task-id]').forEach(function(node) {
            node.addEventListener('click', function() {
                openTask(node.getAttribute('data-task-id'));
            });
        });
        panel.querySelectorAll('[data-session-id]').forEach(function(node) {
            node.addEventListener('click', function() {
                openSession(node.getAttribute('data-session-id'));
            });
        });
        panel.querySelectorAll('[data-open-approvals]').forEach(function(node) {
            node.addEventListener('click', openApprovals);
        });
        panel.querySelectorAll('[data-project-history]').forEach(function(node) {
            node.addEventListener('click', function() {
                openProjectHistory(node.getAttribute('data-project-history'));
            });
        });
        panel.querySelectorAll('[data-task-history]').forEach(function(node) {
            node.addEventListener('click', function() {
                openTaskHistory(node.getAttribute('data-task-history'));
            });
        });
        panel.querySelectorAll('[data-task-replay]').forEach(function(node) {
            node.addEventListener('click', function() {
                loadTaskReplay(node.getAttribute('data-task-replay'));
            });
        });
        panel.querySelectorAll('[data-update-workstream]').forEach(function(node) {
            node.addEventListener('click', function() {
                updateWorkstreamInline(node.getAttribute('data-update-workstream'), node.getAttribute('data-action') || '');
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
        panel.querySelectorAll('[data-events-load-more]').forEach(function(node) {
            node.addEventListener('click', function() {
                loadEvents(state.currentEventFilters || {}, true);
            });
        });
        panel.querySelectorAll('[data-lane-filter]').forEach(function(node) {
            node.addEventListener('click', function() {
                var kind = node.getAttribute('data-lane-filter-kind');
                var lane = node.getAttribute('data-lane-filter') || 'all';
                if (kind === 'events') {
                    state.selectedEventLane = lane;
                } else if (kind === 'replay') {
                    state.selectedReplayLane = lane;
                }
                persistFilterPreferences();
                render();
            });
        });
        panel.querySelectorAll('[data-filter-input-kind]').forEach(function(node) {
            node.addEventListener('input', function() {
                var kind = node.getAttribute('data-filter-input-kind');
                if (kind === 'events') {
                    state.eventSearchQuery = node.value || '';
                } else if (kind === 'replay') {
                    state.replaySearchQuery = node.value || '';
                }
                persistFilterPreferences();
                render();
            });
        });
        panel.querySelectorAll('[data-create-project]').forEach(function(node) {
            node.addEventListener('click', createProject);
        });
        panel.querySelectorAll('[data-workstream-wizard]').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                submitWorkstreamWizard(form);
            });
        });
        panel.querySelectorAll('[data-workstream-title-input]').forEach(function(node) {
            node.addEventListener('input', function() {
                state.creatingWorkstreamTitle = node.value || '';
            });
        });
        panel.querySelectorAll('[data-workstream-scope-input]').forEach(function(node) {
            node.addEventListener('input', function() {
                state.creatingWorkstreamScope = node.value || '';
            });
        });
        panel.querySelectorAll('[data-cancel-workstream-wizard]').forEach(function(node) {
            node.addEventListener('click', function() {
                resetWorkstreamWizard();
                state.error = '';
                render();
            });
        });
        panel.querySelectorAll('[data-task-wizard]').forEach(function(form) {
            form.addEventListener('submit', function(event) {
                event.preventDefault();
                submitTaskWizard(form);
            });
        });
        panel.querySelectorAll('[data-task-title-input]').forEach(function(node) {
            node.addEventListener('input', function() {
                state.creatingTaskTitle = node.value || '';
            });
        });
        panel.querySelectorAll('[data-task-goal-input]').forEach(function(node) {
            node.addEventListener('input', function() {
                state.creatingTaskGoal = node.value || '';
            });
        });
        panel.querySelectorAll('[data-task-skill-select]').forEach(function(node) {
            node.addEventListener('change', function() {
                var skill = skillForId(node.value || '');
                var runbook = runbookForSkill(skill.id, '');
                state.creatingTaskSelectedSkill = skill.id || '';
                state.creatingTaskRunbookId = runbook.id || '';
                render();
            });
        });
        panel.querySelectorAll('[data-cancel-task-wizard]').forEach(function(node) {
            node.addEventListener('click', function() {
                resetTaskWizard();
                state.error = '';
                render();
            });
        });
        panel.querySelectorAll('[data-create-workstream]').forEach(function(node) {
            node.addEventListener('click', function() {
                createWorkstream(node.getAttribute('data-create-workstream'));
            });
        });
        panel.querySelectorAll('[data-create-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                startTaskWizard(node.getAttribute('data-create-task'));
            });
        });
        panel.querySelectorAll('[data-attach-terminal]').forEach(function(node) {
            node.addEventListener('click', function() {
                var bridge = app();
                if (bridge && typeof bridge.attachTerminal === 'function') {
                    bridge.attachTerminal(node.getAttribute('data-attach-terminal'));
                }
            });
        });
        panel.querySelectorAll('[data-open-terminal-mode]').forEach(function(node) {
            node.addEventListener('click', function() {
                var bridge = app();
                if (bridge && typeof bridge.setMode === 'function') {
                    bridge.setMode('terminal');
                }
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
                        state.currentView = 'task';
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
        if (!panel) {
            return;
        }
        updateBadge();
        if (!state.authenticated) {
            panel.innerHTML = '';
            return;
        }
        if (state.loading && !state.snapshot) {
            panel.innerHTML = '<div class="project-empty-state">' + escapeHTML(tr('project.loading', 'Loading project panel…')) + '</div>';
            return;
        }
        if (state.error && !state.snapshot) {
            panel.innerHTML = '<div class="project-empty-state error">' + escapeHTML(state.error) + '</div>';
            return;
        }
        ensureSelection();
        panel.innerHTML = (state.error ? '<div class="project-banner error">' + escapeHTML(state.error) + '</div>' : '')
            + renderSidebar()
            + '<main class="project-panel-main"><div class="project-breadcrumbs">' + renderBreadcrumbs() + '</div>' + renderMain() + '</main>';
        bindInteractions(panel);
    }

    function handleAuth(event) {
        state.authenticated = Boolean(event && event.detail && event.detail.authenticated);
        if (!state.authenticated) {
            state.snapshot = null;
            state.error = '';
            state.currentView = 'dashboard';
            render();
            return;
        }
        if (app() && app().getMode && app().getMode() === 'project') {
            loadSnapshot();
        }
    }

    function handleModeChange(event) {
        var mode = event && event.detail ? event.detail.mode : '';
        if (mode === 'project' && state.authenticated) {
            loadSnapshot();
        }
    }

    function init() {
        var badge = getBadge();
        loadFilterPreferences();
        if (badge) {
            badge.addEventListener('click', function() {
                if (app() && app().setMode) {
                    app().setMode('project');
                }
                openApprovals();
                if (!state.snapshot && state.authenticated) {
                    loadSnapshot();
                } else {
                    render();
                }
            });
        }
        document.addEventListener('roambench:auth', handleAuth);
        document.addEventListener('roambench:modechange', handleModeChange);
        if (app() && app().getUsername && app().getUsername()) {
            state.authenticated = true;
        }
        render();
    }

    window.addEventListener('load', init);
})();
