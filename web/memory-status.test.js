'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const memoryStatus = require('./js/memory-status.js');

test('normalize preserves task-pool metrics and nullable limits', () => {
    const got = memoryStatus.normalize({
        processRSSBytes: 32,
        systemUsedBytes: 120,
        systemAvailableBytes: 40,
        totalMemoryBytes: 160,
        systemSwapUsedBytes: 3,
        systemSwapTotalBytes: 4,
        taskPoolAvailable: true,
        taskPoolCurrentBytes: 60,
        taskPoolAnonBytes: 20,
        taskPoolFileBytes: 35,
        taskPoolSwapBytes: 5,
        taskPoolMemoryHighBytes: null,
        taskPoolMemoryMaxBytes: 80,
        taskPoolPidsCurrent: 141,
        taskPoolPidsMax: 384,
        memoryPressureFullAvg10: 1.5
    });

    assert.equal(got.available, true);
    assert.equal(got.taskPoolAvailable, true);
    assert.equal(got.taskPoolCurrentBytes, 60);
    assert.equal(got.taskPoolMemoryHighBytes, null);
    assert.equal(got.taskPoolMemoryMaxBytes, 80);
    assert.equal(got.taskPoolPidsCurrent, 141);
});

test('classify warns at memory.high and becomes critical near memory.max', () => {
    const base = {
        available: true,
        taskPoolAvailable: true,
        systemSwapUsedBytes: 0,
        systemSwapTotalBytes: 0,
        memoryPressureFullAvg10: 0,
        taskPoolMemoryHighBytes: 60,
        taskPoolMemoryMaxBytes: 80
    };

    assert.equal(memoryStatus.classify({...base, taskPoolCurrentBytes: 59}), 'ready');
    assert.equal(memoryStatus.classify({...base, taskPoolCurrentBytes: 60}), 'warning');
    assert.equal(memoryStatus.classify({...base, taskPoolCurrentBytes: 76}), 'critical');
});

test('classify uses host swap and PSI as independent pressure signals', () => {
    const base = {
        available: true,
        taskPoolAvailable: true,
        taskPoolCurrentBytes: 1,
        taskPoolMemoryHighBytes: null,
        taskPoolMemoryMaxBytes: null
    };

    assert.equal(memoryStatus.classify({...base, systemSwapUsedBytes: 3, systemSwapTotalBytes: 4, memoryPressureFullAvg10: 0}), 'warning');
    assert.equal(memoryStatus.classify({...base, systemSwapUsedBytes: 0, systemSwapTotalBytes: 0, memoryPressureFullAvg10: 10}), 'critical');
    assert.equal(memoryStatus.classify({available: false}), 'unavailable');
});
