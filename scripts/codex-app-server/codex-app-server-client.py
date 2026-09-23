#!/usr/bin/env python3
"""
Minimal RFC6455 WebSocket client for talking to a local codex app-server's
unix-domain control socket (stdlib only - no websockets/websocket-client
dependency needed).

The control socket speaks WebSocket framing over the unix socket, not raw
newline-delimited JSON - a plain socket write gets rejected with
"WebSocket protocol error: httparse error: invalid token". This performs
the actual HTTP Upgrade handshake, then sends/receives WS text frames.

Usage:
    codex-app-server-client.py [socket_path]
    (defaults to $CODEX_HOME/app-server-control/app-server-control.sock,
    or /tmp/codex-app-server/... if CODEX_HOME is unset - matching
    codex-app-server-ctl.sh's default)

Then type one JSON-RPC object per line and press Enter, e.g.:
    {"method": "initialize", "id": 0, "params": {"clientInfo": {"name": "codex_vscode", "title": "Codex VS Code Extension", "version": "0.1.0"}}}
    {"method": "thread/loaded/list", "id": 1, "params": {}}

Responses and server-pushed notifications are printed as they arrive,
prefixed with "< ". Ctrl-D to quit.
"""
import base64
import hashlib
import json
import os
import socket
import struct
import sys
import threading
import time

GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def default_sock_path():
    codex_home = os.environ.get("CODEX_HOME", "/tmp/codex-app-server")
    return os.path.join(codex_home, "app-server-control", "app-server-control.sock")


def ws_handshake(sock):
    key = base64.b64encode(os.urandom(16)).decode()
    req = (
        "GET / HTTP/1.1\r\n"
        "Host: localhost\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    )
    sock.sendall(req.encode())
    resp = b""
    while b"\r\n\r\n" not in resp:
        chunk = sock.recv(4096)
        if not chunk:
            raise RuntimeError("connection closed during handshake")
        resp += chunk
    header, _, rest = resp.partition(b"\r\n\r\n")
    header_text = header.decode(errors="replace")
    if "101" not in header_text.splitlines()[0]:
        raise RuntimeError(f"handshake failed: {header_text}")
    expected_accept = base64.b64encode(
        hashlib.sha1((key + GUID).encode()).digest()
    ).decode()
    if expected_accept not in header_text:
        raise RuntimeError(f"Sec-WebSocket-Accept mismatch, expected {expected_accept}")
    return rest  # any payload bytes already read past the header


def send_text_frame(sock, text):
    payload = text.encode()
    mask = os.urandom(4)
    masked = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
    length = len(payload)
    header = bytearray()
    header.append(0x80 | 0x1)  # FIN + text opcode
    if length < 126:
        header.append(0x80 | length)
    elif length < 65536:
        header.append(0x80 | 126)
        header += struct.pack(">H", length)
    else:
        header.append(0x80 | 127)
        header += struct.pack(">Q", length)
    header += mask
    sock.sendall(bytes(header) + masked)


def pretty(payload_bytes):
    try:
        return json.dumps(json.loads(payload_bytes.decode()), indent=None)
    except (json.JSONDecodeError, UnicodeDecodeError):
        return payload_bytes.decode(errors="replace")


def recv_loop(sock, buf):
    while True:
        try:
            data = buf + sock.recv(65536)
        except OSError:
            return
        if not data:
            print("[connection closed by server]", file=sys.stderr)
            return
        buf = b""
        while len(data) >= 2:
            b0, b1 = data[0], data[1]
            opcode = b0 & 0x0F
            masked = b1 & 0x80
            length = b1 & 0x7F
            idx = 2
            if length == 126:
                if len(data) < idx + 2:
                    buf = data
                    data = b""
                    break
                length = struct.unpack(">H", data[idx:idx + 2])[0]
                idx += 2
            elif length == 127:
                if len(data) < idx + 8:
                    buf = data
                    data = b""
                    break
                length = struct.unpack(">Q", data[idx:idx + 8])[0]
                idx += 8
            if masked:
                if len(data) < idx + 4:
                    buf = data
                    data = b""
                    break
                mask_key = data[idx:idx + 4]
                idx += 4
            if len(data) < idx + length:
                buf = data
                data = b""
                break
            payload = data[idx:idx + length]
            if masked:
                payload = bytes(b ^ mask_key[i % 4] for i, b in enumerate(payload))
            data = data[idx + length:]
            if opcode == 0x1:
                print(f"< {pretty(payload)}")
            elif opcode == 0x8:
                print("[server sent close frame]", file=sys.stderr)
                return
            # 0x9 ping / 0xA pong: ignored, no keepalive handling needed for
            # this short-lived interactive test client.


def main():
    sock_path = sys.argv[1] if len(sys.argv) > 1 else default_sock_path()
    if not os.path.exists(sock_path):
        print(f"error: socket not found: {sock_path}", file=sys.stderr)
        print("(start one with codex-app-server-ctl.sh start)", file=sys.stderr)
        sys.exit(1)

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(sock_path)
    print(f"Connecting to {sock_path} ...", file=sys.stderr)
    leftover = ws_handshake(sock)
    print("Connected. Type one JSON-RPC object per line (Ctrl-D to quit).", file=sys.stderr)

    receiver = threading.Thread(target=recv_loop, args=(sock, leftover), daemon=True)
    receiver.start()

    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            send_text_frame(sock, line)
    except KeyboardInterrupt:
        pass

    time.sleep(1)  # give the receiver thread a moment to print any final response


if __name__ == "__main__":
    main()
