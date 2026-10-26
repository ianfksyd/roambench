(function(root, factory) {
    'use strict';
    var api = factory();
    if (typeof module === 'object' && module.exports) module.exports = api;
    if (root) root.RoamBenchBoardFlow = api;
})(typeof window !== 'undefined' ? window : globalThis, function() {
    'use strict';
    var WIDTH = 300, HEIGHT = 196, GAP_X = 112, GAP_Y = 28, PAD = 24, LANE_HEAD = 56;

    function layout(tasks, refs) {
        var byId = new Map(), edges = new Map();
        refs = Array.isArray(refs) ? refs : [];
        var reviewTargets = new Set(refs.filter(function(ref) { return ref && ref.relation === 'reviews'; }).map(function(ref) { return ref.targetTaskId; }));
        (Array.isArray(tasks) ? tasks : []).forEach(function(task) {
            if (task && typeof task.id === 'string' && !byId.has(task.id)) byId.set(task.id, task);
        });
        function compare(a, b) {
            var ta = byId.get(a), tb = byId.get(b);
            var delta = (Date.parse(ta.createdAt || '') || 0) - (Date.parse(tb.createdAt || '') || 0);
            return delta || a.localeCompare(b);
        }
        var ids = Array.from(byId.keys()).sort(compare);
        function edge(from, to, relation) {
            if (!byId.has(from) || !byId.has(to) || from === to) return;
            var key = JSON.stringify([from, to]);
            if (!edges.has(key)) edges.set(key, {source: from, target: to, relations: []});
            var item = edges.get(key);
            if (item.relations.indexOf(relation) < 0) item.relations.push(relation);
        }
        ids.forEach(function(id) {
            var deps = byId.get(id).dependsOn;
            (Array.isArray(deps) ? deps : []).forEach(function(parent) { edge(parent, id, 'depends_on'); });
        });
        refs.forEach(function(ref) {
            if (ref) edge(ref.sourceTaskId, ref.targetTaskId, ref.relation || 'references');
        });
        var links = Array.from(edges.values()).sort(function(a, b) { return compare(a.source, b.source) || compare(a.target, b.target); });
        var children = new Map(), parents = new Map();
        ids.forEach(function(id) { children.set(id, []); parents.set(id, []); });
        links.forEach(function(link) { children.get(link.source).push(link.target); parents.get(link.target).push(link.source); });

        // Source references can form a cycle even though execution dependencies
        // cannot. Condense each strongly connected component before layering.
        var sequence = 0, indices = new Map(), low = new Map(), stack = [], active = new Set(), components = [], owner = new Map();
        function visit(id) {
            indices.set(id, sequence); low.set(id, sequence++); stack.push(id); active.add(id);
            children.get(id).forEach(function(next) {
                if (!indices.has(next)) { visit(next); low.set(id, Math.min(low.get(id), low.get(next))); }
                else if (active.has(next)) low.set(id, Math.min(low.get(id), indices.get(next)));
            });
            if (low.get(id) !== indices.get(id)) return;
            var members = [], next;
            do { next = stack.pop(); active.delete(next); members.push(next); } while (next !== id);
            members.sort(compare);
            var index = components.length;
            members.forEach(function(member) { owner.set(member, index); });
            components.push({members: members, next: new Set(), incoming: 0, level: 0});
        }
        ids.forEach(function(id) { if (!indices.has(id)) visit(id); });
        links.forEach(function(link) {
            var from = owner.get(link.source), to = owner.get(link.target);
            if (from !== to && !components[from].next.has(to)) { components[from].next.add(to); components[to].incoming++; }
        });
        function componentCompare(a, b) { return compare(components[a].members[0], components[b].members[0]); }
        var queue = components.map(function(_, i) { return i; }).filter(function(i) { return components[i].incoming === 0; }).sort(componentCompare);
        while (queue.length) {
            var current = queue.shift();
            components[current].next.forEach(function(next) {
                components[next].level = Math.max(components[next].level, components[current].level + 1);
                if (--components[next].incoming === 0) { queue.push(next); queue.sort(componentCompare); }
            });
        }
        var nodes = ids.map(function(id) {
            var component = components[owner.get(id)];
            var kind = reviewTargets.has(id) ? 'review'
                : parents.get(id).length > 1 ? 'synthesis'
                : parents.get(id).some(function(parent) { return children.get(parent).length > 1; }) ? 'branch' : 'task';
            return {id: id, task: byId.get(id), projectId: byId.get(id).projectId || '', level: component.level,
                parents: parents.get(id), kind: kind, cyclic: component.members.length > 1, width: WIDTH, height: HEIGHT};
        });
        var nodeById = new Map(nodes.map(function(node) { return [node.id, node]; }));
        var projectIds = Array.from(new Set(nodes.map(function(node) { return node.projectId; }))).sort();
        var top = PAD, lanes = [], maxLevel = 0;
        projectIds.forEach(function(projectId) {
            var members = nodes.filter(function(node) { return node.projectId === projectId; });
            var columns = new Map(), maxRows = 1;
            members.forEach(function(node) {
                if (!columns.has(node.level)) columns.set(node.level, []);
                columns.get(node.level).push(node); maxLevel = Math.max(maxLevel, node.level);
            });
            columns.forEach(function(column) { maxRows = Math.max(maxRows, column.length); });
            Array.from(columns.keys()).sort(function(a, b) { return a - b; }).forEach(function(level) {
                var column = columns.get(level);
                function parentCenter(node) {
                    var ys = node.parents.map(function(id) { return nodeById.get(id).y; }).filter(Number.isFinite);
                    return ys.length ? ys.reduce(function(a, b) { return a + b; }, 0) / ys.length : top;
                }
                column.sort(function(a, b) { return parentCenter(a) - parentCenter(b) || compare(a.id, b.id); });
                column.forEach(function(node, row) {
                    node.x = PAD + level * (WIDTH + GAP_X);
                    node.y = top + LANE_HEAD + (row + (maxRows - column.length) / 2) * (HEIGHT + GAP_Y);
                });
            });
            var height = LANE_HEAD + maxRows * (HEIGHT + GAP_Y) - GAP_Y + PAD;
            lanes.push({projectId: projectId, y: top, height: height, count: members.length});
            top += height + PAD;
        });
        var hasCycle = nodes.some(function(node) { return node.cyclic; });
        links.forEach(function(link) {
            var from = nodeById.get(link.source), to = nodeById.get(link.target);
            var sx = from.x + WIDTH, sy = from.y + HEIGHT / 2, tx = to.x, ty = to.y + HEIGHT / 2;
            link.kind = link.relations.indexOf('reviews') >= 0 ? 'review' : link.relations.indexOf('depends_on') >= 0 ? 'dependency' : 'reference';
            if (to.level <= from.level) {
                tx = to.x + WIDTH;
                var loopX = Math.max(sx, tx) + 76;
                link.path = 'M ' + sx + ' ' + sy + ' C ' + loopX + ' ' + sy + ', ' + loopX + ' ' + ty + ', ' + tx + ' ' + ty;
                link.labelX = loopX - 10; link.labelY = (sy + ty) / 2;
            } else {
                var bend = (tx - sx) / 2;
                link.path = 'M ' + sx + ' ' + sy + ' C ' + (sx + bend) + ' ' + sy + ', ' + (tx - bend) + ' ' + ty + ', ' + tx + ' ' + ty;
                link.labelX = (sx + tx) / 2; link.labelY = (sy + ty) / 2 - 13;
            }
        });
        nodes.sort(function(a, b) { return a.level - b.level || a.y - b.y || compare(a.id, b.id); });
        return {nodes: nodes, edges: links, lanes: lanes,
            width: nodes.length ? (maxLevel + 1) * WIDTH + maxLevel * GAP_X + PAD * 2 + (hasCycle ? 100 : 0) : 0,
            height: nodes.length ? top : 0};
    }
    return {layout: layout};
});
