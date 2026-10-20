import React, { useEffect, useState } from 'react';
import { X, RotateCcw, AlertTriangle, FileCode, Sliders, CheckCircle, Play, ShieldAlert } from 'lucide-react';
import { api } from '../api/client';
import { Stack, StackRevision } from '../types';

interface RestoreStackModalProps {
  hostId: string;
  hostName: string;
  stack: Stack;
  onClose: () => void;
  onRestored: (restoredStack: Stack, startNow: boolean) => void;
}

export const RestoreStackModal: React.FC<RestoreStackModalProps> = ({
  hostId,
  hostName,
  stack,
  onClose,
  onRestored,
}) => {
  const [targetPath, setTargetPath] = useState(stack.path);
  const [startAfterRestore, setStartAfterRestore] = useState(true);
  const [revisions, setRevisions] = useState<StackRevision[]>([]);
  const [selectedRevisionId, setSelectedRevisionId] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'compose' | 'env'>('compose');
  const [loading, setLoading] = useState(true);
  const [restoring, setRestoring] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadRevisions();
  }, [hostId, stack.id]);

  const loadRevisions = async () => {
    try {
      setLoading(true);
      setError(null);
      const revs = await api.listStackRevisions(hostId, stack.id);
      setRevisions(revs);
      if (revs.length > 0) {
        setSelectedRevisionId(revs[0].id);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load stack revision history');
    } finally {
      setLoading(false);
    }
  };

  const selectedRevision = revisions.find((r) => r.id === selectedRevisionId) || revisions[0];

  const handleRestore = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!targetPath.trim()) {
      setError('Please provide a target directory path.');
      return;
    }

    try {
      setRestoring(true);
      setError(null);
      const res = await api.restoreStack(hostId, stack.id, {
        path: targetPath.trim(),
        revision_id: selectedRevision?.id,
        start: startAfterRestore,
      });

      if (res.success && res.stack) {
        onRestored(res.stack, startAfterRestore);
      } else {
        throw new Error('Restore command did not return updated stack');
      }
    } catch (err: any) {
      setError(err.message || 'Failed to restore stack files to host');
      setRestoring(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4 overflow-y-auto">
      <div className="w-full max-w-2xl rounded-2xl bg-slate-900 border border-slate-800 shadow-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-6 py-4 bg-slate-900/50">
          <div className="flex items-center gap-3">
            <div className="p-2 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
              <RotateCcw className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-100 flex items-center gap-2">
                Restore Stack: <span className="text-amber-400 font-mono">{stack.name}</span>
              </h2>
              <p className="text-xs text-slate-400">
                Recover compose configuration and recreate directory on <span className="font-semibold text-slate-300">{hostName}</span>
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleRestore} className="flex-1 overflow-y-auto p-6 space-y-5">
          {error && (
            <div className="rounded-xl bg-rose-500/10 border border-rose-500/20 p-3.5 flex items-start gap-3 text-rose-400 text-xs font-mono">
              <ShieldAlert className="w-4 h-4 shrink-0 mt-0.5" />
              <div className="flex-1">{error}</div>
            </div>
          )}

          {/* Info Banner */}
          <div className="rounded-xl bg-amber-500/10 border border-amber-500/20 p-3.5 flex items-start gap-3">
            <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
            <div className="text-xs text-slate-300 leading-relaxed">
              This stack's directory was deleted on the host filesystem. DockerPulse has safely preserved its compose files and revision history. Restoring will recreate the directory on the host and write back the files.
            </div>
          </div>

          {/* Target Host Directory Input */}
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-slate-300 flex items-center justify-between">
              <span>Target Directory on Host</span>
              <span className="text-[10px] text-slate-500 font-mono">Editable</span>
            </label>
            <input
              type="text"
              value={targetPath}
              onChange={(e) => setTargetPath(e.target.value)}
              placeholder="/home/user/docker/my-stack"
              className="w-full rounded-xl bg-slate-950 border border-slate-800 px-3.5 py-2.5 text-xs font-mono text-slate-200 placeholder-slate-600 focus:border-amber-500 focus:outline-none focus:ring-1 focus:ring-amber-500"
              required
            />
            <p className="text-[11px] text-slate-500">
              The directory on the host where the folder and files will be reconstructed.
            </p>
          </div>

          {/* Revision Selector */}
          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-slate-300">
              Backup Revision to Restore
            </label>
            {loading ? (
              <div className="text-xs text-slate-400 font-mono py-2">Loading stored revisions...</div>
            ) : revisions.length === 0 ? (
              <div className="rounded-xl border border-rose-800/40 bg-rose-950/20 p-3 text-xs text-rose-400">
                No backup revisions were found in DockerPulse for this stack.
              </div>
            ) : (
              <select
                value={selectedRevisionId}
                onChange={(e) => setSelectedRevisionId(e.target.value)}
                className="w-full rounded-xl bg-slate-950 border border-slate-800 px-3.5 py-2.5 text-xs font-mono text-slate-200 focus:border-amber-500 focus:outline-none focus:ring-1 focus:ring-amber-500"
              >
                {revisions.map((rev) => (
                  <option key={rev.id} value={rev.id}>
                    Revision #{rev.revision_num} • {new Date(rev.created_at).toLocaleString()} {rev.note ? `(${rev.note})` : ''}
                  </option>
                ))}
              </select>
            )}
          </div>

          {/* File Preview */}
          {selectedRevision && (
            <div className="space-y-2 border border-slate-800 rounded-xl overflow-hidden bg-slate-950/50">
              <div className="flex items-center justify-between border-b border-slate-800 bg-slate-900/40 px-3 py-1.5">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setActiveTab('compose')}
                    className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-mono rounded-lg transition-colors ${
                      activeTab === 'compose'
                        ? 'bg-amber-500/10 text-amber-300 font-semibold border border-amber-500/20'
                        : 'text-slate-400 hover:text-slate-200'
                    }`}
                  >
                    <FileCode className="w-3.5 h-3.5" />
                    docker-compose.yml
                  </button>
                  {selectedRevision.env_content && (
                    <button
                      type="button"
                      onClick={() => setActiveTab('env')}
                      className={`flex items-center gap-1.5 px-2.5 py-1 text-xs font-mono rounded-lg transition-colors ${
                        activeTab === 'env'
                          ? 'bg-amber-500/10 text-amber-300 font-semibold border border-amber-500/20'
                          : 'text-slate-400 hover:text-slate-200'
                      }`}
                    >
                      <Sliders className="w-3.5 h-3.5" />
                      .env
                    </button>
                  )}
                </div>
                <span className="text-[10px] text-slate-500 font-mono">
                  {selectedRevision.note || 'Snapshot'}
                </span>
              </div>
              <div className="p-3">
                <textarea
                  readOnly
                  value={activeTab === 'compose' ? selectedRevision.compose_content : selectedRevision.env_content}
                  rows={8}
                  className="w-full bg-transparent font-mono text-[11px] text-slate-300 leading-relaxed resize-none focus:outline-none selection:bg-amber-500/30"
                />
              </div>
            </div>
          )}

          {/* Start Stack Option */}
          <div className="rounded-xl border border-slate-800/80 bg-slate-900/30 p-3.5 flex items-center justify-between cursor-pointer hover:border-slate-700 transition-colors"
               onClick={() => setStartAfterRestore(!startAfterRestore)}>
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-sky-500/10 text-sky-400 border border-sky-500/20">
                <Play className="w-4 h-4 fill-sky-400" />
              </div>
              <div>
                <div className="text-xs font-semibold text-slate-200">
                  Launch Stack After Restore
                </div>
                <div className="text-[11px] text-slate-400">
                  Automatically run <code className="text-sky-300">docker compose up -d</code> once files are written.
                </div>
              </div>
            </div>
            <input
              type="checkbox"
              checked={startAfterRestore}
              onChange={(e) => setStartAfterRestore(e.target.checked)}
              className="w-4 h-4 rounded border-slate-700 bg-slate-800 text-amber-500 focus:ring-amber-500 focus:ring-offset-slate-900 cursor-pointer"
            />
          </div>

          {/* Action Buttons */}
          <div className="flex items-center justify-end gap-3 pt-4 border-t border-slate-800">
            <button
              type="button"
              onClick={onClose}
              disabled={restoring}
              className="rounded-xl px-4 py-2 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={restoring || !selectedRevision || !targetPath.trim()}
              className="flex items-center gap-2 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-50 px-5 py-2 text-xs font-semibold text-white shadow-lg shadow-amber-600/20 transition-all cursor-pointer"
            >
              <RotateCcw className={`w-3.5 h-3.5 ${restoring ? 'animate-spin' : ''}`} />
              {restoring
                ? 'Restoring Files...'
                : startAfterRestore
                ? 'Restore & Start Stack'
                : 'Restore Files to Host'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
