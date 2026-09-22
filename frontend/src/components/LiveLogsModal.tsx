import React, { useEffect, useRef, useState } from 'react';
import { X, Play, Pause, Download, Trash2, Search, FileText } from 'lucide-react';
import { api } from '../api/client';

interface LiveLogsModalProps {
  hostId: string;
  containerId: string;
  containerName: string;
  onClose: () => void;
}

export const LiveLogsModal: React.FC<LiveLogsModalProps> = ({
  hostId,
  containerId,
  containerName,
  onClose,
}) => {
  const [logs, setLogs] = useState<string[]>([]);
  const [follow, setFollow] = useState(true);
  const [tail, setTail] = useState('200');
  const [filter, setFilter] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    setLogs([]);
    const wsUrl = api.getLogWebSocketURL(hostId, containerId, follow, tail);
    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    socketRef.current = ws;

    ws.onmessage = (event) => {
      let text = '';
      if (event.data instanceof ArrayBuffer) {
        text = new TextDecoder().decode(event.data);
      } else {
        text = event.data;
      }
      const lines = text.split('\n').filter((l) => l.length > 0);
      setLogs((prev) => [...prev, ...lines]);
    };

    return () => {
      ws.close();
    };
  }, [hostId, containerId, follow, tail]);

  useEffect(() => {
    if (autoScroll && logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  const filteredLogs = filter
    ? logs.filter((line) => line.toLowerCase().includes(filter.toLowerCase()))
    : logs;

  const copyLogs = () => {
    navigator.clipboard.writeText(logs.join('\n'));
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-6xl h-[85vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between border-b border-slate-800 px-4 py-3 bg-slate-950/60 gap-3">
          <div className="flex items-center gap-2">
            <FileText className="w-5 h-5 text-sky-400" />
            <span className="font-semibold text-slate-100">Live Logs:</span>
            <span className="font-mono text-sm text-sky-300">{containerName}</span>
          </div>

          {/* Controls */}
          <div className="flex items-center gap-3">
            {/* Search Filter */}
            <div className="relative">
              <Search className="w-4 h-4 absolute left-2.5 top-2.5 text-slate-500" />
              <input
                type="text"
                placeholder="Filter logs..."
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                className="w-44 rounded-lg bg-slate-800/80 border border-slate-700 py-1 pl-8 pr-2 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:ring-1 focus:ring-sky-500"
              />
            </div>

            {/* Tail selector */}
            <select
              value={tail}
              onChange={(e) => setTail(e.target.value)}
              className="rounded-lg bg-slate-800 border border-slate-700 py-1 px-2 text-xs text-slate-200 focus:outline-none"
            >
              <option value="50">Tail 50</option>
              <option value="100">Tail 100</option>
              <option value="200">Tail 200</option>
              <option value="500">Tail 500</option>
              <option value="1000">Tail 1000</option>
            </select>

            {/* Follow Toggle */}
            <button
              onClick={() => setFollow(!follow)}
              className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium border transition-colors ${
                follow
                  ? 'bg-sky-500/10 border-sky-500/30 text-sky-400'
                  : 'bg-slate-800 border-slate-700 text-slate-400'
              }`}
            >
              {follow ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
              {follow ? 'Follow (-f)' : 'Paused'}
            </button>

            {/* Copy */}
            <button
              onClick={copyLogs}
              title="Copy all logs"
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
            >
              <Download className="w-4 h-4" />
            </button>

            {/* Clear */}
            <button
              onClick={() => setLogs([])}
              title="Clear view"
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
            >
              <Trash2 className="w-4 h-4" />
            </button>

            {/* Close */}
            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors ml-2"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Logs Stream Container */}
        <div className="flex-1 overflow-y-auto p-4 font-mono text-xs text-slate-300 bg-[#090d16] leading-relaxed selection:bg-sky-500/30">
          {filteredLogs.length === 0 ? (
            <div className="text-slate-600 italic">Waiting for log output...</div>
          ) : (
            filteredLogs.map((line, idx) => (
              <div key={idx} className="hover:bg-slate-800/40 py-0.5 px-1 rounded break-all whitespace-pre-wrap">
                {line}
              </div>
            ))
          )}
          <div ref={logsEndRef} />
        </div>
      </div>
    </div>
  );
};
