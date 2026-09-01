(function(root, factory) {
    'use strict';

    var api = factory();
    if (typeof module === 'object' && module.exports) {
        module.exports = api;
    }
    if (root) {
        root.RoamBenchMemoryStatus = api;
    }
})(typeof window !== 'undefined' ? window : globalThis, function() {
    'use strict';

    function numberOrZero(value) {
        value = Number(value);
        return Number.isFinite(value) && value >= 0 ? value : 0;
    }

    function nullableLimit(value) {
        if (value === null || value === undefined || value === '') {
            return null;
        }
        value = Number(value);
        return Number.isFinite(value) && value >= 0 ? value : null;
    }

    function normalize(data) {
        data = data || {};
        var status = {
            processRSSBytes: numberOrZero(data.processRSSBytes),
            systemUsedBytes: numberOrZero(data.systemUsedBytes),
            systemAvailableBytes: numberOrZero(data.systemAvailableBytes),
            totalMemoryBytes: numberOrZero(data.totalMemoryBytes),
            systemSwapUsedBytes: numberOrZero(data.systemSwapUsedBytes),
            systemSwapTotalBytes: numberOrZero(data.systemSwapTotalBytes),
            taskPoolAvailable: Boolean(data.taskPoolAvailable),
            taskPoolCurrentBytes: numberOrZero(data.taskPoolCurrentBytes),
            taskPoolAnonBytes: numberOrZero(data.taskPoolAnonBytes),
            taskPoolFileBytes: numberOrZero(data.taskPoolFileBytes),
            taskPoolSwapBytes: numberOrZero(data.taskPoolSwapBytes),
            taskPoolMemoryHighBytes: nullableLimit(data.taskPoolMemoryHighBytes),
            taskPoolMemoryMaxBytes: nullableLimit(data.taskPoolMemoryMaxBytes),
            taskPoolPidsCurrent: numberOrZero(data.taskPoolPidsCurrent),
            taskPoolPidsMax: nullableLimit(data.taskPoolPidsMax),
            memoryPressureSomeAvg10: numberOrZero(data.memoryPressureSomeAvg10),
            memoryPressureFullAvg10: numberOrZero(data.memoryPressureFullAvg10)
        };
        status.available = status.processRSSBytes > 0 && status.systemUsedBytes > 0 && status.totalMemoryBytes > 0;
        return status;
    }

    function classify(status) {
        status = status || {};
        if (!status.available) {
            return 'unavailable';
        }

        var swapPercent = 0;
        if (status.systemSwapTotalBytes > 0) {
            swapPercent = status.systemSwapUsedBytes * 100 / status.systemSwapTotalBytes;
        }
        if (status.memoryPressureFullAvg10 >= 10 || swapPercent >= 85) {
            return 'critical';
        }
        if (status.taskPoolAvailable && status.taskPoolMemoryMaxBytes > 0 && status.taskPoolCurrentBytes >= status.taskPoolMemoryMaxBytes * 0.95) {
            return 'critical';
        }
        if (status.memoryPressureFullAvg10 >= 1 || swapPercent >= 70) {
            return 'warning';
        }
        if (status.taskPoolAvailable && status.taskPoolMemoryHighBytes > 0 && status.taskPoolCurrentBytes >= status.taskPoolMemoryHighBytes) {
            return 'warning';
        }
        return 'ready';
    }

    return {
        normalize: normalize,
        classify: classify
    };
});
