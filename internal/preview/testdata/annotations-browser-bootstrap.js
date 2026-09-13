// ABOUTME: Reports test-module loading failures to the bounded native runner.
// ABOUTME: Keeps a visible diagnostic when the browser cannot return its results.
import('./tests.js').catch(async error => {
  const message = `Browser test module could not run: ${error.message}`;
  document.getElementById('test-results').textContent = message;
  try {
    const response = await fetch('./results', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ browser: navigator.userAgent, passed: 0, failures: [message] }) });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
  } catch (deliveryError) {
    document.getElementById('test-results').textContent += ` Result delivery failed: ${deliveryError.message}`;
  }
});
