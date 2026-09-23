import React, { useEffect, useState, useMemo } from 'react';
import {
  Server,
  Layers,
  Box,
  Terminal,
  FileText,
  Play,
  Square,
  RotateCw,
  Trash2,
  Edit,
  ArrowUpCircle,
  Network,
  HardDrive,
  Plus,
  RefreshCw,
  LogOut,
  Folder,
  Activity,
  CheckCircle2,
  AlertCircle,
  Cpu,
  Settings
} from 'lucide-react';
import { api } from './api/client';
import { Host, ContainerInfo, Stack, SystemInfo, User } from './types';
import { LiveLogsModal } from './components/LiveLogsModal';
import { TerminalModal } from './components/TerminalModal';
import { ComposeEditorModal } from './components/ComposeEditorModal';
import { UpdateModal } from './components/UpdateModal';
import { NetworksModal } from './components/NetworksModal';
import { StorageModal } from './components/StorageModal';
import { AddHostModal } from './components/AddHostModal';
import { DeployAgentModal } from './components/DeployAgentModal';
import { HostSettingsModal } from './components/HostSettingsModal';
import { AuthModal } from './components/AuthModal';

function matchesStack(c: ContainerInfo, s: Stack): boolean {
  if (!c || !s) return false;

  // 1. Direct stack name match (exact or case-insensitive)
  if (c.stack) {
    const cStackLower = c.stack.toLowerCase().trim();
    const sNameLower = s.name.toLowerCase().trim();
    if (cStackLower === sNameLower) return true;

    // Docker Compose normalizes project names to alphanumeric (e.g. "llama.cpp" -> "llamacpp")
    const cStackAlpha = cStackLower.replace(/[^a-z0-9]/g, '');
    const sNameAlpha = sNameLower.replace(/[^a-z0-9]/g, '');
    if (cStackAlpha && sNameAlpha && cStackAlpha === sNameAlpha) return true;
  }

  // 2. Working dir match (e.g. /home/farmers00/docker/llama.cpp)
  if (c.working_dir && s.path) {
    const normWorkDir = c.working_dir.replace(/[\\/]+$/, '').toLowerCase();
    const normStackPath = s.path.replace(/[\\/]+$/, '').toLowerCase();
    if (normWorkDir === normStackPath) return true;

    // Basename of working dir matches stack name
    const workDirBase = normWorkDir.split(/[\\/]/).filter(Boolean).pop();
    const sNameLower = s.name.toLowerCase().trim();
    const sNameAlpha = sNameLower.replace(/[^a-z0-9]/g, '');
    if (workDirBase && (workDirBase === sNameLower || workDirBase.replace(/[^a-z0-9]/g, '') === sNameAlpha)) {
      return true;
    }
  }

  // 3. Config file match (e.g. /home/farmers00/docker/llama.cpp/docker-compose.yml)
  if (c.config_file && s.path) {
    const normConfigFile = c.config_file.replace(/\\/g, '/').toLowerCase();
    const normStackPath = s.path.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase();
    if (normConfigFile.startsWith(normStackPath + '/')) return true;
  }

  return false;
}

function formatImage(img: string): string {
  if (!img) return '';
  if (img.startsWith('sha256:')) {
    return 'sha256:' + img.slice(7, 19) + '…';
  }
  return img;
}

function isContainerUpdateAvailable(c: ContainerInfo, updateMap: Record<string, boolean>): boolean {
  if (!c) return false;
  if (c.has_update) return true;
  if (!updateMap) return false;
  if (c.image && updateMap[c.image]) return true;
  if (c.image_id && updateMap[c.image_id]) return true;
  if (c.image) {
    if (updateMap[c.image + ':latest']) return true;
    if (c.image.endsWith(':latest') && updateMap[c.image.replace(/:latest$/, '')]) return true;
    const parts = c.image.split('/');
    const short = parts[parts.length - 1];
    if (updateMap[short]) return true;
    if (updateMap[short + ':latest']) return true;
    if (short.endsWith(':latest') && updateMap[short.replace(/:latest$/, '')]) return true;
  }
  return false;
}

export const App: React.FC = () => {
  const [user, setUser] = useState<User | null>(null);
  const [authNeeded, setAuthNeeded] = useState<boolean | null>(null);
  const [isSetup, setIsSetup] = useState(false);

  const [hosts, setHosts] = useState<Host[]>([]);
  const [selectedHostId, setSelectedHostId] = useState<string>('');
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [containers, setContainers] = useState<ContainerInfo[]>([]);
  const [stacks, setStacks] = useState<Stack[]>([]);
  const [updates, setUpdates] = useState<Record<string, boolean>>({});

  const [viewMode, setViewMode] = useState<'containers' | 'stacks'>('containers');
  const [loading, setLoading] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [checkingUpdates, setCheckingUpdates] = useState(false);

  // Active Modals
  const [logContainer, setLogContainer] = useState<ContainerInfo | null>(null);
  const [terminalContainer, setTerminalContainer] = useState<ContainerInfo | null>(null);
  const [editStack, setEditStack] = useState<Stack | null>(null);
  const [updateAction, setUpdateAction] = useState<{ stack?: Stack; stacks?: Stack[]; action: string } | null>(null);
  const [showNetworks, setShowNetworks] = useState(false);
  const [showStorage, setShowStorage] = useState(false);
  const [showAddHost, setShowAddHost] = useState(false);
  const [showDeployAgent, setShowDeployAgent] = useState(false);
  const [showHostSettings, setShowHostSettings] = useState(false);
  const [fleetStats, setFleetStats] = useState<Record<string, { running: number; total: number; updates: number }>>({});

  // Check auth status on boot
  useEffect(() => {
    checkAuth();
  }, []);

  const checkAuth = async () => {
    try {
      const status = await api.getAuthStatus();
      if (!status.initialized) {
        setIsSetup(true);
        setAuthNeeded(true);
        return;
      }

      const me = await api.getMe();
      setUser(me);
      setAuthNeeded(false);
      loadHosts();
    } catch {
      setAuthNeeded(true);
    }
  };

  const loadHosts = async () => {
    try {
      const raw = await api.listHosts();
      const list = Array.isArray(raw) ? raw : [];
      setHosts(list);
      if (list.length > 0) {
        setSelectedHostId((prev) => (list.some((h) => h.id === prev) ? prev : list[0].id));
      }
    } catch (err) {
      console.error(err);
      setHosts([]);
    }
  };

  const handleConnectLocal = async () => {
    try {
      const created = await api.createHost({
        name: 'Local Server',
        driver: 'socket',
        address: 'local',
        base_dir: '~/docker',
      });
      setHosts([created]);
      setSelectedHostId(created.id);
    } catch (err: any) {
      alert(err.message || 'Failed to connect local host');
    }
  };

  // Load host data when selectedHostId changes
  useEffect(() => {
    if (selectedHostId) {
      setUpdates({});
      refreshHostData();
    }
  }, [selectedHostId]);

  const refreshHostData = async () => {
    if (!selectedHostId) return;
    setLoading(true);
    try {
      const [cList, sList, sys] = await Promise.all([
        api.listContainers(selectedHostId).catch((err) => {
          console.error('Failed to list containers:', err);
          return [];
        }),
        api.listStacks(selectedHostId).catch((err) => {
          console.error('Failed to list stacks:', err);
          return [];
        }),
        api.getHostSystem(selectedHostId).catch((err) => {
          console.error('Failed to get host system:', err);
          return null;
        }),
      ]);
      setContainers(Array.isArray(cList) ? cList : []);
      setStacks(Array.isArray(sList) ? sList : []);
      setSystemInfo(sys);
    } finally {
      setLoading(false);
    }
  };

  const handleDiscoverStacks = async () => {
    if (!selectedHostId) return;
    try {
      setScanning(true);
      const discovered = await api.discoverStacks(selectedHostId);
      setStacks(Array.isArray(discovered) ? discovered : []);
      loadHosts();
    } catch (err: any) {
      alert(err.message || 'Discovery failed');
    } finally {
      setScanning(false);
    }
  };

  const handleDeleteStack = async (s: Stack) => {
    if (!window.confirm(`Remove stack "${s.name}" from DockerPulse? (Files on disk will not be deleted)`)) {
      return;
    }
    try {
      await api.deleteStack(selectedHostId, s.id);
      refreshHostData();
    } catch (err: any) {
      alert(err.message || 'Failed to remove stack');
    }
  };

  const handleCheckUpdates = async (silent = false) => {
    if (!selectedHostId) return;
    try {
      setCheckingUpdates(true);
      const results = await api.checkUpdates(selectedHostId);
      const map: Record<string, boolean> = {};
      (Array.isArray(results) ? results : []).forEach((r) => {
        if (r.has_update) {
          map[r.image] = true;
          if (r.current_digest) {
            map[r.current_digest] = true;
          }
          if (r.image.endsWith(':latest')) {
            map[r.image.replace(/:latest$/, '')] = true;
          } else if (!r.image.includes(':')) {
            map[r.image + ':latest'] = true;
          }
          const parts = r.image.split('/');
          const short = parts[parts.length - 1];
          map[short] = true;
          if (short.endsWith(':latest')) {
            map[short.replace(/:latest$/, '')] = true;
          } else if (!short.includes(':')) {
            map[short + ':latest'] = true;
          }
        }
      });
      setUpdates(map);
    } catch (err: any) {
      if (!silent) {
        alert(err.message || 'Update check failed');
      } else {
        console.warn('Daily update check failed:', err);
      }
    } finally {
      setCheckingUpdates(false);
    }
  };

  // Sync fleetStats for current selected host
  useEffect(() => {
    if (!selectedHostId) return;
    const running = containerList.filter((c) => c && c.state === 'running').length;
    const updateCount = containerList.filter((c) => isContainerUpdateAvailable(c, updates)).length;
    setFleetStats((prev) => ({
      ...prev,
      [selectedHostId]: {
        running,
        total: containerList.length,
        updates: updateCount,
      },
    }));
  }, [selectedHostId, containers, updates]);

  // Background fetch container counts for other fleet hosts
  useEffect(() => {
    if (hosts.length === 0) return;
    hosts.forEach(async (h) => {
      if (h.id === selectedHostId) return;
      try {
        const list = await api.listContainers(h.id);
        if (Array.isArray(list)) {
          const rCount = list.filter((c) => c && c.state === 'running').length;
          const uCount = list.filter((c) => c && c.has_update).length;
          setFleetStats((prev) => ({
            ...prev,
            [h.id]: {
              running: rCount,
              total: list.length,
              updates: uCount > 0 ? uCount : prev[h.id]?.updates || 0,
            },
          }));
        }
      } catch {
        // host offline
      }
    });
  }, [hosts]);

  // Daily auto-update check per host on first visit each day
  useEffect(() => {
    if (!selectedHostId || loading) return;

    const today = new Date().toISOString().slice(0, 10);
    const key = `dockerpulse_daily_update_${selectedHostId}`;
    const lastChecked = localStorage.getItem(key);

    if (lastChecked !== today) {
      localStorage.setItem(key, today);
      handleCheckUpdates(true);
    }
  }, [selectedHostId, loading]);

  const handleContainerOp = async (cid: string, op: 'start' | 'stop' | 'restart' | 'remove') => {
    try {
      if (op === 'start') await api.startContainer(selectedHostId, cid);
      if (op === 'stop') await api.stopContainer(selectedHostId, cid);
      if (op === 'restart') await api.restartContainer(selectedHostId, cid);
      if (op === 'remove') {
        if (!confirm('Remove container?')) return;
        await api.removeContainer(selectedHostId, cid, true);
      }
      refreshHostData();
    } catch (err: any) {
      alert(err.message || `Failed to ${op} container`);
    }
  };

  const hostList = Array.isArray(hosts) ? hosts : [];
  const containerList = Array.isArray(containers) ? containers : [];
  const stackList = Array.isArray(stacks) ? stacks : [];
  const currentHost = hostList.find((h) => h.id === selectedHostId);

  const stacksToUpdate = useMemo(() => {
    return stackList.filter((s) => {
      const sc = containerList.filter((c) => matchesStack(c, s));
      return sc.some((c) => isContainerUpdateAvailable(c, updates));
    });
  }, [stackList, containerList, updates]);

  const handleUpdateAll = () => {
    if (stacksToUpdate.length > 0) {
      setUpdateAction({ stacks: stacksToUpdate, action: 'pull_up' });
    } else {
      if (stackList.length === 0) {
        alert('No compose stacks found on this host.');
        return;
      }
      if (window.confirm(`No pending updates detected. Run Pull & Up on all ${stackList.length} stack(s) anyway?`)) {
        setUpdateAction({ stacks: stackList, action: 'pull_up' });
      }
    }
  };

  if (authNeeded) {
    return (
      <AuthModal
        isSetup={isSetup}
        onSuccess={(u) => {
          setUser(u);
          setAuthNeeded(false);
          loadHosts();
        }}
      />
    );
  }

  return (
    <div className="min-h-screen bg-slate-950 flex flex-col selection:bg-sky-500/30">
      {/* Top Navbar */}
      <header className="border-b border-slate-800 bg-slate-900/90 backdrop-blur sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 h-16 flex items-center justify-between gap-4">
          {/* Logo & Brand */}
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-tr from-sky-600 to-indigo-600 shadow-md shadow-sky-600/20">
              <Box className="w-5 h-5 text-white" />
            </div>
            <div>
              <h1 className="text-base font-bold text-slate-100 flex items-center gap-2">
                DockerPulse
                <span className="text-[10px] uppercase font-mono px-1.5 py-0.2 rounded bg-sky-500/10 text-sky-400 border border-sky-500/20">
                  Fleet
                </span>
              </h1>
              <p className="text-[11px] text-slate-400 font-medium">Native Directory Docker Manager</p>
            </div>
          </div>

          {/* Server Selector Dropdown */}
          <div className="flex items-center gap-2">
            <div className="relative">
              <select
                value={selectedHostId}
                onChange={(e) => setSelectedHostId(e.target.value)}
                className="rounded-lg bg-slate-800/90 border border-slate-700 py-1.5 pl-3 pr-8 text-xs font-semibold text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500 cursor-pointer"
              >
                {hostList.length === 0 ? (
                  <option value="">No servers added</option>
                ) : (
                  hostList.map((h) => (
                    <option key={h.id} value={h.id}>
                      {h.name} ({h.driver.toUpperCase()} &bull; {h.status})
                    </option>
                  ))
                )}
              </select>
            </div>

            {currentHost && (
              <button
                onClick={() => setShowHostSettings(true)}
                title="Server Settings"
                className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 p-2 text-xs font-medium text-slate-300 hover:text-sky-400 transition-colors"
              >
                <Settings className="w-3.5 h-3.5" />
              </button>
            )}

            <button
              onClick={() => setShowAddHost(true)}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-3 py-1.5 text-xs font-medium text-slate-200 transition-colors"
            >
              <Plus className="w-3.5 h-3.5 text-sky-400" />
              Add Server
            </button>

            <button
              onClick={() => setShowDeployAgent(true)}
              className="flex items-center gap-1.5 rounded-lg bg-sky-600/10 hover:bg-sky-600/20 border border-sky-500/30 px-3 py-1.5 text-xs font-medium text-sky-400 transition-colors"
            >
              <Cpu className="w-3.5 h-3.5" />
              Deploy Agent
            </button>
          </div>

          {/* Global Operations & User menu */}
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowNetworks(true)}
              disabled={!selectedHostId}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800/80 hover:bg-slate-700/80 border border-slate-700/60 px-3 py-1.5 text-xs font-medium text-slate-300 transition-colors disabled:opacity-50"
            >
              <Network className="w-3.5 h-3.5 text-sky-400" />
              Networks
            </button>

            <button
              onClick={() => setShowStorage(true)}
              disabled={!selectedHostId}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800/80 hover:bg-slate-700/80 border border-slate-700/60 px-3 py-1.5 text-xs font-medium text-slate-300 transition-colors disabled:opacity-50"
            >
              <HardDrive className="w-3.5 h-3.5 text-amber-400" />
              Storage & Prune
            </button>

            <button
              onClick={() => handleCheckUpdates(false)}
              disabled={checkingUpdates || !selectedHostId}
              className="flex items-center gap-1.5 rounded-lg bg-sky-600/10 hover:bg-sky-600/20 border border-sky-500/30 px-3 py-1.5 text-xs font-medium text-sky-400 transition-colors disabled:opacity-50"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${checkingUpdates ? 'animate-spin' : ''}`} />
              Check Updates
            </button>

            <button
              onClick={() => {
                localStorage.removeItem('dockpulse_token');
                setUser(null);
                setAuthNeeded(true);
              }}
              title="Sign Out"
              className="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors ml-1"
            >
              <LogOut className="w-4 h-4" />
            </button>
          </div>
        </div>
      </header>

      {/* Fleet Overview Summary Bar */}
      {hostList.length > 0 && (
        <div className="border-b border-slate-800/80 bg-slate-900/40 backdrop-blur-sm py-2.5 px-4">
          <div className="max-w-7xl mx-auto flex items-center justify-center gap-2.5 overflow-x-auto no-scrollbar">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500 shrink-0 mr-1 hidden sm:inline-block">
              Fleet:
            </span>
            {hostList.map((h) => {
              const isSelected = h.id === selectedHostId;
              const stats = fleetStats[h.id];
              const isOnline = h.status === 'online';

              return (
                <button
                  key={h.id}
                  onClick={() => setSelectedHostId(h.id)}
                  className={`flex items-center gap-2 px-3 py-1.5 rounded-xl border text-xs font-mono transition-all shrink-0 ${
                    isSelected
                      ? 'border-sky-500/60 bg-sky-950/40 text-sky-200 shadow-sm shadow-sky-500/10'
                      : 'border-slate-800/80 bg-slate-900/60 text-slate-400 hover:border-slate-700 hover:text-slate-200 hover:bg-slate-800/50'
                  }`}
                >
                  <span
                    className={`w-2 h-2 rounded-full shrink-0 ${
                      isOnline
                        ? 'bg-emerald-400 shadow-sm shadow-emerald-400/50'
                        : 'bg-slate-600'
                    }`}
                  />
                  <span className="font-semibold font-sans">{h.name}</span>

                  <span className="text-slate-600">&bull;</span>

                  <span className={isSelected ? 'text-slate-200 font-semibold' : 'text-slate-400'}>
                    {stats ? `${stats.running}/${stats.total} Running` : isOnline ? '...' : 'Offline'}
                  </span>

                  {stats && stats.updates > 0 && (
                    <span className="flex items-center gap-1 rounded bg-amber-500/15 border border-amber-500/30 px-1.5 py-0.5 text-[10px] font-semibold text-amber-400">
                      <ArrowUpCircle className="w-3 h-3" />
                      {stats.updates} {stats.updates === 1 ? 'update' : 'updates'}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Main Container */}
      <main className="flex-1 max-w-7xl mx-auto w-full px-4 py-6 space-y-6">
        {hostList.length === 0 ? (
          <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-10 text-center max-w-xl mx-auto my-12 shadow-2xl">
            <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-sky-500/10 border border-sky-500/20 text-sky-400 mx-auto mb-4">
              <Server className="h-8 w-8" />
            </div>
            <h3 className="text-xl font-bold text-slate-100">Welcome to DockerPulse!</h3>
            <p className="text-xs text-slate-400 mt-2 max-w-md mx-auto leading-relaxed">
              No Docker servers are connected yet. Click below to connect this local machine's Docker daemon, or deploy an agent to a remote node.
            </p>
            <div className="flex items-center justify-center gap-3 mt-6">
              <button
                onClick={handleConnectLocal}
                className="flex items-center gap-2 rounded-lg bg-sky-600 hover:bg-sky-500 px-5 py-2.5 text-xs font-semibold text-white shadow-lg shadow-sky-600/20 transition-all"
              >
                <Server className="w-4 h-4" /> Connect Local Server
              </button>
              <button
                onClick={() => setShowDeployAgent(true)}
                className="flex items-center gap-2 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-5 py-2.5 text-xs font-semibold text-slate-200 transition-colors"
              >
                <Cpu className="w-4 h-4 text-sky-400" /> Deploy Remote Agent
              </button>
            </div>
          </div>
        ) : (
          <>
            {/* Host Banner & Telemetry Bar */}
            {currentHost && (
          <div className="rounded-xl border border-slate-800 bg-slate-900/60 p-4 shadow-lg flex flex-wrap items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <div
                className={`w-3 h-3 rounded-full ${
                  currentHost.status === 'online' ? 'bg-emerald-500 shadow-lg shadow-emerald-500/50' : 'bg-rose-500'
                }`}
              />
              <div>
                <h2 className="text-sm font-semibold text-slate-100 flex items-center gap-2">
                  {currentHost.name}
                  <span className="text-xs font-mono font-normal text-slate-400">
                    ({currentHost.base_dir})
                  </span>
                </h2>
                <p className="text-[11px] text-slate-400 font-mono">
                  Driver: {currentHost.driver.toUpperCase()} &bull; Last Seen:{' '}
                  {new Date(currentHost.last_seen).toLocaleTimeString()}
                </p>
              </div>
            </div>

            {systemInfo && (
              <div className="flex items-center gap-6 text-xs font-mono text-slate-300">
                <div>
                  <span className="text-slate-500 text-[10px] block">DOCKER ENGINE</span>
                  {systemInfo.docker_version || '27.x'}
                </div>
                <div>
                  <span className="text-slate-500 text-[10px] block">CPU CORES</span>
                  {systemInfo.total_cpus} Cores
                </div>
                <div>
                  <span className="text-slate-500 text-[10px] block">MEMORY</span>
                  {(systemInfo.total_ram_bytes / (1024 * 1024 * 1024)).toFixed(1)} GB Total
                </div>
              </div>
            )}
          </div>
        )}

        {/* View Mode Tabs (Containers vs Compose Stacks) */}
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setViewMode('containers')}
              className={`flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-semibold transition-all ${
                viewMode === 'containers'
                  ? 'bg-sky-600 text-white shadow-md shadow-sky-600/20'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
              }`}
            >
              <Box className="w-4 h-4" />
              Containers ({containerList.length})
            </button>
            <button
              onClick={() => setViewMode('stacks')}
              className={`flex items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-semibold transition-all ${
                viewMode === 'stacks'
                  ? 'bg-sky-600 text-white shadow-md shadow-sky-600/20'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
              }`}
            >
              <Layers className="w-4 h-4" />
              Compose Stacks ({stackList.length})
            </button>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={handleUpdateAll}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-semibold shadow-md transition-all ${
                stacksToUpdate.length > 0
                  ? 'bg-amber-600 hover:bg-amber-500 text-white shadow-amber-600/20 ring-1 ring-amber-400/40'
                  : 'bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700'
              }`}
              title={
                stacksToUpdate.length > 0
                  ? `Update all ${stacksToUpdate.length} stacks with pending updates`
                  : 'Pull & Up all stacks'
              }
            >
              <ArrowUpCircle className={`w-3.5 h-3.5 ${stacksToUpdate.length > 0 ? 'text-amber-200' : 'text-slate-400'}`} />
              <span>Update All{stacksToUpdate.length > 0 ? ` (${stacksToUpdate.length})` : ''}</span>
            </button>

            {viewMode === 'stacks' && (
              <button
                onClick={handleDiscoverStacks}
                disabled={scanning}
                className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-3 py-1.5 text-xs font-medium text-slate-200 transition-colors"
              >
                <Folder className="w-3.5 h-3.5 text-sky-400" />
                {scanning ? 'Scanning ~/docker...' : 'Scan Directory'}
              </button>
            )}

            <button
              onClick={refreshHostData}
              disabled={loading}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-3 py-1.5 text-xs font-medium text-slate-200 transition-colors"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </div>

        {/* Containers List View */}
        {viewMode === 'containers' && (
          <div className="space-y-3">
            {containerList.length === 0 ? (
              <div className="py-16 text-center text-slate-500 font-mono text-sm border border-dashed border-slate-800 rounded-xl">
                No containers detected on this host.
              </div>
            ) : (
              containerList.map((c) => {
                const name = c.names[0]?.replace('/', '') || c.id.slice(0, 12);
                const hasUpdate = isContainerUpdateAvailable(c, updates);
                const isRunning = c.state === 'running';

                return (
                  <div
                    key={c.id}
                    className="rounded-xl border border-slate-800/80 bg-slate-900/60 hover:border-slate-700/80 p-4 transition-all flex flex-col md:flex-row md:items-center justify-between gap-4"
                  >
                    {/* Left: Container Info & State */}
                    <div className="flex items-center gap-3.5 flex-1 min-w-0 pr-2">
                      <div
                        className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                          isRunning ? 'bg-emerald-400 shadow-sm shadow-emerald-400/50' : 'bg-slate-600'
                        }`}
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-semibold text-sm text-slate-100 truncate">{name}</span>
                          {c.stack && (
                            <span className="rounded bg-sky-500/10 px-1.5 py-0.5 text-[10px] font-mono text-sky-400 border border-sky-500/20 shrink-0">
                              {c.stack}
                            </span>
                          )}
                          {hasUpdate && (
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                const st = stackList.find(
                                  (s) =>
                                    s.name === c.stack ||
                                    s.path.endsWith('/' + c.stack) ||
                                    (c.working_dir && s.path === c.working_dir) ||
                                    matchesStack(c, s)
                                );
                                if (st) {
                                  setUpdateAction({ stack: st, action: 'pull_up' });
                                } else {
                                  alert(`No matching compose stack found for '${c.stack || name}'. Ensure the stack directory is scanned in DockerPulse.`);
                                }
                              }}
                              className="flex items-center gap-1 rounded bg-amber-500/15 hover:bg-amber-500/25 active:bg-amber-500/35 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300 hover:text-amber-200 border border-amber-500/30 hover:border-amber-400/50 shadow-sm transition-all cursor-pointer"
                              title="Update Available: Click to run Pull & Up for this stack"
                            >
                              <ArrowUpCircle className="w-3 h-3 text-amber-400" /> Update Available
                            </button>
                          )}
                        </div>
                        <div className="flex items-center gap-2 text-xs text-slate-400 font-mono mt-0.5 min-w-0">
                          <span className="truncate max-w-[200px] sm:max-w-[260px] md:max-w-[320px]" title={c.image}>
                            {formatImage(c.image)}
                          </span>
                          <span className="shrink-0">&bull;</span>
                          <span className="shrink-0">{c.status}</span>
                        </div>
                      </div>
                    </div>

                    {/* Middle: Live Stats (CPU / RAM / Net) */}
                    <div className="flex items-center gap-6 text-xs font-mono text-slate-300 shrink-0">
                      <div>
                        <span className="text-[10px] text-slate-500 block">CPU</span>
                        <div className="flex items-center gap-1.5">
                          <span className="font-semibold">{(c.cpu_pct || 0).toFixed(1)}%</span>
                          <div className="w-16 bg-slate-800 h-1.5 rounded-full overflow-hidden">
                            <div
                              className="bg-sky-500 h-full rounded-full"
                              style={{ width: `${Math.min(c.cpu_pct || 0, 100)}%` }}
                            />
                          </div>
                        </div>
                      </div>

                      <div>
                        <span className="text-[10px] text-slate-500 block">MEM</span>
                        <span className="font-semibold">
                          {(c.memory_mb || 0).toFixed(0)} MB{' '}
                          <span className="text-slate-500">({(c.memory_pct || 0).toFixed(0)}%)</span>
                        </span>
                      </div>

                      <div>
                        <span className="text-[10px] text-slate-500 block">NET I/O</span>
                        <span>
                          {(c.net_input_mb || 0).toFixed(1)}M / {(c.net_output_mb || 0).toFixed(1)}M
                        </span>
                      </div>
                    </div>

                    {/* Right: Actions */}
                    <div className="flex items-center gap-1.5 shrink-0">
                      {isRunning ? (
                        <>
                          <button
                            onClick={() => handleContainerOp(c.id, 'restart')}
                            title="Restart"
                            className="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
                          >
                            <RotateCw className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => handleContainerOp(c.id, 'stop')}
                            title="Stop"
                            className="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-amber-400 transition-colors"
                          >
                            <Square className="w-4 h-4" />
                          </button>
                        </>
                      ) : (
                        <button
                          onClick={() => handleContainerOp(c.id, 'start')}
                          title="Start"
                          className="rounded-lg p-2 text-emerald-400 hover:bg-emerald-500/10 transition-colors"
                        >
                          <Play className="w-4 h-4" />
                        </button>
                      )}

                      <button
                        onClick={() => setLogContainer(c)}
                        title="Live Logs (-f)"
                        className="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-sky-400 transition-colors"
                      >
                        <FileText className="w-4 h-4" />
                      </button>

                      <button
                        onClick={() => setTerminalContainer(c)}
                        title="Interactive Shell (Terminal)"
                        className="rounded-lg p-2 text-slate-400 hover:bg-slate-800 hover:text-purple-400 transition-colors"
                      >
                        <Terminal className="w-4 h-4" />
                      </button>

                      <button
                        onClick={() => handleContainerOp(c.id, 'remove')}
                        title="Remove container"
                        className="rounded-lg p-2 text-slate-500 hover:bg-rose-500/10 hover:text-rose-400 transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        )}

        {/* Compose Stacks View */}
        {viewMode === 'stacks' && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {stackList.length === 0 ? (
              <div className="col-span-2 py-16 text-center text-slate-500 font-mono text-sm border border-dashed border-slate-800 rounded-xl">
                No Compose stacks discovered. Click "Scan Directory" to find stacks in {currentHost?.base_dir || '~/docker'}.
              </div>
            ) : (
              stackList.map((s) => {
                const stackContainers = containerList.filter((c) => matchesStack(c, s));
                const runningCount = stackContainers.filter((c) => c && c.state === 'running').length;
                const updateCount = stackContainers.filter((c) => isContainerUpdateAvailable(c, updates)).length;

                return (
                  <div
                    key={s.id}
                    className="rounded-xl border border-slate-800/80 bg-slate-900/60 p-5 space-y-4 hover:border-slate-700/80 transition-all flex flex-col justify-between"
                  >
                    <div>
                      <div className="flex items-center justify-between mb-2">
                        <div className="flex items-center gap-2">
                          <Folder className="w-4 h-4 text-sky-400" />
                          <h3 className="font-bold text-sm text-slate-100">{s.name}</h3>
                        </div>
                        <div className="flex items-center gap-1.5">
                          {updateCount > 0 && (
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                setUpdateAction({ stack: s, action: 'pull_up' });
                              }}
                              className="flex items-center gap-1 rounded bg-amber-500/15 hover:bg-amber-500/25 active:bg-amber-500/35 border border-amber-500/30 hover:border-amber-400/50 px-2 py-0.5 text-[10px] font-mono font-semibold text-amber-300 hover:text-amber-200 shadow-sm transition-all cursor-pointer"
                              title={`Click to update this stack (${updateCount} container update${updateCount === 1 ? '' : 's'} available)`}
                            >
                              <ArrowUpCircle className="w-3 h-3 text-amber-400" />
                              {updateCount} {updateCount === 1 ? 'Update' : 'Updates'}
                            </button>
                          )}
                          <span
                            className={`rounded px-2 py-0.5 text-[10px] font-mono font-semibold ${
                              runningCount > 0
                                ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                                : 'bg-slate-800 text-slate-400'
                            }`}
                          >
                            {runningCount}/{stackContainers.length} Running
                          </span>
                        </div>
                      </div>
                      <p className="text-xs font-mono text-slate-400 break-all">{s.path}</p>
                    </div>

                    {/* Services preview */}
                    {stackContainers.length > 0 && (
                      <div className="space-y-1.5">
                        <span className="text-[10px] font-semibold text-slate-500 uppercase tracking-wider">
                          Services
                        </span>
                        <div className="flex flex-wrap gap-1.5">
                          {stackContainers.map((sc) => {
                            const hasUp = isContainerUpdateAvailable(sc, updates);
                            return (
                              <span
                                key={sc.id}
                                className={`rounded px-2 py-1 text-xs font-mono flex items-center gap-1.5 border transition-all ${
                                  hasUp
                                    ? 'bg-amber-500/10 border-amber-500/30 text-amber-200'
                                    : 'bg-slate-800/80 border-slate-700/60 text-slate-300'
                                }`}
                              >
                                <span>{sc.service || (sc.names && sc.names[0] ? sc.names[0].replace('/', '') : sc.id?.slice(0, 12))}</span>
                                {hasUp && (
                                  <span title={`Update available: ${sc.image}`}>
                                    <ArrowUpCircle className="w-3 h-3 text-amber-400 shrink-0" />
                                  </span>
                                )}
                                <span
                                  className={`w-1.5 h-1.5 rounded-full ${
                                    sc.state === 'running' ? 'bg-emerald-400' : 'bg-slate-600'
                                  }`}
                                />
                              </span>
                            );
                          })}
                        </div>
                      </div>
                    )}

                    {/* Push-Button Actions */}
                    <div className="flex flex-wrap items-center gap-2 pt-3 border-t border-slate-800/80">
                      {/* Push-Button Update (pull && up -d) */}
                      <button
                        onClick={() => setUpdateAction({ stack: s, action: 'pull_up' })}
                        className={`flex-1 flex items-center justify-center gap-1.5 rounded-lg py-1.5 px-3 text-xs font-medium text-white shadow-md transition-all ${
                          updateCount > 0
                            ? 'bg-amber-600 hover:bg-amber-500 shadow-amber-600/30 ring-1 ring-amber-400/50 font-semibold'
                            : 'bg-sky-600 hover:bg-sky-500 shadow-sky-600/20'
                        }`}
                      >
                        <ArrowUpCircle className="w-3.5 h-3.5" />
                        {updateCount > 0 ? `Pull & Update (${updateCount})` : 'Pull & Up'}
                      </button>

                      <button
                        onClick={() => setEditStack(s)}
                        className="flex items-center gap-1 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 py-1.5 px-3 text-xs font-medium text-slate-200 transition-colors"
                      >
                        <Edit className="w-3.5 h-3.5 text-sky-400" />
                        Edit & Env
                      </button>

                      <button
                        onClick={() => setUpdateAction({ stack: s, action: 'restart' })}
                        className="flex items-center gap-1 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 p-2 text-xs font-medium text-slate-400 hover:text-slate-200 transition-colors"
                        title="Restart Stack"
                      >
                        <RotateCw className="w-3.5 h-3.5" />
                      </button>

                      <button
                        onClick={() => setUpdateAction({ stack: s, action: 'down' })}
                        className="flex items-center gap-1 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 p-2 text-xs font-medium text-slate-400 hover:text-amber-400 transition-colors"
                        title="Stop Stack (Down)"
                      >
                        <Square className="w-3.5 h-3.5" />
                      </button>

                      <button
                        onClick={() => handleDeleteStack(s)}
                        className="flex items-center gap-1 rounded-lg bg-slate-800 hover:bg-rose-500/20 border border-slate-700 hover:border-rose-500/30 p-2 text-xs font-medium text-slate-400 hover:text-rose-400 transition-colors"
                        title="Remove Stack from DockerPulse"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        )}
          </>
        )}
      </main>

      {/* Modals */}
      {logContainer && (
        <LiveLogsModal
          hostId={selectedHostId}
          containerId={logContainer.id}
          containerName={logContainer.names[0]?.replace('/', '') || logContainer.id}
          onClose={() => setLogContainer(null)}
        />
      )}

      {terminalContainer && (
        <TerminalModal
          hostId={selectedHostId}
          containerId={terminalContainer.id}
          containerName={terminalContainer.names[0]?.replace('/', '') || terminalContainer.id}
          onClose={() => setTerminalContainer(null)}
        />
      )}

      {editStack && (
        <ComposeEditorModal
          hostId={selectedHostId}
          stackId={editStack.id}
          stackName={editStack.name}
          onClose={() => setEditStack(null)}
          onDeploy={(action) => setUpdateAction({ stack: editStack, action })}
        />
      )}

      {updateAction && (
        <UpdateModal
          hostId={selectedHostId}
          stackId={updateAction.stack?.id || updateAction.stacks?.[0]?.id || ''}
          stackName={updateAction.stack?.name || updateAction.stacks?.[0]?.name || ''}
          stacks={updateAction.stacks || (updateAction.stack ? [updateAction.stack] : [])}
          action={updateAction.action}
          onClose={() => setUpdateAction(null)}
          onOpenEditor={() => {
            const st = updateAction.stack || updateAction.stacks?.[0];
            setUpdateAction(null);
            if (st) setEditStack(st);
          }}
          onFinished={() => {
            refreshHostData();
            handleCheckUpdates(true);
          }}
        />
      )}

      {showNetworks && (
        <NetworksModal
          hostId={selectedHostId}
          hostName={currentHost?.name || ''}
          containers={containerList}
          onClose={() => setShowNetworks(false)}
        />
      )}

      {showStorage && (
        <StorageModal
          hostId={selectedHostId}
          hostName={currentHost?.name || ''}
          onClose={() => setShowStorage(false)}
        />
      )}

      {showAddHost && (
        <AddHostModal
          onClose={() => setShowAddHost(false)}
          onAdded={(newHost) => {
            setHosts((prev) => [...prev, newHost]);
            setSelectedHostId(newHost.id);
          }}
        />
      )}

      {showDeployAgent && (
        <DeployAgentModal
          onClose={() => setShowDeployAgent(false)}
          hosts={hostList}
          onRefreshHosts={loadHosts}
          onSelectHost={(id) => setSelectedHostId(id)}
        />
      )}

      {showHostSettings && currentHost && (
        <HostSettingsModal
          host={currentHost}
          onClose={() => setShowHostSettings(false)}
          onUpdated={(updated) => {
            setHosts((prev) => prev.map((h) => (h.id === updated.id ? updated : h)));
            // Trigger stack discovery for updated path
            api.discoverStacks(updated.id).then((disc) => {
              if (Array.isArray(disc)) setStacks(disc);
            }).catch(() => {});
          }}
          onDeleted={(delId) => {
            setHosts((prev) => {
              const next = prev.filter((h) => h.id !== delId);
              if (next.length > 0) setSelectedHostId(next[0].id);
              else setSelectedHostId('');
              return next;
            });
          }}
        />
      )}
    </div>
  );
};
