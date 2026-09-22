import React, { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { X, Maximize2, Terminal as TerminalIcon } from 'lucide-react';
import { api } from '../api/client';

interface TerminalModalProps {
  hostId: string;
  containerId: string;
  containerName: string;
  onClose: () => void;
}

export const TerminalModal: React.FC<TerminalModalProps> = ({
  hostId,
  containerId,
  containerName,
  onClose,
}) => {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermInstance = useRef<Terminal | null>(null);
  const fitAddonInstance = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      theme: {
        background: '#090d16',
        foreground: '#e2e8f0',
        cursor: '#38bdf8',
        selectionBackground: 'rgba(56, 189, 248, 0.3)',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    xtermInstance.current = term;
    fitAddonInstance.current = fitAddon;

    term.writeln(`\x1b[36mConnecting to ${containerName} terminal...\x1b[0m\r\n`);

    const wsUrl = api.getTerminalWebSocketURL(hostId, containerId);
    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    socketRef.current = ws;

    ws.onopen = () => {
      term.writeln('\x1b[32mConnected!\x1b[0m\r\n');
      // Send initial size
      const { rows, cols } = term;
      ws.send(JSON.stringify({ type: 'resize', rows, cols }));
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        term.write(event.data);
      }
    };

    ws.onerror = () => {
      term.writeln('\r\n\x1b[31mTerminal connection error.\x1b[0m');
    };

    ws.onclose = () => {
      term.writeln('\r\n\x1b[33mSession ended.\x1b[0m');
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    const handleResize = () => {
      if (fitAddonInstance.current && xtermInstance.current && ws.readyState === WebSocket.OPEN) {
        fitAddonInstance.current.fit();
        const { rows, cols } = xtermInstance.current;
        ws.send(JSON.stringify({ type: 'resize', rows, cols }));
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      ws.close();
      term.dispose();
    };
  }, [hostId, containerId, containerName]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-5xl h-[80vh] flex flex-col rounded-xl border border-slate-800 bg-slate-900 shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3 bg-slate-950/60">
          <div className="flex items-center gap-2">
            <TerminalIcon className="w-5 h-5 text-sky-400" />
            <span className="font-semibold text-slate-100">Terminal:</span>
            <span className="font-mono text-sm text-sky-300">{containerName}</span>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Terminal Canvas */}
        <div className="flex-1 p-3 bg-[#090d16]" ref={terminalRef} />
      </div>
    </div>
  );
};
