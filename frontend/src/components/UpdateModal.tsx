import React, { useEffect, useRef, useState, useCallback } from 'react';
import { X, RefreshCw, CheckCircle, AlertCircle, AlertTriangle, FileCode } from 'lucide-react';
import { api } from '../api/client';

interface UpdateModalProps {
  hostId: string;
  stackId: string;
  stackName: string;
  action: string; // 'pull_up', 'pull', 'up', 'down', 'restart'
  onClose: () => void;
  onFinished: () => void;
  onOpenEditor?: () => void;
}

export const UpdateModal: React.FC<UpdateModalProps> = ({
  hostId,
  stackId,
  stackName,
  action,
  onClose,
  onFinished,
  onOpenEditor,
}) => {
  const [output, setOutput] = useState<string>('');
  const [status, setStatus] = useState<'running' | 'success' | 'error'>('running');
  const [errorMsg, setErrorMsg] = useState<string>('');
  const [detectedIssue, setDetectedIssue] = useState<{ oldName: string; proposedName: string } | null>(null);
  const [applyingFix, setApplyingFix] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);

  const runAction = useCallback(() => {
    setOutput(`[DockerPulse] Initiating action '${action}' on stack '${stackName}'...\n`);
    setStatus('running');
    setErrorMsg('');
    setDetectedIssue(null);

    api.streamComposeAction(
      hostId,
      stackId,
      action,
      (chunk) => {
        setOutput((prev) => prev + chunk);
      },
      (err) => {
        if (err) {
          setStatus('error');
          setErrorMsg(err.message);
        } else {
          setStatus('success');
          onFinished();
        }
      }
    );
  }, [hostId, stackId, action, stackName, onFinished]);

  useEffect(() => {
    runAction();
  }, [runAction]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [output]);

  // Detect naming issues when an error occurs
  useEffect(() => {
    if (
      status === 'error' &&
      (output.includes("name Does not match pattern '^[a-z0-9][a-z0-9_-]*$'") ||
        output.includes('violates Docker Compose v2 naming rules'))
    ) {
      const matchOld = output.match(/Top-level 'name:\s*([^']+)'/);
      const matchProposed = output.match(/Suggested fix: 'name:\s*([^']+)'/);
      if (matchOld && matchProposed) {
        setDetectedIssue({ oldName: matchOld[1], proposedName: matchProposed[1] });
      } else {
        // Inspect compose file directly to find the invalid name and generate proposal
        api
          .getStackFiles(hostId, stackId)
          .then((files) => {
            const m = files.compose.match(/^name\s*:\s*(.+)$/m);
            if (m) {
              const raw = m[1].split('#')[0].trim().replace(/^['"]|['"]$/g, '');
              if (!/^[a-z0-9][a-z0-9_-]*$/.test(raw)) {
                const proposed = raw
                  .toLowerCase()
                  .replace(/[^a-z0-9_-]/g, '-')
                  .replace(/-+/g, '-')
                  .replace(/^-|-$/g, '');
                setDetectedIssue({ oldName: raw, proposedName: proposed || 'stack' });
              }
            }
          })
          .catch(() => {});
      }
    }
  }, [status, output, hostId, stackId]);

  const handleApplyFixAndRetry = async () => {
    if (!detectedIssue) return;
    try {
      setApplyingFix(true);
      const files = await api.getStackFiles(hostId, stackId);
      const fixedCompose = files.compose.replace(
        /(^name\s*:\s*)(.+)$/m,
        `$1${detectedIssue.proposedName}`
      );
      await api.saveStackFiles(hostId, stackId, {
        compose: fixedCompose,
        env: files.env,
        note: `Updated project name from '${detectedIssue.oldName}' to '${detectedIssue.proposedName}' (User Confirmed)`,
      });
      setOutput(
        (prev) =>
          prev +
          `\n[DockerPulse] Applied proposed fix: updated project name to '${detectedIssue.proposedName}'. Retrying ${action}...\n\n`
      );
      setDetectedIssue(null);
      runAction();
    } catch (err: any) {
      alert(err.message || 'Failed to apply proposed fix');
    } finally {
      setApplyingFix(false);
    }
  };

  const getActionLabel = () => {
    switch (action) {
      case 'pull_up':
        return 'Update (Pull & Up -d)';
      case 'pull':
        return 'Pull Images';
      case 'up':
        return 'Deploy / Up -d';
      case 'down':
        return 'Stop Stack (Down)';
      case 'restart':
        return 'Restart Stack';
      default:
        return action;
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-4xl h-[80vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div className="flex items-center gap-3">
            {status === 'running' ? (
              <RefreshCw className="w-5 h-5 text-sky-400 animate-spin" />
            ) : status === 'success' ? (
              <CheckCircle className="w-5 h-5 text-emerald-400" />
            ) : (
              <AlertCircle className="w-5 h-5 text-rose-400" />
            )}
            <div>
              <h2 className="text-sm font-semibold text-slate-100 flex items-center gap-2">
                <span>{getActionLabel()}</span>
                <span className="text-slate-400 font-normal">on</span>
                <span className="text-sky-300 font-mono">{stackName}</span>
              </h2>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Issue Warning & Proposal Banner (Shown when issue is caught, asking for user confirmation) */}
        {detectedIssue && (
          <div className="mx-4 my-3 p-4 rounded-xl border border-amber-500/40 bg-amber-950/20 text-slate-200 shadow-lg shrink-0">
            <div className="flex items-start gap-3">
              <AlertTriangle className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
              <div className="flex-1">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs font-semibold text-amber-300 uppercase tracking-wider">
                    Compose Specification Issue Detected
                  </h3>
                  <span className="text-[10px] px-2 py-0.5 rounded bg-amber-500/20 text-amber-300 font-mono">
                    Requires User Approval
                  </span>
                </div>
                <p className="text-xs text-slate-300 mt-1.5 leading-relaxed">
                  The top-level project name <code className="bg-slate-900 px-1.5 py-0.5 rounded text-amber-300 font-mono font-semibold">name: {detectedIssue.oldName}</code> in <code className="bg-slate-900 px-1.5 py-0.5 rounded text-slate-300 font-mono">compose.yml</code> contains spaces or characters that Docker Compose v2 rejects with pattern <code className="bg-slate-900 px-1 py-0.5 rounded text-slate-400 font-mono">^[a-z0-9][a-z0-9_-]*$</code>.
                </p>

                <div className="mt-3 p-3 rounded-lg bg-slate-900/90 border border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                  <div className="text-xs font-mono">
                    <span className="text-slate-400">Proposed change: </span>
                    <span className="text-rose-400 line-through mr-2 font-semibold">name: {detectedIssue.oldName}</span>
                    <span className="text-emerald-400 font-semibold">name: {detectedIssue.proposedName}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    {onOpenEditor && (
                      <button
                        onClick={onOpenEditor}
                        className="flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs rounded-lg transition-colors border border-slate-700"
                      >
                        <FileCode className="w-3.5 h-3.5" />
                        Edit File Manually
                      </button>
                    )}
                    <button
                      onClick={handleApplyFixAndRetry}
                      disabled={applyingFix}
                      className="px-3.5 py-1.5 bg-amber-500 hover:bg-amber-400 text-slate-950 font-semibold text-xs rounded-lg transition-colors shadow-sm disabled:opacity-50"
                    >
                      {applyingFix ? 'Applying Fix...' : 'Apply Fix & Retry'}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Live Terminal Output */}
        <div className="flex-1 overflow-y-auto p-4 font-mono text-xs text-slate-300 bg-[#090d16] leading-relaxed selection:bg-sky-500/30 whitespace-pre-wrap">
          {output}
          <div ref={endRef} />
        </div>

        {/* Footer status bar */}
        <div className="flex items-center justify-between border-t border-slate-800 px-5 py-3 bg-slate-950/60">
          <div className="text-xs">
            {status === 'running' && (
              <span className="text-sky-400 animate-pulse font-medium">Executing command in stack directory...</span>
            )}
            {status === 'success' && (
              <span className="text-emerald-400 font-medium">Operation completed successfully!</span>
            )}
            {status === 'error' && (
              <span className="text-rose-400 font-medium">Operation failed: {errorMsg}</span>
            )}
          </div>
          <button
            onClick={onClose}
            className="rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-4 py-1.5 text-xs font-medium text-slate-200 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
