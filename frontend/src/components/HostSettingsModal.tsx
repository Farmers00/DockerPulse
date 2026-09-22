import React, { useState } from 'react';
import { X, Settings, Folder, Server, Trash2, CheckCircle2, AlertCircle, Cpu } from 'lucide-react';
import { Host } from '../types';
import { api } from '../api/client';

interface HostSettingsModalProps {
  host: Host;
  onClose: () => void;
  onUpdated: (updatedHost: Host) => void;
  onDeleted: (hostId: string) => void;
}

export const HostSettingsModal: React.FC<HostSettingsModalProps> = ({
  host,
  onClose,
  onUpdated,
  onDeleted,
}) => {
  const [name, setName] = useState(host.name);
  const [baseDir, setBaseDir] = useState(host.base_dir || '~/docker');
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      setSaving(true);
      const updated = await api.updateHost(host.id, {
        name: name.trim() || host.name,
        base_dir: baseDir.trim() || '~/docker',
      });
      onUpdated(updated);
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to update host settings');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    const confirmed = window.confirm(
      `Are you sure you want to remove "${host.name}" from DockerPulse? Containers running on that server will not be affected.`
    );
    if (!confirmed) return;

    try {
      setDeleting(true);
      await api.deleteHost(host.id);
      onDeleted(host.id);
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to delete host');
      setDeleting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-lg rounded-2xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 pb-4 mb-5">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-400">
              <Settings className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold text-slate-100">Server Settings</h2>
              <p className="text-xs text-slate-400 font-mono">{host.name}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {error && (
          <div className="mb-4 rounded-lg bg-rose-500/10 border border-rose-500/20 p-3 text-xs text-rose-400 font-medium flex items-center gap-2">
            <AlertCircle className="w-4 h-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSave} className="space-y-4">
          {/* Server Name */}
          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1">
              Server Display Name
            </label>
            <div className="relative">
              <Server className="w-4 h-4 absolute left-3 top-2.5 text-slate-500" />
              <input
                type="text"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-lg bg-slate-800/90 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          {/* Docker Stacks Directory */}
          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1">
              Docker Stacks Directory
            </label>
            <div className="relative">
              <Folder className="w-4 h-4 absolute left-3 top-2.5 text-sky-400" />
              <input
                type="text"
                required
                value={baseDir}
                onChange={(e) => setBaseDir(e.target.value)}
                placeholder="~/docker"
                className="w-full rounded-lg bg-slate-800/90 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
            <p className="text-[11px] text-slate-400 mt-1">
              Base path on the server where Compose folders are stored (e.g.{' '}
              <span className="font-mono text-slate-300">~/docker</span>,{' '}
              <span className="font-mono text-slate-300">/home/user/docker</span>, or{' '}
              <span className="font-mono text-slate-300">/opt/docker</span>).
            </p>
          </div>

          {/* Server Diagnostics & Meta */}
          <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-3.5 space-y-2 text-xs">
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Connection Driver</span>
              <span className="font-mono font-semibold text-sky-400 uppercase">
                {host.driver}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Status</span>
              <span className="flex items-center gap-1.5 font-medium">
                <span
                  className={`w-2 h-2 rounded-full ${
                    host.status === 'online' ? 'bg-emerald-400' : 'bg-rose-500'
                  }`}
                />
                <span className={host.status === 'online' ? 'text-emerald-400' : 'text-rose-400'}>
                  {host.status === 'online' ? 'Connected' : 'Offline'}
                </span>
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-slate-500">Host ID</span>
              <span className="font-mono text-slate-400 text-[11px]">{host.id}</span>
            </div>
          </div>

          {/* Buttons */}
          <div className="flex items-center justify-between pt-3 border-t border-slate-800">
            <button
              type="button"
              onClick={handleDelete}
              disabled={deleting || saving}
              className="flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-semibold text-rose-400 hover:bg-rose-500/10 border border-rose-500/20 transition-colors disabled:opacity-50"
            >
              <Trash2 className="w-3.5 h-3.5" />
              {deleting ? 'Removing...' : 'Remove Server'}
            </button>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={onClose}
                className="rounded-lg px-4 py-2 text-xs font-semibold text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={saving || deleting}
                className="flex items-center gap-1.5 rounded-lg bg-sky-600 hover:bg-sky-500 px-4 py-2 text-xs font-semibold text-white shadow-md shadow-sky-600/20 transition-all disabled:opacity-50"
              >
                <CheckCircle2 className="w-3.5 h-3.5" />
                {saving ? 'Saving...' : 'Save Settings'}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
};
