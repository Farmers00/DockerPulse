import React, { useState } from 'react';
import { X, Server, Terminal, Key, Cpu, Copy, Check } from 'lucide-react';
import { api } from '../api/client';
import { Host } from '../types';

interface AddHostModalProps {
  onClose: () => void;
  onAdded: (host: Host) => void;
}

export const AddHostModal: React.FC<AddHostModalProps> = ({ onClose, onAdded }) => {
  const [driver, setDriver] = useState<'agent' | 'ssh' | 'socket'>('agent');
  const [name, setName] = useState('');
  const [baseDir, setBaseDir] = useState('~/docker');
  const [address, setAddress] = useState('');
  const [port, setPort] = useState(22);
  const [sshUser, setSshUser] = useState('root');
  const [sshKey, setSshKey] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name) return;

    try {
      setSubmitting(true);
      const created = await api.createHost({
        name,
        driver,
        base_dir: baseDir,
        address: driver === 'socket' ? address || 'local' : address,
        port: driver === 'ssh' ? port : 0,
        ssh_user: driver === 'ssh' ? sshUser : undefined,
        ssh_key: driver === 'ssh' ? sshKey : undefined,
      });
      onAdded(created);
      onClose();
    } catch (err: any) {
      alert(err.message || 'Failed to add host');
    } finally {
      setSubmitting(false);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const agentRunSnippet = `docker run -d \\
  --name dockpulse-agent \\
  --restart unless-stopped \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -v ${baseDir.replace('~', '$HOME')}:/stacks \\
  dockpulse/dockmgr:latest \\
  dockmgr agent --server ws://${window.location.host}/ws/agent --token YOUR_TOKEN --host-id HOST_ID`;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-2xl flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div className="flex items-center gap-2">
            <Server className="w-5 h-5 text-sky-400" />
            <h2 className="text-base font-semibold text-slate-100">Add Docker Host</h2>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-6 space-y-5">
          {/* Driver Selection Tabs */}
          <div>
            <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Connection Driver
            </label>
            <div className="grid grid-cols-3 gap-3">
              <button
                type="button"
                onClick={() => setDriver('agent')}
                className={`p-3 rounded-lg border text-left flex flex-col gap-1 transition-all ${
                  driver === 'agent'
                    ? 'border-sky-500 bg-sky-500/10 text-sky-300'
                    : 'border-slate-800 bg-slate-950/40 text-slate-400 hover:bg-slate-800/40'
                }`}
              >
                <div className="flex items-center gap-1.5 font-semibold text-xs text-slate-200">
                  <Cpu className="w-4 h-4 text-sky-400" /> DockPulse Agent
                </div>
                <span className="text-[11px] text-slate-500">
                  Lightweight container on node, low-latency live streams.
                </span>
              </button>

              <button
                type="button"
                onClick={() => setDriver('ssh')}
                className={`p-3 rounded-lg border text-left flex flex-col gap-1 transition-all ${
                  driver === 'ssh'
                    ? 'border-sky-500 bg-sky-500/10 text-sky-300'
                    : 'border-slate-800 bg-slate-950/40 text-slate-400 hover:bg-slate-800/40'
                }`}
              >
                <div className="flex items-center gap-1.5 font-semibold text-xs text-slate-200">
                  <Key className="w-4 h-4 text-sky-400" /> Direct SSH
                </div>
                <span className="text-[11px] text-slate-500">
                  Zero agent needed. Connects directly using SSH keys.
                </span>
              </button>

              <button
                type="button"
                onClick={() => setDriver('socket')}
                className={`p-3 rounded-lg border text-left flex flex-col gap-1 transition-all ${
                  driver === 'socket'
                    ? 'border-sky-500 bg-sky-500/10 text-sky-300'
                    : 'border-slate-800 bg-slate-950/40 text-slate-400 hover:bg-slate-800/40'
                }`}
              >
                <div className="flex items-center gap-1.5 font-semibold text-xs text-slate-200">
                  <Terminal className="w-4 h-4 text-sky-400" /> Docker Socket / TLS
                </div>
                <span className="text-[11px] text-slate-500">
                  Local /var/run/docker.sock or exposed TCP TLS daemon.
                </span>
              </button>
            </div>
          </div>

          {/* Common fields */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">Server Name</label>
              <input
                type="text"
                required
                placeholder="e.g. media-server or vps-prod"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Compose Base Directory
              </label>
              <input
                type="text"
                required
                placeholder="~/docker"
                value={baseDir}
                onChange={(e) => setBaseDir(e.target.value)}
                className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          {/* Driver specific fields */}
          {driver === 'ssh' && (
            <div className="space-y-4 pt-2 border-t border-slate-800">
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-medium text-slate-300 mb-1">Host IP / Domain</label>
                  <input
                    type="text"
                    required
                    placeholder="192.168.1.100"
                    value={address}
                    onChange={(e) => setAddress(e.target.value)}
                    className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-300 mb-1">Port</label>
                  <input
                    type="number"
                    value={port}
                    onChange={(e) => setPort(parseInt(e.target.value) || 22)}
                    className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-300 mb-1">SSH Username</label>
                <input
                  type="text"
                  placeholder="root or ubuntu"
                  value={sshUser}
                  onChange={(e) => setSshUser(e.target.value)}
                  className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-300 mb-1">
                  SSH Private Key (OpenSSH format)
                </label>
                <textarea
                  rows={4}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;..."
                  value={sshKey}
                  onChange={(e) => setSshKey(e.target.value)}
                  className="w-full rounded-lg bg-slate-950 font-mono text-xs border border-slate-700 p-2.5 text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
                />
              </div>
            </div>
          )}

          {driver === 'socket' && (
            <div className="pt-2 border-t border-slate-800">
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Docker Socket Path or TCP URL
              </label>
              <input
                type="text"
                placeholder="local (default) or tcp://192.168.1.50:2375"
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
              <p className="text-[11px] text-slate-500 mt-1">
                Leave empty or use "local" to bind to the local Docker socket on this machine.
              </p>
            </div>
          )}

          {driver === 'agent' && (
            <div className="pt-2 border-t border-slate-800">
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Agent Run Command (Preview)
              </label>
              <div className="relative rounded-lg bg-slate-950 p-3 font-mono text-[11px] text-sky-300 border border-slate-800">
                <pre className="whitespace-pre-wrap">{agentRunSnippet}</pre>
                <button
                  type="button"
                  onClick={() => copyToClipboard(agentRunSnippet)}
                  className="absolute right-2 top-2 rounded bg-slate-800 hover:bg-slate-700 p-1.5 text-slate-300 transition-colors"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
              <p className="text-[11px] text-slate-500 mt-2">
                Clicking "Add Host" will register the host record and provide you with the exact join token.
              </p>
            </div>
          )}

          {/* Footer Submit */}
          <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg bg-slate-800 hover:bg-slate-700 px-4 py-2 text-xs font-medium text-slate-300 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="rounded-lg bg-sky-600 hover:bg-sky-500 px-4 py-2 text-xs font-medium text-white transition-colors shadow-md shadow-sky-600/20"
            >
              {submitting ? 'Adding...' : 'Add Host'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
