import React, { useEffect, useState } from 'react';
import { X, HardDrive, Trash2, CheckCircle, AlertTriangle, ShieldCheck, Info } from 'lucide-react';
import { api } from '../api/client';
import { DiskUsageInfo } from '../types';

interface StorageModalProps {
  hostId: string;
  hostName: string;
  onClose: () => void;
}

export const StorageModal: React.FC<StorageModalProps> = ({
  hostId,
  hostName,
  onClose,
}) => {
  const [usage, setUsage] = useState<DiskUsageInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [pruning, setPruning] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  useEffect(() => {
    loadUsage();
  }, [hostId]);

  const loadUsage = async () => {
    try {
      setLoading(true);
      const data = await api.getStorageUsage(hostId);
      setUsage(data);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const handlePrune = async (all = false) => {
    const msg = all
      ? 'Perform Deep Prune?\n\nThis will remove all images that are not actively running. Your container data, volumes, and compose files will NOT be deleted, but stopped stacks will need to re-download their images when started again.\n\nProceed?'
      : 'Perform Safe Cleanup?\n\nThis will remove orphaned update remnants (<none> images). All active images, container data, and volumes will be preserved.\n\nProceed?';
    if (!confirm(msg)) return;

    try {
      setPruning(true);
      setResult(null);
      const rep = await api.pruneStorage(hostId, all);
      const mbReclaimed = (rep.space_reclaimed / (1024 * 1024)).toFixed(1);
      setResult(`Prune completed! Reclaimed ${mbReclaimed} MB across ${rep.images_deleted} images.`);
      await loadUsage();
    } catch (err: any) {
      alert(err.message || 'Prune failed');
    } finally {
      setPruning(false);
    }
  };

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const totalBytes =
    (usage?.images_size || 0) +
    (usage?.volumes_size || 0) +
    (usage?.containers_size || 0) +
    (usage?.build_cache_size || 0);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-3xl flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-5 py-3.5 bg-slate-950/70">
          <div className="flex items-center gap-2">
            <HardDrive className="w-5 h-5 text-sky-400" />
            <h2 className="text-base font-semibold text-slate-100">Docker Disk Usage & Prune Wizard</h2>
            <span className="text-xs text-slate-400 font-mono">({hostName})</span>
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
          {loading ? (
            <div className="py-12 text-center text-slate-500 font-mono text-sm">
              Calculating disk breakdown...
            </div>
          ) : usage ? (
            <>
              {/* Total Usage Banner */}
              <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4 flex items-center justify-between">
                <div>
                  <span className="text-xs text-slate-400 font-medium">Total Docker Space</span>
                  <div className="text-2xl font-bold text-slate-100 mt-1">{formatBytes(totalBytes)}</div>
                </div>
                <div className="flex gap-4 text-xs text-slate-400">
                  <div>
                    <span className="text-amber-400 font-bold">{usage.dangling_images}</span> dangling images
                  </div>
                  <div>
                    <span className="text-amber-400 font-bold">{usage.unused_volumes}</span> unused volumes
                  </div>
                </div>
              </div>

              {/* Breakdown Grid */}
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-lg border border-slate-800 bg-slate-950/40 p-3.5">
                  <div className="flex justify-between items-center text-xs text-slate-400">
                    <span>Images</span>
                    <span className="font-mono text-slate-200">{formatBytes(usage.images_size)}</span>
                  </div>
                  <div className="w-full bg-slate-800 h-1.5 rounded-full mt-2 overflow-hidden">
                    <div
                      className="bg-sky-500 h-full rounded-full"
                      style={{ width: `${totalBytes > 0 ? (usage.images_size / totalBytes) * 100 : 0}%` }}
                    />
                  </div>
                </div>

                <div className="rounded-lg border border-slate-800 bg-slate-950/40 p-3.5">
                  <div className="flex justify-between items-center text-xs text-slate-400">
                    <span>Volumes</span>
                    <span className="font-mono text-slate-200">{formatBytes(usage.volumes_size)}</span>
                  </div>
                  <div className="w-full bg-slate-800 h-1.5 rounded-full mt-2 overflow-hidden">
                    <div
                      className="bg-emerald-500 h-full rounded-full"
                      style={{ width: `${totalBytes > 0 ? (usage.volumes_size / totalBytes) * 100 : 0}%` }}
                    />
                  </div>
                </div>

                <div className="rounded-lg border border-slate-800 bg-slate-950/40 p-3.5">
                  <div className="flex justify-between items-center text-xs text-slate-400">
                    <span>Container Layers</span>
                    <span className="font-mono text-slate-200">{formatBytes(usage.containers_size)}</span>
                  </div>
                  <div className="w-full bg-slate-800 h-1.5 rounded-full mt-2 overflow-hidden">
                    <div
                      className="bg-purple-500 h-full rounded-full"
                      style={{ width: `${totalBytes > 0 ? (usage.containers_size / totalBytes) * 100 : 0}%` }}
                    />
                  </div>
                </div>

                <div className="rounded-lg border border-slate-800 bg-slate-950/40 p-3.5">
                  <div className="flex justify-between items-center text-xs text-slate-400">
                    <span>Build Cache</span>
                    <span className="font-mono text-slate-200">{formatBytes(usage.build_cache_size)}</span>
                  </div>
                  <div className="w-full bg-slate-800 h-1.5 rounded-full mt-2 overflow-hidden">
                    <div
                      className="bg-amber-500 h-full rounded-full"
                      style={{ width: `${totalBytes > 0 ? (usage.build_cache_size / totalBytes) * 100 : 0}%` }}
                    />
                  </div>
                </div>
              </div>

              {result && (
                <div className="flex items-center gap-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 p-3 text-xs text-emerald-400 font-medium">
                  <CheckCircle className="w-4 h-4 shrink-0" />
                  {result}
                </div>
              )}

              {/* Action Cards */}
              <div className="pt-2 flex flex-col gap-3.5">
                {/* Safe Cleanup Card */}
                <div className="rounded-xl border border-slate-800 bg-slate-950/50 p-4 transition-all hover:border-slate-700/80">
                  <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
                    <div className="space-y-1.5 flex-1">
                      <div className="flex items-center gap-2">
                        <ShieldCheck className="w-4 h-4 text-emerald-400 shrink-0" />
                        <h4 className="text-sm font-semibold text-slate-100">Safe Cleanup</h4>
                        <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                          Recommended &bull; Zero Risk
                        </span>
                      </div>
                      <p className="text-xs text-slate-300 leading-relaxed">
                        Removes orphaned, untagged image layers (<code className="text-sky-300 font-mono text-[11px]">&lt;none&gt;</code>) left behind after updating containers.
                      </p>
                      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 pt-1 text-[11px] text-slate-400">
                        <span className="flex items-center gap-1">
                          <strong className="text-slate-300 font-medium">Deletes:</strong> {usage.dangling_images} dangling image remnants
                        </span>
                        <span className="flex items-center gap-1">
                          <strong className="text-slate-300 font-medium">Preserves:</strong> All tagged images, active containers & volumes
                        </span>
                        <span className="text-emerald-400/90 font-medium">
                          &bull; Nothing needs to be re-downloaded
                        </span>
                      </div>
                    </div>

                    <button
                      onClick={() => handlePrune(false)}
                      disabled={pruning}
                      className="self-start sm:self-center flex items-center gap-1.5 rounded-lg bg-sky-600 hover:bg-sky-500 disabled:opacity-50 py-2 px-3.5 text-xs font-medium text-white transition-all shadow-sm shrink-0"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      {pruning ? 'Pruning...' : 'Prune Safe'}
                    </button>
                  </div>
                </div>

                {/* Deep Prune Card */}
                <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 transition-all hover:border-amber-500/40">
                  <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
                    <div className="space-y-1.5 flex-1">
                      <div className="flex items-center gap-2">
                        <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0" />
                        <h4 className="text-sm font-semibold text-amber-300">Deep Prune</h4>
                        <span className="text-[10px] font-medium px-2 py-0.5 rounded-full bg-amber-500/10 text-amber-400 border border-amber-500/20">
                          Maximum Space Reclaim
                        </span>
                      </div>
                      <p className="text-xs text-slate-300 leading-relaxed">
                        Removes <strong className="text-slate-200">all images</strong> not currently in use by an active running container.
                      </p>
                      <div className="flex flex-col gap-1 pt-1 text-[11px] text-slate-400">
                        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
                          <span className="flex items-center gap-1">
                            <strong className="text-slate-300 font-medium">Deletes:</strong> Inactive & old tagged images (up to {formatBytes(usage.images_size)})
                          </span>
                          <span className="flex items-center gap-1">
                            <strong className="text-slate-300 font-medium">Preserves:</strong> Volumes, persistent data & running stacks
                          </span>
                        </div>
                        <div className="flex items-center gap-1 text-amber-400/90 font-medium mt-0.5">
                          <Info className="w-3.5 h-3.5 shrink-0" />
                          <span>Stopped stacks or templates will re-download their image next time they start.</span>
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handlePrune(true)}
                      disabled={pruning}
                      className="self-start sm:self-center flex items-center gap-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 disabled:opacity-50 py-2 px-3.5 text-xs font-medium text-white transition-all shadow-sm shrink-0"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      {pruning ? 'Pruning...' : 'Deep Prune'}
                    </button>
                  </div>
                </div>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
};
