import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { createServer } from 'vite';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const server = await createServer({ server: { middlewareMode: true }, appType: 'custom' });
after(() => server.close());
const { runStopState } = await server.ssrLoadModule('/src/contexts/FineTuningRunnerContext.tsx');
const { Badge } = await server.ssrLoadModule('/src/components/PreparationRuns.tsx');
const { trainingLabel } = await server.ssrLoadModule('/src/components/TrainingRun.tsx');

const run = (changes = {}) => ({
  status: 'running', control_action: 'pause', control_revision: 3,
  control_acknowledged_revision: 2, stages: [], ...changes,
});

test('stop waits for the current control revision to be acknowledged', () => {
  for (const status of ['running', 'pause_requested', 'paused', 'waiting', 'recovering']) {
    const pending = run({ status });
    assert.equal(runStopState(pending), 'stopping');
    assert.match(renderToStaticMarkup(createElement(Badge, { run: pending })), /Stopping…/);
    assert.equal(trainingLabel(pending), 'Stopping training…');
    const acknowledged = run({ status, control_acknowledged_revision: 3 });
    assert.equal(runStopState(acknowledged), 'stopped');
    assert.match(renderToStaticMarkup(createElement(Badge, { run: acknowledged })), />Stopped</);
    assert.equal(trainingLabel(acknowledged), 'Training stopped · can resume');
  }
});

test('missing acknowledgements never claim a resumable stop is complete', () => {
  assert.equal(runStopState(run({ control_acknowledged_revision: undefined })), 'stopping');
  assert.equal(runStopState(run({ control_revision: 0, control_acknowledged_revision: 0 })), 'stopping');
});

test('terminal cancellation is permanent and distinct from a resumable stop', () => {
  const cancelled = run({ status: 'stopped', control_acknowledged_revision: 3 });
  assert.equal(runStopState(cancelled), null);
  assert.match(renderToStaticMarkup(createElement(Badge, { run: cancelled })), /Cancelled/);
  assert.equal(trainingLabel(cancelled), 'Training cancelled');
  for (const status of ['completed', 'failed', 'needs_improvement']) {
    assert.equal(runStopState(run({ status, control_acknowledged_revision: 3 })), null);
  }
});

test('resume intent clears the stopped presentation', () => {
  assert.equal(runStopState(run({ control_action: 'resume', control_revision: 4 })), null);
  assert.equal(runStopState(run({ control_action: undefined })), null);
});
