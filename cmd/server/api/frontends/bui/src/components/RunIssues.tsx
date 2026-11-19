import { useEffect, useState } from 'react';
import { api } from '../services/api';
import { requestMessage, stepLabel, trainingStepLabels } from '../contexts/FineTuningRunnerContext';
import type { TrainingEvaluation, TrainingRun } from '../types';

const failures: Record<NonNullable<TrainingRun['failure_reason']>, string> = {
  'dataset-document-exists': 'This document and page range are already in the dataset.',
  'dataset-questions-exist': 'Every record repeats a question already in the dataset.',
  'dataset-changed': 'The dataset changed before this run could save its records.',
  'preparation-no-examples': 'The model did not produce usable examples from the selected content.',
  'preparation-no-source-units': 'No supported source passages were available for this preparation goal.',
  'preparation-request-budget-exhausted': 'Preparation reached its generation request limit before resolving all source passages.',
  'preparation-no-verified-examples': 'None of the generated training records passed verification.',
  'preparation-checkpoint-version': 'Saved progress belongs to an older checkpoint format and cannot be resumed.',
  'invalid-training': 'The server rejected the run’s input.',
  'preparation-extractor-unavailable': 'The document conversion service (Docling) was unavailable.',
  'preparation-page-range': 'The selected page range starts after the document’s last page.',
  'preparation-extraction-failed': 'The document conversion service could not read the document.',
  'training-records-insufficient': 'The dataset has too few training records.',
};

export const failureText = (run: TrainingRun) => {
  const step = run.stages.flatMap((stage) => stage.steps).find((step) => step.status === 'failed')?.name ?? run.step;
  const label = run.preparation_only ? stepLabel(step) : trainingStepLabels[step ?? ''] ?? 'Training';
  const failure = run.failure_log?.at(-1);
  return (failure && failure.code !== 'unclassified-failure' ? failure.message : '') || (run.failure_reason && failures[run.failure_reason]) || `${label} failed.`;
};
export const notAccepted = (run: TrainingRun) => run.status === 'needs_improvement' || (run.status === 'completed' && run.evaluation_result === 'needs_improvement');

const preparationWarnings: Record<string, [string, string]> = {
  grounding: ['Source support', 'The verifier flagged answers or question assumptions that were not supported by their source passages.'],
  task_quality: ['Question quality', 'The verifier flagged poor standalone questions or questions that disclosed the answer.'],
  user_requirements: ['Instructions', 'The verifier flagged conflicts with the preparation instructions.'],
  'preparation-omissions': ['Source content left out', 'Preparation omitted some source content, for example unsupported document structures or passages for which the model did not produce a usable example.'],
  'preparation-generation-failed': ['Generation attempts exhausted', 'The model exhausted the allowed attempts for these passages without producing a complete set of usable candidates. Invalid JSON, duplicate questions, or unsupported answers can cause this; it does not prove that the source lacks useful facts.'],
};

export interface RunIssue {
  code: string;
  title: string;
  detail: string;
}

// Accepted completed runs do not show Issues, even when they recorded warnings.
// Stopped-by-user is not an execution failure.
export function runIssues(run: TrainingRun): RunIssue[] {
  if (run.status === 'stopped') return [];
  if (run.status === 'completed' && !notAccepted(run)) return [];
  const issues: RunIssue[] = [];
  if (run.status === 'failed') {
    const minimum = run.issues?.find((issue) => issue.code === 'training-records-insufficient' && issue.required && issue.required > issue.count);
    issues.push({
      code: run.failure_reason ?? 'unclassified-failure',
      title: minimum ? `${minimum.required! - minimum.count} more training records are needed` : failureText(run),
      detail: minimum ? `This run had ${minimum.count} training records; it required at least ${minimum.required}. Evaluation questions do not count toward this minimum.` : run.preparation_only ? 'No records were saved; earlier versions are unchanged.' : 'Training did not finish; the dataset is unchanged.',
    });
  } else if (notAccepted(run)) {
    issues.push({ code: 'evaluation-not-accepted', title: 'The adapter did not meet the acceptance requirements.', detail: 'Training finished, but the adapter was not accepted.' });
  }
  if (run.status !== 'failed' && !notAccepted(run)) return issues;
  for (const issue of run.issues ?? []) {
    const text = preparationWarnings[issue.code];
    const noun = issue.code === 'preparation-omissions' ? 'omission' : issue.code === 'preparation-generation-failed' ? 'passage' : 'rejected example';
    if (text && issue.count > 0) issues.push({ code: issue.code, title: `${issue.count} ${noun}${issue.count === 1 ? '' : 's'} · ${text[0]}`, detail: text[1] });
  }
  return issues;
}

export function useEvaluation(run: TrainingRun) {
  const [report, setReport] = useState<TrainingEvaluation | null>(null);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setReport(null);
    setError('');
    if (run.evaluation_available) api.trainingEvaluation(run.workflow_id, run.run_id, controller.signal)
      .then((next) => { if (!controller.signal.aborted) setReport(next); })
      .catch((err) => { if (!controller.signal.aborted) setError(requestMessage(err, 'Could not load evaluation results.')); });
    return () => controller.abort();
  }, [run.workflow_id, run.run_id, run.evaluation_available, attempt]);
  return { report, error, retry: () => setAttempt((value) => value + 1) };
}

const checkLabels: Record<string, string> = { 'answer-score': 'Answer score', 'gain-over-base': 'Gain over base model', 'empty-answers': 'Empty answers', 'test-questions': 'Test questions' };

export function EvaluationChecks({ report }: { report: TrainingEvaluation }) {
  if (!report.checks?.length) return <p className="prep-copy">{report.accepted ? 'Individual acceptance checks weren’t saved for this run.' : 'This run was not accepted, but its report did not save which checks failed.'}</p>;
  return <div className="run-issues-table"><table><caption className="prep-sr-only">Recorded acceptance checks</caption><thead><tr><th>Acceptance check</th><th>This run / requirement</th><th>Outcome</th></tr></thead><tbody>{report.checks.map((check) => <tr key={check.code}>
    <th scope="row">{checkLabels[check.code] ?? 'Acceptance check'}</th><td>{check.code === 'answer-score' ? `${check.actual} of ${report.total} passed / at least ${check.required}` : check.code === 'gain-over-base' ? `${check.actual} more passed / at least ${check.required}` : check.code === 'empty-answers' ? `${check.actual} empty / ${check.required} allowed` : `${check.actual} questions / at least ${check.required}`}</td><td className={check.passed ? 'run-issue-met' : 'run-issue-not-met'}>{check.passed ? '✓ Met' : '× Not met'}</td>
  </tr>)}</tbody></table></div>;
}

export function RunIssues({ run, issues, evaluation }: { run: TrainingRun; issues: RunIssue[]; evaluation?: ReturnType<typeof useEvaluation> }) {
  const failed = run.stages.flatMap((stage) => stage.steps).find((step) => step.status === 'failed');
  const step = failed?.name ?? (notAccepted(run) ? 'evaluate' : run.step);
  const date = (value?: string) => value ? new Date(value).toLocaleString() : 'Not recorded';
  const label = (name?: string) => run.preparation_only ? stepLabel(name) : trainingStepLabels[name ?? ''] ?? 'Training';
  const log: NonNullable<TrainingRun['failure_log']> = run.failure_log?.length ? run.failure_log : run.status === 'failed' ? [
    ...run.stages.flatMap((stage) => stage.steps).flatMap<NonNullable<TrainingRun['failure_log']>[number]>((item) => item.status === 'completed'
      ? [{ step: item.name, code: 'step-completed', message: `${label(item.name)} completed.`, time: item.closed_at, attempt: item.attempt }]
      : item.status === 'failed' && item.started_at ? [{ step: item.name, code: 'step-started', message: `${label(item.name)} started.`, time: item.started_at }] : []),
    { step: step ?? '', code: run.failure_reason ?? 'unclassified-failure', message: run.failure_reason ? failureText(run) : 'The saved run does not contain a recoverable failure cause.', time: failed?.closed_at ?? run.closed_at, attempt: failed?.attempt },
  ] : notAccepted(run) ? [
    { step: 'train', code: 'step-completed', message: 'Adapter training completed.', time: run.stages.flatMap((stage) => stage.steps).find((item) => item.name === 'train' && item.status === 'completed')?.closed_at },
    ...(evaluation?.report?.checks?.filter((check) => !check.passed).map((check) => ({ step: 'evaluate', code: check.code, time: run.closed_at,
      message: check.code === 'answer-score' ? `Answer score: ${check.actual} of ${evaluation.report!.total} passed; at least ${check.required} required.`
        : check.code === 'gain-over-base' ? `Gain over base model: ${check.actual} more passed; at least ${check.required} required.`
        : check.code === 'empty-answers' ? `Empty answers: ${check.actual}; at most ${check.required} allowed.`
        : check.code === 'test-questions' ? `Test questions: ${check.actual}; at least ${check.required} required.` : `${checkLabels[check.code] ?? 'Acceptance check'} was not met.`,
    })) ?? []),
    { step: 'evaluate', code: 'evaluation-not-accepted', message: evaluation?.report?.checks?.length || (!evaluation?.report && run.evaluation_available) ? 'The adapter did not meet the acceptance requirements.' : 'The adapter was not accepted. Individual acceptance checks are unavailable.', time: run.closed_at },
  ] : [];
  return <div className="run-issues">
    <div className="run-issues-heading"><h3>{run.status === 'failed' ? 'Why the run stopped' : notAccepted(run) ? 'Why it wasn’t accepted' : 'Warnings from this run'}</h3></div>
    {issues.map((issue) => <section className="run-issue" key={issue.code}>
      <h4>{issue.title}</h4><p>{issue.detail}</p>
    </section>)}
    {log.length > 0 && <section className="run-failure-log" aria-label="Failure log"><h4>Failure log</h4><ol>{log.map((entry, index) => <li key={`${entry.code}-${index}`}>
      <time dateTime={entry.time} title={date(entry.time)}>{entry.time ? new Date(entry.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : 'No time recorded'}</time>
      <div><p className={entry.code === 'step-completed' || entry.code === 'step-started' ? '' : 'run-log-failure'}>{entry.message}</p>
        {(entry.step || !!entry.attempt) && <small>{[entry.step ? label(entry.step) : '', entry.attempt ? `Attempt ${entry.attempt}` : ''].filter(Boolean).join(' · ')}</small>}
      </div>
    </li>)}</ol>
      {notAccepted(run) && evaluation && (evaluation.error ? <div className="run-log-status" role="alert"><p>Could not load acceptance checks: {evaluation.error}</p><button className="btn btn-secondary" onClick={evaluation.retry}>Try again</button></div> : !evaluation.report && run.evaluation_available ? <p className="run-log-status" role="status">Loading acceptance checks…</p> : null)}
    </section>}
    <details className="run-issue-technical"><summary>Technical details</summary><dl className="prep-facts">
      {run.source_document && <div><dt>Document</dt><dd>{run.source_document}</dd></div>}
      <div><dt>Step</dt><dd>{run.preparation_only ? stepLabel(step) : trainingStepLabels[step ?? ''] ?? step ?? 'Not recorded'}</dd></div>
      <div><dt>Diagnostic codes</dt><dd>{[...new Set([...issues.map((issue) => issue.code), ...log.filter((entry) => !['step-started', 'step-completed'].includes(entry.code)).map((entry) => entry.code)])].map((code) => <code key={code}>{code}<br /></code>)}</dd></div>
      <div><dt>Run ID</dt><dd><code>{run.run_id}</code></dd></div><div><dt>Workflow ID</dt><dd><code>{run.workflow_id}</code></dd></div>
      <div><dt>Stopped / evaluated</dt><dd>{date(failed?.closed_at ?? run.closed_at)}</dd></div><div><dt>Attempts at failed step</dt><dd>{failed?.attempt ?? 'Not recorded'}</dd></div>
    </dl></details>
  </div>;
}
