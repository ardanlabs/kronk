import { useEffect, useMemo, useState } from 'react';
import { api } from '../services/api';
import type { CatalogModelResponse, DecisionAnswer, DecisionRequest, DecisionResponse } from '../types';
import { FieldLabel } from './ParamTooltips';

const STORAGE_KEY = 'kronk_decision_model';

const questions: DecisionRequest['questions'] = {
  route: {
    type: 'choice',
    instructions: 'Which team should handle this request?',
    criteria: {
      billing: 'Payments, invoices, refunds, and duplicate charges',
      technical_support: 'Product bugs and technical problems',
      sales: 'Plans, pricing, and new purchases',
    },
  },
  urgency: {
    type: 'score',
    instructions: 'How urgent is this request?',
    criteria: ['not urgent', 'normal', 'urgent', 'critical'],
  },
  requires_human: {
    type: 'noul',
    instructions: 'Should a human review this request?',
  },
};

const questionTitles: Record<string, string> = {
  route: 'Route',
  urgency: 'Urgency',
  requires_human: 'Human review',
};

function formatPercent(value = 0): string {
  return `${(value * 100).toFixed(1)}%`;
}

function ProbabilityList({ probabilities }: { probabilities?: Record<string, number> }) {
  if (!probabilities) return null;

  const entries = Object.entries(probabilities).sort((a, b) => b[1] - a[1]);
  return (
    <div className="decision-probabilities">
      {entries.map(([name, probability]) => (
        <div className="decision-probability" key={name}>
          <div className="decision-probability-label">
            <span>{name.replaceAll('_', ' ')}</span>
            <strong>{formatPercent(probability)}</strong>
          </div>
          <div className="decision-probability-track">
            <span style={{ width: `${Math.max(0, Math.min(100, probability * 100))}%` }} />
          </div>
        </div>
      ))}
    </div>
  );
}

function AnswerCard({ id, answer }: { id: string; answer: DecisionAnswer }) {
  let primary = '';
  let detail = '';

  switch (answer.type) {
    case 'choice':
      primary = answer.choice?.replaceAll('_', ' ') ?? 'No choice';
      detail = `${formatPercent(answer.confidence)} confidence`;
      break;
    case 'score': {
      primary = answer.score?.toFixed(2) ?? 'No score';
      const level = answer.legend?.[String(Math.round(answer.score ?? 0))];
      detail = typeof level === 'string' ? level : `${formatPercent(answer.confidence)} confidence`;
      break;
    }
    case 'noul':
      primary = formatPercent(answer.noul);
      detail = (answer.noul ?? 0) >= 0.5 ? 'Human review recommended' : 'Automation recommended';
      break;
  }

  return (
    <article className="decision-answer-card">
      <div className="decision-answer-heading">
        <span className={`decision-answer-type decision-answer-type-${answer.type}`}>{answer.type}</span>
        <h4>{questionTitles[id] ?? id}</h4>
      </div>
      <div className="decision-answer-primary">{primary}</div>
      <p>{detail}</p>
      <ProbabilityList probabilities={answer.probabilities} />
    </article>
  );
}

export default function Decision() {
  const [model, setModel] = useState(() => localStorage.getItem(STORAGE_KEY) || '');
  const [decisionModels, setDecisionModels] = useState<CatalogModelResponse[]>([]);
  const [modelsLoading, setModelsLoading] = useState(true);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [message, setMessage] = useState('I was charged twice and need this fixed today.');
  const [accountTier, setAccountTier] = useState('business');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<DecisionResponse | null>(null);

  const questionSummary = useMemo(() => Object.entries(questions), []);
  const selectedModel = decisionModels.find((entry) => entry.id === model);
  const modelReady = selectedModel?.downloaded === true && selectedModel.validated === true;
  const canSubmit = modelReady && message.trim() !== '' && !submitting;

  useEffect(() => {
    let cancelled = false;

    api.listCatalog()
      .then((catalog) => {
        if (cancelled) return;

        const models = catalog.filter((entry) => (
          entry.capabilities?.decision === true || entry.capabilities?.endpoint === 'decision'
        ));
        setDecisionModels(models);
        setModel((current) => models.some((entry) => entry.id === current) ? current : (models[0]?.id ?? ''));
      })
      .catch((err) => {
        if (!cancelled) setModelsError((err as Error).message);
      })
      .finally(() => {
        if (!cancelled) setModelsLoading(false);
      });

    return () => { cancelled = true; };
  }, []);

  const handleModelChange = (modelID: string) => {
    setModel(modelID);
    localStorage.setItem(STORAGE_KEY, modelID);
    setError(null);
    setResult(null);
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!canSubmit) return;

    const modelID = model.trim();
    localStorage.setItem(STORAGE_KEY, modelID);
    setSubmitting(true);
    setError(null);
    setResult(null);

    try {
      setResult(await api.decide({
        model: modelID,
        state: {
          customer_message: message.trim(),
          account_tier: accountTier,
        },
        questions,
      }));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="decision-page">
      <div className="page-header">
        <h2>Decision</h2>
        <p>Evaluate structured questions against shared state with an installed decision model.</p>
      </div>

      <div className="decision-grid">
        <form className="decision-card decision-form" onSubmit={handleSubmit}>
          <div>
            <span className="decision-eyebrow">Customer support example</span>
            <h3>Request state</h3>
          </div>

          <FieldLabel htmlFor="decision-model" tooltipKey="decisionModel">Model</FieldLabel>
          <select
            id="decision-model"
            className="form-select"
            value={model}
            onChange={(event) => handleModelChange(event.target.value)}
            disabled={modelsLoading || submitting || decisionModels.length === 0}
          >
            {modelsLoading && <option value="">Loading decision models…</option>}
            {!modelsLoading && decisionModels.length === 0 && <option value="">No decision models in catalog</option>}
            {decisionModels.map((entry) => (
              <option key={entry.id} value={entry.id}>
                {entry.id}{entry.downloaded ? (entry.validated ? ' — ready' : ' — validation failed') : ' — download required'}
              </option>
            ))}
          </select>

          <div className="decision-model-status" aria-live="polite">
            {modelsLoading ? (
              <span>Checking model availability…</span>
            ) : modelsError ? (
              <span className="decision-model-status-error">Unable to verify model availability: {modelsError}</span>
            ) : !selectedModel ? (
              <span className="decision-model-status-required">No decision models are available in the catalog.</span>
            ) : modelReady ? (
              <span className="decision-model-status-ready">● Downloaded and ready</span>
            ) : selectedModel.downloaded ? (
              <span className="decision-model-status-required">
                This model did not pass integrity validation. Re-download it from <strong>Kronk → Catalog</strong> before evaluating a decision.
              </span>
            ) : (
              <span className="decision-model-status-required">
                Download this model from <strong>Kronk → Catalog</strong> before evaluating a decision.
              </span>
            )}
          </div>

          <FieldLabel htmlFor="decision-message" tooltipKey="decisionState">Customer message</FieldLabel>
          <textarea
            id="decision-message"
            className="decision-message"
            rows={5}
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            disabled={submitting}
          />

          <FieldLabel htmlFor="decision-account-tier" tooltipKey="decisionAccountTier">Account tier</FieldLabel>
          <select
            id="decision-account-tier"
            className="form-select"
            value={accountTier}
            onChange={(event) => setAccountTier(event.target.value)}
            disabled={submitting}
          >
            <option value="free">Free</option>
            <option value="pro">Pro</option>
            <option value="business">Business</option>
            <option value="enterprise">Enterprise</option>
          </select>

          <button className="btn btn-primary decision-submit" type="submit" disabled={!canSubmit}>
            {submitting ? 'Evaluating…' : 'Evaluate decision'}
          </button>
        </form>

        <section className="decision-card decision-questions" aria-labelledby="decision-questions-title">
          <div>
            <span className="decision-eyebrow">Fixed for this example</span>
            <h3 id="decision-questions-title">Questions</h3>
          </div>
          {questionSummary.map(([id, question]) => (
            <div className="decision-question" key={id}>
              <div className="decision-question-heading">
                <strong>{questionTitles[id]}</strong>
                <span>{question.type}</span>
              </div>
              <p>{question.instructions}</p>
              {question.type === 'choice' && !Array.isArray(question.criteria) && (
                <div className="decision-option-list">
                  {Object.keys(question.criteria ?? {}).map((option) => (
                    <span key={option}>{option.replaceAll('_', ' ')}</span>
                  ))}
                </div>
              )}
              {question.type === 'score' && Array.isArray(question.criteria) && (
                <div className="decision-scale">
                  {question.criteria.map((level, index) => (
                    <span key={String(level)}><b>{index}</b>{String(level)}</span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </section>
      </div>

      {error && <div className="alert alert-error decision-alert">Decision failed: {error}</div>}

      <section className="decision-results" aria-live="polite">
        <div className="decision-results-header">
          <div>
            <span className="decision-eyebrow">Model output</span>
            <h3>Answers</h3>
          </div>
          {result && <span>{result.usage.input_tokens} input tokens</span>}
        </div>

        {!result ? (
          <div className="decision-empty">
            <div className="decision-empty-icon">◇</div>
            <strong>No decision yet</strong>
            <span>Run the example to see typed answers and calibrated probabilities.</span>
          </div>
        ) : (
          <>
            <div className="decision-answer-grid">
              {questionSummary.map(([id]) => {
                const answer = result.answers[id];
                return answer ? <AnswerCard id={id} answer={answer} key={id} /> : null;
              })}
            </div>
            <details className="decision-raw">
              <summary>Raw response</summary>
              <pre>{JSON.stringify(result, null, 2)}</pre>
            </details>
          </>
        )}
      </section>
    </div>
  );
}
