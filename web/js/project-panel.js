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
        updatingTaskId: ''
    };

    function app() {
        return window.RoamBenchApp || null;
    }

    function tr(key, fallback, vars) {
        var bridge = app();
        var value = bridge && typeof bridge.t === 'function' ? bridge.t(key, vars) : key;
        return value === key ? fallback : value;
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

    function fetchJSON(url, options) {
        var bridge = app();
        return fetch(bridge && bridge.withBasePath ? bridge.withBasePath(url) : url, options).then(function(response) {
            return response.json().catch(function() { return null; }).then(function(data) {
                var error;
                if (!response.ok) {
                    error = new Error(data && data.error ? data.error : 'Request failed');
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

    function workstreams() {
        return state.snapshot && Array.isArray(state.snapshot.workstreams) ? state.snapshot.workstreams : [];
    }

    function sessions() {
        return state.snapshot && Array.isArray(state.snapshot.sessions) ? state.snapshot.sessions : [];
    }

    function checkpoints() {
        return state.snapshot && Array.isArray(state.snapshot.checkpoints) ? state.snapshot.checkpoints : [];
    }

    function runtimes() {
        return state.snapshot && Array.isArray(state.snapshot.runtimes) ? state.snapshot.runtimes : [];
    }

    function findById(items, id) {
        return (items || []).find(function(item) { return item.id === id; }) || null;
    }

    function getSelectedProject() {
        return findById(state.snapshot && state.snapshot.projects, state.selectedProjectId || (state.snapshot && state.snapshot.activeProjectId));
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
            state.error = err.message || 'Failed to load project panel';
            render();
            if (retryCount < 2) {
                setTimeout(function() { loadSnapshot(retryCount + 1); }, 3000 * (retryCount + 1));
            }
        });
    }

    function openDashboard(projectId) {
        state.selectedProjectId = projectId || state.selectedProjectId;
        state.selectedWorkstreamId = '';
        state.selectedTaskId = '';
        state.selectedSessionId = '';
        state.currentView = 'dashboard';
        render();
    }

    function openWorkstream(workstreamId) {
        var workstream = findById(workstreams(), workstreamId);
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
                state.eventsError = err.message || 'Failed to load events';
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
                state.eventsError = err.message || 'Failed to load replay';
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

    function renderMetric(label, value, tone) {
        return '<div class="project-metric ' + (tone ? 'tone-' + tone : '') + '">'
            + '<div class="project-metric-label">' + escapeHTML(label) + '</div>'
            + '<div class="project-metric-value">' + escapeHTML(value) + '</div>'
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
            state.error = err.message || message || 'Failed to refresh project panel';
            render();
            return null;
        });
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
                refreshSnapshotAfterConflict((err.message || 'Update workstream failed') + '. Reloaded latest state.');
                return;
            }
            state.error = err.message || 'Update workstream failed';
            render();
        });
    }

    function updateTaskInline(taskId, action) {
        var task = findById(tasks(), taskId);
        if (!task) {
            return;
        }
        state.updatingTaskId = taskId;
        state.error = '';
        render();
        fetchJSON('/api/project-control/tasks/' + encodeURIComponent(taskId), {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                expectedRowVersion: task.rowVersion,
                action: action
            })
        }).then(function(snapshot) {
            state.updatingTaskId = '';
            state.snapshot = snapshot;
            openTask(taskId);
            updateBadge();
            render();
        }).catch(function(err) {
            state.updatingTaskId = '';
            if (err && err.status === 409) {
                refreshSnapshotAfterConflict((err.message || 'Update task failed') + '. Reloaded latest state.');
                return;
            }
            state.error = err.message || 'Update task failed';
            render();
        });
    }

    function renderActionButtons(buttons, dataAttr, entityId, updating) {
        var disabled = updating ? ' disabled' : '';
        return (buttons || []).map(function(button) {
            var tone = button.tone ? ' ' + button.tone : '';
            return '<button type="button" class="project-inline-btn' + tone + '" ' + dataAttr + '="' + escapeHTML(entityId) + '" data-action="' + escapeHTML(button.action) + '"' + disabled + '>' + escapeHTML(button.label) + '</button>';
        }).join('');
    }

    function recommendedWorkstreamActions(workstream) {
        switch (workstream.status) {
        case 'planned':
            return [{ action: 'start_execution', label: 'Start', tone: 'primary' }];
        case 'running':
            return [
                { action: 'request_human_input', label: 'Need human input' },
                { action: 'mark_blocked', label: 'Mark blocked' },
                { action: 'mark_completed', label: 'Complete', tone: 'primary' }
            ];
        case 'waiting_human':
            return [
                { action: 'resume_execution', label: 'Resume', tone: 'primary' },
                { action: 'mark_blocked', label: 'Mark blocked' }
            ];
        case 'blocked':
            return [{ action: 'resume_execution', label: 'Resume', tone: 'primary' }];
        case 'completed':
            return [{ action: 'archive', label: 'Archive' }];
        default:
            return [];
        }
    }

    function recommendedTaskActions(task) {
        if (task.acceptanceStatus === 'ready_for_acceptance') {
            return [
                { action: 'request_acceptance_review', label: 'Send to approvals', tone: 'primary' },
                { action: 'reopen_task', label: 'Reopen task' }
            ];
        }
        if (task.acceptanceStatus === 'under_human_review') {
            return [{ action: 'reopen_task', label: 'Reopen task' }];
        }
        if (task.acceptanceStatus === 'rejected') {
            return [{ action: 'reopen_task', label: 'Resume revisions', tone: 'primary' }];
        }
        switch (task.state) {
        case 'planned':
            return [
                { action: 'queue_task', label: 'Queue' },
                { action: 'start_execution', label: 'Start', tone: 'primary' }
            ];
        case 'queued':
            return [
                { action: 'start_execution', label: 'Start', tone: 'primary' },
                { action: 'mark_blocked', label: 'Mark blocked' }
            ];
        case 'running':
            return [
                { action: 'request_human_input', label: 'Need human input' },
                { action: 'mark_waiting_review', label: 'Waiting review' },
                { action: 'mark_blocked', label: 'Mark blocked' },
                { action: 'mark_execution_complete', label: 'Execution complete', tone: 'primary' }
            ];
        case 'waiting_review':
            return [
                { action: 'resume_execution', label: 'Resume' },
                { action: 'mark_execution_complete', label: 'Execution complete', tone: 'primary' }
            ];
        case 'waiting_human':
            return [
                { action: 'resume_execution', label: 'Resume', tone: 'primary' },
                { action: 'mark_blocked', label: 'Mark blocked' }
            ];
        case 'blocked':
            return [{ action: 'resume_execution', label: 'Resume', tone: 'primary' }];
        case 'execution_complete':
            if (task.acceptanceStatus === 'accepted') {
                return [
                    { action: 'archive', label: 'Archive', tone: 'primary' },
                    { action: 'reopen_task', label: 'Reopen task' }
                ];
            }
            if (task.acceptanceStatus === 'not_ready') {
                return [
                    { action: 'mark_ready_for_acceptance', label: 'Ready for acceptance', tone: 'primary' },
                    { action: 'request_archive_override', label: 'Request archive override' },
                    { action: 'reopen_task', label: 'Reopen task' }
                ];
            }
            return [{ action: 'reopen_task', label: 'Reopen task' }];
        case 'archived':
            return [{ action: 'unarchive', label: 'Unarchive', tone: 'primary' }];
        default:
            return [];
        }
    }

    function renderWorkstreamInlineControls(workstream) {
        var updating = state.updatingWorkstreamId === workstream.id;
        var actions = recommendedWorkstreamActions(workstream);
        return '<div class="project-inline-controls" data-workstream-controls="' + escapeHTML(workstream.id) + '">'
            + '<div class="project-inline-meta"><strong>State:</strong> ' + escapeHTML(workstream.status) + ' • <strong>Priority:</strong> ' + escapeHTML(workstream.priority) + ' • row v' + escapeHTML(workstream.rowVersion) + '</div>'
            + '<div class="project-action-group">'
            + renderActionButtons(actions, 'data-update-workstream', workstream.id, updating)
            + (updating ? '<span class="project-inline-meta">Saving…</span>' : '')
            + (!actions.length ? '<span class="project-inline-meta">No recommended actions.</span>' : '')
            + '</div>'
            + '</div>';
    }

    function renderTaskInlineControls(task) {
        var updating = state.updatingTaskId === task.id;
        var actions = recommendedTaskActions(task);
        return '<div class="project-inline-controls" data-task-controls="' + escapeHTML(task.id) + '">'
            + '<div class="project-inline-meta"><strong>State:</strong> ' + escapeHTML(task.state) + ' • <strong>Acceptance:</strong> ' + escapeHTML(task.acceptanceStatus) + ' • row v' + escapeHTML(task.rowVersion) + '</div>'
            + '<div class="project-action-group">'
            + renderActionButtons(actions, 'data-update-task', task.id, updating)
            + (updating ? '<span class="project-inline-meta">Saving…</span>' : '')
            + (!actions.length ? '<span class="project-inline-meta">No recommended actions.</span>' : '')
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
            return 'Decision lane';
        case 'checkpoint':
            return 'Checkpoint lane';
        case 'acceptance':
            return 'Acceptance lane';
        default:
            return 'Execution lane';
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
            { value: 'all', label: 'All lanes' },
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
        var placeholder = kind === 'events' ? 'Search history…' : 'Search replay…';
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
                + '<span class="project-list-meta">' + escapeHTML(String(group.items.length) + ' events') + '</span>'
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
            return '<div class="project-list-item muted">No decision recorded yet.</div>';
        }
        return '<div class="project-decision-card">'
            + '<div class="project-card-title">' + escapeHTML(title || 'Decision') + '</div>'
            + '<div class="project-task-badges">'
            + '<span class="project-pill lane-decision">' + escapeHTML(decision.decisionType || 'decision') + '</span>'
            + (decision.checkpointId ? '<span class="project-pill lane-checkpoint">' + escapeHTML(decision.checkpointId) + '</span>' : '')
            + '</div>'
            + '<div class="project-list-item"><strong>Actor:</strong> ' + escapeHTML(decision.actor || '—') + '</div>'
            + '<div class="project-list-item"><strong>When:</strong> ' + escapeHTML(formatTime(decision.timestamp)) + '</div>'
            + '<div class="project-list-item"><strong>Summary:</strong> ' + escapeHTML(decision.summary || '—') + '</div>'
            + '<div class="project-list-item"><strong>Decision ID:</strong> ' + escapeHTML(decision.id || '—') + '</div>'
            + '</div>';
    }

    function approvalKindLabel(kind) {
        switch (String(kind || '').trim()) {
        case 'final_acceptance':
            return 'Final acceptance';
        case 'archive_override':
            return 'Archive override';
        default:
            return String(kind || 'checkpoint').trim() || 'checkpoint';
        }
    }

    function approvalStatusLabel(status) {
        switch (String(status || '').trim()) {
        case 'pending':
            return 'Pending review';
        case 'approved':
            return 'Approved';
        case 'rejected':
            return 'Rejected';
        case 'rerouted':
            return 'Rerouted';
        case 'expired':
            return 'Expired';
        default:
            return String(status || 'unknown').trim() || 'unknown';
        }
    }

    function approvalActionLabel(action) {
        switch (String(action || '').trim()) {
        case 'approve':
            return 'Approve';
        case 'reject':
            return 'Reject';
        case 'reroute':
            return 'Reroute';
        default:
            return String(action || '').trim() || 'Action';
        }
    }

    function approvalKindSummary(item) {
        var kind = String(item && item.kind || '').trim();
        if (kind === 'final_acceptance') {
            return item && item.status === 'pending'
                ? 'Human sign-off is required before this task can be treated as accepted work.'
                : 'Records the outcome of explicit final acceptance review for this task.';
        }
        if (kind === 'archive_override') {
            return item && item.status === 'pending'
                ? 'Human override is required before archiving execution-complete work that has not been accepted.'
                : 'Records the outcome of an explicit archive override decision for this task.';
        }
        return 'Checkpoint-backed approval item.';
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
                    statusLabel: acceptanceDecision.decisionType === 'final_acceptance_approved' ? 'Approved' : 'Rejected',
                    summary: acceptanceDecision.summary || 'Final acceptance decision recorded.',
                    meta: formatTime(acceptanceDecision.timestamp) + ' • ' + (acceptanceDecision.id || '')
                };
            }
        }
        if (kind === 'archive_override' && task.archiveDecisionId) {
            var archiveDecision = decisionById(task.archiveDecisionId);
            if (archiveDecision) {
                return {
                    statusLabel: archiveDecision.decisionType === 'archive_override_approved' ? 'Approved' : 'Rejected',
                    summary: archiveDecision.summary || 'Archive override decision recorded.',
                    meta: formatTime(archiveDecision.timestamp) + ' • ' + (archiveDecision.id || '')
                };
            }
        }
        return null;
    }

    function renderTaskApprovalStatusCard(task) {
        var finalAcceptance = currentTaskApprovalState(task, 'final_acceptance');
        var archiveOverride = currentTaskApprovalState(task, 'archive_override');

        function renderStatusRow(label, approval) {
            if (!approval) {
                return '<div class="project-list-item"><strong>' + escapeHTML(label) + ':</strong> None</div>';
            }
            return '<div class="project-list-item">'
                + '<strong>' + escapeHTML(label) + ':</strong> '
                + '<span class="project-pill lane-checkpoint">' + escapeHTML(approval.statusLabel) + '</span>'
                + '<div class="project-approval-summary">' + escapeHTML(approval.summary || '') + '</div>'
                + '<div class="project-list-meta">' + escapeHTML(approval.meta || '') + '</div>'
                + ((approval.actions || []).length
                    ? '<div class="project-approval-actions">'
                        + approval.actions.map(function(action) {
                            return '<button type="button" class="project-inline-btn ' + (action === 'approve' ? 'primary' : '') + '" data-checkpoint-id="' + escapeHTML(approval.checkpointId) + '" data-checkpoint-action="' + escapeHTML(action) + '" data-checkpoint-source="task">' + escapeHTML(approvalActionLabel(action)) + '</button>';
                        }).join('')
                        + '</div>'
                    : '')
                + '</div>';
        }

        return '<div class="project-card-list"><h3>' + escapeHTML(tr('project.currentApprovals', 'Current approvals')) + '</h3>'
            + renderStatusRow('Final acceptance', finalAcceptance)
            + renderStatusRow('Archive override', archiveOverride)
            + '</div>';
    }

    function taskApprovalBadges(task) {
        var badges = [];
        var finalAcceptance = currentTaskApprovalState(task, 'final_acceptance');
        var archiveOverride = currentTaskApprovalState(task, 'archive_override');

        if (finalAcceptance) {
            if (finalAcceptance.actions && finalAcceptance.actions.length) {
                badges.push({ className: 'approval-pending', label: 'Final acceptance pending' });
            } else if (task && task.acceptanceStatus === 'accepted') {
                badges.push({ className: 'approval-approved', label: 'Accepted' });
            } else if (task && task.acceptanceStatus === 'rejected') {
                badges.push({ className: 'approval-rejected', label: 'Final acceptance rejected' });
            }
        }

        if (archiveOverride) {
            if (archiveOverride.actions && archiveOverride.actions.length) {
                badges.push({ className: 'approval-pending', label: 'Archive override pending' });
            } else if (task && task.archiveDecisionId) {
                badges.push({ className: archiveOverride.statusLabel === 'Approved' ? 'approval-approved' : 'approval-rejected', label: 'Archive override ' + archiveOverride.statusLabel.toLowerCase() });
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

    function pendingApprovalCountByKind(kind) {
        return checkpoints().filter(function(item) {
            return item.kind === kind && item.status === 'pending';
        }).length;
    }

    function recentDecisionSummaries(limit) {
        var items = decisions().slice();
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

    function renderApprovalOverviewCard() {
        var pendingFinalAcceptance = pendingApprovalCountByKind('final_acceptance');
        var pendingArchiveOverride = pendingApprovalCountByKind('archive_override');
        var recent = recentDecisionSummaries(4);
        return '<div class="project-card-list"><h3>' + escapeHTML(tr('project.approvalOverview', 'Approval Overview')) + '</h3>'
            + '<div class="project-list-item"><strong>Pending final acceptance:</strong> ' + escapeHTML(String(pendingFinalAcceptance)) + '</div>'
            + '<div class="project-list-item"><strong>Pending archive override:</strong> ' + escapeHTML(String(pendingArchiveOverride)) + '</div>'
            + '<div class="project-list-item"><strong>Total pending approvals:</strong> ' + escapeHTML(String(checkpoints().filter(function(item) { return item.status === 'pending'; }).length)) + '</div>'
            + '<h3>' + escapeHTML(tr('project.recentDecisions', 'Recent Decisions')) + '</h3>'
            + (recent.length
                ? recent.map(function(item) {
                    return '<div class="project-list-item">'
                        + '<div class="project-list-title">' + escapeHTML(item.decisionType || 'decision') + '</div>'
                        + '<div>' + escapeHTML(item.summary || '—') + '</div>'
                        + '<div class="project-list-meta">' + escapeHTML(formatTime(item.timestamp) + ' • ' + (item.taskID || item.taskId || 'task-scope')) + '</div>'
                        + '</div>';
                }).join('')
                : '<div class="project-list-item muted">' + escapeHTML(tr('project.noRecentDecisions', 'No decisions recorded yet.')) + '</div>')
            + '</div>';
    }

    function renderSidebar() {
        var projectList = (state.snapshot && state.snapshot.projects) || [];
        var runtimeList = runtimes();
        var approvalsPending = checkpoints().filter(function(item) { return item.status === 'pending'; });

        return '<aside class="project-panel-sidebar">'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarProjects', 'Projects')) + '</div>'
            + projectList.map(function(project) {
                return '<button type="button" class="project-nav-btn' + (project.id === state.selectedProjectId && state.currentView === 'dashboard' ? ' active' : '') + '" data-project-id="' + escapeHTML(project.id) + '">'
                    + '<span class="project-nav-title">' + escapeHTML(project.name) + '</span>'
                    + '<span class="project-nav-meta">' + escapeHTML(project.status) + '</span>'
                    + '</button>';
            }).join('')
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarWorkstreams', 'Workstreams')) + '</div>'
            + workstreams().map(function(item) {
                return '<button type="button" class="project-nav-btn' + (item.id === state.selectedWorkstreamId && state.currentView === 'workstream' ? ' active' : '') + '" data-workstream-id="' + escapeHTML(item.id) + '">'
                    + '<span class="project-nav-title">' + escapeHTML(item.title) + '</span>'
                    + '<span class="project-nav-meta">' + escapeHTML(item.status + ' • ' + item.priority) + '</span>'
                    + '</button>';
            }).join('')
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarActions', 'Actions')) + '</div>'
            + '<button type="button" class="project-nav-btn' + (state.currentView === 'approvals' ? ' active' : '') + '" data-open-approvals="1">'
            + '<span class="project-nav-title">' + escapeHTML(tr('project.approvals', 'Approvals Inbox')) + '</span>'
            + '<span class="project-nav-meta">' + escapeHTML(String(approvalsPending.length) + ' pending') + '</span>'
            + '</button>'
            + '</div>'
            + '<div class="project-sidebar-section">'
            + '<div class="project-sidebar-kicker">' + escapeHTML(tr('project.sidebarRuntimes', 'Runtimes')) + '</div>'
            + runtimeList.map(function(runtime) {
                return '<div class="project-runtime-card">'
                    + '<div class="project-runtime-name">' + escapeHTML(runtime.name) + '</div>'
                    + '<div class="project-runtime-meta">' + escapeHTML(runtime.status + ' • ' + runtime.kind) + '</div>'
                    + '<div class="project-runtime-health">' + escapeHTML(runtime.healthSummary) + '</div>'
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
        var dashboard = state.snapshot && state.snapshot.dashboard;
        var items = workstreams();
        if (!project || !dashboard) {
            return '<div class="project-empty-state">' + escapeHTML(tr('project.empty', 'No project data yet.')) + '</div>';
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header">'
            + '<div><div class="project-section-kicker">' + escapeHTML(tr('project.dashboard', 'Project Dashboard')) + '</div>'
            + '<h2>' + escapeHTML(project.name) + '</h2><p>' + escapeHTML(project.currentGoal || project.description) + '</p></div>'
            + '<div class="project-header-actions">'
            + '<button type="button" class="project-inline-btn" data-create-project="1">' + escapeHTML(tr('project.newProject', 'New Project')) + '</button>'
            + '<button type="button" class="project-inline-btn" data-create-workstream="' + escapeHTML(project.id) + '">' + escapeHTML(tr('project.newWorkstream', 'New Workstream')) + '</button>'
            + '<button type="button" class="project-inline-btn" data-project-history="' + escapeHTML(project.id) + '">' + escapeHTML(tr('project.viewHistory', 'View history')) + '</button>'
            + '<button type="button" class="project-inline-btn" data-open-approvals="1">' + escapeHTML(tr('project.openApprovals', 'Open approvals')) + '</button>'
            + '</div>'
            + '</div>'
            + '<div class="project-metrics-grid">'
            + renderMetric(tr('project.metricRunningWorkstreams', 'Running workstreams'), dashboard.runningWorkstreams, 'info')
            + renderMetric(tr('project.metricRunningTasks', 'Running tasks'), dashboard.runningTasks, 'info')
            + renderMetric(tr('project.metricBlockedTasks', 'Blocked tasks'), dashboard.blockedTasks, 'warning')
            + renderMetric(tr('project.metricPendingApprovals', 'Pending approvals'), dashboard.pendingApprovals, dashboard.pendingApprovals ? 'danger' : 'success')
            + '</div>'
            + '<div class="project-card-grid">'
            + items.map(function(item) {
                return '<button type="button" class="project-card" data-workstream-id="' + escapeHTML(item.id) + '">'
                    + '<div class="project-card-title">' + escapeHTML(item.title) + '</div>'
                    + '<div class="project-card-meta">' + escapeHTML(item.status + ' • ' + item.priority) + '</div>'
                    + '<div class="project-card-copy">' + escapeHTML(item.scopeSummary) + '</div>'
                    + '</button>';
            }).join('')
            + '</div>'
            + '<div class="project-two-column">'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.runtimeHealth', 'Runtime Health')) + '</h3>'
            + (dashboard.runtimeHealth || []).map(function(line) { return '<div class="project-list-item">' + escapeHTML(line) + '</div>'; }).join('') + '</div>'
            + renderApprovalOverviewCard()
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.timeline', 'Recent Timeline')) + '</h3>'
            + (dashboard.projectTimeline || []).map(function(line) { return '<div class="project-list-item">' + escapeHTML(line) + '</div>'; }).join('') + '</div>'
            + '</div>'
            + '</section>';
    }

    function renderWorkstreamBoard() {
        var workstream = getSelectedWorkstream();
        var columns = ['planned', 'running', 'waiting_human', 'blocked', 'execution_complete'];
        var labels = {
            planned: tr('project.columnPlanned', 'Planned'),
            running: tr('project.columnRunning', 'Running'),
            waiting_human: tr('project.columnWaitingHuman', 'Waiting Human'),
            blocked: tr('project.columnBlocked', 'Blocked'),
            execution_complete: tr('project.columnExecutionComplete', 'Execution Complete')
        };
        if (!workstream) {
            return renderDashboard();
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header">'
            + '<div><div class="project-section-kicker">' + escapeHTML(tr('project.board', 'Workstream Board')) + '</div>'
            + '<h2>' + escapeHTML(workstream.title) + '</h2><p>' + escapeHTML(workstream.description) + '</p></div>'
            + '<div class="project-header-actions">'
            + '<button type="button" class="project-inline-btn" data-create-task="' + escapeHTML(workstream.id) + '">' + escapeHTML(tr('project.newTask', 'New Task')) + '</button>'
            + '</div>'
            + renderWorkstreamInlineControls(workstream)
            + '</div>'
            + '<div class="project-board">'
            + columns.map(function(column) {
                return '<div class="project-board-column"><div class="project-board-column-title">' + escapeHTML(labels[column]) + '</div>'
                    + tasksForWorkstream(workstream.id).filter(function(task) { return task.state === column; }).map(function(task) {
                        return '<button type="button" class="project-task-card" data-task-id="' + escapeHTML(task.id) + '">'
                            + '<div class="project-task-title">' + escapeHTML(task.title) + '</div>'
                            + '<div class="project-task-badges"><span class="project-pill state-' + escapeHTML(task.state) + '">' + escapeHTML(task.state) + '</span>'
                            + '<span class="project-pill acceptance-' + escapeHTML(task.acceptanceStatus) + '">' + escapeHTML(task.acceptanceStatus) + '</span>'
                            + renderTaskApprovalBadges(task) + '</div>'
                            + '<div class="project-task-copy">' + escapeHTML(task.recentSummary) + '</div>'
                            + '<div class="project-task-meta">' + escapeHTML(task.agentLabel + ' • ' + task.riskLevel) + '</div>'
                            + '</button>';
                    }).join('')
                    + '</div>';
            }).join('')
            + '</div>'
            + '</section>';
    }

    function renderTaskDetail() {
        var task = getSelectedTask();
        var acceptanceDecision = task && task.acceptanceDecisionId ? decisionById(task.acceptanceDecisionId) : null;
        var archiveDecision = task && task.archiveDecisionId ? decisionById(task.archiveDecisionId) : null;
        if (!task) {
            return renderDashboard();
        }
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.taskDetail', 'Task Detail')) + '</div>'
            + '<h2>' + escapeHTML(task.title) + '</h2><p>' + escapeHTML(task.goal) + '</p></div></div>'
            + '<div class="project-tab-sections">'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.overview', 'Overview')) + '</h3>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.state', 'State')) + ':</strong> ' + escapeHTML(task.state) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.acceptance', 'Acceptance')) + ':</strong> ' + escapeHTML(task.acceptanceStatus) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.risk', 'Risk')) + ':</strong> ' + escapeHTML(task.riskLevel) + '</div>'
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.nextStep', 'Next step')) + ':</strong> ' + escapeHTML(task.nextStep) + '</div>'
            + renderTaskInlineControls(task)
            + '</div>'
            + renderTaskApprovalStatusCard(task)
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.timeline', 'Timeline')) + '</h3>'
            + (task.timeline || []).map(function(item) {
                return '<div class="project-list-item"><div class="project-list-title">' + escapeHTML(item.action) + '</div><div>' + escapeHTML(item.detail) + '</div><div class="project-list-meta">' + escapeHTML(item.actor + ' • ' + formatTime(item.timestamp)) + '</div></div>';
            }).join('') + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.evidence', 'Evidence')) + '</h3>'
            + (task.evidence || []).map(function(item) { return '<div class="project-list-item"><div class="project-list-title">' + escapeHTML(item.label) + '</div><div>' + escapeHTML(item.value) + '</div></div>'; }).join('') + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.filesDiff', 'Files & Diff')) + '</h3>'
            + (task.filesChanged || []).map(function(path) { return '<div class="project-list-item">' + escapeHTML(path) + '</div>'; }).join('')
            + '<div class="project-list-item">' + escapeHTML(task.diffSummary || '—') + '</div></div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.sessions', 'Sessions')) + '</h3>'
            + sessionsForTask(task.id).map(function(session) {
                return '<button type="button" class="project-list-button" data-session-id="' + escapeHTML(session.id) + '">'
                    + '<span>' + escapeHTML(session.name) + '</span><span class="project-list-meta">' + escapeHTML(session.state + ' • ' + session.durationLabel) + '</span></button>';
            }).join('') + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.audit', 'Audit')) + '</h3>'
            + ((task.audit || []).length ? task.audit.map(function(item) {
                return '<div class="project-list-item"><div class="project-list-title">' + escapeHTML(item.action) + '</div><div>' + escapeHTML(item.detail) + '</div><div class="project-list-meta">' + escapeHTML(item.actor + ' • ' + formatTime(item.timestamp)) + '</div></div>';
            }).join('') : '<div class="project-list-item muted">' + escapeHTML(tr('project.noAudit', 'No audit entries yet.')) + '</div>')
            + '<button type="button" class="project-inline-btn" data-task-history="' + escapeHTML(task.id) + '">' + escapeHTML(tr('project.viewHistory', 'View history')) + '</button>'
            + '<button type="button" class="project-inline-btn" data-task-replay="' + escapeHTML(task.id) + '">' + escapeHTML(tr('project.openReplay', 'Open replay')) + '</button>'
            + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.acceptanceDecision', 'Acceptance Decision')) + '</h3>'
            + renderDecisionCard(acceptanceDecision, 'Acceptance Decision')
            + '</div>'
            + '<div class="project-card-list"><h3>' + escapeHTML(tr('project.archiveDecision', 'Archive Decision')) + '</h3>'
            + renderDecisionCard(archiveDecision, 'Archive Decision')
            + '</div>'
            + '</div>'
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
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.role', 'Role')) + ':</strong> ' + escapeHTML(session.role) + '</div>'
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
        return '<div class="project-approval-card">'
            + '<div class="project-approval-header"><div><div class="project-card-title">' + escapeHTML(item.title) + '</div><div class="project-card-meta">' + escapeHTML(kindLabel + ' • ' + statusLabel) + '</div></div>'
            + '<button type="button" class="project-inline-btn" data-task-id="' + escapeHTML(item.taskId) + '">' + escapeHTML(tr('project.openTask', 'Open task')) + '</button></div>'
            + '<div class="project-task-badges">'
            + '<span class="project-pill lane-checkpoint">' + escapeHTML(kindLabel) + '</span>'
            + '<span class="project-pill lane-decision">' + escapeHTML(statusLabel) + '</span>'
            + '</div>'
            + '<div class="project-approval-summary">' + escapeHTML(approvalKindSummary(item)) + '</div>'
            + '<div class="project-card-copy">' + escapeHTML(item.reason) + '</div>'
            + '<div class="project-card-meta">' + escapeHTML(formatTime(item.requestedAt)) + '</div>'
            + '<div class="project-approval-actions">'
            + (item.allowedActions || []).map(function(action) {
                return '<button type="button" class="project-inline-btn ' + (action === 'approve' ? 'primary' : '') + '" data-checkpoint-id="' + escapeHTML(item.id) + '" data-checkpoint-action="' + escapeHTML(action) + '">' + escapeHTML(approvalActionLabel(action)) + '</button>';
            }).join('')
            + (item.decisionSummary ? '<div class="project-approval-note">' + escapeHTML(item.decisionSummary) + '</div>' : '')
            + '</div></div>';
    }

    function renderApprovals() {
        var all = checkpoints();
        var pending = all.filter(function(item) { return item.status === 'pending'; });
        var resolved = all.filter(function(item) { return item.status !== 'pending'; });
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.approvals', 'Approvals Inbox')) + '</div>'
            + '<h2>' + escapeHTML(tr('project.pendingCheckpoints', 'Pending checkpoints')) + '</h2>'
            + '<p>' + escapeHTML(tr('project.checkpointSource', 'All approval items are filtered views over checkpoint records.')) + '</p></div></div>'
            + (pending.length
                ? pending.map(renderApprovalCard).join('')
                : '<div class="project-list-item muted">' + escapeHTML(tr('project.noPending', 'No pending approvals.')) + '</div>')
            + (resolved.length
                ? '<div class="project-sidebar-kicker" style="margin-top:1.5rem">' + escapeHTML(tr('project.resolvedCheckpoints', 'Resolved')) + '</div>'
                  + resolved.map(renderApprovalCard).join('')
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
            + renderDecisionCard(state.currentReplay.acceptanceDecision, 'Replay Acceptance Decision')
            + '<h3>' + escapeHTML(tr('project.archiveDecision', 'Archive Decision')) + '</h3>'
            + renderDecisionCard(state.currentReplay.archiveDecision, 'Replay Archive Decision')
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
        var name = window.prompt(tr('project.promptProjectName', 'Project name:'), 'New Project');
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
            state.error = err.message || 'Create project failed';
            render();
        });
    }

    function createWorkstream(projectId) {
        var title = window.prompt(tr('project.promptWorkstreamTitle', 'Workstream title:'), 'New Workstream');
        var description;
        var scopeSummary;
        if (title === null || !String(title).trim()) {
            return;
        }
        description = window.prompt(tr('project.promptWorkstreamDescription', 'Workstream description:'), '');
        if (description === null) {
            description = '';
        }
        scopeSummary = window.prompt(tr('project.promptWorkstreamScope', 'Scope summary:'), '');
        if (scopeSummary === null) {
            scopeSummary = '';
        }
        fetchJSON('/api/project-control/workstreams', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                projectId: projectId,
                title: String(title).trim(),
                description: String(description || '').trim(),
                priority: 'medium',
                scopeSummary: String(scopeSummary || '').trim()
            })
        }).then(function(snapshot) {
            state.snapshot = snapshot;
            state.selectedProjectId = projectId;
            updateBadge();
            render();
        }).catch(function(err) {
            state.error = err.message || 'Create workstream failed';
            render();
        });
    }

    function createTask(workstreamId) {
        var workstream = findById(workstreams(), workstreamId);
        var title = window.prompt(tr('project.promptTaskTitle', 'Task title:'), 'New Task');
        var goal;
        if (!workstream || title === null || !String(title).trim()) {
            return;
        }
        goal = window.prompt(tr('project.promptTaskGoal', 'Task goal:'), '');
        if (goal === null) {
            goal = '';
        }
        fetchJSON('/api/project-control/tasks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                projectId: workstream.projectId,
                workstreamId: workstream.id,
                title: String(title).trim(),
                goal: String(goal || '').trim(),
                priority: 'medium',
                riskLevel: 'medium'
            })
        }).then(function(snapshot) {
            state.snapshot = snapshot;
            openWorkstream(workstreamId);
            updateBadge();
            render();
        }).catch(function(err) {
            state.error = err.message || 'Create task failed';
            render();
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
        panel.querySelectorAll('[data-create-workstream]').forEach(function(node) {
            node.addEventListener('click', function() {
                createWorkstream(node.getAttribute('data-create-workstream'));
            });
        });
        panel.querySelectorAll('[data-create-task]').forEach(function(node) {
            node.addEventListener('click', function() {
                createTask(node.getAttribute('data-create-task'));
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
                    state.error = err.message || 'Decision failed';
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
