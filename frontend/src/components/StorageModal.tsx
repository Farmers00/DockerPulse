import React, { useEffect, useState } from 'react';
import { X, HardDrive, Trash2, CheckCircle, AlertTriangle } from 'lucide-react';
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
      ? 'Perform Deep Prune? This will delete ALL unused images, stopped containers, and unused networks.'
      : 'Prune dangling images and stopped containers?';
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

              {/* Action Buttons */}
              <div className="pt-2 flex flex-col gap-3">
                <div className="flex items-center justify-between p-3.5 rounded-lg border border-slate-800 bg-slate-950/40">
                  <div>
                    <h4 className="text-xs font-semibold text-slate-200">Safe Cleanup</h4>
                    <p className="text-[11px] text-slate-400 mt-0.5">
                      Prunes stopped containers, dangling (&lt;none&gt;) images, and unused networks.
                    </p>
                  </div>
                  <button
                    onClick={() => handlePrune(false)}
                    disabled={pruning}
                    className="flex items-center gap-1.5 rounded-lg bg-sky-600 hover:bg-sky-500 disabled:opacity-50 py-1.5 px-3 text-xs font-medium text-white transition-colors"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                    {pruning ? 'Pruning...' : 'Prune Safe'}
                  </button>
                </div>

                <div className="flex items-center justify-between p-3.5 rounded-lg border border-amber-500/20 bg-amber-500/5">
                  <div>
                    <h4 className="text-xs font-semibold text-amber-400 flex items-center gap-1.5">
                      <AlertTriangle className="w-3.5 h-3.5" /> Deep Prune (All Unused Images)
                    </h4>
                    <p className="text-[11px] text-slate-400 mt-0.5">
                      Deletes all images not currently attached to running or stopped containers.
                    </p>
                  </div>
                  <button
                    onClick={() => handlePrune(true)}
                    disabled={pruning}
                    className="flex items-center gap-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 disabled:opacity-50 py-1.5 px-3 text-xs font-medium text-white transition-colors"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                    Deep Prune
                  </button>
                </div>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
};
