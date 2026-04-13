(function() {
    'use strict';

    var state = {
        authenticated: false,
        snapshot: null,
        loading: false,
        error: '',
        selectedProjectId: '',
        selectedWorkstreamId: '',
        selectedTaskId: '',
        selectedSessionId: '',
        currentView: 'dashboard'
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

    function fetchJSON(url, options) {
        var bridge = app();
        return fetch(bridge && bridge.withBasePath ? bridge.withBasePath(url) : url, options).then(function(response) {
            return response.json().catch(function() { return null; }).then(function(data) {
                if (!response.ok) {
                    throw new Error(data && data.error ? data.error : 'Request failed');
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

    function renderMetric(label, value, tone) {
        return '<div class="project-metric ' + (tone ? 'tone-' + tone : '') + '">'
            + '<div class="project-metric-label">' + escapeHTML(label) + '</div>'
            + '<div class="project-metric-value">' + escapeHTML(value) + '</div>'
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
            + '</div>'
            + '<div class="project-board">'
            + columns.map(function(column) {
                return '<div class="project-board-column"><div class="project-board-column-title">' + escapeHTML(labels[column]) + '</div>'
                    + tasksForWorkstream(workstream.id).filter(function(task) { return task.state === column; }).map(function(task) {
                        return '<button type="button" class="project-task-card" data-task-id="' + escapeHTML(task.id) + '">'
                            + '<div class="project-task-title">' + escapeHTML(task.title) + '</div>'
                            + '<div class="project-task-badges"><span class="project-pill state-' + escapeHTML(task.state) + '">' + escapeHTML(task.state) + '</span>'
                            + '<span class="project-pill acceptance-' + escapeHTML(task.acceptanceStatus) + '">' + escapeHTML(task.acceptanceStatus) + '</span></div>'
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
            + '<div class="project-list-item"><strong>' + escapeHTML(tr('project.nextStep', 'Next step')) + ':</strong> ' + escapeHTML(task.nextStep) + '</div></div>'
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

    function renderApprovals() {
        var items = checkpoints();
        return '<section class="project-main-section">'
            + '<div class="project-section-header"><div><div class="project-section-kicker">' + escapeHTML(tr('project.approvals', 'Approvals Inbox')) + '</div>'
            + '<h2>' + escapeHTML(tr('project.pendingCheckpoints', 'Pending checkpoints')) + '</h2>'
            + '<p>' + escapeHTML(tr('project.checkpointSource', 'All approval items are filtered views over checkpoint records.')) + '</p></div></div>'
            + items.map(function(item) {
                return '<div class="project-approval-card">'
                    + '<div class="project-approval-header"><div><div class="project-card-title">' + escapeHTML(item.title) + '</div><div class="project-card-meta">' + escapeHTML(item.kind + ' • ' + item.status) + '</div></div>'
                    + '<button type="button" class="project-inline-btn" data-task-id="' + escapeHTML(item.taskId) + '">' + escapeHTML(tr('project.openTask', 'Open task')) + '</button></div>'
                    + '<div class="project-card-copy">' + escapeHTML(item.reason) + '</div>'
                    + '<div class="project-card-meta">' + escapeHTML(formatTime(item.requestedAt)) + '</div>'
                    + '<div class="project-approval-actions">'
                    + (item.allowedActions || []).map(function(action) {
                        return '<button type="button" class="project-inline-btn ' + (action === 'approve' ? 'primary' : '') + '" data-checkpoint-id="' + escapeHTML(item.id) + '" data-checkpoint-action="' + escapeHTML(action) + '">' + escapeHTML(action) + '</button>';
                    }).join('')
                    + (item.decisionSummary ? '<div class="project-approval-note">' + escapeHTML(item.decisionSummary) + '</div>' : '')
                    + '</div></div>';
            }).join('')
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
                fetchJSON('/api/project-control/checkpoints/' + encodeURIComponent(checkpointId) + '/decision', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ action: action })
                }).then(function(snapshot) {
                    state.snapshot = snapshot;
                    state.currentView = 'approvals';
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
