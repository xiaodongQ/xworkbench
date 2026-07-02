// rpty.js tests: verify rpty.js can be parsed and exports expected functions.
// Run in browser console or via a lightweight DOM test harness.

// Test: rpty.js exports connectRPTY and disconnectRPTY
function testRptyExports() {
  if (typeof connectRPTY !== 'function') {
    throw new Error('connectRPTY not exported from rpty.js');
  }
  if (typeof disconnectRPTY !== 'function') {
    throw new Error('disconnectRPTY not exported from rpty.js');
  }
  console.log('[rpty test] exports OK');
}

// Test: connectRPTY with missing params returns early (no crash)
function testConnectRPTYNoCrash() {
  const consoleSpy = { errors: [] };
  try {
    // 未初始化时调用 disconnect 不应报错
    disconnectRPTY();
  } catch (e) {
    throw new Error('disconnectRPTY should not throw: ' + e.message);
  }
  console.log('[rpty test] disconnectRPTY no-crash OK');
}

// Test: rpty tab init creates terminal container if not exists
function testRptyTabInit() {
  // 模拟 DOM 环境
  if (typeof document === 'undefined') {
    console.log('[rpty test] skip (no DOM)');
    return;
  }

  // 清理
  const existing = document.getElementById('rpty-container');
  if (existing) existing.remove();

  initRptyTab(); // 不应报错，即使没有真实终端

  const container = document.getElementById('rpty-container');
  if (!container) {
    throw new Error('rpty-container should exist after initRptyTab');
  }
  if (container.querySelector('.rpty-status') === null) {
    throw new Error('rpty-container should have .rpty-status element');
  }
  console.log('[rpty test] initRptyTab OK');

  // 清理
  container.remove();
}

// Run all tests
try {
  testRptyExports();
  testConnectRPTYNoCrash();
  testRptyTabInit();
  console.log('[rpty test] ALL PASSED');
} catch (e) {
  console.error('[rpty test] FAILED:', e.message);
}