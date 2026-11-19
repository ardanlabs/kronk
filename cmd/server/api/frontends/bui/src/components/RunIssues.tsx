import { useEffect, useState } from 'react';
import { api } from '../services/api';
import { requestMessage, stepLabel, trainingStepLabels } from '../contexts/FineTuningRunnerContext';
import type { TrainingEvaluation, TrainingRun } from '../types';

const failures: Record<NonNullable<TrainingRun['failure_reason']>, [string, string]> = {
  'dataset-document-exists': ['This document and page range are already in the dataset.', 'Use different source material or a page range that has not already been prepared.'],
  'dataset-questions-exist': ['Every record repeats a question already in the dataset.', 'Use source material with new facts or questions. Repeating the same preparation will not add these records.'],
  'dataset-changed': ['The dataset changed before this run could save its records.', 'Open the current dataset from the dashboard and start a new preparation.'],
  'preparation-no-examples': ['The model did not produce usable examples from the selected content.', 'Review the selected content, preparation goal, and instructions. The result does not prove the document contains no useful facts.'],
  'preparation-no-source-units': ['No supported source passages were available for this preparation goal.', 'Check the selected pages and goal. Try source content with readable text and facts relevant to the task.'],
  'preparation-request-budget-exhausted': ['Preparation exhausted its generation request budget before resolving all source passages.', 'Check the preparation model and instructions before starting a new run. This alone does not establish a problem with the source content.'],
  'preparation-no-verified-examples': ['None of the generated training records passed verification.', 'Compare the preparation goal and instructions with the selected source content. Verification checks source support, question quality, and instruction compliance.'],
  'preparation-checkpoint-version': ['Saved progress belongs to an older checkpoint format and cannot be resumed.', 'Start a new preparation with a new preview. Existing dataset versions are unchanged.'],
  'invalid-training': ['The server rejected the run’s input.', 'Check that the selected document or dataset version is still available and that the run settings are valid. Detailed validation evidence was not recorded for this run; do not assume that more records will fix it.'],
  'preparation-extractor-unavailable': ['The document conversion service (Docling) was unavailable.', 'Check that the conversion service is running and reachable, or ask the person managing the server to restore it. Start a new preparation once service is restored.'],
  'preparation-page-range': ['The selected page range starts after the document’s last page.', 'Select a valid page range and start a new preparation.'],
  'preparation-extraction-failed': ['The document conversion service could not read the document.', 'Check that the file opens correctly and that its selected pages contain readable content. Try another supported document if needed.'],
  'training-records-insufficient': ['The dataset has too few training records.', 'Add source material and prepare more verified training records. Evaluation questions do not count toward the training-record minimum.'],
};

export const failureText = (run: TrainingRun) => run.failure_reason ? failures[run.failure_reason]?.[0] ?? '' : '';
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
  guidance: string;
}

// Completed runs never show Issues, even when they recorded warnings.
// Stopped-by-user is not an execution failure.
export function runIssues(run: TrainingRun): RunIssue[] {
  if (run.status === 'completed') return [];
  const issues: RunIssue[] = [];
  if (run.status === 'failed') {
    const minimum = run.issues?.find((issue) => issue.code === 'training-records-insufficient' && issue.required && issue.required > issue.count);
    issues.push({
      code: run.failure_reason ?? 'unclassified-failure',
      title: minimum ? `${minimum.required! - minimum.count} more training records are needed` : failureText(run) || 'An unexpected error stopped this run.',
      detail: minimum ? `This run had ${minimum.count} training records; it required at least ${minimum.required}. Evaluation questions do not count toward this minimum.` : 'Earlier dataset versions are unchanged.',
      guidance: run.failure_reason ? failures[run.failure_reason]?.[1] ?? 'Ask the person managing the server to check the failed step.' : 'The recorded error does not identify the cause. Ask the person managing the server to check the failed step before changing the dataset.',
    });
  } else if (notAccepted(run)) {
    issues.push({ code: 'evaluation-not-accepted', title: 'The adapter did not meet the acceptance requirements.', detail: 'Training finished, but the adapter was not accepted.', guidance: 'Review the missed questions and expected answers in Evaluation. Correct or add training examples for the gaps you find before training again.' });
  }
  if (!['failed', 'needs_improvement', 'stopped'].includes(run.status)) return issues;
  for (const issue of run.issues ?? []) {
    const text = preparationWarnings[issue.code];
    const noun = issue.code === 'preparation-omissions' ? 'omission' : issue.code === 'preparation-generation-failed' ? 'passage' : 'rejected example';
    if (text && issue.count > 0) issues.push({ code: issue.code, title: `${issue.count} ${noun}${issue.count === 1 ? '' : 's'} · ${text[0]}`, detail: text[1], guidance: issue.code === 'preparation-omissions' ? 'Check whether the omitted content matters to your task. If it does, use a supported text source that includes it.' : 'Review the source content and preparation instructions. These are model-based assessments, not proof that the source document is wrong.' });
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
  const guidance = [...new Set(issues.map((issue) => issue.guidance))];
  return <div className="run-issues">
    <div className="run-issues-heading"><h3>{run.status === 'failed' ? 'Why the run stopped' : notAccepted(run) ? 'Why it wasn’t accepted' : 'Warnings from this run'}</h3></div>
    {issues.map((issue, at) => <section className="run-issue" key={issue.code}>
      {issue.code !== 'evaluation-not-accepted' && <><h4>{issue.title}</h4><p>{issue.detail}</p></>}
      {at === 0 && notAccepted(run) && evaluation && <>
        {evaluation.error ? <div role="alert"><p className="prep-field-error">Could not load evaluation evidence: {evaluation.error}</p><button className="btn btn-secondary" onClick={evaluation.retry}>Try again</button></div> : !evaluation.report && run.evaluation_available ? <p className="prep-copy" role="status">Loading evaluation evidence…</p> : evaluation.report ? <><p className="prep-copy">Adapter: {evaluation.report.adapter_correct} / {evaluation.report.total} passed · Base model: {evaluation.report.base_correct} / {evaluation.report.total} passed</p><EvaluationChecks report={evaluation.report} /></> : <p className="prep-copy">Evaluation evidence is not available for this run.</p>}
      </>}
    </section>)}
    <div className="run-issue-guidance"><h4>What to check next</h4>{guidance.length === 1 ? <p>{guidance[0]}</p> : <ul>{guidance.map((text) => <li key={text}>{text}</li>)}</ul>}</div>
    <details className="run-issue-technical"><summary>Technical details</summary><dl className="prep-facts">
      <div><dt>Step</dt><dd>{run.preparation_only ? stepLabel(step) : trainingStepLabels[step ?? ''] ?? step ?? 'Not recorded'}</dd></div>
      <div><dt>Diagnostic codes</dt><dd>{issues.map((issue) => <code key={issue.code}>{issue.code}<br /></code>)}</dd></div>
      <div><dt>Run ID</dt><dd><code>{run.run_id}</code></dd></div><div><dt>Workflow ID</dt><dd><code>{run.workflow_id}</code></dd></div>
      <div><dt>Stopped / evaluated</dt><dd>{date(failed?.closed_at ?? run.closed_at)}</dd></div><div><dt>Attempts at failed step</dt><dd>{failed?.attempt ?? 'Not recorded'}</dd></div>
    </dl></details>
  </div>;
}
