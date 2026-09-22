import React, { useEffect, useState } from 'react';
import { X, Network, Plus, Trash2, Link2, Unlink } from 'lucide-react';
import { api } from '../api/client';
import { NetworkInfo, ContainerInfo } from '../types';

interface NetworksModalProps {
  hostId: string;
  hostName: string;
  containers: ContainerInfo[];
  onClose: () => void;
}

export const NetworksModal: React.FC<NetworksModalProps> = ({
  hostId,
  hostName,
  containers,
  onClose,
}) => {
  const [networks, setNetworks] = useState<NetworkInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [newNetName, setNewNetName] = useState('');
  const [newNetDriver, setNewNetDriver] = useState('bridge');
  const [selectedNetwork, setSelectedNetwork] = useState<string | null>(null);
  const [connectContainerId, setConnectContainerId] = useState('');

  useEffect(() => {
    loadNetworks();
  }, [hostId]);

  const loadNetworks = async () => {
    try {
      setLoading(true);
      const data = await api.listNetworks(hostId);
      const list = Array.isArray(data) ? data : [];
      setNetworks(list);
      if (list.length > 0 && !selectedNetwork) {
        setSelectedNetwork(list[0].id);
      }
    } catch (err) {
      console.error(err);
      setNetworks([]);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newNetName) return;
    try {
      await api.createNetwork(hostId, newNetName, newNetDriver);
      setNewNetName('');
      await loadNetworks();
    } catch (err: any) {
      alert(err.message || 'Failed to create network');
    }
  };

  const handleDelete = async (nid: string, name: string) => {
    if (confirm(`Delete Docker network '${name}'?`)) {
      try {
        await api.removeNetwork(hostId, nid);
        await loadNetworks();
      } catch (err: any) {
        alert(err.message || 'Failed to remove network');
      }
    }
  };

  const handleConnect = async (nid: string) => {
    if (!connectContainerId) return;
    try {
      await api.connectNetwork(hostId, nid, connectContainerId);
      setConnectContainerId('');
      await loadNetworks();
    } catch (err: any) {
      alert(err.message || 'Failed to connect container');
    }
  };

  const handleDisconnect = async (nid: string, cid: string) => {
    try {
      await api.disconnectNetwork(hostId, nid, cid);
      await loadNetworks();
    } catch (err: any) {
      alert(err.message || 'Failed to disconnect container');
    }
  };

  const currentNet = networks.find((n) => n.id === selectedNetwork);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-5xl h-[80vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div className="flex items-center gap-2">
            <Network className="w-5 h-5 text-sky-400" />
            <h2 className="text-base font-semibold text-slate-100">Docker Networks</h2>
            <span className="text-xs text-slate-400 font-mono">({hostName})</span>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content body split: network list on left, detail on right */}
        <div className="flex-1 flex overflow-hidden">
          {/* Left panel: Network list & Create form */}
          <div className="w-80 border-r border-slate-800 flex flex-col bg-slate-950/40">
            {/* Create Network Form */}
            <form onSubmit={handleCreate} className="p-3 border-b border-slate-800 flex flex-col gap-2">
              <input
                type="text"
                placeholder="New network name..."
                value={newNetName}
                onChange={(e) => setNewNetName(e.target.value)}
                className="w-full rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
              <div className="flex gap-2">
                <select
                  value={newNetDriver}
                  onChange={(e) => setNewNetDriver(e.target.value)}
                  className="rounded-lg bg-slate-800 border border-slate-700 py-1 px-2 text-xs text-slate-200 focus:outline-none"
                >
                  <option value="bridge">bridge</option>
                  <option value="overlay">overlay</option>
                  <option value="macvlan">macvlan</option>
                </select>
                <button
                  type="submit"
                  className="flex-1 flex items-center justify-center gap-1 rounded-lg bg-sky-600 hover:bg-sky-500 py-1 text-xs font-medium text-white transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Create
                </button>
              </div>
            </form>

            {/* Network list items */}
            <div className="flex-1 overflow-y-auto p-2 space-y-1">
              {loading ? (
                <div className="p-4 text-center text-xs text-slate-500">Loading networks...</div>
              ) : (
                networks.map((net) => (
                  <button
                    key={net.id}
                    onClick={() => setSelectedNetwork(net.id)}
                    className={`w-full text-left p-2.5 rounded-lg border transition-all flex items-center justify-between ${
                      selectedNetwork === net.id
                        ? 'bg-sky-500/10 border-sky-500/30 text-sky-300'
                        : 'border-transparent hover:bg-slate-800/60 text-slate-300'
                    }`}
                  >
                    <div>
                      <div className="text-xs font-semibold">{net.name}</div>
                      <div className="text-[10px] text-slate-400 font-mono">
                        {net.driver} &bull; {net.scope}
                      </div>
                    </div>
                    {net.subnet && (
                      <span className="text-[10px] font-mono text-slate-400 bg-slate-800 px-1.5 py-0.5 rounded">
                        {net.subnet}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>

          {/* Right panel: Selected Network Details & Containers */}
          <div className="flex-1 overflow-y-auto p-6 bg-[#090d16]">
            {currentNet ? (
              <div className="space-y-6">
                <div className="flex items-center justify-between border-b border-slate-800 pb-4">
                  <div>
                    <h3 className="text-lg font-semibold text-slate-100">{currentNet.name}</h3>
                    <p className="text-xs font-mono text-slate-500">ID: {currentNet.id}</p>
                  </div>
                  {/* Delete network button */}
                  {!['bridge', 'host', 'none'].includes(currentNet.name) && (
                    <button
                      onClick={() => handleDelete(currentNet.id, currentNet.name)}
                      className="flex items-center gap-1.5 rounded-lg bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 px-3 py-1.5 text-xs font-medium transition-colors"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      Delete Network
                    </button>
                  )}
                </div>

                {/* Subnet / Gateway IPAM */}
                <div className="grid grid-cols-3 gap-4">
                  <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-3">
                    <span className="text-xs text-slate-500">Driver</span>
                    <p className="font-mono text-sm text-slate-200 mt-1">{currentNet.driver}</p>
                  </div>
                  <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-3">
                    <span className="text-xs text-slate-500">Subnet</span>
                    <p className="font-mono text-sm text-slate-200 mt-1">{currentNet.subnet || 'Auto'}</p>
                  </div>
                  <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-3">
                    <span className="text-xs text-slate-500">Gateway</span>
                    <p className="font-mono text-sm text-slate-200 mt-1">{currentNet.gateway || 'Auto'}</p>
                  </div>
                </div>

                {/* Connect Container Section */}
                <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4">
                  <h4 className="text-xs font-semibold text-slate-300 mb-2 flex items-center gap-1.5">
                    <Link2 className="w-4 h-4 text-sky-400" />
                    Attach Container to Network
                  </h4>
                  <div className="flex gap-2">
                    <select
                      value={connectContainerId}
                      onChange={(e) => setConnectContainerId(e.target.value)}
                      className="flex-1 rounded-lg bg-slate-800 border border-slate-700 py-1.5 px-3 text-xs text-slate-200 focus:outline-none"
                    >
                      <option value="">Select a container...</option>
                      {containers.map((c) => (
                        <option key={c.id} value={c.id}>
                          {c.names[0]?.replace('/', '')} ({c.image})
                        </option>
                      ))}
                    </select>
                    <button
                      onClick={() => handleConnect(currentNet.id)}
                      disabled={!connectContainerId}
                      className="rounded-lg bg-sky-600 hover:bg-sky-500 disabled:opacity-50 px-4 py-1.5 text-xs font-medium text-white transition-colors"
                    >
                      Connect
                    </button>
                  </div>
                </div>

                {/* Attached Containers */}
                <div>
                  <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">
                    Connected Containers ({Object.keys(currentNet.containers || {}).length})
                  </h4>
                  {Object.keys(currentNet.containers || {}).length === 0 ? (
                    <div className="p-4 rounded-lg border border-slate-800 bg-slate-900/30 text-center text-xs text-slate-500">
                      No containers currently attached to this network.
                    </div>
                  ) : (
                    <div className="space-y-2">
                      {Object.entries(currentNet.containers).map(([cid, ip]) => {
                        const matched = containers.find((c) => c.id === cid || c.id.startsWith(cid));
                        const name = matched?.names[0]?.replace('/', '') || cid.slice(0, 12);
                        return (
                          <div
                            key={cid}
                            className="flex items-center justify-between p-3 rounded-lg border border-slate-800 bg-slate-900/60"
                          >
                            <div>
                              <div className="text-xs font-semibold text-slate-200">{name}</div>
                              <div className="text-xs font-mono text-sky-400">{ip}</div>
                            </div>
                            <button
                              onClick={() => handleDisconnect(currentNet.id, cid)}
                              title="Disconnect from network"
                              className="rounded p-1.5 text-slate-400 hover:bg-rose-500/10 hover:text-rose-400 transition-colors"
                            >
                              <Unlink className="w-4 h-4" />
                            </button>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              </div>
            ) : (
              <div className="h-full flex items-center justify-center text-slate-500 text-sm">
                Select a network from the left to inspect
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
