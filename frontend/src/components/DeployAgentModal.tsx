import React, { useEffect, useState } from 'react';
import { X, Copy, Check, Terminal, FileCode, GitBranch, Cpu, CheckCircle2 } from 'lucide-react';
import { api } from '../api/client';
import { Host } from '../types';

interface DeployAgentModalProps {
  onClose: () => void;
  hosts: Host[];
}

export const DeployAgentModal: React.FC<DeployAgentModalProps> = ({ onClose, hosts }) => {
  const [nodeName, setNodeName] = useState('remote-node-1');
  const [agentToken, setAgentToken] = useState('fetching...');
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<'curl' | 'compose' | 'git'>('curl');

  const serverHost = window.location.host;
  const protocol = window.location.protocol;

  useEffect(() => {
    fetch('/api/agent/token', {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('dockpulse_token') || ''}`,
      },
    })
      .then((res) => res.json())
      .then((data) => {
        if (data.token) {
          setAgentToken(data.token);
        }
      })
      .catch(() => {
        setAgentToken('dockerpulse_agent_shared_join_token_2026');
      });
  }, []);

  const copyText = (text: string, idx: number) => {
    navigator.clipboard.writeText(text);
    setCopiedIndex(idx);
    setTimeout(() => setCopiedIndex(null), 2000);
  };

  const curlInstallerCmd = `curl -fsSL "${protocol}//${serverHost}/install-agent.sh?id=${nodeName}&token=${agentToken}" | bash`;

  const curlComposeCmd = `mkdir -p ~/docker/dockerpulse-agent && cd ~/docker/dockerpulse-agent
curl -fsSL "${protocol}//${serverHost}/docker-compose.agent.yml?id=${nodeName}&token=${agentToken}" -o docker-compose.yml
docker compose up -d`;

  const gitCloneCmd = `git clone https://github.com/Farmers00/DockerPulse.git ~/docker/dockerpulse
cd ~/docker/dockerpulse/deploy
docker compose -f docker-compose.agent.yml up -d`;

  const isConnected = hosts.some(
    (h) => h.id === nodeName || h.name.toLowerCase() === nodeName.toLowerCase()
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-3xl flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div className="flex items-center gap-2">
            <Cpu className="w-5 h-5 text-sky-400" />
            <h2 className="text-base font-semibold text-slate-100">Deploy DockerPulse Agent</h2>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-6">
          {/* Node name input */}
          <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-4">
            <label className="block text-xs font-semibold text-slate-300 uppercase tracking-wider mb-1.5">
              Remote Server Identifier
            </label>
            <div className="flex items-center gap-3">
              <input
                type="text"
                value={nodeName}
                onChange={(e) => setNodeName(e.target.value.replace(/\s+/g, '-'))}
                placeholder="e.g. media-server or storage-node"
                className="flex-1 rounded-lg bg-slate-800/90 border border-slate-700 py-1.5 px-3 text-xs font-mono text-slate-100 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
              {isConnected ? (
                <div className="flex items-center gap-1.5 text-xs text-emerald-400 font-semibold bg-emerald-500/10 border border-emerald-500/20 px-3 py-1.5 rounded-lg">
                  <CheckCircle2 className="w-4 h-4" /> Connected & Active
                </div>
              ) : (
                <span className="text-[11px] text-slate-500 italic">Waiting for connection...</span>
              )}
            </div>
          </div>

          {/* Deployment Method Tabs */}
          <div className="space-y-4">
            <div className="flex items-center gap-2 border-b border-slate-800 pb-2">
              <button
                onClick={() => setActiveTab('curl')}
                className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                  activeTab === 'curl'
                    ? 'bg-sky-500/10 text-sky-400 border border-sky-500/30'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <Terminal className="w-3.5 h-3.5" />
                1-Line Curl Script (Fastest)
              </button>

              <button
                onClick={() => setActiveTab('compose')}
                className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                  activeTab === 'compose'
                    ? 'bg-sky-500/10 text-sky-400 border border-sky-500/30'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <FileCode className="w-3.5 h-3.5" />
                Docker Compose Download
              </button>

              <button
                onClick={() => setActiveTab('git')}
                className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                  activeTab === 'git'
                    ? 'bg-sky-500/10 text-sky-400 border border-sky-500/30'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <GitBranch className="w-3.5 h-3.5" />
                Git Clone
              </button>
            </div>

            {/* Method 1: 1-Line Curl */}
            {activeTab === 'curl' && (
              <div className="space-y-3">
                <p className="text-xs text-slate-400">
                  Log into your remote Linux server and paste this single command. It will download the agent compose setup, mount <code className="text-sky-300">~/docker</code>, and start the agent automatically:
                </p>
                <div className="relative rounded-lg bg-slate-950 p-4 font-mono text-xs text-sky-300 border border-slate-800 break-all leading-relaxed">
                  {curlInstallerCmd}
                  <button
                    onClick={() => copyText(curlInstallerCmd, 1)}
                    className="absolute right-2.5 top-2.5 flex items-center gap-1 rounded bg-slate-800 hover:bg-slate-700 px-2.5 py-1 text-xs text-slate-200 transition-colors"
                  >
                    {copiedIndex === 1 ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    {copiedIndex === 1 ? 'Copied!' : 'Copy'}
                  </button>
                </div>
              </div>
            )}

            {/* Method 2: Compose Download */}
            {activeTab === 'compose' && (
              <div className="space-y-3">
                <p className="text-xs text-slate-400">
                  Downloads the pre-configured <code className="text-sky-300">docker-compose.yml</code> file into <code className="text-sky-300">~/docker/dockerpulse-agent/</code> and starts it:
                </p>
                <div className="relative rounded-lg bg-slate-950 p-4 font-mono text-xs text-sky-300 border border-slate-800 whitespace-pre-wrap leading-relaxed">
                  {curlComposeCmd}
                  <button
                    onClick={() => copyText(curlComposeCmd, 2)}
                    className="absolute right-2.5 top-2.5 flex items-center gap-1 rounded bg-slate-800 hover:bg-slate-700 px-2.5 py-1 text-xs text-slate-200 transition-colors"
                  >
                    {copiedIndex === 2 ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    {copiedIndex === 2 ? 'Copied!' : 'Copy'}
                  </button>
                </div>
              </div>
            )}

            {/* Method 3: Git Clone */}
            {activeTab === 'git' && (
              <div className="space-y-3">
                <p className="text-xs text-slate-400">
                  Clone your private repository directly onto the remote host and run the agent compose file:
                </p>
                <div className="relative rounded-lg bg-slate-950 p-4 font-mono text-xs text-sky-300 border border-slate-800 whitespace-pre-wrap leading-relaxed">
                  {gitCloneCmd}
                  <button
                    onClick={() => copyText(gitCloneCmd, 3)}
                    className="absolute right-2.5 top-2.5 flex items-center gap-1 rounded bg-slate-800 hover:bg-slate-700 px-2.5 py-1 text-xs text-slate-200 transition-colors"
                  >
                    {copiedIndex === 3 ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    {copiedIndex === 3 ? 'Copied!' : 'Copy'}
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="flex justify-end border-t border-slate-800 px-5 py-3 bg-slate-950/60">
          <button
            onClick={onClose}
            className="rounded-lg bg-slate-800 hover:bg-slate-700 px-4 py-1.5 text-xs font-medium text-slate-300 transition-colors"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
};
