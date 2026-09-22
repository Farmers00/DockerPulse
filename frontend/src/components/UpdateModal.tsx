import React, { useEffect, useRef, useState } from 'react';
import { X, RefreshCw, CheckCircle, AlertCircle } from 'lucide-react';
import { api } from '../api/client';

interface UpdateModalProps {
  hostId: string;
  stackId: string;
  stackName: string;
  action: string; // 'pull_up', 'pull', 'up', 'down', 'restart'
  onClose: () => void;
  onFinished: () => void;
}

export const UpdateModal: React.FC<UpdateModalProps> = ({
  hostId,
  stackId,
  stackName,
  action,
  onClose,
  onFinished,
}) => {
  const [output, setOutput] = useState<string>('');
  const [status, setStatus] = useState<'running' | 'success' | 'error'>('running');
  const [errorMsg, setErrorMsg] = useState<string>('');
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setOutput(`[DockPulse] Initiating action '${action}' on stack '${stackName}'...\n`);
    setStatus('running');

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
  }, [hostId, stackId, action]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [output]);

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
      <div className="relative w-full max-w-4xl h-[75vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
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
