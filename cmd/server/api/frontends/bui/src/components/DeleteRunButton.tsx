import { useEffect, useId, useRef, useState } from 'react';
import { api, ApiError } from '../services/api';
import { requestMessage, useFineTuningRunner } from '../contexts/FineTuningRunnerContext';
import type { RunDeletionPreview, TrainingRun } from '../types';

// One confirmation flow for preparation rows and their version's training rows.
export default function DeleteRunButton({ run, onDeleted }: { run: TrainingRun; onDeleted: () => void }) {
  const { refreshRuns } = useFineTuningRunner();
  const [open, setOpen] = useState(false);
  const [preview, setPreview] = useState<RunDeletionPreview | null>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [deleting, setDeleting] = useState(false);
  const deletingRef = useRef(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const titleID = useId();
  const descriptionID = useId();
  const training = !run.stages.some((stage) => stage.name === 'preparation');
  const name = training ? run.training_dataset_name ?? 'Training run' : run.target_dataset_name ?? 'Preparation run';
  const close = () => { if (!deletingRef.current) setOpen(false); };

  useEffect(() => {
    if (!open) { dialog.current?.close(); return; }
    dialog.current?.showModal();
    cancel.current?.focus();
    const controller = new AbortController();
    setPreview(null);
    setError('');
    api.previewRunDeletion(run.workflow_id, run.run_id, controller.signal).then((next) => {
      if (!controller.signal.aborted) setPreview(next);
    }).catch((err) => {
      if (!controller.signal.aborted) setError(requestMessage(err, 'Could not load the deletion details.'));
    });
    return () => controller.abort();
  }, [open, attempt, run.workflow_id, run.run_id]);

  const remove = async () => {
    if (!preview || preview.blockers.length || deletingRef.current) return;
    deletingRef.current = true;
    setDeleting(true);
    setError('');
    setNotice('');
    try {
      await api.deleteTrainingRun(run.workflow_id, run.run_id, preview.revision);
      dialog.current?.close();
      setOpen(false);
      refreshRuns();
      onDeleted();
    } catch (err) {
      setError(requestMessage(err, 'Could not finish deleting this run.'));
      if (err instanceof ApiError && err.status === 409) {
        setNotice('Deletion details changed. Review the updated details before confirming again.');
        setPreview(null);
        setAttempt((value) => value + 1);
      } else {
        setNotice('Deletion could not be confirmed and may be incomplete. Retry to finish the same deletion.');
      }
    } finally {
      deletingRef.current = false;
      setDeleting(false);
    }
  };
  const dependents = Math.max(0, (preview?.executions.length ?? 1) - 1);

  return <>
    <button className="btn btn-danger run-delete-trigger" type="button" aria-label={`Delete ${training ? 'training' : 'preparation'} run for ${name}, created ${new Date(run.started_at).toLocaleString()}`} onClick={() => { setPreview(null); setError(''); setNotice(''); setOpen(true); }}>Delete</button>
    <dialog ref={dialog} className="run-delete-dialog" aria-labelledby={titleID} aria-describedby={descriptionID} onCancel={(event) => { event.preventDefault(); close(); }}>
      <div className="run-delete-body">
        <h2 id={titleID}>Delete this {training ? 'training' : 'preparation'} run?</h2>
        <p className="run-delete-identity"><strong>{name}</strong><span>Created {new Date(run.started_at).toLocaleString()}</span></p>
        <div id={descriptionID}>
          {!preview && !error && <p role="status">Checking what will be deleted…</p>}
          {preview && <>
            <p>This permanently deletes this run and all its artifacts{preview.dataset_version ? `, including dataset version ${preview.dataset_version}` : ''}.</p>
            {dependents > 0 && <p>Also deletes {dependents} training {dependents === 1 ? 'run' : 'runs'} attached to this version, including their adapters and artifacts.</p>}
            {preview.deletes_dataset && <p className="prep-warning"><strong>This will also delete the dataset.</strong> This is its last saved version.</p>}
            {!preview.deletes_dataset && <p>{training ? 'The dataset and other training runs will not be changed.' : 'Other dataset versions and their training artifacts will not be changed.'}</p>}
            {preview.continuing_preparation.length > 0 && <p>Other preparation runs will continue. Their saved versions keep their own records.</p>}
          </>}
        </div>
        {notice && <p className="prep-warning" role="status">{notice}</p>}
        {error && <p className="fine-error" role="alert">{error}</p>}
        {!!preview?.blockers.length && <div className="prep-warning" role="alert"><strong>Cannot delete this run yet</strong><ul>{preview.blockers.map((blocker, index) => <li key={index}>{blocker}</li>)}</ul></div>}
        <div className="run-delete-actions">
          <button ref={cancel} className="btn btn-secondary" type="button" disabled={deleting} onClick={close}>Cancel</button>
          {!preview && error && <button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Retry details</button>}
          {!!preview?.blockers.length && <button className="btn btn-secondary" type="button" onClick={() => setAttempt((value) => value + 1)}>Refresh details</button>}
          <button className="btn btn-danger" type="button" disabled={!preview || !!preview.blockers.length || deleting} onClick={() => void remove()}>{deleting ? 'Deleting…' : error && preview ? 'Retry deletion' : 'Delete run'}</button>
        </div>
      </div>
    </dialog>
  </>;
}
