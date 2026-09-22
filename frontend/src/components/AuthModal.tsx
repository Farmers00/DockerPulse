import React, { useState } from 'react';
import { Lock, User as UserIcon, ShieldCheck, Folder } from 'lucide-react';
import { api } from '../api/client';
import { User } from '../types';

interface AuthModalProps {
  isSetup: boolean;
  onSuccess: (user: User) => void;
}

export const AuthModal: React.FC<AuthModalProps> = ({ isSetup, onSuccess }) => {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [baseDir, setBaseDir] = useState('~/docker');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (isSetup && password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    try {
      setLoading(true);
      if (isSetup) {
        const res = await api.setup({ username, password, base_dir: baseDir.trim() || '~/docker' });
        localStorage.setItem('dockpulse_token', res.token);
        onSuccess(res.user);
      } else {
        const res = await api.login({ username, password });
        localStorage.setItem('dockpulse_token', res.token);
        onSuccess(res.user);
      }
    } catch (err: any) {
      setError(err.message || 'Authentication failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/90 p-4">
      <div className="relative w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8 shadow-2xl">
        <div className="flex flex-col items-center text-center mb-6">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-400 mb-3">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <h2 className="text-xl font-bold text-slate-100">
            {isSetup ? 'Welcome to DockerPulse' : 'Sign in to DockerPulse'}
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            {isSetup
              ? 'Create your primary administrator account to get started.'
              : 'Enter your credentials to manage your Docker servers.'}
          </p>
        </div>

        {error && (
          <div className="mb-4 rounded-lg bg-rose-500/10 border border-rose-500/20 p-3 text-xs text-rose-400 font-medium text-center">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-slate-300 mb-1">Username</label>
            <div className="relative">
              <UserIcon className="w-4 h-4 absolute left-3 top-2.5 text-slate-500" />
              <input
                type="text"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full rounded-lg bg-slate-800/80 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-slate-300 mb-1">Password</label>
            <div className="relative">
              <Lock className="w-4 h-4 absolute left-3 top-2.5 text-slate-500" />
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                className="w-full rounded-lg bg-slate-800/80 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>
          </div>

          {isSetup && (
            <>
              <div>
                <label className="block text-xs font-medium text-slate-300 mb-1">
                  Confirm Password
                </label>
                <div className="relative">
                  <Lock className="w-4 h-4 absolute left-3 top-2.5 text-slate-500" />
                  <input
                    type="password"
                    required
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    placeholder="••••••••"
                    className="w-full rounded-lg bg-slate-800/80 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-300 mb-1">
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
                    className="w-full rounded-lg bg-slate-800/80 border border-slate-700 py-2 pl-9 pr-3 text-xs text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:ring-1 focus:ring-sky-500"
                  />
                </div>
                <p className="text-[11px] text-slate-400 mt-1">
                  Folder where your compose projects are stored (e.g. <span className="font-mono text-slate-300">~/docker</span> or <span className="font-mono text-slate-300">/opt/docker</span>).
                </p>
              </div>
            </>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full rounded-lg bg-sky-600 hover:bg-sky-500 py-2.5 text-xs font-semibold text-white shadow-lg shadow-sky-600/20 transition-all mt-2"
          >
            {loading ? 'Authenticating...' : isSetup ? 'Initialize DockerPulse' : 'Sign In'}
          </button>
        </form>
      </div>
    </div>
  );
};
