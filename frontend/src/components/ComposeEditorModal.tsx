import React, { useEffect, useState } from 'react';
import { X, Save, Rocket, History, FileCode, Sliders, CheckCircle, AlertCircle } from 'lucide-react';
import { api } from '../api/client';
import { StackRevision } from '../types';

interface ComposeEditorModalProps {
  hostId: string;
  stackId: string;
  stackName: string;
  onClose: () => void;
  onDeploy: (action: string) => void;
}

export const ComposeEditorModal: React.FC<ComposeEditorModalProps> = ({
  hostId,
  stackId,
  stackName,
  onClose,
  onDeploy,
}) => {
  const [activeTab, setActiveTab] = useState<'compose' | 'env' | 'revisions'>('compose');
  const [composeContent, setComposeContent] = useState('');
  const [envContent, setEnvContent] = useState('');
  const [stackPath, setStackPath] = useState('');
  const [revisions, setRevisions] = useState<StackRevision[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saveNote, setSaveNote] = useState('');
  const [notification, setNotification] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  useEffect(() => {
    loadStackFiles();
    loadRevisions();
  }, [hostId, stackId]);

  const loadStackFiles = async () => {
    try {
      setLoading(true);
      const data = await api.getStackFiles(hostId, stackId);
      setComposeContent(data.compose);
      setEnvContent(data.env);
      setStackPath(data.path);
    } catch (err: any) {
      setNotification({ message: err.message || 'Failed to load files', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const loadRevisions = async () => {
    try {
      const data = await api.listStackRevisions(hostId, stackId);
      setRevisions(data);
    } catch (err) {
      console.error(err);
    }
  };

  const handleSave = async (deployAfter = false) => {
    try {
      setSaving(true);
      await api.saveStackFiles(hostId, stackId, {
        compose: composeContent,
        env: envContent,
        note: saveNote || (deployAfter ? 'Saved & Deployed' : 'Manual Edit'),
      });
      setSaveNote('');
      setNotification({ message: 'Files saved successfully!', type: 'success' });
      await loadRevisions();

      if (deployAfter) {
        onClose();
        onDeploy('up');
      }
    } catch (err: any) {
      setNotification({ message: err.message || 'Failed to save', type: 'error' });
    } finally {
      setSaving(false);
    }
  };

  const restoreRevision = (rev: StackRevision) => {
    if (confirm(`Restore revision #${rev.revision_num} from ${new Date(rev.created_at).toLocaleString()}?`)) {
      setComposeContent(rev.compose_content);
      setEnvContent(rev.env_content);
      setActiveTab('compose');
      setNotification({ message: `Loaded revision #${rev.revision_num}. Click Save to apply.`, type: 'success' });
    }
  };

  // Check for Docker Compose v2 specification issue in composeContent
  const nameIssue = (() => {
    if (!composeContent) return null;
    const m = composeContent.match(/^name\s*:\s*(.+)$/m);
    if (!m) return null;
    const raw = m[1].split('#')[0].trim().replace(/^['"]|['"]$/g, '');
    if (!raw || /^[a-z0-9][a-z0-9_-]*$/.test(raw)) return null;
    const proposed = raw
      .toLowerCase()
      .replace(/[^a-z0-9_-]/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '');
    return {
      oldName: raw,
      proposedName: proposed || 'stack',
    };
  })();

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-5xl h-[90vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div>
            <div className="flex items-center gap-2">
              <FileCode className="w-5 h-5 text-sky-400" />
              <h2 className="text-base font-semibold text-slate-100">{stackName}</h2>
              <span className="rounded bg-sky-500/10 px-2 py-0.5 text-xs font-mono text-sky-400 border border-sky-500/20">
                Compose Stack
              </span>
            </div>
            <p className="text-xs text-slate-400 font-mono mt-0.5">{stackPath}</p>
          </div>

          {/* Action buttons */}
          <div className="flex items-center gap-2.5">
            <input
              type="text"
              placeholder="Revision note (optional)"
              value={saveNote}
              onChange={(e) => setSaveNote(e.target.value)}
              className="rounded-lg bg-slate-800/80 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
            />
            <button
              onClick={() => handleSave(false)}
              disabled={saving}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 py-1.5 px-3 text-xs font-medium text-slate-200 transition-colors"
            >
              <Save className="w-3.5 h-3.5 text-sky-400" />
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button
              onClick={() => handleSave(true)}
              disabled={saving}
              className="flex items-center gap-1.5 rounded-lg bg-sky-600 hover:bg-sky-500 py-1.5 px-3.5 text-xs font-medium text-white shadow-md shadow-sky-600/20 transition-all"
            >
              <Rocket className="w-3.5 h-3.5" />
              Save & Deploy
            </button>
            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors ml-1"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Subheader / Tabs */}
        <div className="flex items-center justify-between border-b border-slate-800 bg-slate-900/80 px-5 py-2">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setActiveTab('compose')}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === 'compose'
                  ? 'bg-slate-800 text-sky-400 border border-slate-700'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <FileCode className="w-3.5 h-3.5" />
              docker-compose.yml
            </button>
            <button
              onClick={() => setActiveTab('env')}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === 'env'
                  ? 'bg-slate-800 text-sky-400 border border-slate-700'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Sliders className="w-3.5 h-3.5" />
              .env
            </button>
            <button
              onClick={() => setActiveTab('revisions')}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                activeTab === 'revisions'
                  ? 'bg-slate-800 text-sky-400 border border-slate-700'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <History className="w-3.5 h-3.5" />
              History ({revisions.length})
            </button>
          </div>

          {notification && (
            <div
              className={`flex items-center gap-1.5 text-xs font-medium ${
                notification.type === 'success' ? 'text-emerald-400' : 'text-rose-400'
              }`}
            >
              {notification.type === 'success' ? (
                <CheckCircle className="w-4 h-4" />
              ) : (
                <AlertCircle className="w-4 h-4" />
              )}
              {notification.message}
            </div>
          )}
        </div>

        {/* Specification Warning Banner (Proposes Fix to User) */}
        {nameIssue && activeTab === 'compose' && (
          <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-2.5 bg-amber-500/10 border-b border-amber-500/20 text-xs shrink-0">
            <div className="flex items-center gap-2 text-amber-300">
              <AlertCircle className="w-4 h-4 shrink-0 text-amber-400" />
              <span>
                Top-level <code className="bg-slate-900/80 px-1 py-0.5 rounded font-mono font-semibold">name: {nameIssue.oldName}</code> contains spaces or characters that Docker Compose v2 will reject (<code className="text-slate-400">^[a-z0-9][a-z0-9_-]*$</code>).
              </span>
            </div>
            <button
              type="button"
              onClick={() => {
                setComposeContent((prev) =>
                  prev.replace(/(^name\s*:\s*)(.+)$/m, `$1${nameIssue.proposedName}`)
                );
                setNotification({
                  message: `Proposed fix applied to editor. Click Save to persist.`,
                  type: 'success',
                });
              }}
              className="px-2.5 py-1 rounded bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 border border-amber-500/30 font-medium transition-colors cursor-pointer"
            >
              Suggested Fix: change to <span className="font-mono font-semibold text-emerald-400">"{nameIssue.proposedName}"</span>
            </button>
          </div>
        )}

        {/* Main Content Area */}
        <div className="flex-1 overflow-hidden relative">
          {loading ? (
            <div className="flex h-full items-center justify-center text-slate-500 font-mono text-sm">
              Loading files from server...
            </div>
          ) : activeTab === 'compose' ? (
            <textarea
              value={composeContent}
              onChange={(e) => setComposeContent(e.target.value)}
              placeholder="# Enter docker-compose.yml here..."
              spellCheck={false}
              className="w-full h-full p-4 font-mono text-xs bg-[#090d16] text-slate-200 resize-none focus:outline-none leading-relaxed selection:bg-sky-500/30"
            />
          ) : activeTab === 'env' ? (
            <textarea
              value={envContent}
              onChange={(e) => setEnvContent(e.target.value)}
              placeholder="# Enter environment variables here (KEY=VALUE)..."
              spellCheck={false}
              className="w-full h-full p-4 font-mono text-xs bg-[#090d16] text-slate-200 resize-none focus:outline-none leading-relaxed selection:bg-sky-500/30"
            />
          ) : (
            <div className="h-full overflow-y-auto p-5 space-y-3 bg-[#090d16]">
              {revisions.length === 0 ? (
                <div className="text-center py-12 text-slate-500 text-sm">
                  No revisions recorded yet. A revision snapshot is created automatically every time you save.
                </div>
              ) : (
                revisions.map((rev) => (
                  <div
                    key={rev.id}
                    className="flex items-center justify-between p-3.5 rounded-lg border border-slate-800 bg-slate-900/60 hover:border-slate-700 transition-colors"
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono font-semibold text-sky-400 text-xs">
                          Rev #{rev.revision_num}
                        </span>
                        <span className="text-xs text-slate-400">
                          by {rev.created_by || 'Admin'}
                        </span>
                        <span className="text-xs text-slate-500">
                          {new Date(rev.created_at).toLocaleString()}
                        </span>
                      </div>
                      {rev.note && (
                        <p className="text-xs text-slate-300 mt-1 italic font-sans">
                          "{rev.note}"
                        </p>
                      )}
                    </div>
                    <button
                      onClick={() => restoreRevision(rev)}
                      className="rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 px-3 py-1 text-xs font-medium text-slate-200 transition-colors"
                    >
                      Rollback to this
                    </button>
                  </div>
                ))
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
